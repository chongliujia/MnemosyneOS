package memory

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// TestSoakHighVolume simulates a burst of mixed operations and verifies
// the store remains consistent. This is a compressed soak test suitable for CI.
func TestSoakHighVolume(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	store, err := NewPersistentStore(dir)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	defer store.Close()

	rng := rand.New(rand.NewSource(42))
	const numCards = 100
	const numEdges = 50
	const numUpdates = 200
	const numTouches = 100

	// Phase 1: create many cards
	for i := 0; i < numCards; i++ {
		status := CardStatusActive
		if i%5 == 0 {
			status = CardStatusCandidate
		}
		_, err := store.CreateCard(CreateCardRequest{
			CardID:   fmt.Sprintf("soak:card:%d", i),
			CardType: "fact",
			Scope:    ScopeProject,
			Status:   status,
			Content:  map[string]any{"index": i, "data": fmt.Sprintf("value-%d", i)},
			Provenance: Provenance{
				Source:     "soak-test",
				Confidence: 0.5 + rng.Float64()*0.5,
			},
		})
		if err != nil {
			t.Fatalf("CreateCard %d: %v", i, err)
		}
	}

	// Phase 2: create edges between random pairs
	for i := 0; i < numEdges; i++ {
		a := rng.Intn(numCards)
		b := rng.Intn(numCards)
		if a == b {
			b = (a + 1) % numCards
		}
		store.CreateEdge(CreateEdgeRequest{
			EdgeID:     fmt.Sprintf("soak:edge:%d", i),
			FromCardID: fmt.Sprintf("soak:card:%d", a),
			ToCardID:   fmt.Sprintf("soak:card:%d", b),
			EdgeType:   "related",
			Weight:     rng.Float64(),
		})
	}

	// Phase 3: random updates
	for i := 0; i < numUpdates; i++ {
		cardIdx := rng.Intn(numCards)
		store.UpdateCard(fmt.Sprintf("soak:card:%d", cardIdx), UpdateCardRequest{
			Content: map[string]any{"index": cardIdx, "data": fmt.Sprintf("updated-%d-%d", cardIdx, i)},
		})
	}

	// Phase 4: random touches
	for i := 0; i < numTouches; i++ {
		cardIdx := rng.Intn(numCards)
		store.TouchCard(fmt.Sprintf("soak:card:%d", cardIdx), 0.01, 0.005)
	}

	// Phase 5: run decay
	futureTime := time.Now().UTC().Add(2 * time.Hour)
	result, err := store.DecayAndCompact(GovernanceOptions{
		Now:           futureTime,
		BaseDecayRate: 0.05,
	})
	if err != nil {
		t.Fatalf("DecayAndCompact: %v", err)
	}
	if result.Examined == 0 {
		t.Fatalf("expected some cards examined in decay")
	}

	// Phase 6: verify integrity
	report, err := store.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	if !report.OK {
		t.Fatalf("integrity check failed after soak: missing_cards=%v extra_cards=%v mismatch=%v",
			report.MissingCards, report.ExtraCards, report.MismatchCards)
	}

	// Phase 7: rebuild and re-verify
	if err := store.RebuildProjections(); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}
	report2, err := store.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity after rebuild: %v", err)
	}
	if !report2.OK {
		t.Fatalf("integrity failed after rebuild: %+v", report2)
	}

	// Phase 8: verify card count
	allCards := store.LatestCards()
	if len(allCards) != numCards {
		t.Fatalf("expected %d cards, got %d", numCards, len(allCards))
	}
}
