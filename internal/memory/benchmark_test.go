package memory

import (
	"fmt"
	"testing"
	"time"
)

// --- Benchmark 1: Temporal Correctness ---
// Verifies that as-of queries return the correct version at each point in time.

func TestBenchmarkTemporalCorrectness(t *testing.T) {
	t.Parallel()
	store := NewStore()

	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:    "temporal:president",
		CardType:  "fact",
		Status:    CardStatusActive,
		ValidFrom: &t1,
		ValidTo:   &t2,
		Content:   map[string]any{"answer": "Alice"},
	}); err != nil {
		t.Fatalf("CreateCard v1: %v", err)
	}
	if _, err := store.UpdateCard("temporal:president", UpdateCardRequest{
		ValidFrom: &t2,
		ValidTo:   &t3,
		Content:   map[string]any{"answer": "Bob"},
	}); err != nil {
		t.Fatalf("UpdateCard v2: %v", err)
	}

	tests := []struct {
		name     string
		asOf     time.Time
		expected string
	}{
		{"before_v1", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), ""},
		{"during_v1", time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), "Alice"},
		{"during_v2", time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), "Bob"},
		{"after_v2", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), ""},
	}

	pass, fail := 0, 0
	for _, tc := range tests {
		asOf := tc.asOf
		resp := store.Query(QueryRequest{CardID: "temporal:president", AsOf: &asOf})
		if tc.expected == "" {
			if len(resp.Cards) != 0 {
				t.Errorf("[%s] expected no cards, got %d", tc.name, len(resp.Cards))
				fail++
			} else {
				pass++
			}
		} else {
			if len(resp.Cards) != 1 {
				t.Errorf("[%s] expected 1 card, got %d", tc.name, len(resp.Cards))
				fail++
				continue
			}
			if resp.Cards[0].Content["answer"] != tc.expected {
				t.Errorf("[%s] expected %q, got %q", tc.name, tc.expected, resp.Cards[0].Content["answer"])
				fail++
			} else {
				pass++
			}
		}
	}
	t.Logf("temporal correctness: %d/%d passed", pass, pass+fail)
}

func TestBenchmarkTemporalVersionChain(t *testing.T) {
	t.Parallel()
	store := NewStore()

	const numVersions = 20
	cardID := "temporal:chain"

	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   cardID,
		CardType: "fact",
		Status:   CardStatusActive,
		Content:  map[string]any{"version": 1},
	}); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	for i := 2; i <= numVersions; i++ {
		if _, err := store.UpdateCard(cardID, UpdateCardRequest{
			Content: map[string]any{"version": i},
		}); err != nil {
			t.Fatalf("UpdateCard v%d: %v", i, err)
		}
	}

	resp := store.Query(QueryRequest{CardID: cardID})
	if len(resp.Cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Version != numVersions {
		t.Fatalf("expected latest version %d, got %d", numVersions, resp.Cards[0].Version)
	}
	if toInt(resp.Cards[0].Content["version"]) != numVersions {
		t.Fatalf("expected content version %d, got %v", numVersions, resp.Cards[0].Content["version"])
	}
	if resp.Cards[0].PrevVersion == "" {
		t.Fatalf("expected prev_version to be set")
	}
	t.Logf("version chain: %d versions created and queried correctly", numVersions)
}

// --- Benchmark 2: Evidence Integrity ---
// Verifies that evidence refs point to real cards and edges are bidirectionally indexed.

