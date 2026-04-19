package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentStoreRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}

	card, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:capital:france",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content:  map[string]any{"answer": "Paris"},
		Provenance: Provenance{
			Source:     "user",
			Confidence: 0.95,
		},
	})
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if card.Version != 1 {
		t.Fatalf("expected version 1, got %d", card.Version)
	}

	_, err = store.CreateEdge(CreateEdgeRequest{
		EdgeID:     "edge:capital:france-europe",
		FromCardID: "fact:capital:france",
		ToCardID:   "fact:capital:france",
		EdgeType:   "self_ref",
		Weight:     0.8,
	})
	if err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}

	if _, err := store.UpdateCard("fact:capital:france", UpdateCardRequest{
		Content: map[string]any{"answer": "Paris", "note": "updated"},
	}); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen from same directory — data should survive.
	store2, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore (reopen): %v", err)
	}
	defer store2.Close()

	resp := store2.Query(QueryRequest{CardID: "fact:capital:france"})
	if len(resp.Cards) != 1 {
		t.Fatalf("expected 1 card after reopen, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Version != 2 {
		t.Fatalf("expected version 2 after update, got %d", resp.Cards[0].Version)
	}
	if resp.Cards[0].Content["note"] != "updated" {
		t.Fatalf("expected updated content, got %v", resp.Cards[0].Content)
	}

	if len(resp.Edges) != 1 {
		t.Fatalf("expected 1 edge after reopen, got %d", len(resp.Edges))
	}
	if resp.Edges[0].EdgeID != "edge:capital:france-europe" {
		t.Fatalf("expected edge id preserved, got %q", resp.Edges[0].EdgeID)
	}
}

func TestPersistentStoreTouchCardSurvivesRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:     "proc:audit:v1",
		CardType:   "procedure",
		Status:     CardStatusActive,
		Content:    map[string]any{"summary": "Audit"},
		Provenance: Provenance{Confidence: 0.6},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := store.TouchCard("proc:audit:v1", 0.1, 0.05); err != nil {
		t.Fatalf("TouchCard: %v", err)
	}
	store.Close()

	store2, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore (reopen): %v", err)
	}
	defer store2.Close()

	resp := store2.Query(QueryRequest{CardID: "proc:audit:v1"})
	if len(resp.Cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Version != 2 {
		t.Fatalf("expected version 2 from touch, got %d", resp.Cards[0].Version)
	}
	if resp.Cards[0].Provenance.Confidence < 0.64 {
		t.Fatalf("expected confidence >= 0.64, got %f", resp.Cards[0].Provenance.Confidence)
	}
}

func TestPersistentStoreDecaySurvivesRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "web:search:result1",
		CardType: "web_result",
		Status:   CardStatusActive,
		Content:  map[string]any{"snippet": "hello world"},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	futureTime := time.Now().UTC().Add(24 * time.Hour)
	result, err := store.DecayAndCompact(GovernanceOptions{
		Now:           futureTime,
		BaseDecayRate: 0.05,
	})
	if err != nil {
		t.Fatalf("DecayAndCompact: %v", err)
	}
	if result.Decayed == 0 {
		t.Fatalf("expected at least one card to decay")
	}
	store.Close()

	store2, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore (reopen): %v", err)
	}
	defer store2.Close()

	resp := store2.Query(QueryRequest{CardID: "web:search:result1"})
	if len(resp.Cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Activation.Score >= 1.0 {
		t.Fatalf("expected decayed score < 1.0, got %f", resp.Cards[0].Activation.Score)
	}
}

func TestJournalRecordsEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "j:card:1",
		CardType: "fact",
		Status:   CardStatusActive,
		Content:  map[string]any{"x": "y"},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := store.UpdateCard("j:card:1", UpdateCardRequest{
		Content: map[string]any{"x": "z"},
	}); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	store.Close()

	events, err := ReadJournal(filepath.Join(dir, "journal.jsonl"))
	if err != nil {
		t.Fatalf("ReadJournal: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least 2 journal events, got %d", len(events))
	}
	if events[0].EventType != "card_created" {
		t.Fatalf("expected first event to be card_created, got %q", events[0].EventType)
	}
	if events[1].EventType != "card_updated" {
		t.Fatalf("expected second event to be card_updated, got %q", events[1].EventType)
	}
}

func TestJournalFileCreatedOnDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	store.Close()

	journalPath := filepath.Join(dir, "journal.jsonl")
	if _, err := os.Stat(journalPath); os.IsNotExist(err) {
		t.Fatalf("expected journal file to exist at %s", journalPath)
	}
}

func TestPersistentStoreFileNamesDoNotCollide(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	for _, id := range []string{"a/b", "a__b"} {
		if _, err := store.CreateCard(CreateCardRequest{
			CardID:   id,
			CardType: "fact",
			Status:   CardStatusActive,
			Content:  map[string]any{"id": id},
		}); err != nil {
			t.Fatalf("CreateCard %s: %v", id, err)
		}
	}
	store.Close()

	store2, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore reopen: %v", err)
	}
	defer store2.Close()
	for _, id := range []string{"a/b", "a__b"} {
		resp := store2.Query(QueryRequest{CardID: id})
		if len(resp.Cards) != 1 {
			t.Fatalf("expected card %s after reopen, got %d", id, len(resp.Cards))
		}
	}
}

func TestInMemoryStoreIgnoresPersistence(t *testing.T) {
	t.Parallel()

	store := NewStore()
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "mem:only",
		CardType: "fact",
		Status:   CardStatusActive,
		Content:  map[string]any{"x": "y"},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if store.persistent() {
		t.Fatalf("in-memory store should not be persistent")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close on in-memory store should not error: %v", err)
	}
}

func TestGovernanceSchedulerStartsAndStops(t *testing.T) {
	t.Parallel()
	store := NewStore()
	sched := NewGovernanceScheduler(store, &GovernanceSchedulerConfig{
		Interval: 50 * time.Millisecond,
	})
	sched.Start()
	time.Sleep(150 * time.Millisecond)
	sched.Stop()
}
