package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/model"
	"mnemosyneos/internal/recall"
	"mnemosyneos/internal/skills"
)

func TestSendCreatesTaskAndTranscript(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "email:test:summary",
		CardType: "email_summary",
		Content: map[string]any{
			"summary": "Repository planning needs approval context",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	resp, err := service.Send(SendRequest{
		SessionID:   "default",
		Message:     "Plan the next repository step with approval context",
		RequestedBy: "chat-test",
		// Source="intent-confirmation" bypasses the always-confirm gate that
		// new task_request turns go through, simulating the second turn after
		// the user approved the preview. See shouldConfirmTaskIntent.
		Source: "intent-confirmation",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.UserMessage.Role != "user" {
		t.Fatalf("expected user message role, got %s", resp.UserMessage.Role)
	}
	if resp.AssistantMessage.Role != "assistant" {
		t.Fatalf("expected assistant message role, got %s", resp.AssistantMessage.Role)
	}
	if resp.AssistantMessage.TaskState != airuntime.TaskStateDone {
		t.Fatalf("expected done task state in assistant message, got %s", resp.AssistantMessage.TaskState)
	}
	if resp.AssistantMessage.IntentKind != IntentKindTask {
		t.Fatalf("expected task intent kind, got %s", resp.AssistantMessage.IntentKind)
	}
	if strings.TrimSpace(resp.AssistantMessage.Content) == "" {
		t.Fatalf("expected non-empty assistant reply content")
	}
	if len(resp.AssistantMessage.Links) == 0 {
		t.Fatalf("expected assistant links to be populated")
	}
	if resp.AssistantMessage.Context == nil {
		t.Fatalf("expected assistant context to be populated")
	}
	if len(resp.AssistantMessage.Context.RecallHits) == 0 {
		t.Fatalf("expected recall hits in assistant context")
	}
	foundArtifact := false
	for _, link := range resp.AssistantMessage.Links {
		if strings.Contains(link.Href, "/ui/artifacts/view") {
			foundArtifact = true
			break
		}
	}
	if !foundArtifact {
		t.Fatalf("expected artifact link in assistant links")
	}

	messages, err := service.Messages("default", 10)
	if err != nil {
		t.Fatalf("Messages returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected two chat messages, got %d", len(messages))
	}
	if messages[1].TaskState != airuntime.TaskStateDone {
		t.Fatalf("expected hydrated done task state, got %s", messages[1].TaskState)
	}
}

func TestAgentLoopDoesNotBypassTaskRuntime(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	modelGateway := fakeTextGateway{text: "assistant summary"}
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, modelGateway, memoryStore)
	service.SetAgentLoop(NewAgentLoop(modelGateway, skills.NewAgentSkillRegistry()))

	resp, err := service.Send(SendRequest{
		SessionID: "default",
		Message:   "Plan the next repository step",
		Source:    "intent-confirmation", // bypass always-confirm gate for this scenario
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.AssistantMessage.IntentKind != IntentKindTask {
		t.Fatalf("expected task intent, got %s", resp.AssistantMessage.IntentKind)
	}
	if strings.TrimSpace(resp.AssistantMessage.TaskID) == "" {
		t.Fatalf("expected task runtime path to create a task, got %+v", resp.AssistantMessage)
	}
	if resp.AssistantMessage.SelectedSkill != SkillTaskPlan {
		t.Fatalf("expected task-plan skill, got %s", resp.AssistantMessage.SelectedSkill)
	}
}

func TestSendChineseSearchRequestUsesWebSearchSkill(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	resp, err := service.Send(SendRequest{
		SessionID: "default",
		Message:   "帮我搜索一下 OpenClaw 的 memory 设计",
		Source:    "intent-confirmation", // bypass always-confirm gate for this scenario
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.AssistantMessage.SelectedSkill != SkillWebSearch {
		t.Fatalf("expected %s skill, got %s", SkillWebSearch, resp.AssistantMessage.SelectedSkill)
	}
}

type fakeTextGateway struct {
	text string
}

func (f fakeTextGateway) GenerateText(context.Context, model.TextRequest) (model.TextResponse, error) {
	return model.TextResponse{Text: f.text}, nil
}

func (f fakeTextGateway) StreamText(_ context.Context, req model.TextRequest, onDelta func(model.TextDelta) error) (model.TextResponse, error) {
	if onDelta != nil && f.text != "" {
		if err := onDelta(model.TextDelta{Text: f.text}); err != nil {
			return model.TextResponse{}, err
		}
	}
	return model.TextResponse{Text: f.text}, nil
}

func TestSendGreetingDoesNotCreateTask(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	resp, err := service.Send(SendRequest{
		SessionID:   "default",
		Message:     "你好",
		RequestedBy: "chat-test",
		Source:      "chat-test",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.AssistantMessage.TaskID != "" {
		t.Fatalf("expected direct reply without task, got task id %s", resp.AssistantMessage.TaskID)
	}

	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no runtime tasks for greeting, got %d", len(tasks))
	}
	if resp.AssistantMessage.IntentKind != IntentKindDirect {
		t.Fatalf("expected direct reply intent, got %s", resp.AssistantMessage.IntentKind)
	}
	if !strings.Contains(resp.AssistantMessage.Content, "你好") {
		t.Fatalf("expected greeting response, got %q", resp.AssistantMessage.Content)
	}
	intentObservation := filepath.Join(runtimeRoot, "observations", "os", resp.UserMessage.MessageID+"-intent.json")
	if _, err := os.Stat(intentObservation); err != nil {
		t.Fatalf("expected intent observation file, got stat error: %v", err)
	}
}

func TestSendFollowupUsesSessionStateInsteadOfCreatingNewTask(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	chatStore := NewStore(runtimeRoot)
	service := NewService(chatStore, orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	task, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:         "帮我搜索一下 OpenClaw 的 memory 设计",
		Goal:          "帮我搜索一下 OpenClaw 的 memory 设计",
		SelectedSkill: SkillWebSearch,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	artifactPath := filepath.Join(runtimeRoot, "artifacts", "reports", task.TaskID+"-web-search.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("# Web Search Result\n\nOpenClaw memory uses Markdown files plus indexed retrieval.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	doneTask, err := runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["search_artifact"] = artifactPath
		t.NextAction = "search completed"
	})
	if err != nil {
		t.Fatalf("MoveTask returned error: %v", err)
	}
	if err := chatStore.SaveSessionState(SessionState{
		SessionID:       "default",
		Topic:           doneTask.Title,
		FocusTaskID:     doneTask.TaskID,
		PendingQuestion: "需要我帮你总结这些资料的核心内容吗？",
		PendingAction:   "summarize_focus_task",
		WorkingSet: SessionWorkset{
			ArtifactPaths: []string{artifactPath},
		},
	}); err != nil {
		t.Fatalf("SaveSessionState returned error: %v", err)
	}

	resp, err := service.Send(SendRequest{
		SessionID: "default",
		Message:   "需要",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.AssistantMessage.TaskID != "" {
		t.Fatalf("expected follow-up reply without new task, got task id %s", resp.AssistantMessage.TaskID)
	}
	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected no extra task creation, got %d tasks", len(tasks))
	}
	if !strings.Contains(resp.AssistantMessage.Content, "OpenClaw") {
		t.Fatalf("expected follow-up reply to use prior artifact content, got %q", resp.AssistantMessage.Content)
	}
}

func TestSendEnglishGreetingRepliesInEnglish(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	resp, err := service.Send(SendRequest{SessionID: "default", Message: "hello"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp.AssistantMessage.Content), "hi") && !strings.Contains(strings.ToLower(resp.AssistantMessage.Content), "help") {
		t.Fatalf("expected english direct reply, got %q", resp.AssistantMessage.Content)
	}
}

func TestSendAmbiguousTaskRequestRequiresConfirmation(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	resp, err := service.Send(SendRequest{SessionID: "default", Message: "plan this"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.AssistantMessage.Stage != "awaiting_confirmation" {
		t.Fatalf("expected awaiting confirmation stage, got %s", resp.AssistantMessage.Stage)
	}
	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks before confirmation, got %d", len(tasks))
	}

	resp2, err := service.Send(SendRequest{SessionID: "default", Message: "yes"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if strings.TrimSpace(resp2.AssistantMessage.TaskID) == "" {
		t.Fatalf("expected task creation after confirmation")
	}
}

// TestSendAlwaysConfirmsTaskIntent verifies that even a concrete, well-formed
// task_request goes through the preview + approve step first, and that the
// preview shows the goal, skill, and profile the runtime would actually use.
func TestSendAlwaysConfirmsTaskIntent(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	resp, err := service.Send(SendRequest{
		SessionID: "default",
		Message:   "帮我搜索一下 OpenClaw 的 memory 设计",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.AssistantMessage.Stage != "awaiting_confirmation" {
		t.Fatalf("expected awaiting_confirmation stage on first turn, got %s", resp.AssistantMessage.Stage)
	}
	if strings.TrimSpace(resp.AssistantMessage.TaskID) != "" {
		t.Fatalf("expected no task to be created before confirmation, got %s", resp.AssistantMessage.TaskID)
	}
	content := resp.AssistantMessage.Content
	for _, want := range []string{"目标", "Skill", "执行权限", "OpenClaw"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected confirmation preview to contain %q, got %q", want, content)
		}
	}

	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks yet, got %d", len(tasks))
	}

	resp2, err := service.Send(SendRequest{SessionID: "default", Message: "开始"})
	if err != nil {
		t.Fatalf("Send (confirm) returned error: %v", err)
	}
	if strings.TrimSpace(resp2.AssistantMessage.TaskID) == "" {
		t.Fatalf("expected task creation after 开始 confirmation, got message %+v", resp2.AssistantMessage)
	}
}

// TestSendAmbiguousReplyInConfirmationTreatedAsRefinedGoal verifies that when
// the user replies with something that's neither yes nor no during
// confirmation, we treat it as a refined goal and re-enter the confirmation
// flow instead of looping "please reply yes or no".
func TestSendAmbiguousReplyInConfirmationTreatedAsRefinedGoal(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	if _, err := service.Send(SendRequest{SessionID: "default", Message: "plan this"}); err != nil {
		t.Fatalf("Send 1 returned error: %v", err)
	}
	resp2, err := service.Send(SendRequest{SessionID: "default", Message: "帮我搜索一下 OpenClaw 的 memory 设计"})
	if err != nil {
		t.Fatalf("Send 2 returned error: %v", err)
	}
	if resp2.AssistantMessage.Stage != "awaiting_confirmation" {
		t.Fatalf("expected refined goal to re-enter awaiting_confirmation, got stage %q", resp2.AssistantMessage.Stage)
	}
	if !strings.Contains(resp2.AssistantMessage.Content, "OpenClaw") {
		t.Fatalf("expected refined goal preview to mention OpenClaw, got %q", resp2.AssistantMessage.Content)
	}

	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks created yet, got %d", len(tasks))
	}
}

// TestSendNegativeReplyCancelsConfirmation verifies that cancel/取消 short
// circuits the pending confirmation without creating a task.
func TestSendNegativeReplyCancelsConfirmation(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	if _, err := service.Send(SendRequest{SessionID: "default", Message: "plan this"}); err != nil {
		t.Fatalf("Send 1 returned error: %v", err)
	}
	resp2, err := service.Send(SendRequest{SessionID: "default", Message: "取消"})
	if err != nil {
		t.Fatalf("Send 2 returned error: %v", err)
	}
	if strings.TrimSpace(resp2.AssistantMessage.TaskID) != "" {
		t.Fatalf("expected no task after cancel, got %s", resp2.AssistantMessage.TaskID)
	}
	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks after cancel, got %d", len(tasks))
	}
}

func TestSendFocusedTaskContinuationUsesExistingTaskContext(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	chatStore := NewStore(runtimeRoot)
	service := NewService(chatStore, orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	task, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:         "Search OpenClaw memory",
		Goal:          "Search OpenClaw memory",
		SelectedSkill: SkillWebSearch,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	artifactPath := filepath.Join(runtimeRoot, "artifacts", "reports", task.TaskID+"-web-search.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("OpenClaw uses durable files plus retrieval layers."), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	doneTask, err := runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["search_artifact"] = artifactPath
	})
	if err != nil {
		t.Fatalf("MoveTask returned error: %v", err)
	}
	if err := chatStore.SaveSessionState(SessionState{
		SessionID:   "default",
		Topic:       doneTask.Title,
		FocusTaskID: doneTask.TaskID,
		WorkingSet: SessionWorkset{
			ArtifactPaths: []string{artifactPath},
		},
	}); err != nil {
		t.Fatalf("SaveSessionState returned error: %v", err)
	}

	resp, err := service.Send(SendRequest{
		SessionID: "default",
		Message:   "继续展开",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.AssistantMessage.TaskID != "" {
		t.Fatalf("expected no new task, got %s", resp.AssistantMessage.TaskID)
	}
	if !strings.Contains(resp.AssistantMessage.Content, "OpenClaw") {
		t.Fatalf("expected focused task followup content, got %q", resp.AssistantMessage.Content)
	}
}

func TestSendRootProfileRunsTaskPlanWithoutOrchestratorApprovalGate(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	runner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), runner, nil, memoryStore)

	resp, err := service.Send(SendRequest{
		SessionID:        "default",
		Message:          "Summarize in one short paragraph what root execution profile means for MnemosyneOS.",
		RequestedBy:      "chat-test",
		Source:           "chat-test",
		ExecutionProfile: "root",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resp.AssistantMessage.TaskID == "" {
		t.Fatalf("expected a task id on assistant message")
	}
	if resp.AssistantMessage.TaskState != airuntime.TaskStateDone {
		t.Fatalf("expected task-plan to finish without pre-run orchestrator approval gate, got state=%s skill=%s",
			resp.AssistantMessage.TaskState, resp.AssistantMessage.SelectedSkill)
	}

	sessions, err := service.Sessions(10)
	if err != nil {
		t.Fatalf("Sessions returned error: %v", err)
	}
	if len(sessions) == 0 || sessions[0].SessionID != "default" {
		t.Fatalf("expected default session summary to be present")
	}
}

func TestBuildActionsAwaitingApprovalIncludesApprove(t *testing.T) {
	t.Parallel()
	actions := buildActions(airuntime.Task{
		TaskID: "task-test",
		State:  airuntime.TaskStateAwaitingApproval,
		Metadata: map[string]string{
			"root_approval_id": "appr-test-1",
		},
	})
	found := false
	for _, action := range actions {
		if action.Label == "Approve and Continue" && strings.Contains(action.Href, "appr-test-1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected approve action, got %#v", actions)
	}
}

func TestRenameArchiveAndDeleteSession(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	memoryStore := memory.NewStore()
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), nil, nil, memoryStore)

	if _, err := service.EnsureSession("session-1"); err != nil {
		t.Fatalf("EnsureSession returned error: %v", err)
	}
	if err := service.RenameSession("session-1", "Planning Thread"); err != nil {
		t.Fatalf("RenameSession returned error: %v", err)
	}

	sessions, err := service.Sessions(10)
	if err != nil {
		t.Fatalf("Sessions returned error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Title != "Planning Thread" {
		t.Fatalf("expected renamed session, got %#v", sessions)
	}

	if err := service.ArchiveSession("session-1"); err != nil {
		t.Fatalf("ArchiveSession returned error: %v", err)
	}
	sessions, err = service.Sessions(10)
	if err != nil {
		t.Fatalf("Sessions returned error after archive: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected archived session to disappear from active sessions, got %#v", sessions)
	}
	archived, err := service.ArchivedSessions(10)
	if err != nil {
		t.Fatalf("ArchivedSessions returned error: %v", err)
	}
	if len(archived) != 1 || archived[0].SessionID != "session-1" {
		t.Fatalf("expected archived session summary, got %#v", archived)
	}
	if err := service.RestoreSession("session-1"); err != nil {
		t.Fatalf("RestoreSession returned error: %v", err)
	}
	sessions, err = service.Sessions(10)
	if err != nil {
		t.Fatalf("Sessions returned error after restore: %v", err)
	}
	found := false
	for _, session := range sessions {
		if session.SessionID == "session-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected restored session to return to active list")
	}

	if _, err := service.EnsureSession("session-delete"); err != nil {
		t.Fatalf("EnsureSession for delete returned error: %v", err)
	}
	if err := service.DeleteSession("session-delete"); err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	sessions, err = service.Sessions(10)
	if err != nil {
		t.Fatalf("Sessions returned error after delete: %v", err)
	}
	for _, session := range sessions {
		if session.SessionID == "session-delete" {
			t.Fatalf("expected deleted session to disappear from active sessions")
		}
	}
}

func TestBuildFastContextSnapshotUsesSessionWorkingSet(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "procedure:expense-audit:v1",
		CardType: "procedure",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusActive,
		Content: map[string]any{
			"summary": "Repository planning procedure with explicit approval validation.",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "search:test:summary",
		CardType: "search_summary",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusActive,
		Content: map[string]any{
			"summary": "Focused task requires repository planning context.",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), nil, nil, memoryStore)

	task, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:         "Focused task",
		Goal:          "Continue the focused thread",
		SelectedSkill: SkillWebSearch,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	ctx := service.buildFastContextSnapshot(SessionState{
		SessionID:       "default",
		Topic:           "repository planning",
		PendingQuestion: "continue the focused thread?",
		FocusTaskID:     task.TaskID,
		WorkingSet: SessionWorkset{
			RecallCardIDs: []string{"search:test:summary", "email:test:summary"},
			SourceRefs:    []string{"web", "email"},
		},
	})
	if ctx == nil {
		t.Fatalf("expected fast context snapshot")
	}
	if len(ctx.WorkingNotes) == 0 {
		t.Fatalf("expected working notes in fast context")
	}
	if len(ctx.RecentTasks) != 1 || ctx.RecentTasks[0].TaskID != task.TaskID {
		t.Fatalf("expected focused task in fast context, got %#v", ctx.RecentTasks)
	}
	if len(ctx.ProcedureHits) != 1 {
		t.Fatalf("expected procedure hit in fast context, got %#v", ctx.ProcedureHits)
	}
	if len(ctx.SemanticHits) == 0 {
		t.Fatalf("expected semantic hits in fast context, got %#v", ctx.SemanticHits)
	}
	if len(ctx.RecallHits) < 2 {
		t.Fatalf("expected working-set recall hits, got %#v", ctx.RecallHits)
	}
}

func TestDirectReplyPromptIncludesMemorySections(t *testing.T) {
	service := &Service{}
	prompt := service.directReplyPrompt("继续", &Context{
		WorkingNotes:  []string{"topic: reimbursement audit", "pending question: continue the summary?"},
		SemanticHits:  []RecallRef{{CardID: "search:test:summary", Snippet: "User prefers concise audit summaries."}},
		ProcedureHits: []RecallRef{{CardID: "procedure:expense-audit:v1", Snippet: "extract fields then validate policy"}},
	}, "assistant: 已经总结了报销流程", "继续总结", localeEN)

	for _, snippet := range []string{
		"Working memory:",
		"Relevant long-term facts:",
		"Relevant procedure guidance:",
		"topic: reimbursement audit",
		"extract fields then validate policy",
	} {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("expected %q in prompt, got %q", snippet, prompt)
		}
	}
}

func TestFinalizeSessionStateRecordsMemoryUse(t *testing.T) {
	runtimeRoot := tempChatRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "procedure:expense-audit:v1",
		CardType: "procedure",
		Status:   memory.CardStatusActive,
		Content:  map[string]any{"summary": "Audit reimbursements."},
		Provenance: memory.Provenance{
			Confidence: 0.7,
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	service := NewService(NewStore(runtimeRoot), orchestrator, runtimeStore, recall.NewService(memoryStore), nil, nil, memoryStore)
	task, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:         "Audit reimbursements",
		Goal:          "Audit reimbursements",
		SelectedSkill: SkillMemoryConsolidate,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	service.finalizeSessionState("default", task, nil, "Done.", "task_request", &Context{
		ProcedureHits: []RecallRef{{CardID: "procedure:expense-audit:v1", Source: "procedure", CardType: "procedure", Snippet: "extract fields then validate policy"}},
	})

	card := memoryStore.Query(memory.QueryRequest{CardID: "procedure:expense-audit:v1"}).Cards[0]
	if card.Version != 2 {
		t.Fatalf("expected memory use feedback to create new version, got %d", card.Version)
	}
	if card.Activation.LastAccessAt == nil {
		t.Fatalf("expected last access time to be updated")
	}
}

func TestBuildTaskResultEnvelopeProducesLightweightSummary(t *testing.T) {
	task := airuntime.Task{
		TaskID:        "task-1",
		Title:         "Search OpenClaw memory",
		State:         airuntime.TaskStateDone,
		SelectedSkill: SkillWebSearch,
		NextAction:    "review the summary",
	}
	runResult := &skills.RunResult{
		ArtifactPaths:    []string{"/tmp/result.md"},
		ObservationPaths: []string{"/tmp/result.json"},
	}

	envelope := buildTaskResultEnvelope(task, runResult)
	if envelope.Outcome != airuntime.TaskStateDone {
		t.Fatalf("expected done outcome, got %s", envelope.Outcome)
	}
	if envelope.Headline == "" {
		t.Fatalf("expected non-empty headline")
	}
	if envelope.NextAction != "review the summary" {
		t.Fatalf("expected next action to be preserved, got %q", envelope.NextAction)
	}
	if len(envelope.ArtifactPaths) != 1 || len(envelope.ObservationPaths) != 1 {
		t.Fatalf("expected artifact and observation paths in envelope")
	}
}

func TestStageMessageUsesSkillSpecificText(t *testing.T) {
	task := airuntime.Task{SelectedSkill: "web-search"}
	if got := stageMessage(task, "queued", localeEN); !strings.Contains(got, "search query") {
		t.Fatalf("expected queued web-search stage text, got %q", got)
	}
	if got := stageMessage(task, "running", localeEN); !strings.Contains(got, "Searching the web") {
		t.Fatalf("expected running web-search stage text, got %q", got)
	}
}

func TestNormalizeAssistantTextRemovesMarkdownMarkers(t *testing.T) {
	got := normalizeAssistantText("## 标题\n\n1. **重点**\n* 项目\n`code`")
	if strings.Contains(got, "**") || strings.Contains(got, "`") || strings.Contains(got, "## ") {
		t.Fatalf("expected markdown markers removed, got %q", got)
	}
}

func tempChatRuntimeRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	dirs := []string{
		filepath.Join(root, "state"),
		filepath.Join(root, "tasks", airuntime.TaskStateInbox),
		filepath.Join(root, "tasks", airuntime.TaskStatePlanned),
		filepath.Join(root, "tasks", airuntime.TaskStateActive),
		filepath.Join(root, "tasks", airuntime.TaskStateBlocked),
		filepath.Join(root, "tasks", airuntime.TaskStateAwaitingApproval),
		filepath.Join(root, "tasks", airuntime.TaskStateDone),
		filepath.Join(root, "tasks", airuntime.TaskStateFailed),
		filepath.Join(root, "tasks", airuntime.TaskStateArchived),
		filepath.Join(root, "approvals", approval.StatusPending),
		filepath.Join(root, "approvals", approval.StatusApproved),
		filepath.Join(root, "approvals", approval.StatusDenied),
		filepath.Join(root, "approvals", approval.StatusConsumed),
		filepath.Join(root, "actions", execution.ActionStatusPending),
		filepath.Join(root, "actions", execution.ActionStatusRunning),
		filepath.Join(root, "actions", execution.ActionStatusCompleted),
		filepath.Join(root, "actions", execution.ActionStatusFailed),
		filepath.Join(root, "artifacts", "reports"),
		filepath.Join(root, "observations", "filesystem"),
		filepath.Join(root, "observations", "os"),
		filepath.Join(root, "sessions", "current"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}

	state := airuntime.RuntimeState{
		RuntimeID:        "chat-test-runtime",
		ActiveUserID:     "default-user",
		Status:           "idle",
		ExecutionProfile: "user",
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "state", "runtime.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return root
}

func TestModelReplyFailureMessageLocales(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("siliconflow API error: 401")
	zh := modelReplyFailureMessage("zh-CN", err)
	if !strings.Contains(zh, "401") || !strings.Contains(zh, "doctor") {
		t.Fatalf("unexpected zh failure message: %q", zh)
	}
	en := modelReplyFailureMessage("en-US", err)
	if !strings.Contains(en, "401") || !strings.Contains(en, "doctor") {
		t.Fatalf("unexpected en failure message: %q", en)
	}
}

func TestPreviewStringRunesTruncatesOnRunesNotBytes(t *testing.T) {
	t.Parallel()
	in := strings.Repeat("你", 10)
	got := previewStringRunes(in, 5)
	if want := "你你..."; got != want {
		t.Fatalf("previewStringRunes(%q, 5) = %q want %q", in, got, want)
	}
}

func TestAgentDirectReplyUserContentPassthroughWhenNoTranscript(t *testing.T) {
	t.Parallel()
	if got := agentDirectReplyUserContent(localeZH, "  hello  ", "", ""); got != "hello" {
		t.Fatalf("expected trimmed user text, got %q", got)
	}
}

func TestAgentDirectReplyUserContentIncludesTranscript(t *testing.T) {
	t.Parallel()
	got := agentDirectReplyUserContent(localeZH, "列出内容", "", "assistant: /Users/x/Lab/")
	if !strings.Contains(got, "近期对话") || !strings.Contains(got, "/Users/x/Lab/") || !strings.Contains(got, "列出内容") {
		t.Fatalf("unexpected combined prompt: %q", got)
	}
}

func TestAgentDirectReplyUserContentIncludesFocusBlock(t *testing.T) {
	t.Parallel()
	focus := formatFocusPathsForAgent([]string{"/Users/x/Lab/"}, localeZH)
	got := agentDirectReplyUserContent(localeZH, "列出内容", focus, "")
	if !strings.Contains(got, "工作记忆") || !strings.Contains(got, "/Users/x/Lab/") {
		t.Fatalf("unexpected combined prompt: %q", got)
	}
}

func TestExtractChatFilesystemPaths(t *testing.T) {
	t.Parallel()
	got := extractChatFilesystemPaths("路径在 /Users/me/Lab/ 下，详见 /tmp/a/b.txt。")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 paths, got %#v", got)
	}
}

func TestMergeFocusPathsKeepsOrderAndCap(t *testing.T) {
	t.Parallel()
	got := mergeFocusPaths([]string{"/a"}, []string{"/b", "/a"}, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique paths, got %#v", got)
	}
}
