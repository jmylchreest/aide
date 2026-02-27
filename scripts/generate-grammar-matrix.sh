#!/usr/bin/env bash
# generate-grammar-matrix.sh — Read dynamic grammar pack.json files and emit
# the GRAMMARS bash array entries used by the CI release workflow.
#
# Usage:
#   source <(./scripts/generate-grammar-matrix.sh)
#   # Now GRAMMARS is set as an array of "name|repo|c_symbol|tag|extra_src" entries.
#
# Or, to just print the entries (one per line):
#   ./scripts/generate-grammar-matrix.sh --print
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKS_DIR="${SCRIPT_DIR}/../aide/pkg/grammar/packs"

print_mode=false
if [[ "${1:-}" == "--print" ]]; then
  print_mode=true
fi

entries=()

for pack_json in "${PACKS_DIR}"/*/pack.json; do
  # Only dynamic grammars have both source_repo and c_symbol
  name=$(python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('name',''))" < "$pack_json")
  c_symbol=$(python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('c_symbol',''))" < "$pack_json")
  source_repo=$(python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('source_repo',''))" < "$pack_json")
  source_tag=$(python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('source_tag',''))" < "$pack_json")

  # Skip packs without a grammar binary (meta-only or compiled-in)
  if [[ -z "$c_symbol" || -z "$source_repo" || -z "$source_tag" ]]; then
    continue
  fi

  source_src_dir=$(python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('source_src_dir',''))" < "$pack_json")

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
