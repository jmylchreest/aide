package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/anchor"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
)

// anchorSchemaVersion identifies the anchor JSON contract. Bump on any
// breaking change to the payload shape; readers must check it.
const anchorSchemaVersion = 1

// anchorLink is one member of the anchor chain: the anchored project itself
// (relation "self") followed by VCS-evidenced ancestor scopes (relation
// "parent"), nearest first. Chain membership requires VCS evidence — a
// submodule gitdir or an ancestor repository that physically contains the
// anchor — never mere .aide/ presence, so a stray .aide/ above unrelated
// repos can never become a context source.
type anchorLink struct {
	Root     string `json:"root"`
	RealRoot string `json:"realRoot"`
	Relation string `json:"relation"`           // "self" | "parent"
	Evidence string `json:"evidence,omitempty"` // "submodule-gitdir" | "ancestor-vcs-root"
}

type anchorProvenance struct {
	// Marker that decided the anchor: ".git", ".hg", ".svn", ".bzr",
	// ".fossil", ".aide", or "" when no marker was found.
	Marker string `json:"marker,omitempty"`
	// GitdirShape classifies a .git marker: "directory" (plain repo),
	// "worktree" (linked worktree — anchored at the main repo when its
	// gitdir contains a ".git" component to resolve through; a worktree of
	// a BARE repo has none and anchors at the checkout itself),
	// "submodule" (anchored at the submodule checkout itself), or
	// "invalid" (a .git file that is not a gitdir pointer — anchored at
	// the directory, surfaced so tooling can flag the misconfiguration).
	GitdirShape string `json:"gitdirShape,omitempty"`
}

type anchorIdentityInfo struct {
	ProjectName string `json:"projectName"`
	Source      string `json:"source"` // "git-remote" | "basename"
}

// anchorInfo is the full resolution result: identity and topology only,
// never liveness — whether an ancestor's daemon is up is probed at use
// time, not recorded here.
type anchorInfo struct {
	SchemaVersion   int                `json:"schemaVersion"`
	ResolverVersion string             `json:"resolverVersion"`
	Root            string             `json:"root"`
	RealRoot        string             `json:"realRoot"`
	HasMarker       bool               `json:"hasMarker"`
	Source          string             `json:"source"` // "env" | "walk" | "none"
	Provenance      anchorProvenance   `json:"provenance"`
	Identity        anchorIdentityInfo `json:"identity"`
	DBPath          string             `json:"dbPath"`
	SocketPath      string             `json:"socketPath"`
	Chain           []anchorLink       `json:"chain"`
}

