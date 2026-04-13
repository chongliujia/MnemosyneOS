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

func TestRecallSkipsCandidateCards(t *testing.T) {
	store := memory.NewStore()
	mustCreateCard(t, store, memory.CreateCardRequest{
		CardID:   "search:1:summary",
		CardType: "search_summary",
		Status:   memory.CardStatusActive,
		Content:  map[string]any{"summary": "stable summary"},
	})
	mustCreateCard(t, store, memory.CreateCardRequest{
		CardID:   "web:1",
		CardType: "web_result",
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"snippet": "candidate-only detail"},
	})

	service := NewService(store)
	resp := service.Recall(Request{
		Query: "candidate-only detail",
		Limit: 10,
	})
	if len(resp.Hits) != 0 {
		t.Fatalf("expected candidate card to be excluded from recall, got %d hits", len(resp.Hits))
	}
}

func TestRecallIncludesActiveProcedures(t *testing.T) {
	store := memory.NewStore()
	mustCreateCard(t, store, memory.CreateCardRequest{
		CardID:   "procedure:expense_audit:v1",
		CardType: "procedure",
		Status:   memory.CardStatusActive,
		Content: map[string]any{
			"name":    "expense_audit_v1",
			"summary": "Audit reimbursements with explicit policy validation.",
			"steps":   "extract_fields\nvalidate_policy\nflag_missing_evidence",
		},
	})

	service := NewService(store)
	resp := service.Recall(Request{
		Query:   "expense audit validate policy",
		Sources: []string{"procedure"},
		Limit:   10,
	})
	if len(resp.Hits) != 1 {
		t.Fatalf("expected 1 procedure hit, got %d", len(resp.Hits))
	}
	if resp.Hits[0].Source != "procedure" {
		t.Fatalf("expected procedure source, got %s", resp.Hits[0].Source)
	}
	if resp.Hits[0].CardID != "procedure:expense_audit:v1" {
		t.Fatalf("unexpected procedure hit: %+v", resp.Hits[0])
	}
}

func mustCreateCard(t *testing.T, store *memory.Store, req memory.CreateCardRequest) {
	t.Helper()
	if _, err := store.CreateCard(req); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
}
