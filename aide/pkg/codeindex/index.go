// Package codeindex reconciles a checkout's generated index with its source.
package codeindex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/anchor"
	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

type Seed func(path, hash, parser string) (code.FileBatch, error)
type Result struct{ Indexed, Symbols, References, Skipped, Seeded, Removed int }
type Progress struct {
	Path    string
	Symbols int
	Skipped bool
}

// Run reads actual bytes before skipping or seeding. A full-root walk also drops
// deleted and newly ignored files. All paths and persisted keys are checkout-local.
func Run(ctx context.Context, cs store.CodeIndexStore, parser *code.Parser, root string, paths []string, force bool, seed Seed, progress func(Progress) error) (Result, error) {
	if owner, ok := cs.(interface{ LockIndex() func() }); ok {
		release := owner.LockIndex()
		defer release()
	}
	var result Result
	if len(paths) == 0 {
		paths = []string{"."}
	}
	ignore, err := aideignore.New(root)
	if err != nil {
		return result, err
	}
	skip := ignore.WalkFunc(root)
	batch := make([]code.FileBatch, 0, 32)
	batchBytes := int64(0)
	flush := func() error {
		if err := commitFiles(ctx, cs, batch, &result, progress); err != nil {
			return err
		}
		batch = batch[:0]
		batchBytes = 0
		return nil
	}
	seen := map[string]bool{}
	full := false
	fingerprints := map[string]string{}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		path = filepath.Clean(path)
		if _, _, err := checkout.SourcePath(root, path); err != nil {
			return result, err
		}
		if anchor.RealPath(path) == anchor.RealPath(root) {
			full = true
		}
		err := filepath.Walk(path, func(abs string, info os.FileInfo, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				return walkErr
			}
			if yes, dir := skip(abs, info); yes {
				if dir {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() || !code.SupportedFile(abs) {
				return nil
			}
			if !anchor.Contains(anchor.RealPath(root), anchor.RealPath(abs)) {
				return fmt.Errorf("source symlink escapes checkout: %s", abs)
			}
			rel, err := filepath.Rel(root, abs)
			if err != nil {
				return err
			}
			if seen[rel] {
				return nil
			}
			seen[rel] = true
			prepared, err := prepareFile(cs, parser, abs, rel, fingerprints, force, seed)
			if err != nil {
				return err
			}
			if prepared.unchanged {
				result.Skipped++
				if progress != nil {
					return progress(Progress{Path: rel, Skipped: true})
				}
				return nil
			}
			if prepared.batch == nil {
				return nil
			}
			f := *prepared.batch
			if prepared.seeded {
				result.Seeded++
			}
			f.ModTime = info.ModTime()
			batch = append(batch, f)
			batchBytes += f.SizeBytes
			if len(batch) >= 32 || batchBytes >= 8<<20 {
				return flush()
			}
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	if err := flush(); err != nil {
		return result, err
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if full {
		n, err := removeUnseen(ctx, cs, seen)
		result.Removed = n
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// prepareFile either verifies the current bundle, seeds compatible records, or
// parses the same source bytes for symbols and references.
type preparedFile struct {
	batch     *code.FileBatch
	unchanged bool
	seeded    bool
}

func prepareFile(cs store.CodeIndexStore, parser *code.Parser, abs, rel string, fingerprints map[string]string, force bool, seed Seed) (preparedFile, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return preparedFile{}, err
	}
	lang := code.DetectLanguage(abs, data)
	fingerprint := fingerprints[lang]
	if fingerprint == "" {
		fingerprint, err = parser.Fingerprint(lang)
		if err != nil {
			return preparedFile{}, nil
		}
		fingerprints[lang] = fingerprint
	}
	hash := code.ContentHash(data)
	if !force {
		if old, err := cs.GetFileInfo(rel); err == nil && old.ContentHash == hash && old.ParserFingerprint == fingerprint {
			return preparedFile{unchanged: true}, nil
		}
	}
	f := code.FileBatch{Path: rel, ContentHash: hash, ParserFingerprint: fingerprint, SizeBytes: int64(len(data))}
	if seed != nil && !force {
		if candidate, err := seed(rel, hash, fingerprint); err == nil {
			candidate.SizeBytes = int64(len(data))
			return preparedFile{batch: &candidate, seeded: true}, nil
		}
	}
	f.Symbols, err = parser.ParseContent(data, lang, rel)
	if err != nil {
		return preparedFile{}, err
	}
	f.References, err = parser.ParseContentReferences(data, lang, rel)
	if err != nil {
		return preparedFile{}, err
	}
	return preparedFile{batch: &f}, nil
}
func removeUnseen(ctx context.Context, cs store.CodeIndexStore, seen map[string]bool) (int, error) {
	files, err := cs.ListAllFileInfo()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, f := range files {
		if seen[f.Path] {
			continue
		}
		if ctx.Err() != nil {
			return removed, ctx.Err()
		}
		if err := cs.ClearFile(f.Path); err != nil {
			return removed, err
		}
		if err := cs.ClearFileReferences(f.Path); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func commitFiles(ctx context.Context, cs store.CodeIndexStore, batch []code.FileBatch, result *Result, progress func(Progress) error) error {
	if len(batch) == 0 {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if bulk, ok := cs.(interface{ IndexFiles([]code.FileBatch) error }); ok {
		if err := bulk.IndexFiles(batch); err != nil {
			return err
		}
	} else {
		for _, f := range batch {
			if err := cs.IndexFileBatch(f.Path, f.Symbols, f.References, f.ModTime, f.SizeBytes); err != nil {
				return err
			}
		}
	}
	for _, f := range batch {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		result.Indexed++
		result.Symbols += len(f.Symbols)
		result.References += len(f.References)
		if progress != nil {
			if err := progress(Progress{Path: f.Path, Symbols: len(f.Symbols)}); err != nil {
				return err
			}
		}
	}
	return nil
}
