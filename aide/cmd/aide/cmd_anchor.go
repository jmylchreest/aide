package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
)

// anchorSchemaVersion identifies the anchor JSON contract. Bump on any
// breaking change to the payload shape; readers must check it.
const anchorSchemaVersion = 1

// anchorScope is one member of the scope chain: the anchored project itself
// (relation "self") followed by VCS-evidenced ancestor scopes (relation
// "parent"), nearest first. Chain membership requires VCS evidence — a
// submodule gitdir or an ancestor repository that physically contains the
// anchor — never mere .aide/ presence, so a stray .aide/ above unrelated
// repos can never become a context source.
type anchorScope struct {
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
	Chain           []anchorScope      `json:"chain"`
}

// resolveAnchor is the single authoritative project-root resolution,
// returning full provenance and the scope chain. findProjectRoot is a thin
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

	chain := []anchorScope{{
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
			chain = append(chain, anchorScope{
				Root:     c.vcsResolved,
				RealRoot: realPath(c.vcsResolved),
				Relation: "parent",
				Evidence: evidence,
			})
		}
	}
	if submoduleOwner != "" && !ownerPlaced && !seen[submoduleOwner] {
		chain = append(chain, anchorScope{
			Root:     submoduleOwner,
			RealRoot: realPath(submoduleOwner),
			Relation: "parent",
			Evidence: "submodule-gitdir",
		})
	}

	info.Chain = chain
}

// classifyAnchorMarker reports which marker anchored root, and the gitdir
// shape when the marker is .git.
func classifyAnchorMarker(root string, hasMarker bool) anchorProvenance {
	if !hasMarker {
		return anchorProvenance{}
	}
	gitPath := filepath.Join(root, ".git")
	if st, err := os.Stat(gitPath); err == nil {
		if st.IsDir() {
			return anchorProvenance{Marker: ".git", GitdirShape: "directory"}
		}
		shape, _ := gitfileInfo(gitPath)
		if shape == "" {
			// os.Stat proved .git is a FILE; an unparseable one is a
			// misconfiguration, not a plain repo — say so.
			shape = "invalid"
		}
		return anchorProvenance{Marker: ".git", GitdirShape: shape}
	}
	for _, marker := range vcsMarkerNames[1:] {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			return anchorProvenance{Marker: marker}
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".aide")); err == nil {
		return anchorProvenance{Marker: ".aide"}
	}
	return anchorProvenance{}
}

// anchorProjectIdentity derives the project name: the last path segment of
// the origin remote URL when one exists (rename-stable), else the directory
// basename. Mirrors the TS layer's getProjectName so both sides agree.
func anchorProjectIdentity(root string) (name, source string) {
	if repo, err := git.PlainOpenWithOptions(root, &git.PlainOpenOptions{}); err == nil {
		if remote, err := repo.Remote("origin"); err == nil {
			urls := remote.Config().URLs
			if len(urls) > 0 {
				if n := lastURLSegment(urls[0]); n != "" {
					return n, "git-remote"
				}
			}
		}
	}
	return filepath.Base(root), "basename"
}

// lastURLSegment extracts the repository name from a git remote URL:
// "git@host:org/repo.git" and "https://host/org/repo.git" both yield "repo".
func lastURLSegment(url string) string {
	s := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(url), "/"), ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// realPath resolves symlinks, falling back to the input on error so aliased
// spellings of one project map to one identity (mirrors registry.NormalizeRoot).
func realPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
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
