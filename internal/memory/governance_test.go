package memory

import (
	"testing"
	"time"
)

func TestDecayAndCompact(t *testing.T) {
	t.Parallel()

	store := NewStore()
	now := time.Now().UTC()

	// 1. Candidate card (should not be examined/decayed)
	store.CreateCard(CreateCardRequest{
		CardID:   "card:candidate",
		CardType: "fact",
		Status:   CardStatusCandidate,
	})

	// 2. Active card (should decay normally)
	store.CreateCard(CreateCardRequest{
		CardID:   "card:active_normal",
		CardType: "fact",
		Status:   CardStatusActive,
		Activation: &ActivationState{
			Score:       1.0,
			DecayPolicy: "default",
		},
	})

	// 3. Active card with session_use (should decay fast, become stale)
	store.CreateCard(CreateCardRequest{
		CardID:   "card:active_fast",
		CardType: "fact",
		Status:   CardStatusActive,
		Activation: &ActivationState{
			Score:       1.0,
			DecayPolicy: "session_use",
		},
	})

	// 4. Stale card (should decay into archived)
	store.CreateCard(CreateCardRequest{
		CardID:   "card:stale",
		CardType: "fact",
		Status:   CardStatusStale,
		Activation: &ActivationState{
			Score:       0.3,
			DecayPolicy: "default",
		},
	})

	// Run Decay (simulate 10 hours passing)
	future := now.Add(10 * time.Hour)
	opts := GovernanceOptions{
		Now: future,
	}

	result, err := store.DecayAndCompact(opts)
	if err != nil {
		t.Fatalf("DecayAndCompact failed: %v", err)
	}

	if result.Examined != 3 {
		t.Fatalf("expected 3 cards to be examined, got %d", result.Examined)
	}
	if result.Decayed != 3 {
		t.Fatalf("expected 3 cards to decay, got %d", result.Decayed)
	}
	if result.Staled != 1 {
		t.Fatalf("expected 1 card to become stale, got %d", result.Staled)
	}
	if result.Archived != 1 {
		t.Fatalf("expected 1 card to become archived, got %d", result.Archived)
	}

	// Verify states
	q := store.Query(QueryRequest{CardID: "card:active_normal"})
	if len(q.Cards) == 0 || q.Cards[0].Status != CardStatusActive {
		t.Fatalf("card:active_normal should remain active")
	}
	if q.Cards[0].Activation.Score >= 1.0 {
		t.Fatalf("card:active_normal score should have decayed: %f", q.Cards[0].Activation.Score)
	}

	q = store.Query(QueryRequest{CardID: "card:active_fast"})
	if len(q.Cards) == 0 || q.Cards[0].Status != CardStatusStale {
		t.Fatalf("card:active_fast should become stale")
	}

	q = store.Query(QueryRequest{CardID: "card:stale"})
	if len(q.Cards) == 0 || q.Cards[0].Status != CardStatusArchived {
		t.Fatalf("card:stale should become archived")
	}
}
