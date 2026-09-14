package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	bolt "go.etcd.io/bbolt"
)

// LockIndex serializes source-to-index operations for one checkout, so a
// watcher cannot publish newer bytes and then be overwritten by an older walk.
func (s *CodeStore) LockIndex() func() { s.indexMu.Lock(); return s.indexMu.Unlock }

// CheckoutSeed opens one existing compatible checkout's Bolt records read-only.
// No Bleve copy or long-lived read transaction is needed. Busy candidates are
// skipped; the daemon uses already-open stores instead.
func CheckoutSeed(dbPath, root string) (func(string, string, string) (code.FileBatch, error), func()) {
	c, err := CheckoutInfo(dbPath, root)
	if err != nil {
		return nil, func() {}
	}
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(dbPath), "checkouts"))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == c.ID {
			continue
		}
		path := filepath.Join(filepath.Dir(dbPath), "checkouts", entry.Name(), "code", "index.db")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true, Timeout: 10 * time.Millisecond})
		if err != nil {
			continue
		}
		source := &CodeStore{db: db}
		return source.SeedFile, func() { db.Close() }
	}
	return nil, func() {}
}

// IndexFiles commits a bounded group of file replacements in one Bolt transaction
// and one Bleve batch. Callers normally flush every 32 files.
func (s *CodeStore) IndexFiles(files []code.FileBatch) error {
	if len(files) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var deleted []string
	var symbols []*code.Symbol
	err := s.db.Update(func(tx *bolt.Tx) error {
		for _, f := range files {
			if f.Path == "" {
				return fmt.Errorf("file path is required")
			}
			if tx.Bucket(BucketFileIndex).Get([]byte(f.Path)) != nil {
				ids, err := s.clearFileTx(tx, f.Path)
				if err != nil {
					return err
				}
				deleted = append(deleted, ids...)
				if err := s.clearFileReferencesTx(tx, f.Path); err != nil {
					return err
				}
			}
			ids := make([]string, 0, len(f.Symbols))
			for _, sym := range f.Symbols {
				if sym == nil {
					return fmt.Errorf("nil symbol")
				}
				sym.FilePath = f.Path
				if err := s.addSymbolTx(tx, sym); err != nil {
					return err
				}
				ids = append(ids, sym.ID)
				symbols = append(symbols, sym)
			}
			for _, ref := range f.References {
				if ref == nil {
					return fmt.Errorf("nil reference")
				}
				ref.FilePath = f.Path
				if err := s.addReferenceTx(tx, ref); err != nil {
					return err
				}
			}
			if err := s.setFileInfoTx(tx, &code.FileInfo{Path: f.Path, ModTime: f.ModTime, SizeBytes: f.SizeBytes, Tokens: code.EstimateTokensFromSize(f.Path, f.SizeBytes), SymbolIDs: ids, ContentHash: f.ContentHash, ParserFingerprint: f.ParserFingerprint}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if s.search == nil {
		return nil
	}
	batch := s.search.NewBatch()
	for _, id := range deleted {
		batch.Delete(id)
	}
	for _, sym := range symbols {
		if err := batch.Index(sym.ID, buildSymbolBleveDoc(sym)); err != nil {
			return err
		}
	}
	return s.search.Batch(batch)
}

// SeedFile reads metadata, symbols and references from the same short snapshot.
// The target must supply hashes of its actual bytes and current parser. Legacy
// records without those proofs are never reused. No source IDs escape the copy.
func (s *CodeStore) SeedFile(path, hash, parser string) (code.FileBatch, error) {
	f := code.FileBatch{Path: path, ContentHash: hash, ParserFingerprint: parser}
	if hash == "" || parser == "" {
		return f, ErrNotFound
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(BucketFileIndex).Get([]byte(path))
		if raw == nil {
			return ErrNotFound
		}
		var info code.FileInfo
		if err := json.Unmarshal(raw, &info); err != nil {
			return err
		}
		if info.ContentHash != hash || info.ParserFingerprint != parser {
			return ErrNotFound
		}
		f.ModTime = info.ModTime
		f.SizeBytes = info.SizeBytes
		for _, id := range info.SymbolIDs {
			data := tx.Bucket(BucketSymbols).Get([]byte(id))
			if data == nil {
				return ErrNotFound
			}
			var sym code.Symbol
			if err := json.Unmarshal(data, &sym); err != nil {
				return err
			}
			if sym.FilePath != path {
				return ErrNotFound
			}
			sym.ID = ""
			f.Symbols = append(f.Symbols, &sym)
		}
		prefix := composeFileKey(path, "")
		cursor := tx.Bucket(BucketReferencesByFile).Cursor()
		for k, _ := cursor.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = cursor.Next() {
			id := string(k[len(prefix):])
			data := tx.Bucket(BucketReferences).Get([]byte(id))
			if data == nil {
				return ErrNotFound
			}
			var ref code.Reference
			if err := json.Unmarshal(data, &ref); err != nil {
				return err
			}
			if ref.FilePath != path {
				return ErrNotFound
			}
			ref.ID = ""
			f.References = append(f.References, &ref)
		}
		return nil
	})
	return f, err
}
