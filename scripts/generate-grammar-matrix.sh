#!/usr/bin/env bash
# generate-grammar-matrix.sh — Read dynamic grammar pack.json files and emit
# the GRAMMARS bash array entries used by the CI release workflow.
#
# When a pack's source_tag is "latest", resolves to the newest tag from the
# upstream GitHub repo. If the repo has no tags, falls back to the default
# branch (usually "main" or "master").
#
# Usage:
#   source <(./scripts/generate-grammar-matrix.sh)
#   # Now GRAMMARS is set as an array of "name|repo|c_symbol|tag|extra_src" entries.
#
# Or, to just print the entries (one per line):
#   ./scripts/generate-grammar-matrix.sh --print
#
# Environment:
#   GITHUB_TOKEN — optional; avoids GitHub API rate limits when resolving "latest"
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKS_DIR="${SCRIPT_DIR}/../aide/pkg/grammar/packs"

print_mode=false
if [[ "${1:-}" == "--print" ]]; then
  print_mode=true
fi

# resolve_latest_tag looks up the newest tag for a GitHub repo.
# Falls back to the default branch if no tags exist.
# Args: $1 = owner/repo (e.g. "tree-sitter/tree-sitter-go")
resolve_latest_tag() {
  local repo="$1"
  local tag=""

  # Build auth header if token available
  local auth_args=()
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    auth_args=(-H "Authorization: token ${GITHUB_TOKEN}")
  fi

  # Try: latest release (most repos use this)
  tag=$(curl -fsSL "${auth_args[@]+"${auth_args[@]}"}" \
    "https://api.github.com/repos/${repo}/releases/latest" 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin).get('tag_name',''))" 2>/dev/null) || true

  if [[ -n "$tag" ]]; then
    echo "$tag"
    return
  fi

  # Try: newest tag by created date (repos that have tags but no "releases")
  tag=$(curl -fsSL "${auth_args[@]+"${auth_args[@]}"}" \
    "https://api.github.com/repos/${repo}/tags?per_page=1" 2>/dev/null \
    | python3 -c "import json,sys; tags=json.load(sys.stdin); print(tags[0]['name'] if tags else '')" 2>/dev/null) || true

  if [[ -n "$tag" ]]; then
    echo "$tag"
    return
  fi

  # Fallback: default branch (repos with no tags at all)
  tag=$(curl -fsSL "${auth_args[@]+"${auth_args[@]}"}" \
    "https://api.github.com/repos/${repo}" 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin).get('default_branch','main'))" 2>/dev/null) || true

  if [[ -n "$tag" ]]; then
    echo "$tag"
    return
  fi

  # Last resort
  echo "main"
}

entries=()

for pack_json in "${PACKS_DIR}"/*/pack.json; do
  # Extract all fields in a single python3 call
  fields=$(python3 -c "
import json,sys
d=json.load(sys.stdin)
print('|'.join(d.get(k,'') for k in ('name','c_symbol','source_repo','source_tag','source_src_dir')))
" < "$pack_json")
  IFS='|' read -r name c_symbol source_repo source_tag source_src_dir <<< "$fields"

  # Skip packs without a grammar binary (meta-only or compiled-in)
  if [[ -z "$c_symbol" || -z "$source_repo" ]]; then
    continue
  fi

  # Skip packs with no source_tag at all (shouldn't happen, but be safe)
  if [[ -z "$source_tag" ]]; then
    continue
  fi

  # Resolve "latest" to actual tag/branch
  if [[ "$source_tag" == "latest" ]]; then
    resolved=$(resolve_latest_tag "$source_repo")
    echo "Resolved ${name}: latest -> ${resolved}" >&2
    source_tag="$resolved"
  fi

  entries+=("${name}|${source_repo}|${c_symbol}|${source_tag}|${source_src_dir}")
done

if $print_mode; then
  for e in "${entries[@]}"; do
    echo "$e"
  done
else
  # Emit a bash snippet that can be eval'd / sourced
  echo "GRAMMARS=("
  for e in "${entries[@]}"; do
    echo "  \"${e}\""
  done
  echo ")"
fi
