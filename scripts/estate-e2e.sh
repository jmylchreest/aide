#!/usr/bin/env bash
# Estate end-to-end test: multi-level nested repos + real submodules, each
# with its own .aide store. Exercises root resolution, session lifecycle,
# anchor cache placement/cleanup, and per-store ISOLATION of decisions,
# memories, and share exports.
#
# The isolation matrix is the pre-M1 BASELINE: today every store is fully
# independent. When the scope-chain read cascade lands, the expected
# results change (parent decisions become visible downward) — update the
# assertions here alongside that feature, per phase.
#
# Usage: scripts/estate-e2e.sh [workdir]   (default: mktemp -d)
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
E="${1:-$(mktemp -d -t aide-estate-XXXXXX)}"
SID="estate-e2e-$$"
PASS=0; FAIL=0
ok()   { PASS=$((PASS+1)); echo "  ok  - $1"; }
bad()  { FAIL=$((FAIL+1)); echo "  FAIL- $1"; }
check() { if eval "$2"; then ok "$1"; else bad "$1"; fi; }

echo "== estate: $E"
mkdir -p "$E/home" "$E/runtime"

echo "== building stamped dev binary"
(cd "$REPO/aide" && go build \
  -ldflags "-X github.com/jmylchreest/aide/aide/internal/version.Version=0.1.99-dev.0 -X github.com/jmylchreest/aide/aide/internal/version.Commit=estate" \
  -o "$E/aide" ./cmd/aide)

G() { git -c user.email=t@t -c user.name=t -c protocol.file.allow=always "$@"; }
run() { env -u AIDE_PLUGIN_ROOT -u CLAUDE_PLUGIN_ROOT -u AIDE_PROJECT_ROOT -u AIDE_PLATFORM \
        HOME="$E/home" XDG_RUNTIME_DIR="$E/runtime" PATH="$E:$PATH" "$@"; }
A() { local root="$1"; shift; run "$E/aide" --project-root "$root" "$@"; }

echo "== building estate (nested repos + real submodules)"
cd "$E"
git init -q standalone-a && (cd standalone-a && echo "content-a" > lib.txt && G add -A && G commit -qm init)
git init -q standalone-b && (cd standalone-b && echo "content-b" > lib.go  && G add -A && G commit -qm init)
git init -q tl && (cd tl && echo top > README.md && G add -A && G commit -qm init \
  && G submodule add -q ../standalone-a proj-sub && G commit -qm sub)
mkdir -p tl/mid && (cd tl/mid && git init -q . && echo mid > mid.txt && G add -A && G commit -qm init \
  && G submodule add -q ../../standalone-b mid-sub && G commit -qm sub)
mkdir -p tl/mid/leaf && (cd tl/mid/leaf && git init -q . && echo leaf > leaf.txt && G add -A && G commit -qm init)

LEVELS=(tl tl/proj-sub tl/mid tl/mid/mid-sub tl/mid/leaf)

echo "== phase 1: anchor chains per instance"
for d in "${LEVELS[@]}"; do
  echo "--- $d:"
  run "$E/aide" anchor --cwd="$E/$d" --json | python3 -c "
import json,sys; a=json.load(sys.stdin)
print('    root   :', a['root'].replace('$E/',''), '(' + (a['provenance'].get('gitdirShape') or a['provenance'].get('marker','?')) + ')')
print('    ident  :', a['identity']['projectName'], '('+a['identity']['source']+')')
for s in a['chain'][1:]: print('    parent :', s['root'].replace('$E/',''), '['+s['evidence']+']')"
done

