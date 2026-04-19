package memory

import (
	"path/filepath"
	"testing"
)

func TestReplayFromJournalRebuildsState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "replay:card:1",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content:  map[string]any{"answer": "42"},
		Provenance: Provenance{Confidence: 0.9},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "replay:card:2",
		CardType: "event",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content:  map[string]any{"summary": "something happened"},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := store.UpdateCard("replay:card:1", UpdateCardRequest{
		Content: map[string]any{"answer": "43"},
	}); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if _, err := store.CreateEdge(CreateEdgeRequest{
		EdgeID:     "replay:edge:1",
		FromCardID: "replay:card:1",
		ToCardID:   "replay:card:2",
		EdgeType:   "related",
		Weight:     0.7,
	}); err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}
	store.Close()

	replayed, err := ReplayFromJournal(filepath.Join(dir, "journal.jsonl"))
	if err != nil {
		t.Fatalf("ReplayFromJournal: %v", err)
	}

	resp := replayed.Query(QueryRequest{CardID: "replay:card:1"})
	if len(resp.Cards) != 1 {
		t.Fatalf("expected 1 card after replay, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Version != 2 {
		t.Fatalf("expected version 2 from update, got %d", resp.Cards[0].Version)
	}
	if resp.Cards[0].Content["answer"] != "43" {
		t.Fatalf("expected updated content, got %v", resp.Cards[0].Content)
	}

	resp2 := replayed.Query(QueryRequest{CardID: "replay:card:2"})
	if len(resp2.Cards) != 1 {
		t.Fatalf("expected card 2 after replay, got %d", len(resp2.Cards))
	}

	if len(replayed.edges) != 1 {
		t.Fatalf("expected 1 edge after replay, got %d", len(replayed.edges))
	}
}

func TestVerifyIntegrityPassesOnCleanStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	defer store.Close()

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "integrity:card:1",
		CardType: "fact",
		Status:   CardStatusActive,
		Content:  map[string]any{"x": "y"},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	report, err := store.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	if !report.OK {
		t.Fatalf("expected clean integrity, got: %+v", report)
	}
}

func TestReplayFromEmptyJournal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	replayed, err := ReplayFromJournal(filepath.Join(dir, "nonexistent.jsonl"))
	if err != nil {
		t.Fatalf("ReplayFromJournal on missing file: %v", err)
	}
	if len(replayed.cards) != 0 {
		t.Fatalf("expected empty store from missing journal, got %d cards", len(replayed.cards))
	}
}
