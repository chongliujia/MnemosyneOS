package memory

import "testing"

func TestStorePersistsAndQueriesCardScope(t *testing.T) {
	t.Parallel()

	store := NewStore()
	if _, err := store.CreateCard(CreateCardRequest{
		CardID:   "search:1:summary",
		CardType: "search_summary",
		Scope:    "project",
		Status:   CardStatusCandidate,
		Content: map[string]any{
			"summary": "OpenClaw uses hybrid retrieval.",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	projectCards := store.Query(QueryRequest{CardType: "search_summary", Scope: "project"}).Cards
	if len(projectCards) != 1 {
		t.Fatalf("expected one project-scoped card, got %d", len(projectCards))
	}
	if projectCards[0].Scope != ScopeProject {
		t.Fatalf("expected stored scope to be %s, got %q", ScopeProject, projectCards[0].Scope)
	}
	if projectCards[0].Status != CardStatusCandidate {
		t.Fatalf("expected initial status %s, got %q", CardStatusCandidate, projectCards[0].Status)
	}

	userCards := store.Query(QueryRequest{CardType: "search_summary", Scope: ScopeUser}).Cards
	if len(userCards) != 0 {
		t.Fatalf("expected no user-scoped cards, got %d", len(userCards))
	}

	updated, err := store.UpdateCard("search:1:summary", UpdateCardRequest{
		Scope:      ScopeArchive,
		Status:     CardStatusArchived,
		Supersedes: "search:0:summary",
	})
	if err != nil {
		t.Fatalf("UpdateCard returned error: %v", err)
	}
	if updated.Scope != ScopeArchive {
		t.Fatalf("expected updated scope %s, got %q", ScopeArchive, updated.Scope)
	}
	if updated.Supersedes != "search:0:summary" {
		t.Fatalf("expected supersedes to be recorded, got %q", updated.Supersedes)
	}

	archivedCards := store.Query(QueryRequest{CardType: "search_summary", Scope: ScopeArchive}).Cards
	if len(archivedCards) != 1 {
		t.Fatalf("expected one archive-scoped card, got %d", len(archivedCards))
	}
	if archivedCards[0].Status != CardStatusArchived {
		t.Fatalf("expected archived status, got %q", archivedCards[0].Status)
	}
}
