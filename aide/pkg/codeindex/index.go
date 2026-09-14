// Package codeindex reconciles a checkout's generated index with its source.
package codeindex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/anchor"
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
		if !anchor.Contains(root, path) || !anchor.Contains(anchor.RealPath(root), anchor.RealPath(path)) {
			return result, fmt.Errorf("path %q is outside checkout", path)
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
			data, err := os.ReadFile(abs)
			if err != nil {
				return err
			}
			lang := code.DetectLanguage(abs, data)
			fingerprint := fingerprints[lang]
			if fingerprint == "" {
				fingerprint, err = parser.Fingerprint(lang)
				if err != nil {
					return nil
				}
				fingerprints[lang] = fingerprint
			}
			hash := code.ContentHash(data)
			if !force {
				if old, err := cs.GetFileInfo(rel); err == nil && old.ContentHash == hash && old.ParserFingerprint == fingerprint {
					result.Skipped++
					if progress != nil {
						return progress(Progress{Path: rel, Skipped: true})
					}
					return nil
				}
			}
			f := code.FileBatch{Path: rel, ContentHash: hash, ParserFingerprint: fingerprint}
			reused := false
			if seed != nil && !force {
				if candidate, err := seed(rel, hash, fingerprint); err == nil {
					f = candidate
					reused = true
				}
			}
			if !reused {
				f.Symbols, err = parser.ParseContent(data, lang, rel)
				if err != nil {
					return err
				}
				f.References, err = parser.ParseContentReferences(data, lang, rel)
				if err != nil {
					return err
				}
			} else {
				result.Seeded++
			}
			f.ModTime = info.ModTime()
			f.SizeBytes = int64(len(data))
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
		files, err := cs.ListAllFileInfo()
		if err != nil {
			return result, err
		}
		for _, f := range files {
			if seen[f.Path] {
				continue
			}
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			if err := cs.ClearFile(f.Path); err != nil {
				return result, err
			}
			if err := cs.ClearFileReferences(f.Path); err != nil {
				return result, err
			}
			result.Removed++
		}
	}
	return result, nil
}
