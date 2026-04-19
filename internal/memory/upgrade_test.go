package memory

import (
	"fmt"
	"testing"
)

func TestUpgradeEventsToFactsCreatesFact(t *testing.T) {
	t.Parallel()
	store := NewStore()

	for i := 0; i < 3; i++ {
		if _, err := store.CreateCard(CreateCardRequest{
			CardID:   fmtID("event:test:%d", i),
			CardType: "event",
			Scope:    ScopeProject,
			Status:   CardStatusActive,
			Content: map[string]any{
				"topic":   "capital of france",
				"summary": "Paris is the capital of France",
			},
			Provenance: Provenance{Confidence: 0.8},
		}); err != nil {
			t.Fatalf("CreateCard event %d: %v", i, err)
		}
	}

	result, err := store.UpgradeEventsToFacts(UpgradeRequest{
		Scope:          ScopeProject,
		MinOccurrences: 2,
	})
	if err != nil {
		t.Fatalf("UpgradeEventsToFacts: %v", err)
	}
	if result.EventsExamined != 3 {
		t.Fatalf("expected 3 events examined, got %d", result.EventsExamined)
	}
	if result.ClustersFound != 1 {
		t.Fatalf("expected 1 cluster, got %d", result.ClustersFound)
	}
	if result.FactsCreated != 1 {
		t.Fatalf("expected 1 fact created, got %d", result.FactsCreated)
	}

	facts := store.Query(QueryRequest{CardType: "fact", Status: CardStatusCandidate}).Cards
	if len(facts) != 1 {
		t.Fatalf("expected 1 candidate fact card, got %d", len(facts))
	}
	if facts[0].Provenance.Source != "event-fact-upgrade" {
		t.Fatalf("expected provenance source 'event-fact-upgrade', got %q", facts[0].Provenance.Source)
	}
	if len(facts[0].EvidenceRefs) != 3 {
		t.Fatalf("expected 3 evidence refs, got %d", len(facts[0].EvidenceRefs))
	}
}

func TestUpgradeEventsToFactsSkipsWhenTooFewEvents(t *testing.T) {
	t.Parallel()
	store := NewStore()

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "event:lonely:0",
		CardType: "event",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content:  map[string]any{"topic": "singleton topic", "summary": "only one"},
		Provenance: Provenance{Confidence: 0.9},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	result, err := store.UpgradeEventsToFacts(UpgradeRequest{
		Scope:          ScopeProject,
		MinOccurrences: 2,
	})
	if err != nil {
		t.Fatalf("UpgradeEventsToFacts: %v", err)
	}
	if result.FactsCreated != 0 {
		t.Fatalf("expected 0 facts, got %d", result.FactsCreated)
	}
}

func TestUpgradeEventsArchivesSources(t *testing.T) {
	t.Parallel()
	store := NewStore()

	for i := 0; i < 2; i++ {
		if _, err := store.CreateCard(CreateCardRequest{
			CardID:   fmtID("event:archive:%d", i),
			CardType: "event",
			Scope:    ScopeProject,
			Status:   CardStatusActive,
			Content: map[string]any{
				"topic":   "archival test",
				"summary": "something repeatable",
			},
			Provenance: Provenance{Confidence: 0.7},
		}); err != nil {
			t.Fatalf("CreateCard: %v", err)
		}
	}

	result, err := store.UpgradeEventsToFacts(UpgradeRequest{
		Scope:          ScopeProject,
		MinOccurrences: 2,
		ArchiveSources: true,
	})
	if err != nil {
		t.Fatalf("UpgradeEventsToFacts: %v", err)
	}
	if result.EventsArchived != 2 {
		t.Fatalf("expected 2 events archived, got %d", result.EventsArchived)
	}

	active := store.Query(QueryRequest{CardType: "event", Status: CardStatusActive}).Cards
	if len(active) != 0 {
		t.Fatalf("expected 0 active events after archiving, got %d", len(active))
	}
}

func TestUpgradeEventsIdempotent(t *testing.T) {
	t.Parallel()
	store := NewStore()

	for i := 0; i < 3; i++ {
		if _, err := store.CreateCard(CreateCardRequest{
			CardID:   fmtID("event:idem:%d", i),
			CardType: "event",
			Scope:    ScopeProject,
			Status:   CardStatusActive,
			Content: map[string]any{
				"topic":   "idempotent topic",
				"summary": "repeated observation",
			},
			Provenance: Provenance{Confidence: 0.85},
		}); err != nil {
			t.Fatalf("CreateCard: %v", err)
		}
	}

	r1, _ := store.UpgradeEventsToFacts(UpgradeRequest{Scope: ScopeProject})
	if r1.FactsCreated != 1 {
		t.Fatalf("first run: expected 1 fact, got %d", r1.FactsCreated)
	}

	r2, _ := store.UpgradeEventsToFacts(UpgradeRequest{Scope: ScopeProject})
	if r2.FactsCreated != 0 {
		t.Fatalf("second run: expected 0 new facts (idempotent), got %d", r2.FactsCreated)
	}
}

func fmtID(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
