package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/url"
	"path/filepath"
	"time"
)

func analysisDir(dbPath, root string) (string, error) {
	c, err := store.CheckoutInfo(dbPath, root)
	if err != nil {
		return "", err
	}
	if err := store.RegisterCheckout(dbPath, c); err != nil {
		return "", err
	}
	return store.CheckoutDir(dbPath, c), nil
}

func (s *MCPServer) sourceRoot() string {
	if s.checkoutRoot != "" {
		return s.checkoutRoot
	}
	return store.CheckoutRoot(s.dbPath)
}

func (s *MCPServer) requestCheckout(ctx context.Context, req *mcp.CallToolRequest) (*MCPServer, func(), error) {
	root := s.sourceRoot()
	explicit := false
	if req != nil && req.Params != nil {
		if value, ok := req.Params.Meta["aide/checkout_root"]; ok {
			var valid bool
			root, valid = value.(string)
			if !valid || !filepath.IsAbs(root) {
				return nil, nil, fmt.Errorf("aide/checkout_root must be an absolute directory")
			}
			explicit = true
		}
	}
	if !explicit && req != nil && req.Params != nil {
		caps := req.ClientCapabilities()
		if caps != nil && caps.RootsV2 != nil {
			var roots *mcp.ListRootsResult
			if req.ProtocolVersion() >= "2026-07-28" {
				if value, ok := req.Params.InputResponses[checkoutRootsRequest]; ok {
					var valid bool
					roots, valid = value.(*mcp.ListRootsResult)
					if !valid || roots == nil {
						return nil, nil, fmt.Errorf("invalid checkout roots response")
					}
				} else {
					return nil, nil, errCheckoutRootsRequired
				}
			} else {
				if req.Session == nil {
					return nil, nil, fmt.Errorf("checkout roots require an MCP session")
				}
				rootsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				var err error
				roots, err = req.Session.ListRoots(rootsCtx, &mcp.ListRootsParams{})
				cancel()
				if err != nil {
					return nil, nil, fmt.Errorf("resolve caller checkout: %w", err)
				}
			}
			candidates := map[string]string{}
			for _, r := range roots.Roots {
				u, err := url.Parse(r.URI)
				if err != nil || u.Scheme != "file" || u.Host != "" && u.Host != "localhost" {
					continue
				}
				path := filepath.FromSlash(u.Path)
				if len(path) > 2 && path[0] == '/' && path[2] == ':' {
					path = path[1:]
				}
				c, err := store.CheckoutInfo(s.dbPath, path)
				if err == nil {
					candidates[c.ID] = c.Root
				}
			}
			if len(candidates) > 1 {
				return nil, nil, fmt.Errorf("multiple checkout roots; provide _meta.aide/checkout_root")
			}
			if len(roots.Roots) > 0 && len(candidates) == 0 {
				return nil, nil, fmt.Errorf("client roots do not belong to this project")
			}
			for _, candidate := range candidates {
				root = candidate
			}
		}
	}
	if checkout.RootFor(store.ProjectRootFromDB(s.dbPath), root) != root {
		if _, err := store.CheckoutInfo(s.dbPath, root); err != nil {
			return nil, nil, err
		}
	}
	if root == s.sourceRoot() {
		return s, func() {}, nil
	}
	view := newMCPServer(&mcpBackend{store: s.store(), instinctStore: s.instinctStore()})
	view.dbPath = s.dbPath
	view.checkoutRoot = root
	view.grammarLoader = s.grammarLoader
	if primary := s.grpcSrv(); primary != nil {
		scoped, release, err := primary.AcquireCheckout(ctx, root)
		if err != nil {
			return nil, nil, err
		}
		view.setBackend(&mcpBackend{store: s.store(), codeStore: scoped.GetCodeStore(), findingsStore: scoped.GetFindingsStore(), surveyStore: scoped.GetSurveyStore()})
		view.grpcServer.Store(scoped)
		view.codeStoreReady.Store(true)
		return view, release, nil
	}
	client, err := grpcapi.NewClientForCheckout(s.dbPath, root)
	if err != nil {
		return nil, nil, err
	}
	view.setBackend(&mcpBackend{store: s.store(), codeStore: adapter.NewCodeAdapter(client), findingsStore: adapter.NewFindingsAdapter(client), surveyStore: adapter.NewSurveyAdapter(client), grpcClient: client})
	view.codeStoreReady.Store(true)
	return view, func() { client.Close() }, nil
}

// startBackground prevents new watcher work from racing teardown's Wait.
func (s *MCPServer) startBackground(fn func()) {
	s.unifiedWatcherMu.Lock()
	if s.backgroundStopped {
		s.unifiedWatcherMu.Unlock()
		return
	}
	s.background.Add(1)
	s.unifiedWatcherMu.Unlock()
	go func() { defer s.background.Done(); fn() }()
}

const checkoutRootsRequest = "aide-checkout-roots"

var errCheckoutRootsRequired = errors.New("checkout roots required")

func checkoutToolError(err error) (*mcp.CallToolResult, any, error) {
	if errors.Is(err, errCheckoutRootsRequired) {
		return &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{checkoutRootsRequest: &mcp.ListRootsParams{}}}, nil, nil
	}
	return nil, nil, err
}
