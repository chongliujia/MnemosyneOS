package memory

import "testing"

func TestConsolidatorPromotesCandidateCards(t *testing.T) {
	store := NewStore()
	for _, req := range []CreateCardRequest{
		{
			CardID:   "web_result:1",
			CardType: "web_result",
			Scope:    ScopeProject,
			Status:   CardStatusCandidate,
			Content:  map[string]any{"title": "one"},
		},
		{
			CardID:   "web_result:2",
			CardType: "web_result",
			Scope:    ScopeProject,
			Status:   CardStatusCandidate,
			Content:  map[string]any{"title": "two"},
		},
		{
			CardID:   "email_message:1",
			CardType: "email_message",
			Scope:    ScopeUser,
			Status:   CardStatusCandidate,
			Content:  map[string]any{"subject": "hello"},
		},
	} {
		if _, err := store.CreateCard(req); err != nil {
			t.Fatalf("CreateCard(%s) failed: %v", req.CardID, err)
		}
	}

	result, err := NewConsolidator(store).PromoteCandidates(ConsolidateRequest{
		CardType: "web_result",
		Scope:    ScopeProject,
	})
	if err != nil {
		t.Fatalf("PromoteCandidates failed: %v", err)
	}
	if result.Examined != 2 || result.Promoted != 2 {
		t.Fatalf("unexpected consolidation result: %+v", result)
	}

	webCards := store.Query(QueryRequest{CardType: "web_result", Scope: ScopeProject}).Cards
	if len(webCards) != 2 {
		t.Fatalf("expected 2 web cards, got %d", len(webCards))
	}
	for _, card := range webCards {
		if card.Status != CardStatusActive {
			t.Fatalf("expected web card %s to be active, got %s", card.CardID, card.Status)
		}
	}

	emailCards := store.Query(QueryRequest{CardType: "email_message", Scope: ScopeUser, Status: CardStatusCandidate}).Cards
	if len(emailCards) != 1 {
		t.Fatalf("expected email candidate to remain candidate, got %d", len(emailCards))
	}
}

func TestConsolidatorSupersedesAndArchives(t *testing.T) {
	store := NewStore()
	for _, req := range []CreateCardRequest{
		{
			CardID:   "search:fact:v1",
			CardType: "search_summary",
			Scope:    ScopeProject,
			Status:   CardStatusActive,
			Content:  map[string]any{"summary": "wrong claim"},
		},
		{
			CardID:     "search:fact:v2",
			CardType:   "search_summary",
			Scope:      ScopeProject,
			Status:     CardStatusCandidate,
			Supersedes: "search:fact:v1",
			Content:    map[string]any{"summary": "correct claim"},
		},
		{
			CardID:   "search:fact:v3",
			CardType: "search_summary",
			Scope:    ScopeProject,
			Status:   CardStatusCandidate,
			Content:  map[string]any{"summary": "cold candidate"},
		},
	} {
		if _, err := store.CreateCard(req); err != nil {
			t.Fatalf("CreateCard(%s) failed: %v", req.CardID, err)
		}
	}

	result, err := NewConsolidator(store).PromoteCandidates(ConsolidateRequest{
		CardType:         "search_summary",
		Scope:            ScopeProject,
		Limit:            1,
		ArchiveRemaining: true,
	})
	if err != nil {
		t.Fatalf("PromoteCandidates failed: %v", err)
	}
	if result.Promoted != 1 || result.Superseded != 1 || result.Archived != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	v1 := store.Query(QueryRequest{CardID: "search:fact:v1"}).Cards[0]
	if v1.Status != CardStatusSuperseded {
		t.Fatalf("expected v1 superseded, got %s", v1.Status)
	}
	v2 := store.Query(QueryRequest{CardID: "search:fact:v2"}).Cards[0]
	if v2.Status != CardStatusActive {
		t.Fatalf("expected v2 active, got %s", v2.Status)
	}
	v3 := store.Query(QueryRequest{CardID: "search:fact:v3"}).Cards[0]
	if v3.Status != CardStatusArchived {
		t.Fatalf("expected v3 archived, got %s", v3.Status)
	}
}
