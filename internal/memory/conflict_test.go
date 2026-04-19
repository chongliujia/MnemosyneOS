package memory

import (
	"testing"
)

func TestDetectConflictsFindsContradiction(t *testing.T) {
	t.Parallel()
	store := NewStore()

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:capital:1",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content: map[string]any{
			"topic":  "capital of france",
			"answer": "Paris",
		},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:capital:2",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content: map[string]any{
			"topic":  "capital of france",
			"answer": "Lyon",
		},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	result := store.DetectConflicts(ConflictDetectionRequest{
		Scope: ScopeProject,
	})
	if result.Examined != 2 {
		t.Fatalf("expected 2 examined, got %d", result.Examined)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Conflicts))
	}
	c := result.Conflicts[0]
	if c.Topic == "" {
		t.Fatalf("expected topic to be set")
	}
	if c.Reason == "" {
		t.Fatalf("expected reason to explain the conflict")
	}
}

func TestDetectConflictsNoConflictWhenAgreeing(t *testing.T) {
	t.Parallel()
	store := NewStore()

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:agree:1",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content: map[string]any{
			"topic":  "color of sky",
			"answer": "blue",
		},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:agree:2",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content: map[string]any{
			"topic":  "color of sky",
			"answer": "blue",
		},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	result := store.DetectConflicts(ConflictDetectionRequest{Scope: ScopeProject})
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts for agreeing cards, got %d", len(result.Conflicts))
	}
}

func TestDetectConflictsAcrossActiveAndCandidate(t *testing.T) {
	t.Parallel()
	store := NewStore()

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:cross:active",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content: map[string]any{
			"topic": "best language",
			"claim": "Go is the best language",
		},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:cross:candidate",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusCandidate,
		Content: map[string]any{
			"topic": "best language",
			"claim": "Rust is the best language",
		},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	result := store.DetectConflicts(ConflictDetectionRequest{Scope: ScopeProject})
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict across statuses, got %d", len(result.Conflicts))
	}
}

func TestDetectConflictsNoConflictDifferentTopics(t *testing.T) {
	t.Parallel()
	store := NewStore()

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:diff:1",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content: map[string]any{
			"topic":  "capital of france",
			"answer": "Paris",
		},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "fact:diff:2",
		CardType: "fact",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content: map[string]any{
			"topic":  "capital of germany",
			"answer": "Berlin",
		},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	result := store.DetectConflicts(ConflictDetectionRequest{Scope: ScopeProject})
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts for different topics, got %d", len(result.Conflicts))
	}
}
