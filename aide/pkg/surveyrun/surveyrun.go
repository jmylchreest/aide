// Package surveyrun executes survey analyzers against the stores. It is the
// single implementation behind every entry point — the MCP survey_run tool,
// the CLI `aide survey run`, and the gRPC SurveyService.Run RPC — so a
// daemon, a direct CLI, and a gRPC client all produce identical results.
// Analyzers must run in the process that owns the BoltDB stores; callers on
// the wrong side of a socket delegate here via the RPC.
package surveyrun

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// diffListLimit bounds how many stored entries are fetched to diff a re-run
// against. Well above any real analyzer output.
const diffListLimit = 100000

// Result is one analyzer's outcome.
type Result struct {
	Analyzer string
	Entries  int
	Err      string // non-empty when the analyzer failed
	Summary  string // formatted detail: counts, diff suffix, map changes
}

// AllAnalyzers is the default run set.
func AllAnalyzers() []string {
	return []string{survey.AnalyzerTopology, survey.AnalyzerEntrypoints, survey.AnalyzerChurn, survey.AnalyzerModules}
}

// Run executes the named analyzers (nil/empty = all) and stores their
// entries. codeStore may be nil: entrypoints degrades to file scanning and
// modules reports an error result.
func Run(rootDir string, analyzers []string, surveyStore store.SurveyStore, codeStore store.CodeIndexStore) []Result {
	if len(analyzers) == 0 || (len(analyzers) == 1 && analyzers[0] == "") {
		analyzers = AllAnalyzers()
	}
	headCommit := survey.HeadCommit(rootDir)

	results := make([]Result, 0, len(analyzers))
	for _, name := range analyzers {
		results = append(results, runOne(rootDir, name, headCommit, surveyStore, codeStore))
	}
	return results
}

func runOne(rootDir, name, headCommit string, surveyStore store.SurveyStore, codeStore store.CodeIndexStore) Result {
	res := Result{Analyzer: name}

	switch name {
	case survey.AnalyzerTopology:
		result, err := survey.RunTopology(rootDir)
		if err != nil {
			res.Err = err.Error()
			return res
		}
		return storeEntries(res, surveyStore, name, headCommit, result.Entries, "")

	case survey.AnalyzerEntrypoints:
		var cs survey.CodeSearcher
		if codeStore != nil {
			cs = &codeSearcher{store: codeStore}
		}
		result, err := survey.RunEntrypoints(rootDir, cs)
		if err != nil {
			res.Err = err.Error()
			return res
		}
		note := ""
		if cs == nil {
			note = " (code index not available — entrypoint detection limited)"
		}
		return storeEntries(res, surveyStore, name, headCommit, result.Entries, note)

	case survey.AnalyzerChurn:
		result, err := survey.RunChurn(rootDir, 0, 0)
		if err != nil {
			res.Err = err.Error()
			return res
		}
		return storeEntries(res, surveyStore, name, headCommit, result.Entries, "")

	case survey.AnalyzerModules:
		if codeStore == nil {
			res.Err = "code index not available — run 'aide code index' first"
			return res
		}
		prevEntries, _ := surveyStore.ListEntries(survey.SearchOptions{Analyzer: name, Limit: diffListLimit})
		result, err := survey.RunModules(survey.ModulesConfig{
			RootDir:  rootDir,
			Source:   &modulesSource{store: codeStore},
			Previous: survey.PreviousAssignmentFromEntries(prevEntries),
		})
		if err != nil {
			res.Err = err.Error()
			return res
		}
		note := fmt.Sprintf(" [%d modules, %d unclustered, imports resolved %d/%d]",
			result.Communities, result.Singletons, result.ImportsResolved, result.ImportsTotal)
		res = storeEntries(res, surveyStore, name, headCommit, result.Entries, note)
		if res.Err == "" {
			if diff := survey.DiffModules(prevEntries, result.Entries).Summary(); diff != "" {
				res.Summary += "\n  map changes: " + diff
			}
		}
		return res

	default:
		res.Err = fmt.Sprintf("unknown analyzer: %s", name)
		return res
	}
}

