package adapter

import (
	"context"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TombstoneAdapter implements store.TombstoneStore by delegating to a gRPC
// client. It lets share export/import consult and record tombstones in daemon
// mode, where the daemon holds the single-writer BoltDB and the CLI cannot open
// it directly.
type TombstoneAdapter struct {
	client *grpcapi.Client
}

// Compile-time check that TombstoneAdapter implements store.TombstoneStore.
var _ store.TombstoneStore = (*TombstoneAdapter)(nil)

// NewTombstoneAdapter creates a new gRPC-backed tombstone adapter.
func NewTombstoneAdapter(client *grpcapi.Client) *TombstoneAdapter {
	return &TombstoneAdapter{client: client}
}

func (g *TombstoneAdapter) rpcCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), RPCTimeout)
}

func (g *TombstoneAdapter) AddTombstone(t *memory.Tombstone) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Tombstone.Add(ctx, &grpcapi.TombstoneAddRequest{
		Tombstone: tombstoneToProto(t),
	})
	if err != nil {
		return err
	}

	// The server stamps DeletedAt when zero; reflect the stored value back so
	// callers see the same timestamp the daemon persisted.
	if resp.Tombstone != nil && resp.Tombstone.DeletedAt != nil {
		t.DeletedAt = resp.Tombstone.DeletedAt.AsTime()
	}
	return nil
}

func (g *TombstoneAdapter) GetTombstone(kind, id string) (*memory.Tombstone, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Tombstone.Get(ctx, &grpcapi.TombstoneGetRequest{
		Kind: kind,
		Id:   id,
	})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}

	return protoToTombstone(resp.Tombstone), nil
}

func (g *TombstoneAdapter) ListTombstones() ([]*memory.Tombstone, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Tombstone.List(ctx, &grpcapi.TombstoneListRequest{})
	if err != nil {
		return nil, err
	}

	result := make([]*memory.Tombstone, len(resp.Tombstones))
	for i, pt := range resp.Tombstones {
		result[i] = protoToTombstone(pt)
	}
	return result, nil
}

func (g *TombstoneAdapter) DeleteTombstone(kind, id string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	_, err := g.client.Tombstone.Delete(ctx, &grpcapi.TombstoneDeleteRequest{
		Kind: kind,
		Id:   id,
	})
	return err
}

// tombstoneToProto converts a domain Tombstone to its protobuf form.
func tombstoneToProto(t *memory.Tombstone) *grpcapi.Tombstone {
	if t == nil {
		return nil
	}
	return &grpcapi.Tombstone{
		Id:        t.ID,
		Kind:      t.Kind,
		DeletedAt: timestamppb.New(t.DeletedAt),
	}
}

// protoToTombstone converts a protobuf Tombstone to the domain type.
func protoToTombstone(pt *grpcapi.Tombstone) *memory.Tombstone {
	if pt == nil {
		return nil
	}
	var deletedAt time.Time
	if pt.DeletedAt != nil {
		deletedAt = pt.DeletedAt.AsTime()
	}
	return &memory.Tombstone{
		ID:        pt.Id,
		Kind:      pt.Kind,
		DeletedAt: deletedAt,
	}
}
