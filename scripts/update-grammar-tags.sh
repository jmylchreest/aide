#!/usr/bin/env bash
#
# update-grammar-tags.sh — Maintain tree-sitter grammar versions in release.yml
#
# Queries GitHub for the latest tags of each grammar repository and updates
# .github/workflows/release.yml in-place. Handles repos that use different
# tag formats (no v prefix, -with-generated-files suffixes, etc.).
#
# Usage:
#   scripts/update-grammar-tags.sh              # Check for updates (dry run)
#   scripts/update-grammar-tags.sh --update     # Update release.yml in-place
#   scripts/update-grammar-tags.sh --validate   # Validate current tags exist & have parser.c
#   scripts/update-grammar-tags.sh --json       # Output status as JSON
#
# Environment:
#   GITHUB_TOKEN  Optional; avoids rate limiting on the GitHub API.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
RELEASE_YML="${REPO_ROOT}/.github/workflows/release.yml"

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
gh_api() {
  local url="$1"
  local -a curl_args=(-fsSL --retry 2 --retry-delay 1)
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl_args+=(-H "Authorization: token ${GITHUB_TOKEN}")
  fi
  curl "${curl_args[@]}" "https://api.github.com${url}" 2>/dev/null
}

# Fetch the latest release-quality tag for a repo.
# Filters out pre-releases, draft tags, and -rc / -alpha / -beta suffixes.
# Returns the tag name (e.g. "v0.23.4" or "0.4.0").
latest_tag() {
  local repo="$1"
  local filter="${2:-}"  # Optional jq filter for special cases

  # Fetch up to 30 tags (sorted by creation date, newest first).
  local tags_json
  tags_json=$(gh_api "/repos/${repo}/tags?per_page=30") || {
    echo ""; return
  }

  if [[ -n "${filter}" ]]; then
    echo "${tags_json}" | jq -r "${filter}" | head -1
  else
    # Default: pick the first tag that looks like a release (no -rc, -alpha,
    # -beta, -dev suffixes). Allow -with-generated-files for swift.
    echo "${tags_json}" | jq -r '
      [.[] | .name |
        select(test("-(rc|alpha|beta|dev|pre|nightly)"; "i") | not) |
        select(test("-(pypi|crates-io)"; "i") | not)
      ] | .[0] // empty
    '
  fi
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

# --- Parse grammar definitions from release.yml ---------------------------
# Returns lines of: name|repo|c_symbol|current_tag|extra_src
parse_grammars() {
  sed -n '/^[[:space:]]*GRAMMARS=(/,/^[[:space:]]*)/p' "${RELEASE_YML}" \
    | grep -E '^\s+"[^"]+\|' \
    | sed 's/^[[:space:]]*"//; s/"$//'
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
      echo "  --update    Update release.yml in-place with latest tags"
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

if [[ ! -f "${RELEASE_YML}" ]]; then
  die "release.yml not found at ${RELEASE_YML}"
fi

# Parse current grammar definitions.
mapfile -t ENTRIES < <(parse_grammars)

if [[ ${#ENTRIES[@]} -eq 0 ]]; then
  die "no grammar definitions found in ${RELEASE_YML}"
fi

info "found ${#ENTRIES[@]} grammar definitions in release.yml"

# Track results.
declare -a RESULTS=()
UPDATES=0
ERRORS=0
CURRENT=0

for entry in "${ENTRIES[@]}"; do
  IFS='|' read -r name repo c_symbol current_tag extra_src <<< "${entry}"

  # Look up the latest tag.
  filter=$(tag_filter_for "${name}" "${repo}")
  new_tag=$(latest_tag "${repo}" "${filter}")

  if [[ -z "${new_tag}" ]]; then
    ERRORS=$((ERRORS + 1))
    status="error"
    detail="failed to fetch tags from ${repo}"
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
  info "applying updates to ${RELEASE_YML}..."

  for entry in "${ENTRIES[@]}"; do
    IFS='|' read -r name repo c_symbol current_tag extra_src <<< "${entry}"
    filter=$(tag_filter_for "${name}" "${repo}")
    new_tag=$(latest_tag "${repo}" "${filter}")

    if [[ -n "${new_tag}" && "${current_tag}" != "${new_tag}" ]]; then
      # Escape special characters for sed.
      old_escaped=$(printf '%s\n' "${current_tag}" | sed 's/[[\.*^$/]/\\&/g')
      new_escaped=$(printf '%s\n' "${new_tag}" | sed 's/[[\.*^$/]/\\&/g')

      # Replace only within the matching grammar line (match by name|repo).
      name_escaped=$(printf '%s\n' "${name}" | sed 's/[[\.*^$/]/\\&/g')
      sed -i "/${name_escaped}|/s|${old_escaped}|${new_escaped}|" "${RELEASE_YML}"

      info "  ${name}: ${current_tag} → ${new_tag}"
    fi
  done

  info "release.yml updated. Review changes with: git diff .github/workflows/release.yml"
else
  echo "" >&2
  info "run with --update to apply changes"
fi