echo "== phase 1b: downward estate map (survey subprojects, direct children only)"
A "$E/tl" survey run --analyzer=topology >/dev/null 2>&1
tl_map="$(A "$E/tl" survey list --kind=subproject 2>/dev/null)"
tl_children="$(echo "$tl_map" | grep -c "^\[" || true)"
check "tl lists its 2 direct children (got $tl_children)" "[ \"$tl_children\" = \"2\" ]"
check "tl sees proj-sub as submodule (identity standalone-a)" "echo \"$tl_map\" | grep -q 'submodule-gitdir'"
check "tl sees mid as nested repo" "echo \"$tl_map\" | grep -q 'Subproject mid'"
A "$E/tl/mid" survey run --analyzer=topology >/dev/null 2>&1
mid_map="$(A "$E/tl/mid" survey list --kind=subproject 2>/dev/null)"
mid_children="$(echo "$mid_map" | grep -c "^\[" || true)"
check "mid lists its 2 direct children (got $mid_children)" "[ \"$mid_children\" = \"2\" ]"

echo "== phase 2: per-store decisions + memories (writes)"
for d in "${LEVELS[@]}"; do
  tag="$(basename "$d")"
  A "$E/$d" decision set estate-topic "decided-by-$tag" --rationale="set at $d" >/dev/null
  A "$E/$d" memory add --category=learning "memory only for $tag" >/dev/null
done

echo "== phase 3: store isolation matrix (CLI reads are store-local by design; the cascade is injection-only — see phase 7)"
for d in "${LEVELS[@]}"; do
  tag="$(basename "$d")"
  got="$(A "$E/$d" decision get estate-topic 2>/dev/null | grep -o "decided-by-[a-z-]*" | head -1)"
  check "$d sees ONLY its own decision ($got)" "[ \"$got\" = \"decided-by-$tag\" ]"
  mems="$(A "$E/$d" memory list 2>/dev/null | grep -c "memory only for" || true)"
  check "$d has exactly 1 estate memory (got $mems)" "[ \"$mems\" = \"1\" ]"
done

echo "== phase 3b: --store routing (decision store-routing semantics)"
run "$E/aide" --project-root "$E/tl/mid/leaf" --store parent decision set store-routed "via-parent-from-leaf" >/dev/null 2>&1
got="$(A "$E/tl/mid" decision get store-routed 2>/dev/null | grep -o 'via-parent-from-leaf' | head -1)"
check "leaf --store parent lands in mid ($got)" "[ \"$got\" = \"via-parent-from-leaf\" ]"
check "leaf's own store untouched by parent write" "! (A $E/tl/mid/leaf decision get store-routed 2>/dev/null | grep -q via-parent)"
run "$E/aide" --project-root "$E/tl/mid/leaf" --store top decision set estate-rule "via-top-from-leaf" >/dev/null 2>&1
got="$(A "$E/tl" decision get estate-rule 2>/dev/null | grep -o 'via-top-from-leaf' | head -1)"
check "leaf --store top lands in tl ($got)" "[ \"$got\" = \"via-top-from-leaf\" ]"
check "estate root rejects --store parent" "! run $E/aide --project-root $E/tl --store parent decision set x y >/dev/null 2>&1"
git init -q "$E/bare-parent" && mkdir -p "$E/bare-parent/child" && git init -q "$E/bare-parent/child"
check "uninitialized parent refused" "! run $E/aide --project-root $E/bare-parent/child --store parent decision set x y >/dev/null 2>&1"
check "sibling path rejected" "! run $E/aide --project-root $E/tl/mid/leaf --store $E/tl/proj-sub decision set x y >/dev/null 2>&1"

echo "== phase 4: share export stays local to its store"
A "$E/tl" share export >/dev/null 2>&1 || true
check "tl has share exports" "ls $E/tl/.aide/shared/decisions/* >/dev/null 2>&1"
for d in tl/proj-sub tl/mid tl/mid/mid-sub tl/mid/leaf; do
  check "$d has NO share dir" "[ ! -d $E/$d/.aide/shared ]"
done
# --store routes share commands wholesale: run from the leaf, export and
# import operate on the PARENT's .aide/shared and store.
run "$E/aide" --project-root "$E/tl/mid/leaf" --store parent share export >/dev/null 2>&1 || true
check "routed export writes mid's shared dir" "ls $E/tl/mid/.aide/shared/decisions/* >/dev/null 2>&1"
check "routed export leaves leaf shared-less" "[ ! -d $E/tl/mid/leaf/.aide/shared ]"
check "routed import targets mid's store" "run $E/aide --project-root $E/tl/mid/leaf --store parent share import --dry-run >/dev/null 2>&1"

echo "== phase 5: session lifecycle against the deepest submodule"
SUB="$E/tl/mid/mid-sub"
echo "{\"session_id\":\"$SID\",\"cwd\":\"$SUB\",\"hook_event_name\":\"SessionStart\"}" \
  | (cd "$SUB" && run bun "$REPO/src/hooks/session-start.ts" > "$E/session-start.json" 2>/dev/null)
check "welcome context has Estate section"      "grep -q '## Estate' $E/session-start.json"
check "estate lists mid as parent"              "grep -q 'submodule-gitdir' $E/session-start.json"
check "estate warns stores are separate"        "grep -q 'OWN aide store' $E/session-start.json"
check "anchor cache in runtime dir"  "[ -f $E/runtime/aide/anchors/$SID.json ]"
check "anchor cache NOT in home"     "[ ! -f $E/home/.aide/anchors/$SID.json ]"
check "project anchor copy present"  "[ -f $SUB/.aide/state/anchor.json ]"
echo "{\"session_id\":\"$SID\",\"cwd\":\"$SUB\",\"hook_event_name\":\"PostToolUse\",\"tool_name\":\"Bash\"}" \
  | (cd "$SUB" && run bun "$REPO/src/hooks/hud-updater.ts" >/dev/null 2>&1)
scoped="$(A "$SUB" state list --json | python3 -c "import json,sys; print(sum(1 for r in json.load(sys.stdin) if r.get('agent')=='$SID'))")"
check "session-scoped counters written ($scoped keys)" "[ \"$scoped\" -ge 3 ]"
hud="$(echo "{\"session_id\":\"$SID\",\"cwd\":\"$SUB\"}" | run bun "$REPO/scripts/aide-hud.ts")"
check "HUD resolves via anchor ($hud)" "echo \"$hud\" | grep -q 'aide(0.1.99'"
echo "{\"session_id\":\"$SID\",\"cwd\":\"$SUB\",\"hook_event_name\":\"SessionEnd\",\"duration\":30000}" \
  | (cd "$SUB" && run bun "$REPO/src/hooks/session-end.ts" >/dev/null 2>&1)
sleep 1
check "teardown message in submodule store" "A $SUB message list 2>/dev/null | grep -q 'Session $SID ended'"
scoped_after="$(A "$SUB" state list --json | python3 -c "import json,sys; print(sum(1 for r in json.load(sys.stdin) if r.get('agent')=='$SID'))")"
check "session-scoped state cleared" "[ \"$scoped_after\" = \"0\" ]"
check "anchor cache entry deleted"   "[ ! -f $E/runtime/aide/anchors/$SID.json ]"

echo "== phase 6: no cross-store pollution from the session"
for d in tl tl/mid; do
  check "$d store has no session artifacts" "! (A $E/$d message list 2>/dev/null | grep -q $SID)"
done

echo "== phase 7: decision cascade in session injection (origin-labeled, nearest-wins)"
CSID="cascade-$$"
echo "{\"session_id\":\"$CSID\",\"cwd\":\"$E/tl/mid/leaf\",\"hook_event_name\":\"SessionStart\"}" \
  | (cd "$E/tl/mid/leaf" && run bun "$REPO/src/hooks/session-start.ts" > "$E/cascade-ctx.json" 2>/dev/null)
check "leaf inherits tl's estate-rule"            "grep -q 'estate-rule' $E/cascade-ctx.json"
check "inherited decision is origin-labeled"      "grep -q 'inherited from parent' $E/cascade-ctx.json"
check "estate-topic shows leaf's OWN value"       "grep -q 'decided-by-leaf' $E/cascade-ctx.json"
check "leaf's estate-topic NOT overridden by parents" "! grep -q 'decided-by-mid[^-]' $E/cascade-ctx.json"
check "store-routed (mid's) cascades to leaf too" "grep -q 'via-parent-from-leaf' $E/cascade-ctx.json"
run "$E/aide" --project-root "$E/tl/mid/leaf" session end --session=$CSID >/dev/null 2>&1

echo
echo "== RESULT: $PASS passed, $FAIL failed  (estate kept at $E; rm -rf it when done)"
[ "$FAIL" = "0" ]
