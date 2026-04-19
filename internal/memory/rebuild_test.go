package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRebuildProjectionsFromJournal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := store.CreateCard(CreateCardRequest{
			CardID:   fmtID("rebuild:card:%d", i),
			CardType: "fact",
			Scope:    ScopeProject,
			Status:   CardStatusActive,
			Content:  map[string]any{"n": i},
		}); err != nil {
			t.Fatalf("CreateCard %d: %v", i, err)
		}
	}
	if _, err := store.CreateEdge(CreateEdgeRequest{
		EdgeID:     "rebuild:edge:0-1",
		FromCardID: "rebuild:card:0",
		ToCardID:   "rebuild:card:1",
		EdgeType:   "related",
	}); err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}
	if _, err := store.UpdateCard("rebuild:card:2", UpdateCardRequest{
		Content: map[string]any{"n": 2, "updated": true},
	}); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}

	// Corrupt a snapshot file by truncating it
	corruptPath := filepath.Join(dir, "cards", cardFileName("rebuild:card:3"))
	if err := os.WriteFile(corruptPath, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}

	// Rebuild should fix corruption
	if err := store.RebuildProjections(); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}

	// Verify state is correct after rebuild
	resp := store.Query(QueryRequest{CardID: "rebuild:card:2"})
	if len(resp.Cards) != 1 {
		t.Fatalf("expected card 2 after rebuild, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Version != 2 {
		t.Fatalf("expected version 2, got %d", resp.Cards[0].Version)
	}

	resp3 := store.Query(QueryRequest{CardID: "rebuild:card:3"})
	if len(resp3.Cards) != 1 {
		t.Fatalf("expected card 3 recovered from journal, got %d", len(resp3.Cards))
	}

	// Verify integrity passes after rebuild
	report, err := store.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	if !report.OK {
		t.Fatalf("integrity check failed after rebuild: %+v", report)
	}

	store.Close()

	// Reopen and verify persistence survived
	store2, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store2.Close()
	if len(store2.LatestCards()) != 5 {
		t.Fatalf("expected 5 cards after reopen, got %d", len(store2.LatestCards()))
	}
}
