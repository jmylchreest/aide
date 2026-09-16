package grpcapi

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/anchor"
	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/codeindex"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const checkoutRootKey = "aide-checkout-root"

func (s *Server) SeedSource() codeindex.Seed { return s.seedFile }

// AcquireCheckout lets an in-process MCP session use the same routing and
// ownership checks as a gRPC client. Release must cover the complete tool call.
func (s *Server) AcquireCheckout(ctx context.Context, root string) (*Server, func(), error) {
	return s.checkoutFor(metadata.NewIncomingContext(ctx, metadata.Pairs(checkoutRootKey, root)))
}

type checkoutOwner struct {
	server *Server
	stop   func()
}

// One manager belongs to the elected memory database owner. Its read lock is
// held for the lifetime of each checkout RPC, including streaming requests.
type checkoutManager struct {
	mu     sync.RWMutex
	owners map[string]checkoutOwner
	onOpen func(*Server, checkout.Info) (func(), error)
	closed bool
}

func (s *Server) SourceRoot() string {
	if s.checkoutInfo != nil {
		return s.checkoutInfo.Root
	}
	return store.ProjectRootFromDB(s.dbPath)
}

// EnableCheckouts registers this process's existing stores and enables lazy
// ownership of other worktrees of the same repository. Called before Start.
func (s *Server) EnableCheckouts(root string, onOpen func(*Server, checkout.Info) (func(), error)) error {
	c, err := store.CheckoutInfo(s.dbPath, root)
	if err != nil {
		return err
	}
	if err := store.RegisterCheckout(s.dbPath, c); err != nil {
		return err
	}
	s.checkoutInfo = &c
	s.checkouts = &checkoutManager{owners: map[string]checkoutOwner{c.ID: {server: s}}, onOpen: onOpen}
	return nil
}

func (s *Server) checkoutFor(ctx context.Context) (*Server, func(), error) {
	m := s.checkouts
	if m == nil {
		return s, func() {}, nil
	}
	root := store.ProjectRootFromDB(s.dbPath)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(checkoutRootKey); len(values) > 0 {
			root = values[0]
		}
	}
	if !filepath.IsAbs(root) {
		return nil, nil, status.Error(codes.InvalidArgument, "checkout root must be absolute")
	}
	c, err := store.CheckoutInfo(s.dbPath, root)
	if err != nil {
		return nil, nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if c.Root != anchor.RealPath(root) {
		return nil, nil, status.Error(codes.InvalidArgument, "checkout root no longer identifies that checkout")
	}
	m.mu.RLock()
	if owner, ok := m.owners[c.ID]; ok && !m.closed {
		if owner.server.SourceRoot() == c.Root {
			return owner.server, m.mu.RUnlock, nil
		}
		if owner.server == s {
			m.mu.RUnlock()
			return nil, nil, status.Error(codes.FailedPrecondition, "daemon checkout moved; restart the daemon to reopen its watcher")
		}
	}
	m.mu.RUnlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, nil, status.Error(codes.Unavailable, "checkout stores are closed")
	}
	if owner, ok := m.owners[c.ID]; ok && owner.server.SourceRoot() != c.Root {
		if owner.stop != nil {
			owner.stop()
		}
		delete(m.owners, c.ID)
	}
	if _, ok := m.owners[c.ID]; !ok {
		owner, err := s.openCheckout(c)
		if err != nil {
			m.mu.Unlock()
			return nil, nil, err
		}
		m.owners[c.ID] = owner
	}
	m.mu.Unlock()
	// Reacquire through the lookup so shutdown cannot race the returned handle.
	return s.checkoutFor(ctx)
}

func (s *Server) openCheckout(c checkout.Info) (checkoutOwner, error) {
	if err := store.RegisterCheckout(s.dbPath, c); err != nil {
		return checkoutOwner{}, err
	}
	dir := store.CheckoutDir(s.dbPath, c)
	cs, err := store.NewCodeStore(filepath.Join(dir, "code", "index.db"), filepath.Join(dir, "code", "search.bleve"))
	if err != nil {
		return checkoutOwner{}, err
	}
	fs, err := store.NewCheckoutFindingsStore(filepath.Join(dir, "findings"), s.store, c)
	if err != nil {
		cs.Close()
		return checkoutOwner{}, err
	}
	ss, err := store.NewSurveyStore(filepath.Join(dir, "survey"))
	if err != nil {
		fs.Close()
		cs.Close()
		return checkoutOwner{}, err
	}
	scoped := NewServer(s.store, s.dbPath, "", s.grammarLoader)
	scoped.checkoutInfo = &c
	scoped.SetCodeStore(cs)
	scoped.SetFindingsStore(fs)
	scoped.SetSurveyStore(ss)
	// Existing stores are independent; only verified record bundles are copied.
	for _, owner := range s.checkouts.owners {
		if source, ok := owner.server.GetCodeStore().(*store.CodeStore); ok {
			scoped.seedFile = source.SeedFile
			break
		}
	}
	var stop func()
	if s.checkouts.onOpen != nil {
		stop, err = s.checkouts.onOpen(scoped, c)
	}
	if err != nil {
		ss.Close()
		fs.Close()
		cs.Close()
		return checkoutOwner{}, fmt.Errorf("open checkout %s: %w", c.ID, err)
	}
	return checkoutOwner{server: scoped, stop: func() {
		if stop != nil {
			stop()
		}
		ss.Close()
		fs.Close()
		cs.Close()
	}}, nil
}

func (s *Server) closeCheckouts() {
	if m := s.checkouts; m != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed {
			return
		}
		m.closed = true
		for _, o := range m.owners {
			if o.stop != nil {
				o.stop()
			}
		}
	}
}

// PruneCheckoutCaches takes the manager's exclusive lock, waiting for all RPCs
// before stopping an orphan's watcher and releasing its store handles.
func (s *Server) PruneCheckoutCaches(now time.Time) (int, error) {
	m := s.checkouts
	if m == nil {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, nil
	}
	pinned := map[string]bool{s.checkoutInfo.ID: true}
	return store.PruneCheckouts(s.dbPath, s.store, now, pinned, func(c checkout.Info) error {
		if owner, ok := m.owners[c.ID]; ok {
			if owner.stop != nil {
				owner.stop()
			}
			delete(m.owners, c.ID)
		}
		return nil
	})
}
