package code

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"
)

// Include extraction code, queries and grammar versions in seed compatibility.
// A binary upgrade must never reuse records produced by different extraction.
//
//go:embed parser.go
var extractionSource string

var buildFingerprint = sync.OnceValue(func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.String()
	}
	return "unknown-build"
})

func ContentHash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func (p *Parser) Fingerprint(language string) (string, error) {
	if p.getLanguage(language) == nil {
		return "", fmt.Errorf("grammar unavailable: %s", language)
	}
	pack := p.registry.Get(language)
	if pack == nil {
		pack = p.registry.GetByAlias(language)
	}
	if pack == nil {
		return "", fmt.Errorf("grammar pack unavailable: %s", language)
	}
	data, err := json.Marshal(pack)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	io.WriteString(h, extractionSource)
	io.WriteString(h, buildFingerprint())
	h.Write(data)
	for _, info := range p.loader.Installed() {
		if info.Name != pack.Name {
			continue
		}
		io.WriteString(h, info.Version)
		if info.Path != "" {
			f, err := os.Open(info.Path)
			if err != nil {
				return "", err
			}
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return "", err
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
