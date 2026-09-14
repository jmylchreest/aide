package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExplicitCheckoutSubdirectoryResolvesRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(root, "src")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	s := newMCPServer(nil)
	s.dbPath = filepath.Join(root, ".aide", "memory", "memory.db")
	s.checkoutRoot = root
	got, release, err := s.requestCheckout(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"aide/checkout_root": subdir}}})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got != s || got.sourceRoot() != root {
		t.Fatalf("subdirectory selected wrong root: %s", got.sourceRoot())
	}
}

func testCodeStorePaths(t *testing.T, dbPath string) (string, string) {
	t.Helper()
	a, b, err := getCodeStorePaths(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}
func testSurveyStorePath(t *testing.T, dbPath string) string {
	t.Helper()
	p, err := getSurveyStorePath(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
