#!/usr/bin/env bash
#
# update-grammar-tags.sh — Maintain tree-sitter grammar versions in pack.json
#
# Queries GitHub for the latest tags of each dynamic grammar repository and
# updates the source_tag field in each pack.json file.
#
# Usage:
#   scripts/update-grammar-tags.sh              # Check for updates (dry run)
#   scripts/update-grammar-tags.sh --update     # Update pack.json files in-place
#   scripts/update-grammar-tags.sh --validate   # Validate current tags exist & have parser.c
#   scripts/update-grammar-tags.sh --json       # Output status as JSON
#
# Environment:
#   GITHUB_TOKEN  Optional; avoids rate limiting on the GitHub API.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PACKS_DIR="${REPO_ROOT}/aide/pkg/grammar/packs"

# --- Colours (disabled when piped) ----------------------------------------
if [[ -t 1 ]]; then
  RED=$'\e[31m'; GREEN=$'\e[32m'; YELLOW=$'\e[33m'; CYAN=$'\e[36m'; RESET=$'\e[0m'; BOLD=$'\e[1m'
else
  RED=''; GREEN=''; YELLOW=''; CYAN=''; RESET=''; BOLD=''
fi

# --- Helpers ---------------------------------------------------------------
die()  { echo "${RED}error:${RESET} $*" >&2; exit 1; }
info() { echo "${CYAN}info:${RESET} $*" >&2; }
warn() { echo "${YELLOW}warn:${RESET} $*" >&2; }

require_cmd() {
  command -v "$1" &>/dev/null || die "required command not found: $1"
}

require_cmd curl
require_cmd jq
require_cmd sed

# --- GitHub API helpers ----------------------------------------------------

# Last API error detail — set by gh_api / latest_tag for callers to inspect.
GH_API_ERROR=""

# Make a GitHub API request. Sets GH_API_ERROR on failure.
# Returns 0 on success (body on stdout), 1 on failure.
gh_api() {
  local url="$1"
  local -a curl_args=(-sSL --retry 2 --retry-delay 1 -w "\n%{http_code}")
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl_args+=(-H "Authorization: token ${GITHUB_TOKEN}")
  fi

  local raw http_code body
  raw=$(curl "${curl_args[@]}" "https://api.github.com${url}" 2>/dev/null) || {
    GH_API_ERROR="network error (curl failed)"
    return 1
  }

  # Last line is the HTTP status code, everything before is the body.
  http_code=$(echo "${raw}" | tail -1)
  body=$(echo "${raw}" | sed '$d')

  case "${http_code}" in
    200) GH_API_ERROR=""; echo "${body}"; return 0 ;;
    403)
      local msg
      msg=$(echo "${body}" | jq -r '.message // empty' 2>/dev/null)
      if [[ "${msg}" == *"rate limit"* ]]; then
        GH_API_ERROR="API rate limit exceeded (set GITHUB_TOKEN)"
      else
        GH_API_ERROR="access denied (HTTP 403)"
      fi
      return 1
      ;;
    404) GH_API_ERROR="repository not found (HTTP 404)"; return 1 ;;
    *)   GH_API_ERROR="HTTP ${http_code}"; return 1 ;;
  esac
}