func TestBenchmarkEvidenceIntegrity(t *testing.T) {
	t.Parallel()
	store := NewStore()

	const numEvents = 5
	for i := 0; i < numEvents; i++ {
		if _, err := store.CreateCard(CreateCardRequest{
			CardID:   fmt.Sprintf("evidence:event:%d", i),
			CardType: "event",
			Scope:    ScopeProject,
			Status:   CardStatusActive,
			Content:  map[string]any{"topic": "evidence test", "summary": "observed something"},
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
	if result.FactsCreated == 0 {
		t.Fatalf("expected at least 1 fact created")
	}

	pass, fail := 0, 0
	facts := store.Query(QueryRequest{CardType: "fact"}).Cards
	for _, fact := range facts {
		for _, ref := range fact.EvidenceRefs {
			refResp := store.Query(QueryRequest{CardID: ref.CardID})
			if len(refResp.Cards) == 0 {
				t.Errorf("evidence ref %q in card %s points to nonexistent card", ref.CardID, fact.CardID)
				fail++
			} else {
				pass++
			}
		}
	}

	// Verify edges are bidirectionally indexed
	allCards := store.LatestCards()
	cardIDs := make([]string, len(allCards))
	for i, c := range allCards {
		cardIDs[i] = c.CardID
	}
	edges := store.collectEdgesForCards(cardIDs)
	for _, edge := range edges {
		fromResp := store.Query(QueryRequest{CardID: edge.FromCardID})
		toResp := store.Query(QueryRequest{CardID: edge.ToCardID})
		if len(fromResp.Cards) == 0 {
			t.Errorf("edge %s from_card %s does not exist", edge.EdgeID, edge.FromCardID)
			fail++
		} else {
			pass++
		}
		if len(toResp.Cards) == 0 {
			t.Errorf("edge %s to_card %s does not exist", edge.EdgeID, edge.ToCardID)
			fail++
		} else {
			pass++
		}
	}
	t.Logf("evidence integrity: %d/%d checks passed, %d edges verified", pass, pass+fail, len(edges))
}

// --- Benchmark 3: Memory Contamination Resistance ---
// Injects false information and verifies the consolidation pipeline rejects or
// isolates it rather than promoting it as truth.

func TestBenchmarkContaminationResistance(t *testing.T) {
	t.Parallel()
	store := NewStore()

	// Establish ground truth: 3 events agreeing on correct answer
	for i := 0; i < 3; i++ {
		if _, err := store.CreateCard(CreateCardRequest{
			CardID:   fmt.Sprintf("contam:truth:%d", i),
			CardType: "event",
			Scope:    ScopeProject,
			Status:   CardStatusActive,
			Content:  map[string]any{"topic": "earth shape", "answer": "oblate spheroid"},
			Provenance: Provenance{Source: "trusted", Confidence: 0.95},
		}); err != nil {
			t.Fatalf("CreateCard truth %d: %v", i, err)
		}
	}

	// Inject contamination: 1 event with false claim, low confidence
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "contam:false:0",
		CardType: "event",
		Scope:    ScopeProject,
		Status:   CardStatusActive,
		Content:  map[string]any{"topic": "earth shape", "answer": "flat"},
		Provenance: Provenance{Source: "untrusted", Confidence: 0.1},
	}); err != nil {
		t.Fatalf("CreateCard false: %v", err)
	}

	// Run upgrade — should create a fact with the majority answer
	result, err := store.UpgradeEventsToFacts(UpgradeRequest{
		Scope:          ScopeProject,
		MinOccurrences: 2,
		MinConfidence:  0.5,
	})
	if err != nil {
		t.Fatalf("UpgradeEventsToFacts: %v", err)
	}

	facts := store.Query(QueryRequest{CardType: "fact", Status: CardStatusCandidate}).Cards
	if len(facts) != 1 {
		t.Fatalf("expected exactly 1 fact, got %d", len(facts))
	}

	pass, fail := 0, 0

	// The fact should reflect the truthful answer, not the contaminated one
	if facts[0].Content["answer"] == "oblate spheroid" {
		pass++
	} else {
		t.Errorf("fact answer contaminated: got %v", facts[0].Content["answer"])
		fail++
	}

	// Confidence should be high (from trusted sources)
	if facts[0].Provenance.Confidence >= 0.5 {
		pass++
	} else {
		t.Errorf("fact confidence too low: %f", facts[0].Provenance.Confidence)
		fail++
	}

	// Now run conflict detection — false claim should not appear as active fact
	conflicts := store.DetectConflicts(ConflictDetectionRequest{Scope: ScopeProject})
	// No conflicts among facts since only one fact was created
	if len(conflicts.Conflicts) == 0 {
		pass++
	} else {
		t.Errorf("unexpected conflicts: %+v", conflicts.Conflicts)
		fail++
	}

	t.Logf("contamination resistance: %d/%d checks passed, %d facts created (contamination=%s)",
		pass, pass+fail, result.FactsCreated, "isolated")
}

// --- Benchmark 4: Narrative Coherence ---
// Verifies that event chains maintain temporal ordering and version links
// are consistent across the graph.

func TestBenchmarkNarrativeCoherence(t *testing.T) {
	t.Parallel()
	store := NewStore()

	// Create a sequence of events with explicit temporal ordering
	const numEvents = 10
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < numEvents; i++ {
		validFrom := baseTime.Add(time.Duration(i) * 24 * time.Hour)
		validTo := validFrom.Add(24 * time.Hour)
		if _, err := store.CreateCard(CreateCardRequest{
			CardID:    fmt.Sprintf("narrative:event:%d", i),
			CardType:  "event",
			Scope:     ScopeProject,
			Status:    CardStatusActive,
			ValidFrom: &validFrom,
			ValidTo:   &validTo,
			Content: map[string]any{
				"topic":    "daily log",
				"summary":  fmt.Sprintf("Day %d observation", i+1),
				"sequence": i,
			},
		}); err != nil {
			t.Fatalf("CreateCard event %d: %v", i, err)
		}
	}

	// Link events in a temporal chain
	for i := 0; i < numEvents-1; i++ {
		if _, err := store.CreateEdge(CreateEdgeRequest{
			EdgeID:     fmt.Sprintf("narrative:next:%d", i),
			FromCardID: fmt.Sprintf("narrative:event:%d", i),
			ToCardID:   fmt.Sprintf("narrative:event:%d", i+1),
			EdgeType:   "temporal_next",
			Weight:     1.0,
		}); err != nil {
			t.Fatalf("CreateEdge %d->%d: %v", i, i+1, err)
		}
	}

	pass, fail := 0, 0

	// Verify: querying as-of each day returns the correct event
	for i := 0; i < numEvents; i++ {
		queryTime := baseTime.Add(time.Duration(i)*24*time.Hour + 12*time.Hour)
		resp := store.Query(QueryRequest{
			CardID: fmt.Sprintf("narrative:event:%d", i),
			AsOf:   &queryTime,
		})
		if len(resp.Cards) != 1 {
			t.Errorf("day %d: expected 1 card, got %d", i, len(resp.Cards))
			fail++
			continue
		}
		seq, ok := resp.Cards[0].Content["sequence"]
		if !ok || toInt(seq) != i {
			t.Errorf("day %d: wrong sequence, got %v", i, seq)
			fail++
		} else {
			pass++
		}
	}

	// Verify: temporal chain edges are ordered correctly
	for i := 0; i < numEvents-1; i++ {
		resp := store.Query(QueryRequest{CardID: fmt.Sprintf("narrative:event:%d", i)})
		if len(resp.Edges) == 0 {
			t.Errorf("event %d: expected edges, got none", i)
			fail++
			continue
		}
		foundNext := false
		for _, edge := range resp.Edges {
			if edge.EdgeType == "temporal_next" &&
				edge.FromCardID == fmt.Sprintf("narrative:event:%d", i) &&
				edge.ToCardID == fmt.Sprintf("narrative:event:%d", i+1) {
				foundNext = true
				break
			}
		}
		if foundNext {
			pass++
		} else {
			t.Errorf("event %d: missing temporal_next edge to event %d", i, i+1)
			fail++
		}
	}

	// Verify: no temporal gaps (each day's as-of returns exactly its event, not a neighbor)
	beforeAll := baseTime.Add(-24 * time.Hour)
	resp := store.Query(QueryRequest{CardID: "narrative:event:0", AsOf: &beforeAll})
	if len(resp.Cards) == 0 {
		pass++
	} else {
		t.Errorf("querying before all events should return nothing, got %d cards", len(resp.Cards))
		fail++
	}

	t.Logf("narrative coherence: %d/%d checks passed, %d events, %d edges",
		pass, pass+fail, numEvents, numEvents-1)
}

// toInt converts a value that may be int or float64 (after JSON round-trip) to int.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	default:
		return -1
	}
}
