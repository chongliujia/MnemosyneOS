package memoryorchestrator

import (
	"path/filepath"
	"testing"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/recall"
)

func TestBuildFastPacketSeparatesProcedureAndSemantic(t *testing.T) {
	t.Parallel()

	runtimeRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(runtimeRoot)
	task, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:         "Repository planning",
		Goal:          "Continue repository planning",
		SelectedSkill: "web-search",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	store := memory.NewStore()
	for _, req := range []memory.CreateCardRequest{
		{
			CardID:   "procedure:repo-plan:v1",
			CardType: "procedure",
			Scope:    memory.ScopeProject,
			Status:   memory.CardStatusActive,
			Content: map[string]any{
				"summary": "Repository planning procedure with explicit approval validation.",
			},
		},
		{
			CardID:   "search:repo:summary",
			CardType: "search_summary",
			Scope:    memory.ScopeProject,
			Status:   memory.CardStatusActive,
			Content: map[string]any{
				"summary": "Repository planning should retain approval context.",
			},
		},
	} {
		if _, err := store.CreateCard(req); err != nil {
			t.Fatalf("CreateCard returned error: %v", err)
		}
	}

	orchestrator := New(recall.NewService(store), runtimeStore)
	packet := orchestrator.BuildFastPacket(SessionView{
		Topic:           "repository planning",
		PendingQuestion: "continue the focused thread?",
		FocusTaskID:     task.TaskID,
		WorkingRecallIDs: []string{
			"search:repo:summary",
		},
		WorkingSources: []string{"web"},
	})
	if packet == nil {
		t.Fatalf("expected fast packet")
	}
	if len(packet.WorkingNotes) == 0 {
		t.Fatalf("expected working notes in packet")
	}
	if len(packet.RecentTasks) != 1 || packet.RecentTasks[0].TaskID != task.TaskID {
		t.Fatalf("expected focused task in packet, got %#v", packet.RecentTasks)
	}
	if len(packet.ProcedureHits) != 1 {
		t.Fatalf("expected one procedure hit, got %#v", packet.ProcedureHits)
	}
	if len(packet.SemanticHits) != 1 {
		t.Fatalf("expected one semantic hit, got %#v", packet.SemanticHits)
	}
}