# Fetch the latest release-quality tag for a repo.
# Filters out pre-releases, draft tags, and -rc / -alpha / -beta suffixes.
# Returns the tag name (e.g. "v0.23.4" or "0.4.0").
# On failure, returns empty string and GH_API_ERROR describes the problem.
latest_tag() {
  local repo="$1"
  local filter="${2:-}"  # Optional jq filter for special cases

  # Fetch up to 30 tags (sorted by creation date, newest first).
  local tags_json
  tags_json=$(gh_api "/repos/${repo}/tags?per_page=30") || {
    echo ""; return
  }

  local result
  if [[ -n "${filter}" ]]; then
    result=$(echo "${tags_json}" | jq -r "${filter}" | head -1)
  else
    # Default: pick the first tag that looks like a release (no -rc, -alpha,
    # -beta, -dev suffixes). Allow -with-generated-files for swift.
    result=$(echo "${tags_json}" | jq -r '
      [.[] | .name |
        select(test("-(rc|alpha|beta|dev|pre|nightly)"; "i") | not) |
        select(test("-(pypi|crates-io)"; "i") | not)
      ] | .[0] // empty
    ')
  fi

  if [[ -z "${result}" ]]; then
    GH_API_ERROR="no matching release tag found"
  fi
  echo "${result}"
}

# Check whether a file exists at a given path in a repo at a tag.
file_exists_at_tag() {
  local repo="$1" tag="$2" path="$3"
  local status
  status=$(curl -sSL -o /dev/null -w "%{http_code}" \
    ${GITHUB_TOKEN:+-H "Authorization: token ${GITHUB_TOKEN}"} \
    "https://api.github.com/repos/${repo}/contents/${path}?ref=${tag}" 2>/dev/null)
  [[ "${status}" == "200" ]]
}

# --- Parse grammar definitions from pack.json files -----------------------
# Returns lines of: name|repo|c_symbol|current_tag|extra_src
parse_grammars() {
  for pack_json in "${PACKS_DIR}"/*/pack.json; do
    local line
    line=$(python3 -c "
import json, sys
d = json.load(sys.stdin)
name = d.get('name', '')
c_sym = d.get('c_symbol', '')
repo = d.get('source_repo', '')
tag = d.get('source_tag', '')
src_dir = d.get('source_src_dir', '')
if c_sym and repo and tag:
    print(f'{name}|{repo}|{c_sym}|{tag}|{src_dir}')
" < "$pack_json")
    if [[ -n "${line}" ]]; then
      echo "${line}"
    fi
  done
}

# --- Per-grammar tag selection rules ---------------------------------------
# Some repos have non-standard tag formats. This function returns a jq filter
# to select the right tag, or empty string for default behaviour.
tag_filter_for() {
  local name="$1" repo="$2"
  case "${name}" in
    swift)
      # swift uses bare tags with -with-generated-files suffix
      echo '[.[] | .name | select(endswith("-with-generated-files")) | select(test("-(pypi|crates-io)"; "i") | not)] | .[0] // empty'
      ;;
    *)
      echo ""
      ;;
  esac
}

# Determine the source directory where parser.c should be found.
parser_c_path() {
  local name="$1" extra_src="$2"
  if [[ -n "${extra_src}" ]]; then
    echo "${extra_src}/parser.c"
  else
    echo "src/parser.c"
  fi
}

# --- Main ------------------------------------------------------------------
MODE="check"
JSON_OUTPUT=false

for arg in "$@"; do
  case "${arg}" in
    --update)   MODE="update" ;;
    --validate) MODE="validate" ;;
    --check)    MODE="check" ;;
    --json)     JSON_OUTPUT=true ;;
    --help|-h)
      echo "Usage: $0 [--check|--update|--validate] [--json]"
      echo ""
      echo "Modes:"
      echo "  --check     (default) Show available updates without changing anything"
      echo "  --update    Update pack.json files in-place with latest tags"
      echo "  --validate  Verify current tags exist and have parser.c"
      echo ""
      echo "Options:"
      echo "  --json      Output results as JSON"
      echo ""
      echo "Set GITHUB_TOKEN to avoid API rate limits."
      exit 0
      ;;
    *)
      die "unknown argument: ${arg}"
      ;;
  esac
done

if [[ ! -d "${PACKS_DIR}" ]]; then
  die "packs directory not found at ${PACKS_DIR}"
fi

# Parse current grammar definitions.
mapfile -t ENTRIES < <(parse_grammars)

if [[ ${#ENTRIES[@]} -eq 0 ]]; then
  die "no grammar definitions found in pack.json files under ${PACKS_DIR}"
fi

info "found ${#ENTRIES[@]} grammar definitions in pack.json files"

# Track results.
declare -a RESULTS=()
declare -A RESOLVED_TAGS=()  # Cache: name → latest tag (avoids re-fetching in --update)
UPDATES=0
ERRORS=0
CURRENT=0

for entry in "${ENTRIES[@]}"; do
  IFS='|' read -r name repo c_symbol current_tag extra_src <<< "${entry}"

  # Look up the latest tag.
  filter=$(tag_filter_for "${name}" "${repo}")
  new_tag=$(latest_tag "${repo}" "${filter}")

  # Cache resolved tag for reuse in --update mode.
  RESOLVED_TAGS["${name}"]="${new_tag}"

  if [[ -z "${new_tag}" ]]; then
    ERRORS=$((ERRORS + 1))
    status="error"
    detail="${GH_API_ERROR:-failed to fetch tags from ${repo}}"
    if ! ${JSON_OUTPUT}; then
      echo "  ${RED}✗${RESET} ${BOLD}${name}${RESET}  ${current_tag}  →  ${RED}error: ${detail}${RESET}"
    fi
    RESULTS+=("{\"name\":\"${name}\",\"repo\":\"${repo}\",\"current\":\"${current_tag}\",\"latest\":\"\",\"status\":\"error\",\"detail\":\"${detail}\"}")
    continue
  fi

  if [[ "${MODE}" == "validate" ]]; then
    # Check that parser.c exists at the current tag.
    pc_path=$(parser_c_path "${name}" "${extra_src}")
    if file_exists_at_tag "${repo}" "${current_tag}" "${pc_path}"; then
      status="valid"
      CURRENT=$((CURRENT + 1))
      if ! ${JSON_OUTPUT}; then
        echo "  ${GREEN}✓${RESET} ${BOLD}${name}${RESET}  ${current_tag}  parser.c: ${GREEN}found${RESET}"
      fi
    else
      status="missing_parser"
      ERRORS=$((ERRORS + 1))
      if ! ${JSON_OUTPUT}; then
        echo "  ${RED}✗${RESET} ${BOLD}${name}${RESET}  ${current_tag}  parser.c: ${RED}NOT FOUND${RESET} at ${pc_path}"
      fi
    fi
    RESULTS+=("{\"name\":\"${name}\",\"repo\":\"${repo}\",\"current\":\"${current_tag}\",\"latest\":\"${new_tag}\",\"status\":\"${status}\",\"parser_c_path\":\"${pc_path}\"}")
    continue
  fi

  # Check / update mode.
  if [[ "${current_tag}" == "${new_tag}" ]]; then
    CURRENT=$((CURRENT + 1))
    status="current"
    if ! ${JSON_OUTPUT}; then
      echo "  ${GREEN}✓${RESET} ${BOLD}${name}${RESET}  ${current_tag}  (up to date)"
    fi
  else
    UPDATES=$((UPDATES + 1))
    status="outdated"
    if ! ${JSON_OUTPUT}; then
      echo "  ${YELLOW}↑${RESET} ${BOLD}${name}${RESET}  ${current_tag}  →  ${GREEN}${new_tag}${RESET}"
    fi
  fi
  RESULTS+=("{\"name\":\"${name}\",\"repo\":\"${repo}\",\"current\":\"${current_tag}\",\"latest\":\"${new_tag}\",\"status\":\"${status}\"}")
done

# --- JSON output -----------------------------------------------------------
if ${JSON_OUTPUT}; then
  echo "["
  for i in "${!RESULTS[@]}"; do
    if [[ $i -gt 0 ]]; then echo ","; fi
    echo "  ${RESULTS[$i]}"
  done
  echo "]"
fi

# --- Summary ---------------------------------------------------------------
echo "" >&2
if [[ "${MODE}" == "validate" ]]; then
  info "validation: ${CURRENT} valid, ${ERRORS} issues"
  if [[ ${ERRORS} -gt 0 ]]; then exit 1; fi
  exit 0
fi

if [[ ${UPDATES} -eq 0 && ${ERRORS} -eq 0 ]]; then
  info "all ${CURRENT} grammars are up to date"
  exit 0
elif [[ ${UPDATES} -eq 0 && ${ERRORS} -gt 0 ]]; then
  info "${CURRENT} current, ${ERRORS} error(s) — set GITHUB_TOKEN to avoid rate limits"
  exit 1
fi

info "${UPDATES} update(s) available, ${CURRENT} current, ${ERRORS} error(s)"

# --- Apply updates ---------------------------------------------------------
if [[ "${MODE}" == "update" ]]; then
  info "applying updates to pack.json files..."

  for entry in "${ENTRIES[@]}"; do
    IFS='|' read -r name repo c_symbol current_tag extra_src <<< "${entry}"

    # Use cached tag from the check pass instead of re-fetching.
    new_tag="${RESOLVED_TAGS["${name}"]:-}"

    if [[ -n "${new_tag}" && "${current_tag}" != "${new_tag}" ]]; then
      pack_file="${PACKS_DIR}/${name}/pack.json"
      if [[ -f "${pack_file}" ]]; then
        python3 - "$pack_file" "$new_tag" <<'PYEOF'
import json, sys
pack_file = sys.argv[1]
new_tag = sys.argv[2]
with open(pack_file) as f:
    data = json.load(f)
data['source_tag'] = new_tag
with open(pack_file, 'w') as f:
    json.dump(data, f, indent=2)
    f.write('\n')
PYEOF
        info "  ${name}: ${current_tag} → ${new_tag}"
      else
        warn "  ${name}: pack.json not found at ${pack_file}"
      fi
    fi
  done

  info "pack.json files updated. Review changes with: git diff aide/pkg/grammar/packs/"
else
  echo "" >&2
  info "run with --update to apply changes"
fi