// resolveAnchor is the single authoritative project-root resolution,
// returning full provenance and the anchor chain. findProjectRoot is a thin
// wrapper over it — do not add resolution logic anywhere else.
//
// startCwd == "" means the process working directory. AIDE_PROJECT_ROOT,
// when set to an existing directory, short-circuits the walk (source
// "env") regardless of startCwd — same contract as findProjectRoot.
//
// Walk semantics (unchanged from the original findProjectRoot): collect
// every ancestor carrying .aide/ or a VCS marker; the closest VCS root
// wins — a VCS boundary, including a submodule, is the project boundary.
// Worktree .git files resolve to the main repository root so all worktrees
// share one store; submodule .git files anchor the submodule checkout
// itself. ~/.aide/ is skipped as a marker unless cwd is $HOME. .aide/-only
// candidates apply only when no VCS marker exists anywhere in the
// ancestry. With no marker at all, the cwd is returned with
// HasMarker=false.
func resolveAnchor(startCwd string) anchorInfo {
	info := anchorInfo{
		SchemaVersion:   anchorSchemaVersion,
		ResolverVersion: version.Short(),
		Source:          "none",
	}

	if override := os.Getenv("AIDE_PROJECT_ROOT"); override != "" {
		abs, err := filepath.Abs(override)
		if err == nil {
			st, statErr := os.Stat(abs)
			switch {
			case statErr != nil || !st.IsDir():
				fmt.Fprintf(os.Stderr, "aide: AIDE_PROJECT_ROOT=%q is not a directory; falling back to walk-up\n", override)
			case !anchorOverrideAllowed(abs):
				// An override pointing at an unmarked directory is almost
				// always stale (repo moved/renamed, leaked env from another
				// project). Anchoring there would plant .aide/ in an
				// arbitrary directory — require a marker, or AIDE_FORCE_INIT
				// to say "yes, really".
				fmt.Fprintf(os.Stderr, "aide: AIDE_PROJECT_ROOT=%q has no .aide/ or VCS marker (set AIDE_FORCE_INIT=1 to use it anyway); falling back to walk-up\n", override)
			default:
				info.Root = abs
				info.HasMarker = true
				info.Source = "env"
				finishAnchor(&info, abs)
				return info
			}
		} else {
			fmt.Fprintf(os.Stderr, "aide: AIDE_PROJECT_ROOT=%q is not a directory; falling back to walk-up\n", override)
		}
	}

	cwd := startCwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			info.Root = "."
			return info
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	cands := collectAnchorCandidates(cwd)

	// Priority: closest VCS root wins (see doc comment).
	for _, c := range cands {
		if c.hasVCS {
			info.Root = c.vcsResolved
			info.HasMarker = true
			info.Source = "walk"
			finishAnchor(&info, c.dir)
			return info
		}
	}
	// Then closest .aide/-only (standalone projects with no VCS).
	for _, c := range cands {
		if c.hasAide {
			info.Root = c.dir
			info.HasMarker = true
			info.Source = "walk"
			finishAnchor(&info, c.dir)
			return info
		}
	}

	info.Root = cwd
	finishAnchor(&info, cwd)
	return info
}

// anchorSessionIDRe matches valid session ids for cache paths — mirrors
// SESSION_ID_RE in src/lib/anchor.ts (path-traversal guard).
var anchorSessionIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// anchorCacheDirs returns the session-anchor cache directories, preferred
// first — the exact contract of anchorCacheDirs in src/lib/anchor.ts:
// XDG_RUNTIME_DIR/aide/anchors when the runtime dir exists (Linux), then
// ~/.aide/anchors as the portable fallback.
func anchorCacheDirs() []string {
	var dirs []string
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		if st, err := os.Stat(xdg); err == nil && st.IsDir() {
			dirs = append(dirs, filepath.Join(xdg, "aide", "anchors"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".aide", "anchors"))
	}
	return dirs
}

// deleteSessionAnchor removes a session's anchor cache entries from every
// candidate location. Called from session end so live sessions are the
// only ones with cache files; the TS TTL sweep remains the crash backstop.
func deleteSessionAnchor(sessionID string) {
	if !anchorSessionIDRe.MatchString(sessionID) {
		return
	}
	for _, dir := range anchorCacheDirs() {
		_ = os.Remove(filepath.Join(dir, sessionID+".json"))
	}
}

// extractStoreFlag pulls --store=parent|top|<path> out of the arg list.
func extractStoreFlag(args []string) (string, []string, bool) {
	for i, a := range args {
		if a == "--store" {
			if i+1 >= len(args) {
				return "", args, false
			}
			return args[i+1], append(append([]string{}, args[:i]...), args[i+2:]...), true
		}
		if strings.HasPrefix(a, "--store=") {
			return a[len("--store="):], append(append([]string{}, args[:i]...), args[i+1:]...), true
		}
	}
	return "", args, false
}

