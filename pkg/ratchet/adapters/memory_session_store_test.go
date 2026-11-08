package adapters_test

import (
	"testing"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

func TestMemorySessionStoreOwnsSnapshots(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := adapters.NewMemorySessionStore()
	source := domain.NewSession("session")
	source.AddToken("discover", "token")
	source.RecordToolCall("discover")
	if err := store.Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	source.Tokens["discover"][0] = "changed"
	source.ToolHistory[0] = "changed"
	read, err := store.Get(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Tokens["discover"][0] != "token" || read.ToolHistory[0] != "discover" {
		t.Fatal("creation retained caller-owned state")
	}
	read.Tokens["discover"][0] = "updated"
	read.ToolHistory[0] = "updated"
	unchanged, err := store.Get(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Tokens["discover"][0] != "token" || unchanged.ToolHistory[0] != "discover" {
		t.Fatal("mutating a snapshot changed stored state")
	}
	if err := store.Update(ctx, read); err != nil {
		t.Fatal(err)
	}
	read.Tokens["discover"][0] = "changed"
	read.ToolHistory[0] = "changed"
	updated, err := store.Get(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Tokens["discover"][0] != "updated" || updated.ToolHistory[0] != "updated" {
		t.Fatal("update retained caller-owned state")
	}
}
