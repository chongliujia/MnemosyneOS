package recall

import (
	"testing"

	"mnemosyneos/internal/memory"
)

func TestRecallAcrossConnectorSources(t *testing.T) {
	store := memory.NewStore()

	mustCreateCard(t, store, memory.CreateCardRequest{
		CardID:   "search:1:summary",
		CardType: "search_summary",
		Content: map[string]any{
			"summary": "AgentOS positioning and memory runtime notes",
			"query":   "agentos positioning",
		},
	})
	mustCreateCard(t, store, memory.CreateCardRequest{
		CardID:   "email:1:thread",
		CardType: "email_thread",
		Content: map[string]any{
			"subject": "Root approval required",
			"summary": "Approval requested for root file write",
		},
	})
	mustCreateCard(t, store, memory.CreateCardRequest{
		CardID:   "github:1:issue",
		CardType: "github_issue",
		Content: map[string]any{
			"title": "Approval flow",
			"body":  "Need root approval flow in AgentOS",
			"repo":  "mnemosyne/agentos",
		},
	})

	service := NewService(store)
	resp := service.Recall(Request{
		Query: "approval agentos",
		Limit: 10,
	})
	if len(resp.Hits) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(resp.Hits))
	}
	sources := map[string]bool{}
	for _, hit := range resp.Hits {
		sources[hit.Source] = true
	}
	for _, source := range []string{"web", "email", "github"} {
		if !sources[source] {
			t.Fatalf("expected source %s in recall hits", source)
		}
	}
}

func TestRecallFiltersBySource(t *testing.T) {
	store := memory.NewStore()
	mustCreateCard(t, store, memory.CreateCardRequest{
		CardID:   "github:1:summary",
		CardType: "github_issue_summary",
		Content:  map[string]any{"summary": "Approval flow in github"},
	})
	mustCreateCard(t, store, memory.CreateCardRequest{
		CardID:   "email:1:summary",
		CardType: "email_summary",
		Content:  map[string]any{"summary": "Approval email"},
	})

	service := NewService(store)
	resp := service.Recall(Request{
		Query:   "approval",
		Sources: []string{"github"},
		Limit:   10,
	})
	if len(resp.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(resp.Hits))
	}
	if resp.Hits[0].Source != "github" {
		t.Fatalf("expected github source, got %s", resp.Hits[0].Source)
	}
}

func mustCreateCard(t *testing.T, store *memory.Store, req memory.CreateCardRequest) {
	t.Helper()
	if _, err := store.CreateCard(req); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
}