// resolveStoreTarget maps a --store selector onto the anchor chain,
// per decision store-routing:
//
//	parent  → chain[1], the nearest containing project
//	top     → the outermost ancestor
//	<path>  → an explicit chain member (self included); anything outside
//	          the chain is rejected — unrelated stores are --project-root's
//	          job, keeping --store scoped to "within my own estate"
//
// Errors are hard: a missing parent or a target without an existing .aide
// store must surface to the caller, never silently fall back to self — a
// write landing in a different store than requested is the misplacement
// bug class the anchor exists to kill.
func resolveStoreTarget(a anchorInfo, selector string) (string, error) {
	var target string
	switch selector {
	case "":
		return "", fmt.Errorf("--store requires a value: parent, top, or a path")
	case "self":
		target = a.Root
	case "parent":
		if len(a.Chain) < 2 {
			return "", fmt.Errorf("no parent in the anchor chain — %s is the estate root (run 'aide anchor' to inspect)", a.Root)
		}
		target = a.Chain[1].Root
	case "top":
		if len(a.Chain) < 2 {
			return "", fmt.Errorf("no ancestors in the anchor chain — %s is the estate root (run 'aide anchor' to inspect)", a.Root)
		}
		target = a.Chain[len(a.Chain)-1].Root
	default:
		abs, err := filepath.Abs(selector)
		if err != nil {
			return "", fmt.Errorf("--store=%q: %w", selector, err)
		}
		resolved := realPath(abs)
		for _, link := range a.Chain {
			if link.Root == abs || link.RealRoot == resolved {
				target = link.Root
				break
			}
		}
		if target == "" {
			return "", fmt.Errorf("--store=%q is not in this project's anchor chain (run 'aide anchor' to inspect; use --project-root for unrelated stores)", selector)
		}
	}

	if !anchorHasStore(target) {
		return "", fmt.Errorf("--store target %s has no .aide store yet — initialize it by running a session there (or use --project-root explicitly)", target)
	}
	return target, nil
}

func anchorHasStore(root string) bool {
	st, err := os.Stat(filepath.Join(root, ".aide"))
	return err == nil && st.IsDir()
}

// anchorOverrideAllowed reports whether an AIDE_PROJECT_ROOT override may
// anchor dir: it carries a project marker, or AIDE_FORCE_INIT is set.
func anchorOverrideAllowed(dir string) bool {
	if os.Getenv("AIDE_FORCE_INIT") == "1" || strings.EqualFold(os.Getenv("AIDE_FORCE_INIT"), "true") {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, ".aide")); err == nil {
		return true
	}
	return isVCSRoot(dir)
}

// anchorCand is one directory on the walk carrying a marker.
type anchorCand struct {
	dir         string
	hasAide     bool
	hasVCS      bool
	vcsResolved string // worktree-resolved root for .git files; dir otherwise
}

