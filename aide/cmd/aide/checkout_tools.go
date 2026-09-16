package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/anchor"
	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CheckoutInput is embedded only in analysis and instance tools. Ordinary MCP
// arguments work even when the host provides neither roots nor custom metadata.
type CheckoutInput struct {
	CheckoutRoot string `json:"checkout_root,omitempty" jsonschema:"Absolute checkout directory. Optional when a concrete absolute file path or MCP roots identify the checkout; otherwise uses the Aide process launch checkout. Does not scope shared memory or decisions."`
}

func (i CheckoutInput) requestedCheckoutRoot() string { return i.CheckoutRoot }

type checkoutArguments interface{ requestedCheckoutRoot() string }
type checkoutScopeKey struct{}

// addCheckoutTool holds the selected store owner for the entire tool call.
// Selection stays in the request context; concurrent sessions never mutate a
// process-wide current checkout. Existing handlers reuse this scoped owner.
func addCheckoutTool[In checkoutArguments, Out any](s *MCPServer, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(s.server, tool, func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		file, filter := checkoutFileArgument(&input)
		var paths []string
		if file != nil && filepath.IsAbs(*file) {
			// A substring/glob filter is not evidence of checkout identity.
			if !filter || concreteFile(*file) {
				paths = []string{*file}
			}
		}
		c, source, err := s.resolveToolCheckout(ctx, req, input.requestedCheckoutRoot(), paths)
		if err != nil {
			result, _, toolErr := checkoutToolError(err)
			return result, zero, toolErr
		}
		scoped, release, err := s.checkoutView(ctx, c.Root)
		if err != nil {
			return nil, zero, err
		}
		defer release()
		if len(paths) > 0 {
			canonical, err := canonicalCheckoutFile(paths[0])
			if err != nil {
				return nil, zero, err
			}
			abs, rel, err := checkout.SourcePath(c.Root, canonical)
			if err != nil {
				return nil, zero, err
			}
			if filter {
				*file = rel
			} else {
				*file = abs
			}
		}
		result, output, err := handler(context.WithValue(ctx, checkoutScopeKey{}, scoped), req, input)
		if result != nil {
			if result.Meta == nil {
				result.Meta = mcp.Meta{}
			}
			result.Meta["aide/checkout"] = map[string]any{"id": c.ID, "root": c.Root, "source": source}
		}
		return result, output, err
	})
}

// Only concrete file selectors may choose a checkout. Absolute filter values
// must name an existing file; missing files still work with the exact-file tools.
func concreteFile(path string) bool {
	if strings.ContainsAny(path, "*?[]") {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func checkoutFileArgument(input any) (*string, bool) {
	switch i := input.(type) {
	case *CodeSymbolsInput:
		return &i.FilePath, false
	case *CodeOutlineInput:
		return &i.File, false
	case *CodeReadCheckInput:
		return &i.File, false
	case *CodeReadSymbolInput:
		return &i.File, false
	case *CodeSearchInput:
		return &i.FilePath, true
	case *CodeReferencesInput:
		return &i.FilePath, true
	case *FindingsSearchInput:
		return &i.FilePath, true
	case *FindingsListInput:
		return &i.FilePath, true
	case *FindingsAcceptInput:
		return &i.FilePath, true
	case *SurveySearchInput:
		return &i.FilePath, true
	case *SurveyListInput:
		return &i.FilePath, true
	default:
		return nil, false
	}
}

// resolveToolCheckout validates every request-specific hint before opening a
// store. A future multi-file tool can pass all its paths here and must agree on
// one checkout. Relative paths and search expressions never select a checkout.
func (s *MCPServer) resolveToolCheckout(ctx context.Context, req *mcp.CallToolRequest, argument string, paths []string) (checkout.Info, string, error) {
	var selected checkout.Info
	var source string
	selectRoot := func(root, via string) error {
		if !filepath.IsAbs(root) {
			return fmt.Errorf("%s must be an absolute checkout path", via)
		}
		c, err := store.CheckoutInfo(s.dbPath, root)
		if err != nil {
			return err
		}
		if selected.ID != "" && selected.ID != c.ID {
			return fmt.Errorf("conflicting checkout hints: %s selects %s, %s selects %s", source, selected.Root, via, c.Root)
		}
		if selected.ID == "" {
			selected, source = c, via
		}
		return nil
	}
	if argument != "" {
		if err := selectRoot(argument, "argument"); err != nil {
			return checkout.Info{}, "", err
		}
	}
	if req != nil && req.Params != nil {
		if value, ok := req.Params.Meta["aide/checkout_root"]; ok {
			root, valid := value.(string)
			if !valid {
				return checkout.Info{}, "", fmt.Errorf("aide/checkout_root must be a string")
			}
			if err := selectRoot(root, "metadata"); err != nil {
				return checkout.Info{}, "", err
			}
		}
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			continue
		}
		canonical, err := canonicalCheckoutFile(path)
		if err != nil {
			return checkout.Info{}, "", err
		}
		if err := selectRoot(filepath.Dir(canonical), "file"); err != nil {
			return checkout.Info{}, "", err
		}
	}
	if selected.ID != "" {
		return selected, source, nil
	}
	root, via, err := s.callerRoot(ctx, req, s.sourceRoot())
	if err != nil {
		return checkout.Info{}, "", err
	}
	c, err := store.CheckoutInfo(s.dbPath, root)
	if err == nil && via == "launch" && c.Root != anchor.RealPath(root) {
		return checkout.Info{}, "", fmt.Errorf("launch checkout %q no longer identifies that checkout; provide checkout_root", root)
	}
	return c, via, err
}

// Resolve existing parent aliases even when the requested file was deleted or
// has not been created yet. Permission failures and dangling links are errors.
func canonicalCheckoutFile(path string) (string, error) {
	path = filepath.Clean(path)
	parent := path
	for {
		_, err := os.Lstat(parent)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(parent, path)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, rel), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", err
		}
		parent = next
	}
}