// storeEntries stamps the run commit, replaces the analyzer's entries, and
// formats the display summary with the entry diff. Listing the old entries
// first is what makes the added/removed diff possible — replace is wholesale.
func storeEntries(res Result, surveyStore store.SurveyStore, analyzer, headCommit string, entries []*survey.Entry, note string) Result {
	oldEntries, err := surveyStore.ListEntries(survey.SearchOptions{Analyzer: analyzer, Limit: diffListLimit})
	if err != nil {
		oldEntries = nil // diff is best-effort; storing still proceeds
	}
	survey.StampRunCommit(entries, headCommit)
	if err := surveyStore.ReplaceEntriesForAnalyzer(analyzer, entries); err != nil {
		res.Err = fmt.Sprintf("store error: %v", err)
		return res
	}

	res.Entries = len(entries)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d entries", len(entries))
	if d := survey.DiffEntries(oldEntries, entries); d.Added > 0 || d.Removed > 0 {
		fmt.Fprintf(&sb, " (+%d, -%d)", d.Added, d.Removed)
	}
	if headCommit != "" {
		fmt.Fprintf(&sb, " @ %.8s", headCommit)
	}
	sb.WriteString(note)
	res.Summary = sb.String()
	return res
}

// FormatResults renders results as the display block shared by every entry
// point: "analyzer: summary" or "analyzer: error: ...", one per line.
func FormatResults(results []Result) string {
	var sb strings.Builder
	for _, r := range results {
		if r.Err != "" {
			fmt.Fprintf(&sb, "%s: error: %s\n", r.Analyzer, r.Err)
			continue
		}
		fmt.Fprintf(&sb, "%s: %s\n", r.Analyzer, r.Summary)
	}
	return sb.String()
}

// codeSearcher bridges store.CodeIndexStore to survey.CodeSearcher for the
// entrypoints analyzer.
type codeSearcher struct {
	store store.CodeIndexStore
}

func (a *codeSearcher) FindSymbols(query string, kind string, limit int) ([]survey.SymbolHit, error) {
	results, err := a.store.SearchSymbols(query, code.SearchOptions{Kind: kind, Limit: limit})
	if err != nil {
		return nil, err
	}
	hits := make([]survey.SymbolHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, survey.SymbolHit{
			Name:     r.Symbol.Name,
			Kind:     r.Symbol.Kind,
			FilePath: r.Symbol.FilePath,
			Line:     r.Symbol.StartLine,
			Language: r.Symbol.Language,
		})
	}
	return hits, nil
}

func (a *codeSearcher) FindReferences(symbolName string, kind string, limit int) ([]survey.ReferenceHit, error) {
	results, err := a.store.SearchReferences(code.ReferenceSearchOptions{SymbolName: symbolName, Kind: kind, Limit: limit})
	if err != nil {
		return nil, err
	}
	hits := make([]survey.ReferenceHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, survey.ReferenceHit{Symbol: r.SymbolName, Kind: r.Kind, FilePath: r.FilePath, Line: r.Line})
	}
	return hits, nil
}

// modulesSource feeds the modules analyzer from the code index. The
// symbol->files map is built once from ListAllSymbols rather than issuing a
// search per referenced name — the analyzer asks about thousands of names.
type modulesSource struct {
	store    store.CodeIndexStore
	symFiles map[string][]string
}

func (a *modulesSource) ListSourceFiles() ([]survey.ModuleFile, error) {
	infos, err := a.store.ListAllFileInfo()
	if err != nil {
		return nil, err
	}
	files := make([]survey.ModuleFile, 0, len(infos))
	for _, fi := range infos {
		lang := code.DetectLanguage(fi.Path, nil)
		if lang == "" {
			continue
		}
		files = append(files, survey.ModuleFile{Path: fi.Path, Language: lang})
	}
	return files, nil
}

func (a *modulesSource) FileReferences(filePath string) ([]survey.ReferenceHit, error) {
	refs, err := a.store.GetFileReferences(filePath)
	if err != nil {
		return nil, err
	}
	hits := make([]survey.ReferenceHit, 0, len(refs))
	for _, r := range refs {
		hits = append(hits, survey.ReferenceHit{Symbol: r.SymbolName, Kind: r.Kind, FilePath: r.FilePath, Line: r.Line})
	}
	return hits, nil
}

func (a *modulesSource) DefiningFiles(symbolName string) ([]string, error) {
	if a.symFiles == nil {
		a.symFiles = make(map[string][]string)
		syms, err := a.store.ListAllSymbols(1 << 22)
		if err != nil {
			return nil, err
		}
		for _, s := range syms {
			existing := a.symFiles[s.Name]
			dup := false
			for _, f := range existing {
				if f == s.FilePath {
					dup = true
					break
				}
			}
			// One extra beyond the ambiguity cap is enough to disqualify.
			if !dup && len(existing) <= survey.MaxSymbolDefiners {
				a.symFiles[s.Name] = append(existing, s.FilePath)
			}
		}
	}
	return a.symFiles[symbolName], nil
}