// collectAnchorCandidates walks from cwd to / collecting marker-bearing
// directories, nearest first.
func collectAnchorCandidates(cwd string) []anchorCand {
	homeDir, _ := os.UserHomeDir()
	var cands []anchorCand
	dir := cwd
	for {
		c := anchorCand{dir: dir}
		if _, err := os.Stat(filepath.Join(dir, ".aide")); err == nil {
			// Skip ~/.aide/ as a project marker unless cwd is $HOME itself.
			// ~/.aide/ is the TS layer's global config dir, not a project.
			if homeDir == "" || dir != homeDir || cwd == homeDir {
				c.hasAide = true
			}
		}
		if vcsDir, resolved, ok := vcsMarker(dir); ok {
			c.hasVCS = true
			if resolved != "" {
				c.vcsResolved = resolved
			} else {
				c.vcsResolved = vcsDir
			}
		}
		if c.hasAide || c.hasVCS {
			cands = append(cands, c)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cands
}

// finishAnchor fills provenance, identity, paths, and the chain for an
// already-decided root. markerDir is the directory whose marker DECIDED
// the anchor — for a linked worktree that is the worktree checkout (whose
// .git file carries the worktree shape), not the resolved main-repo root.
func finishAnchor(info *anchorInfo, markerDir string) {
	info.RealRoot = realPath(info.Root)
	info.Provenance = classifyAnchorMarker(markerDir, info.HasMarker)
	info.Identity.ProjectName, info.Identity.Source = anchorProjectIdentity(info.Root)
	info.DBPath = computeDBPath(info.Root)
	info.SocketPath = grpcapi.SocketPathFromDB(info.DBPath)

	chain := []anchorLink{{
		Root:     info.Root,
		RealRoot: info.RealRoot,
		Relation: "self",
	}}

	// Parents derive from the RESOLVED root's ancestry (nearest-first by
	// construction), never the launch path — same root, same chain, from
	// any entry. A submodule's gitdir owner is usually also an ancestor;
	// when reached it carries the stronger submodule-gitdir evidence, and
	// an out-of-tree owner is appended after the contained ancestors.
	var submoduleOwner string
	if info.Provenance.GitdirShape == "submodule" {
		if _, owner := gitfileInfo(filepath.Join(info.Root, ".git")); owner != "" && owner != info.Root {
			submoduleOwner = owner
		}
	}

	seen := map[string]bool{info.Root: true}
	ownerPlaced := false
	if parentDir := filepath.Dir(info.Root); parentDir != info.Root {
		for _, c := range collectAnchorCandidates(parentDir) {
			if !c.hasVCS || seen[c.vcsResolved] {
				continue
			}
			seen[c.vcsResolved] = true
			evidence := "ancestor-vcs-root"
			if c.vcsResolved == submoduleOwner {
				evidence = "submodule-gitdir"
				ownerPlaced = true
			}
			chain = append(chain, anchorLink{
				Root:     c.vcsResolved,
				RealRoot: realPath(c.vcsResolved),
				Relation: "parent",
				Evidence: evidence,
			})
		}
	}
	if submoduleOwner != "" && !ownerPlaced && !seen[submoduleOwner] {
		chain = append(chain, anchorLink{
			Root:     submoduleOwner,
			RealRoot: realPath(submoduleOwner),
			Relation: "parent",
			Evidence: "submodule-gitdir",
		})
	}

	info.Chain = chain
}

func classifyAnchorMarker(root string, hasMarker bool) anchorProvenance {
	if !hasMarker {
		return anchorProvenance{}
	}
	marker, shape := anchor.ClassifyDir(root)
	return anchorProvenance{Marker: marker, GitdirShape: shape}
}

func anchorProjectIdentity(root string) (name, source string) {
	return anchor.ProjectIdentity(root)
}

func lastURLSegment(url string) string {
	return anchor.LastURLSegment(url)
}

func realPath(p string) string {
	return anchor.RealPath(p)
}

// cmdAnchor implements `aide anchor [--json] [--cwd=PATH]`.
//
// It is a read-only pre-store fast path: it runs before any .aide/
// directory creation, never writes anything, and succeeds (exit 0) even
// when no marker is found — hasMarker/source tell the caller what
// happened. This is the single resolution authority the TS hook layer and
// harness integrations consume instead of re-implementing the walk.
func cmdAnchor(args []string) error {
	cwd := parseFlag(args, "--cwd=")
	info := resolveAnchor(cwd)

	if hasFlag(args, "--json") {
		out, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("root:      %s\n", info.Root)
	if info.RealRoot != info.Root {
		fmt.Printf("realRoot:  %s\n", info.RealRoot)
	}
	fmt.Printf("source:    %s", info.Source)
	if info.Provenance.Marker != "" {
		fmt.Printf(" (marker %s", info.Provenance.Marker)
		if info.Provenance.GitdirShape != "" {
			fmt.Printf(", %s", info.Provenance.GitdirShape)
		}
		fmt.Print(")")
	}
	fmt.Println()
	fmt.Printf("identity:  %s (%s)\n", info.Identity.ProjectName, info.Identity.Source)
	for _, s := range info.Chain {
		if s.Relation == "parent" {
			fmt.Printf("parent:    %s (%s)\n", s.Root, s.Evidence)
		}
	}
	if !info.HasMarker {
		fmt.Println("note:      no .aide/ or VCS marker found (root is the working directory)")
	}
	return nil
}