func TestBuildTaskPacketFallsBackToBroadRecall(t *testing.T) {
	t.Parallel()

	runtimeRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(filepath.Join(runtimeRoot, "runtime"))
	store := memory.NewStore()
	if _, err := store.CreateCard(memory.CreateCardRequest{
		CardID:   "email:test:summary",
		CardType: "email_summary",
		Scope:    memory.ScopeUser,
		Status:   memory.CardStatusActive,
		Content: map[string]any{
			"summary": "approval context for reimbursement review",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	orchestrator := New(recall.NewService(store), runtimeStore)
	packet := orchestrator.BuildTaskPacket("approval context", "", SessionView{}, nil)
	if packet == nil {
		t.Fatalf("expected task packet")
	}
	if len(packet.RecallHits) == 0 {
		t.Fatalf("expected fallback recall hits in packet")
	}
}

func TestBuildTaskPacketHonorsQuota(t *testing.T) {
	t.Parallel()

	runtimeRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(filepath.Join(runtimeRoot, "runtime"))
	focusTask, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:         "Focused task",
		Goal:          "Stay focused",
		SelectedSkill: "memory-consolidate",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	store := memory.NewStore()
	for _, req := range []memory.CreateCardRequest{
		{
			CardID:   "procedure:task:v1",
			CardType: "procedure",
			Scope:    memory.ScopeProject,
			Status:   memory.CardStatusActive,
			Content: map[string]any{
				"summary": "Focused task procedure",
			},
		},
		{
			CardID:   "search:one",
			CardType: "search_summary",
			Scope:    memory.ScopeProject,
			Status:   memory.CardStatusActive,
			Content: map[string]any{
				"summary": "Focused task semantic one",
			},
		},
		{
			CardID:   "search:two",
			CardType: "search_summary",
			Scope:    memory.ScopeProject,
			Status:   memory.CardStatusActive,
			Content: map[string]any{
				"summary": "Focused task semantic two",
			},
		},
		{
			CardID:   "email:one",
			CardType: "email_summary",
			Scope:    memory.ScopeUser,
			Status:   memory.CardStatusActive,
			Content: map[string]any{
				"summary": "Focused task email context",
			},
		},
	} {
		if _, err := store.CreateCard(req); err != nil {
			t.Fatalf("CreateCard returned error: %v", err)
		}
	}

	orchestrator := NewWithQuota(recall.NewService(store), runtimeStore, Quota{
		MaxRecentTasks:        1,
		MaxWorkingNotes:       2,
		MaxProcedureHits:      1,
		MaxSemanticHits:       1,
		MaxWorkingRecallHits:  1,
		MaxFallbackRecallHits: 2,
	})
	packet := orchestrator.BuildTaskPacket("focused task", "", SessionView{
		Topic:            "focused task",
		FocusTaskID:      focusTask.TaskID,
		PendingQuestion:  "what should happen next?",
		PendingAction:    "summarize",
		LastAssistantAct: "task_started",
		WorkingRecallIDs: []string{"search:one", "search:two"},
		WorkingSources:   []string{"web", "web"},
	}, []TaskRef{
		{TaskID: "task-a", Title: "Task A"},
		{TaskID: "task-b", Title: "Task B"},
	})
	if packet == nil {
		t.Fatalf("expected task packet")
	}
	if len(packet.RecentTasks) != 1 {
		t.Fatalf("expected recent tasks clipped to 1, got %#v", packet.RecentTasks)
	}
	if packet.RecentTasks[0].TaskID != focusTask.TaskID {
		t.Fatalf("expected focused task to win recent-task quota, got %#v", packet.RecentTasks)
	}
	if len(packet.WorkingNotes) != 2 {
		t.Fatalf("expected working notes clipped to 2, got %#v", packet.WorkingNotes)
	}
	if len(packet.ProcedureHits) != 1 {
		t.Fatalf("expected one procedure hit, got %#v", packet.ProcedureHits)
	}
	if len(packet.SemanticHits) != 1 {
		t.Fatalf("expected one semantic hit, got %#v", packet.SemanticHits)
	}
	if len(packet.RecallHits) > 3 {
		t.Fatalf("expected aggregate recall to stay bounded, got %#v", packet.RecallHits)
	}
}

func TestBuildTaskPacketUsesSkillAwareSemanticSources(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	for _, req := range []memory.CreateCardRequest{
		{
			CardID:   "search:web",
			CardType: "search_summary",
			Scope:    memory.ScopeProject,
			Status:   memory.CardStatusActive,
			Content: map[string]any{
				"summary": "approval context for repository research",
			},
		},
		{
			CardID:   "email:user",
			CardType: "email_summary",
			Scope:    memory.ScopeUser,
			Status:   memory.CardStatusActive,
			Content: map[string]any{
				"summary": "approval context from inbox triage",
			},
		},
	} {
		if _, err := store.CreateCard(req); err != nil {
			t.Fatalf("CreateCard returned error: %v", err)
		}
	}

	orchestrator := New(recall.NewService(store), nil)

	webPacket := orchestrator.BuildTaskPacket("approval context", "web-search", SessionView{}, nil)
	if webPacket == nil || len(webPacket.SemanticHits) == 0 {
		t.Fatalf("expected web semantic packet")
	}
	for _, hit := range webPacket.SemanticHits {
		if hit.Source != "web" {
			t.Fatalf("expected only web semantic hits for web-search, got %#v", webPacket.SemanticHits)
		}
	}

	emailPacket := orchestrator.BuildTaskPacket("approval context", "email-inbox", SessionView{}, nil)
	if emailPacket == nil || len(emailPacket.SemanticHits) == 0 {
		t.Fatalf("expected email semantic packet")
	}
	for _, hit := range emailPacket.SemanticHits {
		if hit.Source != "email" {
			t.Fatalf("expected only email semantic hits for email-inbox, got %#v", emailPacket.SemanticHits)
		}
	}
}

func TestRecordPacketUseAppliesFeedback(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	for _, req := range []memory.CreateCardRequest{
		{
			CardID:   "procedure:expense_audit:v1",
			CardType: "procedure",
			Status:   memory.CardStatusActive,
			Content:  map[string]any{"summary": "Audit reimbursements."},
			Provenance: memory.Provenance{
				Confidence: 0.7,
			},
		},
		{
			CardID:   "search:repo:summary",
			CardType: "search_summary",
			Status:   memory.CardStatusActive,
			Content:  map[string]any{"summary": "Repository planning summary."},
			Provenance: memory.Provenance{
				Confidence: 0.5,
			},
		},
	} {
		if _, err := store.CreateCard(req); err != nil {
			t.Fatalf("CreateCard returned error: %v", err)
		}
	}

	orchestrator := New(recall.NewService(store), nil)
	if err := orchestrator.RecordPacketUse(&Packet{
		ProcedureHits: []RecallRef{{CardID: "procedure:expense_audit:v1", Source: "procedure"}},
		SemanticHits:  []RecallRef{{CardID: "search:repo:summary", Source: "web"}},
	}, UsageContext{
		TaskClass: "task-plan",
		Outcome:   airuntime.TaskStateDone,
	}); err != nil {
		t.Fatalf("RecordPacketUse returned error: %v", err)
	}

	procedureCard := store.Query(memory.QueryRequest{CardID: "procedure:expense_audit:v1"}).Cards[0]
	if procedureCard.Version != 2 {
		t.Fatalf("expected procedure feedback version bump, got %d", procedureCard.Version)
	}
	if procedureCard.Activation.LastAccessAt == nil {
		t.Fatalf("expected procedure last access to be updated")
	}
	if procedureCard.Provenance.Confidence <= 0.72 {
		t.Fatalf("expected procedure confidence bump, got %f", procedureCard.Provenance.Confidence)
	}

	semanticCard := store.Query(memory.QueryRequest{CardID: "search:repo:summary"}).Cards[0]
	if semanticCard.Version != 2 {
		t.Fatalf("expected semantic feedback version bump, got %d", semanticCard.Version)
	}
	if semanticCard.Provenance.Confidence <= 0.50 {
		t.Fatalf("expected semantic confidence bump, got %f", semanticCard.Provenance.Confidence)
	}
}

func TestRecordPacketUseAdjustsByTaskClassAndOutcome(t *testing.T) {
	t.Parallel()

	buildStore := func() *memory.Store {
		store := memory.NewStore()
		for _, req := range []memory.CreateCardRequest{
			{
				CardID:   "procedure:test:v1",
				CardType: "procedure",
				Status:   memory.CardStatusActive,
				Content:  map[string]any{"summary": "Test procedure."},
				Provenance: memory.Provenance{
					Confidence: 0.70,
				},
			},
			{
				CardID:   "search:test:summary",
				CardType: "search_summary",
				Status:   memory.CardStatusActive,
				Content:  map[string]any{"summary": "Test summary."},
				Provenance: memory.Provenance{
					Confidence: 0.50,
				},
			},
		} {
			if _, err := store.CreateCard(req); err != nil {
				t.Fatalf("CreateCard returned error: %v", err)
			}
		}
		return store
	}

	record := func(taskClass, outcome string) (float64, float64) {
		store := buildStore()
		orchestrator := New(recall.NewService(store), nil)
		if err := orchestrator.RecordPacketUse(&Packet{
			ProcedureHits: []RecallRef{{CardID: "procedure:test:v1", Source: "procedure"}},
			SemanticHits:  []RecallRef{{CardID: "search:test:summary", Source: "web"}},
		}, UsageContext{
			TaskClass: taskClass,
			Outcome:   outcome,
		}); err != nil {
			t.Fatalf("RecordPacketUse returned error: %v", err)
		}
		procedureCard := store.Query(memory.QueryRequest{CardID: "procedure:test:v1"}).Cards[0]
		semanticCard := store.Query(memory.QueryRequest{CardID: "search:test:summary"}).Cards[0]
		return procedureCard.Provenance.Confidence, semanticCard.Provenance.Confidence
	}

	planProcedureConfidence, planSemanticConfidence := record("task-plan", airuntime.TaskStateDone)
	replyProcedureConfidence, replySemanticConfidence := record("direct_reply", "responded")
	failedProcedureConfidence, failedSemanticConfidence := record("task-plan", airuntime.TaskStateFailed)

	if planProcedureConfidence <= replyProcedureConfidence {
		t.Fatalf("expected task-plan to give larger procedure bump than direct reply: plan=%f reply=%f", planProcedureConfidence, replyProcedureConfidence)
	}
	if planSemanticConfidence <= replySemanticConfidence {
		t.Fatalf("expected task-plan to give larger semantic bump than direct reply: plan=%f reply=%f", planSemanticConfidence, replySemanticConfidence)
	}
	if failedProcedureConfidence >= planProcedureConfidence {
		t.Fatalf("expected failed outcome to reduce procedure bump: failed=%f plan=%f", failedProcedureConfidence, planProcedureConfidence)
	}
	if failedSemanticConfidence != 0.50 {
		t.Fatalf("expected failed outcome to avoid semantic confidence bump, got %f", failedSemanticConfidence)
	}
}
