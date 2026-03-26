package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/model"
	"mnemosyneos/internal/recall"
	"mnemosyneos/internal/skills"
)

type Service struct {
	store         *Store
	orchestrator  *airuntime.Orchestrator
	runtimeStore  *airuntime.Store
	recall        *recall.Service
	skillRunner   *skills.Runner
	textModel     model.TextGateway
	routeAgent    *RouteAgent
	dialogueAgent *DialogueAgent
	intentAgent   *IntentAgent
	skillAgent    *SkillAgent
}

func NewService(store *Store, orchestrator *airuntime.Orchestrator, runtimeStore *airuntime.Store, recallService *recall.Service, skillRunner *skills.Runner, textModel model.TextGateway) *Service {
	return &Service{
		store:         store,
		orchestrator:  orchestrator,
		runtimeStore:  runtimeStore,
		recall:        recallService,
		skillRunner:   skillRunner,
		textModel:     textModel,
		routeAgent:    NewRouteAgent(textModel),
		dialogueAgent: NewDialogueAgent(textModel),
		intentAgent:   NewIntentAgent(textModel),
		skillAgent:    NewSkillAgent(textModel),
	}
}

func (s *Service) Messages(sessionID string, limit int) ([]Message, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	sessionID = s.normalizeSessionID(sessionID)
	messages, err := s.store.List(sessionID, limit)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		messages[i] = s.hydrateMessage(messages[i])
	}
	return messages, nil
}

func (s *Service) Sessions(limit int) ([]SessionSummary, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.Sessions(limit)
}

func (s *Service) ArchivedSessions(limit int) ([]SessionSummary, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.ArchivedSessions(limit)
}

func (s *Service) SessionState(sessionID string) (SessionState, error) {
	if s == nil || s.store == nil {
		return SessionState{SessionID: sessionID}, nil
	}
	return s.store.GetSessionState(s.normalizeSessionID(sessionID))
}

func (s *Service) EnsureSession(sessionID string) (string, error) {
	sessionID = s.normalizeSessionID(sessionID)
	if s == nil || s.store == nil {
		return sessionID, fmt.Errorf("chat service is not configured")
	}
	return sessionID, s.store.EnsureSession(sessionID)
}

func (s *Service) RenameSession(sessionID, title string) error {
	sessionID = s.normalizeSessionID(sessionID)
	if s == nil || s.store == nil {
		return fmt.Errorf("chat service is not configured")
	}
	return s.store.RenameSession(sessionID, title)
}

func (s *Service) ArchiveSession(sessionID string) error {
	sessionID = s.normalizeSessionID(sessionID)
	if s == nil || s.store == nil {
		return fmt.Errorf("chat service is not configured")
	}
	return s.store.ArchiveSession(sessionID)
}

func (s *Service) DeleteSession(sessionID string) error {
	sessionID = s.normalizeSessionID(sessionID)
	if s == nil || s.store == nil {
		return fmt.Errorf("chat service is not configured")
	}
	return s.store.DeleteSession(sessionID)
}

func (s *Service) RestoreSession(sessionID string) error {
	sessionID = s.normalizeSessionID(sessionID)
	if s == nil || s.store == nil {
		return fmt.Errorf("chat service is not configured")
	}
	return s.store.RestoreSession(sessionID)
}

func (s *Service) Send(req SendRequest) (SendResponse, error) {
	if s == nil || s.store == nil || s.orchestrator == nil || s.skillRunner == nil {
		return SendResponse{}, fmt.Errorf("chat service is not configured")
	}
	text := strings.TrimSpace(req.Message)
	if text == "" {
		return SendResponse{}, fmt.Errorf("message is required")
	}

	sessionID := s.normalizeSessionID(req.SessionID)
	sessionState := s.loadSessionState(sessionID)
	conversationContext := s.recentConversationContext(sessionID, 6)
	now := time.Now().UTC()
	route := heuristicRouteDecision(text, conversationContext, sessionState)
	if s.routeAgent != nil {
		route = s.routeAgent.Decide(text, conversationContext, sessionState)
	}
	dialogue := DialogueDecision{Act: route.DialogueAct, Reason: route.Reason, Confidence: route.Confidence}
	userMessage := Message{
		MessageID:        fmt.Sprintf("msg-%d", now.UnixNano()),
		SessionID:        sessionID,
		Role:             "user",
		Content:          text,
		DialogueAct:      dialogue.Act,
		ExecutionProfile: firstNonEmpty(req.ExecutionProfile, "user"),
		CreatedAt:        now,
	}
	if err := s.store.Append(sessionID, userMessage); err != nil {
		return SendResponse{}, err
	}

	intent := IntentDecision{Kind: route.IntentKind, Reason: route.Reason, Confidence: route.Confidence}
	_ = s.writeIntentObservation(sessionID, userMessage.MessageID, text, intent)
	userMessage.IntentKind = intent.Kind
	userMessage.IntentReason = intent.Reason
	userMessage.IntentConfidence = intent.Confidence

	fastContext := s.buildFastContextSnapshot(sessionState)

	if followupMessage, handled := s.tryHandleSessionFollowup(sessionID, text, dialogue, sessionState, fastContext, req.ExecutionProfile); handled {
		assistantMessage := Message{
			MessageID:        fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano()),
			SessionID:        sessionID,
			Role:             "assistant",
			Content:          "",
			DialogueAct:      dialogue.Act,
			IntentKind:       IntentKindDirect,
			IntentReason:     "handled as session follow-up",
			IntentConfidence: 0.95,
			Stage:            "running",
			ExecutionProfile: firstNonEmpty(req.ExecutionProfile, "user"),
			Context:          fastContext,
			CreatedAt:        time.Now().UTC(),
		}
		if err := s.store.Append(sessionID, assistantMessage); err != nil {
			return SendResponse{}, err
		}
		assistantMessage = followupMessage(assistantMessage)
		sessionState.LastUserAct = dialogue.Act
		sessionState.LastAssistantAct = "followup_reply"
		sessionState.PendingQuestion = ""
		sessionState.PendingAction = ""
		_ = s.store.SaveSessionState(sessionState)
		return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, nil
	}

	if intent.Kind != IntentKindTask {
		assistantMessage := Message{
			MessageID:        fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano()),
			SessionID:        sessionID,
			Role:             "assistant",
			Content:          "",
			DialogueAct:      dialogue.Act,
			IntentKind:       intent.Kind,
			IntentReason:     intent.Reason,
			IntentConfidence: intent.Confidence,
			Stage:            "running",
			ExecutionProfile: firstNonEmpty(req.ExecutionProfile, "user"),
			Context:          fastContext,
			CreatedAt:        time.Now().UTC(),
		}
		if err := s.store.Append(sessionID, assistantMessage); err != nil {
			return SendResponse{}, err
		}
		assistantMessage = s.composeDirectReply(text, fastContext, conversationContext, assistantMessage)
		sessionState.LastUserAct = dialogue.Act
		sessionState.LastAssistantAct = "direct_reply"
		_ = s.store.SaveSessionState(sessionState)
		return SendResponse{
			UserMessage:      userMessage,
			AssistantMessage: assistantMessage,
		}, nil
	}

	createReq := airuntime.CreateTaskRequest{
		Title:            summarizeTitle(text),
		Goal:             text,
		RequestedBy:      firstNonEmpty(req.RequestedBy, "web-chat"),
		Source:           firstNonEmpty(req.Source, "web-chat"),
		ExecutionProfile: firstNonEmpty(req.ExecutionProfile, "user"),
		Metadata:         map[string]string{"chat_origin": "true"},
	}
	skillDecision := SkillDecision{Skill: route.Skill, Reason: route.Reason, Confidence: route.Confidence}
	createReq.SelectedSkill = skillDecision.Skill
	if createReq.ExecutionProfile == "root" {
		createReq.RequiresApproval = true
	}
	if strings.TrimSpace(conversationContext) != "" {
		createReq.Metadata["chat_conversation_context"] = conversationContext
	}
	effectiveQuery := expandQueryWithConversation(text, conversationContext)
	contextSnapshot := s.buildTaskContextSnapshot(effectiveQuery, sessionState)
	applyChatContextMetadata(&createReq, contextSnapshot)
	if createReq.Metadata == nil {
		createReq.Metadata = map[string]string{}
	}
	if strings.TrimSpace(skillDecision.Skill) != "" {
		createReq.Metadata["skill_reason"] = skillDecision.Reason
		createReq.Metadata["skill_confidence"] = fmt.Sprintf("%.2f", skillDecision.Confidence)
	}
	task, err := s.orchestrator.SubmitTask(createReq)
	if err != nil {
		assistantMessage := Message{
			MessageID: fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano()),
			SessionID: sessionID,
			Role:      "assistant",
			Content:   normalizeAssistantText("I could not create a task for that request: " + err.Error()),
			CreatedAt: time.Now().UTC(),
		}
		_ = s.store.Append(sessionID, assistantMessage)
		return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, nil
	}

	assistantMessage := Message{
		MessageID:        fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano()),
		SessionID:        sessionID,
		Role:             "assistant",
		Content:          stageMessage(task, "queued"),
		DialogueAct:      dialogue.Act,
		IntentKind:       intent.Kind,
		IntentReason:     intent.Reason,
		IntentConfidence: intent.Confidence,
		Stage:            "queued",
		TaskID:           task.TaskID,
		TaskState:        task.State,
		SelectedSkill:    task.SelectedSkill,
		ExecutionProfile: task.ExecutionProfile,
		Links:            buildLinks(task, nil),
		Actions:          buildActions(task),
		Context:          contextSnapshot,
		CreatedAt:        time.Now().UTC(),
	}
	if err := s.store.Append(sessionID, assistantMessage); err != nil {
		return SendResponse{}, err
	}
	sessionState = updateStateForTaskStart(sessionState, task, contextSnapshot, dialogue)
	_ = s.store.SaveSessionState(sessionState)

	if req.Async {
		go s.completeTaskReply(sessionID, assistantMessage, task, contextSnapshot, intent)
		return SendResponse{
			UserMessage:      userMessage,
			AssistantMessage: assistantMessage,
		}, nil
	}

	var runResult *skills.RunResult
	if task.State == airuntime.TaskStateActive || task.State == airuntime.TaskStatePlanned {
		assistantMessage.Stage = activeTaskStage(task)
		assistantMessage.Content = stageMessage(task, assistantMessage.Stage)
		_ = s.store.Upsert(sessionID, assistantMessage)
		result, runErr := s.skillRunner.RunTaskWithProgress(task.TaskID, func(event skills.ProgressEvent) {
			assistantMessage.Stage = normalizeProgressStage(task, event.Stage)
			assistantMessage.Content = firstNonEmpty(strings.TrimSpace(event.Message), stageMessage(task, assistantMessage.Stage))
			_ = s.store.Upsert(sessionID, assistantMessage)
		})
		if runErr == nil {
			runResult = &result
			task = result.Task
		} else if latest, latestErr := s.runtimeStore.GetTask(task.TaskID); latestErr == nil {
			task = latest
		}
	}

	assistantMessage.TaskState = task.State
	assistantMessage.SelectedSkill = task.SelectedSkill
	assistantMessage.ExecutionProfile = task.ExecutionProfile
	assistantMessage.Links = buildLinks(task, runResult)
	assistantMessage.Actions = buildActions(task)
	assistantMessage.Context = contextSnapshot
	assistantMessage.Stage = "summarizing"
	assistantMessage.Content = stageMessage(task, assistantMessage.Stage)
	_ = s.store.Upsert(sessionID, assistantMessage)
	assistantMessage.Content = s.composeAssistantMessage(task, runResult, contextSnapshot, intent, sessionID, assistantMessage.MessageID, assistantMessage.CreatedAt)
	if err := s.store.Upsert(sessionID, assistantMessage); err != nil {
		return SendResponse{}, err
	}
	s.finalizeSessionState(sessionID, task, runResult, assistantMessage.Content, dialogue.Act, contextSnapshot)

	return SendResponse{
		UserMessage:      userMessage,
		AssistantMessage: assistantMessage,
	}, nil
}

func (s *Service) completeTaskReply(sessionID string, assistantMessage Message, task airuntime.Task, contextSnapshot *Context, intent IntentDecision) {
	assistantMessage.Stage = activeTaskStage(task)
	assistantMessage.Content = stageMessage(task, assistantMessage.Stage)
	_ = s.store.Upsert(sessionID, assistantMessage)

	var runResult *skills.RunResult
	if task.State == airuntime.TaskStateActive || task.State == airuntime.TaskStatePlanned {
		result, runErr := s.skillRunner.RunTaskWithProgress(task.TaskID, func(event skills.ProgressEvent) {
			assistantMessage.Stage = normalizeProgressStage(task, event.Stage)
			assistantMessage.Content = firstNonEmpty(strings.TrimSpace(event.Message), stageMessage(task, assistantMessage.Stage))
			_ = s.store.Upsert(sessionID, assistantMessage)
		})
		if runErr == nil {
			runResult = &result
			task = result.Task
		} else if latest, latestErr := s.runtimeStore.GetTask(task.TaskID); latestErr == nil {
			task = latest
			assistantMessage.Stage = "failed"
			assistantMessage.Content = "Task execution failed: " + runErr.Error()
		} else {
			assistantMessage.Stage = "failed"
			assistantMessage.Content = "Task execution failed: " + runErr.Error()
		}
	}

	assistantMessage.TaskState = task.State
	assistantMessage.SelectedSkill = task.SelectedSkill
	assistantMessage.ExecutionProfile = task.ExecutionProfile
	assistantMessage.Links = buildLinks(task, runResult)
	assistantMessage.Actions = buildActions(task)
	assistantMessage.Context = contextSnapshot
	if !strings.HasPrefix(assistantMessage.Content, "Task execution failed: ") {
		assistantMessage.Stage = "summarizing"
		assistantMessage.Content = stageMessage(task, assistantMessage.Stage)
		_ = s.store.Upsert(sessionID, assistantMessage)
		assistantMessage.Stage = finalStage(task)
		assistantMessage.Content = s.composeAssistantMessage(task, runResult, contextSnapshot, intent, sessionID, assistantMessage.MessageID, assistantMessage.CreatedAt)
	}
	_ = s.store.Upsert(sessionID, assistantMessage)
	s.finalizeSessionState(sessionID, task, runResult, assistantMessage.Content, assistantMessage.DialogueAct, contextSnapshot)
}

func (s *Service) normalizeSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "default"
	}
	return sessionID
}

func (s *Service) composeAssistantMessage(task airuntime.Task, runResult *skills.RunResult, contextSnapshot *Context, intent IntentDecision, sessionID, messageID string, createdAt time.Time) string {
	envelope := buildTaskResultEnvelope(task, runResult)
	fallback := composeAssistantReply(task, runResult, contextSnapshot)
	if s == nil || s.textModel == nil {
		return normalizeAssistantText(fallback)
	}

	req := model.TextRequest{
		SystemPrompt: "You are the chat surface of MnemosyneOS. Reply like an operator-facing assistant: concise, conversational, and grounded in the actual runtime state. Mention approvals, blockers, artifacts, or next actions when they matter. Do not invent actions or results. Output plain text only. Do not use Markdown syntax like headings, bold markers, code fences, or bullet stars.",
		UserPrompt:   s.assistantPrompt(task, envelope, contextSnapshot, fallback),
		MaxTokens:    420,
		Temperature:  0.2,
		Profile:      model.ProfileSkills,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var builder strings.Builder
	lastFlush := time.Now()
	resp, err := s.textModel.StreamText(ctx, req, func(delta model.TextDelta) error {
		builder.WriteString(delta.Text)
		current := normalizeAssistantText(strings.TrimSpace(builder.String()))
		if current == "" {
			return nil
		}
		if time.Since(lastFlush) < 150*time.Millisecond && len(current) < 120 {
			return nil
		}
		lastFlush = time.Now()
		return s.store.Upsert(sessionID, Message{
			MessageID:        messageID,
			SessionID:        sessionID,
			Role:             "assistant",
			Content:          current,
			IntentKind:       intent.Kind,
			IntentReason:     intent.Reason,
			IntentConfidence: intent.Confidence,
			Stage:            "running",
			TaskID:           task.TaskID,
			TaskState:        task.State,
			SelectedSkill:    task.SelectedSkill,
			ExecutionProfile: task.ExecutionProfile,
			Links:            buildLinks(task, runResult),
			Actions:          buildActions(task),
			Context:          contextSnapshot,
			CreatedAt:        createdAt,
		})
	})
	if err == nil {
		text := normalizeAssistantText(strings.TrimSpace(resp.Text))
		if text == "" {
			text = normalizeAssistantText(strings.TrimSpace(builder.String()))
		}
		if text != "" {
			return text
		}
	}

	if err == nil {
		resp, err = s.textModel.GenerateText(ctx, req)
	}
	if err == nil && strings.TrimSpace(resp.Text) != "" {
		return normalizeAssistantText(strings.TrimSpace(resp.Text))
	}
	return normalizeAssistantText(fallback)
}

func (s *Service) assistantPrompt(task airuntime.Task, envelope TaskResultEnvelope, contextSnapshot *Context, fallback string) string {
	parts := []string{
		"User request execution summary:",
		fmt.Sprintf("Task ID: %s", task.TaskID),
		fmt.Sprintf("Task title: %s", task.Title),
		fmt.Sprintf("Task state: %s", task.State),
		fmt.Sprintf("Selected skill: %s", firstNonEmpty(task.SelectedSkill, "task-plan")),
		fmt.Sprintf("Execution profile: %s", firstNonEmpty(task.ExecutionProfile, "user")),
	}
	if strings.TrimSpace(envelope.Headline) != "" {
		parts = append(parts, "Outcome headline: "+envelope.Headline)
	}
	if strings.TrimSpace(envelope.NextAction) != "" {
		parts = append(parts, "Next action: "+envelope.NextAction)
	}
	if strings.TrimSpace(envelope.FailureReason) != "" {
		parts = append(parts, "Failure reason: "+envelope.FailureReason)
	}
	if len(envelope.ArtifactPaths) > 0 {
		parts = append(parts, fmt.Sprintf("Artifacts: %d", len(envelope.ArtifactPaths)))
	}
	if len(envelope.ObservationPaths) > 0 {
		parts = append(parts, fmt.Sprintf("Observations: %d", len(envelope.ObservationPaths)))
	}
	if contextSnapshot != nil {
		if len(contextSnapshot.RecallHits) > 0 {
			parts = append(parts, "Relevant memory:")
			for _, hit := range contextSnapshot.RecallHits {
				parts = append(parts, fmt.Sprintf("- %s / %s / %s", hit.Source, hit.CardType, hit.Snippet))
			}
		}
		if len(contextSnapshot.RecentTasks) > 0 {
			parts = append(parts, "Recent tasks:")
			for _, ref := range contextSnapshot.RecentTasks {
				parts = append(parts, fmt.Sprintf("- %s / %s / %s", ref.TaskID, ref.State, ref.Title))
			}
		}
	}
	parts = append(parts, "Deterministic fallback summary:")
	parts = append(parts, fallback)
	return strings.Join(parts, "\n")
}

func (s *Service) writeIntentObservation(sessionID, messageID, message string, decision IntentDecision) error {
	if s == nil || s.runtimeStore == nil {
		return nil
	}
	path := filepath.Join(s.runtimeStore.RootDir(), "observations", "os", messageID+"-intent.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"type":        "chat-intent",
		"session_id":  sessionID,
		"message_id":  messageID,
		"message":     message,
		"intent_kind": decision.Kind,
		"reason":      decision.Reason,
		"confidence":  decision.Confidence,
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func (s *Service) composeDirectReply(userText string, contextSnapshot *Context, conversationContext string, message Message) Message {
	fallback := fallbackDirectReply(userText)
	if s == nil || s.textModel == nil {
		message.Content = normalizeAssistantText(fallback)
		message.Stage = "responded"
		_ = s.store.Upsert(message.SessionID, message)
		return message
	}

	req := model.TextRequest{
		SystemPrompt: "You are the conversational surface of MnemosyneOS. For ordinary chat, respond naturally and briefly. Do not create or mention tasks unless the user clearly asked for work on the system. If the user is greeting you, greet them back and offer help. Output plain text only. Do not use Markdown syntax.",
		UserPrompt:   s.directReplyPrompt(userText, contextSnapshot, conversationContext, fallback),
		MaxTokens:    directReplyMaxTokens(userText),
		Temperature:  0.3,
		Profile:      model.ProfileConversation,
	}
	return s.streamConversationReply(message, req, fallback)
}

func (s *Service) directReplyPrompt(userText string, contextSnapshot *Context, conversationContext, fallback string) string {
	parts := []string{
		"User message:",
		userText,
	}
	if strings.TrimSpace(conversationContext) != "" {
		parts = append(parts, "Recent conversation context:", conversationContext)
	}
	parts = append(parts,
		"Fallback reply:",
		fallback,
	)
	if contextSnapshot != nil && len(contextSnapshot.RecentTasks) > 0 {
		parts = append(parts, "Recent runtime context is available, but only mention it if directly relevant.")
	}
	return strings.Join(parts, "\n")
}

func composeAssistantReply(task airuntime.Task, runResult *skills.RunResult, contextSnapshot *Context) string {
	lines := []string{
		fmt.Sprintf("Task %s started with skill %s.", task.TaskID, firstNonEmpty(task.SelectedSkill, "task-plan")),
		fmt.Sprintf("Current state: %s.", task.State),
	}
	if strings.TrimSpace(task.NextAction) != "" {
		lines = append(lines, "Next: "+task.NextAction+".")
	}
	if strings.TrimSpace(task.FailureReason) != "" {
		lines = append(lines, "Failure: "+task.FailureReason+".")
	}
	if runResult != nil {
		if len(runResult.ArtifactPaths) > 0 {
			lines = append(lines, fmt.Sprintf("Artifacts: %d.", len(runResult.ArtifactPaths)))
		}
		if len(runResult.ObservationPaths) > 0 {
			lines = append(lines, fmt.Sprintf("Observations: %d.", len(runResult.ObservationPaths)))
		}
	}
	switch task.State {
	case airuntime.TaskStateDone:
		lines = append(lines, "Work completed.")
	case airuntime.TaskStateAwaitingApproval:
		lines = append(lines, "Work is waiting for approval before execution.")
	case airuntime.TaskStateBlocked:
		lines = append(lines, "Work is blocked and needs intervention.")
	case airuntime.TaskStateFailed:
		lines = append(lines, "Work failed and needs review.")
	default:
		lines = append(lines, "Work is queued in the runtime.")
	}
	if contextSnapshot != nil {
		if len(contextSnapshot.RecallHits) > 0 {
			labels := make([]string, 0, len(contextSnapshot.RecallHits))
			for _, hit := range contextSnapshot.RecallHits {
				labels = append(labels, fmt.Sprintf("%s:%s", hit.Source, hit.CardID))
			}
			lines = append(lines, "Relevant memory: "+strings.Join(labels, ", ")+".")
		}
		if len(contextSnapshot.RecentTasks) > 0 {
			labels := make([]string, 0, len(contextSnapshot.RecentTasks))
			for _, task := range contextSnapshot.RecentTasks {
				labels = append(labels, task.TaskID)
			}
			lines = append(lines, "Recent tasks: "+strings.Join(labels, ", ")+".")
		}
	}
	return strings.Join(lines, "\n")
}

func (s *Service) hydrateMessage(message Message) Message {
	if strings.TrimSpace(message.TaskID) == "" || s.runtimeStore == nil {
		return message
	}
	task, err := s.runtimeStore.GetTask(message.TaskID)
	if err != nil {
		return message
	}
	message.TaskState = task.State
	message.SelectedSkill = task.SelectedSkill
	message.ExecutionProfile = task.ExecutionProfile
	message.Links = buildLinks(task, nil)
	message.Actions = buildActions(task)
	if strings.EqualFold(message.Role, "assistant") && strings.TrimSpace(message.Content) == "" {
		message.Content = composeAssistantReply(task, nil, message.Context)
	}
	return message
}

func (s *Service) buildTaskContextSnapshot(query string, state SessionState) *Context {
	ctx := &Context{
		RecentTasks: s.recentTasks(3),
		RecallHits:  s.recallHits(query, 4),
	}
	if fast := s.buildFastContextSnapshot(state); fast != nil {
		if len(ctx.RecentTasks) == 0 {
			ctx.RecentTasks = fast.RecentTasks
		}
		if len(ctx.RecallHits) == 0 {
			ctx.RecallHits = fast.RecallHits
		}
	}
	if len(ctx.RecentTasks) == 0 && len(ctx.RecallHits) == 0 {
		return nil
	}
	return ctx
}

func (s *Service) buildFastContextSnapshot(state SessionState) *Context {
	ctx := &Context{}
	if strings.TrimSpace(state.FocusTaskID) != "" && s.runtimeStore != nil {
		if task, err := s.runtimeStore.GetTask(state.FocusTaskID); err == nil {
			ctx.RecentTasks = append(ctx.RecentTasks, TaskRef{
				TaskID:        task.TaskID,
				Title:         task.Title,
				State:         task.State,
				SelectedSkill: task.SelectedSkill,
			})
		}
	}
	for i, cardID := range state.WorkingSet.RecallCardIDs {
		if len(ctx.RecallHits) >= 3 {
			break
		}
		source := ""
		if i < len(state.WorkingSet.SourceRefs) {
			source = state.WorkingSet.SourceRefs[i]
		}
		ctx.RecallHits = append(ctx.RecallHits, RecallRef{
			Source: source,
			CardID: cardID,
		})
	}
	if len(ctx.RecentTasks) == 0 && len(ctx.RecallHits) == 0 {
		return nil
	}
	return ctx
}

func (s *Service) loadSessionState(sessionID string) SessionState {
	if s == nil || s.store == nil {
		return SessionState{SessionID: sessionID}
	}
	state, err := s.store.GetSessionState(sessionID)
	if err != nil {
		return SessionState{SessionID: sessionID}
	}
	return state
}

func (s *Service) recentConversationContext(sessionID string, limit int) string {
	if s == nil || s.store == nil || limit <= 0 {
		return ""
	}
	messages, err := s.store.List(sessionID, limit)
	if err != nil || len(messages) == 0 {
		return ""
	}
	var parts []string
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", msg.Role, previewString(content, 240)))
	}
	return strings.Join(parts, "\n")
}

func expandQueryWithConversation(message, conversationContext string) string {
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 10 || strings.TrimSpace(conversationContext) == "" {
		return message
	}
	return strings.TrimSpace(message + "\n" + conversationContext)
}

func previewString(input string, max int) string {
	if len(input) <= max {
		return input
	}
	if max <= 3 {
		return input[:max]
	}
	return input[:max-3] + "..."
}

func normalizeAssistantText(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	replacements := []struct{ old, new string }{
		{"```", ""},
		{"**", ""},
		{"__", ""},
		{"`", ""},
		{"### ", ""},
		{"## ", ""},
		{"# ", ""},
	}
	for _, item := range replacements {
		input = strings.ReplaceAll(input, item.old, item.new)
	}
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "* "):
			lines[i] = strings.Replace(line, "* ", "• ", 1)
		case strings.HasPrefix(trimmed, "- "):
			lines[i] = strings.Replace(line, "- ", "• ", 1)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (s *Service) tryHandleSessionFollowup(sessionID, userText string, dialogue DialogueDecision, state SessionState, contextSnapshot *Context, executionProfile string) (func(Message) Message, bool) {
	if !isFollowupDialogueAct(dialogue.Act) {
		return nil, false
	}
	if strings.TrimSpace(state.FocusTaskID) == "" || strings.TrimSpace(state.PendingAction) == "" || s.runtimeStore == nil {
		return nil, false
	}
	task, err := s.runtimeStore.GetTask(state.FocusTaskID)
	if err != nil {
		return nil, false
	}
	switch state.PendingAction {
	case "summarize_focus_task":
		return func(message Message) Message {
			return s.composeFollowupArtifactReply(userText, task, state, contextSnapshot, message)
		}, true
	default:
		return nil, false
	}
}

func isFollowupDialogueAct(act string) bool {
	switch act {
	case DialogueActConfirm, DialogueActAnswer, DialogueActContinueTask:
		return true
	default:
		return false
	}
}

func (s *Service) composeFollowupArtifactReply(userText string, task airuntime.Task, state SessionState, contextSnapshot *Context, message Message) Message {
	excerpts := s.focusArtifactExcerpts(task, state, 2, 1600)
	fallback := composeFollowupFallback(task, excerpts)
	if s == nil || s.textModel == nil {
		message.Content = normalizeAssistantText(fallback)
		message.Stage = "responded"
		_ = s.store.Upsert(message.SessionID, message)
		return message
	}
	req := model.TextRequest{
		SystemPrompt: "You are the follow-up layer of MnemosyneOS. The user is continuing a prior thread. Answer directly from the prior task artifacts and do not claim to perform a new search unless one actually happened. Output plain text only. Do not use Markdown syntax like headings, bold markers, code fences, or bullet stars.",
		UserPrompt: strings.Join([]string{
			"User follow-up:",
			userText,
			"Focused task:",
			fmt.Sprintf("Task ID: %s", task.TaskID),
			fmt.Sprintf("Title: %s", task.Title),
			fmt.Sprintf("Skill: %s", task.SelectedSkill),
			"Artifact excerpts:",
			strings.Join(excerpts, "\n\n"),
			"Fallback answer:",
			fallback,
		}, "\n"),
		MaxTokens:   followupMaxTokens(userText),
		Temperature: 0.2,
		Profile:     model.ProfileConversation,
	}
	return s.streamConversationReply(message, req, fallback)
}

func (s *Service) streamConversationReply(message Message, req model.TextRequest, fallback string) Message {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var builder strings.Builder
	lastFlush := time.Now()
	resp, err := s.textModel.StreamText(ctx, req, func(delta model.TextDelta) error {
		builder.WriteString(delta.Text)
		current := normalizeAssistantText(strings.TrimSpace(builder.String()))
		if current == "" {
			return nil
		}
		if time.Since(lastFlush) < 120*time.Millisecond && len(current) < 120 {
			return nil
		}
		lastFlush = time.Now()
		message.Content = current
		message.Stage = "running"
		return s.store.Upsert(message.SessionID, message)
	})
	if err == nil {
		text := normalizeAssistantText(strings.TrimSpace(resp.Text))
		if text == "" {
			text = normalizeAssistantText(strings.TrimSpace(builder.String()))
		}
		if text != "" {
			message.Content = text
			message.Stage = "responded"
			_ = s.store.Upsert(message.SessionID, message)
			return message
		}
	}

	if err == nil {
		resp, err = s.textModel.GenerateText(ctx, req)
	}
	if err == nil && strings.TrimSpace(resp.Text) != "" {
		message.Content = normalizeAssistantText(strings.TrimSpace(resp.Text))
		message.Stage = "responded"
		_ = s.store.Upsert(message.SessionID, message)
		return message
	}

	message.Content = normalizeAssistantText(fallback)
	message.Stage = "responded"
	_ = s.store.Upsert(message.SessionID, message)
	return message
}

func (s *Service) focusArtifactExcerpts(task airuntime.Task, state SessionState, limit, maxChars int) []string {
	paths := make([]string, 0, len(state.WorkingSet.ArtifactPaths))
	paths = append(paths, state.WorkingSet.ArtifactPaths...)
	for key, value := range task.Metadata {
		if strings.HasSuffix(key, "_artifact") && strings.TrimSpace(value) != "" {
			paths = append(paths, value)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	seen := map[string]struct{}{}
	out := make([]string, 0, limit)
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(raw))
		if content == "" {
			continue
		}
		if len(content) > maxChars {
			content = content[:maxChars] + "..."
		}
		out = append(out, fmt.Sprintf("%s:\n%s", filepath.Base(path), content))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func composeFollowupFallback(task airuntime.Task, excerpts []string) string {
	if len(excerpts) == 0 {
		return "我可以继续，但当前会话里没有可复用的任务产出。你可以明确告诉我下一步要我做什么。"
	}
	if task.SelectedSkill == SkillWebSearch {
		return "基于刚才搜索到的资料，OpenClaw 的 memory 设计核心是把记忆沉淀为可持久化内容，再通过检索层按相关性和时间性召回。当前结果显示它主要依赖文件化记忆载体，并在检索上叠加 BM25、向量或混合检索，再配合近期性权重。"
	}
	return "我可以继续基于上一轮任务结果展开，当前会话已经保留了相关 artifact，可继续总结或细化。"
}

func directReplyMaxTokens(userText string) int {
	text := strings.ToLower(strings.TrimSpace(userText))
	switch {
	case containsAny(text,
		"详细", "完整", "展开", "深入", "具体", "架构", "说明",
		"detail", "detailed", "complete", "architecture", "explain", "deep dive"):
		return 1600
	case containsAny(text,
		"总结", "概述", "对比", "compare", "summary", "summarize", "overview"):
		return 900
	default:
		return 240
	}
}

func followupMaxTokens(userText string) int {
	text := strings.ToLower(strings.TrimSpace(userText))
	switch {
	case containsAny(text,
		"详细", "完整", "展开", "继续", "深入", "具体", "架构", "说明",
		"detail", "detailed", "complete", "continue", "architecture", "explain", "deep dive"):
		return 2200
	case containsAny(text,
		"总结", "概述", "对比", "compare", "summary", "summarize", "overview"):
		return 1200
	default:
		return 420
	}
}

func containsAny(input string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(input, needle) {
			return true
		}
	}
	return false
}

func buildTaskResultEnvelope(task airuntime.Task, runResult *skills.RunResult) TaskResultEnvelope {
	envelope := TaskResultEnvelope{
		Outcome:       task.State,
		Headline:      stageMessage(task, finalStage(task)),
		NextAction:    strings.TrimSpace(task.NextAction),
		FailureReason: strings.TrimSpace(task.FailureReason),
	}
	if runResult != nil {
		envelope.ArtifactPaths = append(envelope.ArtifactPaths, runResult.ArtifactPaths...)
		envelope.ObservationPaths = append(envelope.ObservationPaths, runResult.ObservationPaths...)
	}
	switch task.State {
	case airuntime.TaskStateDone:
		if len(envelope.ArtifactPaths) > 0 {
			envelope.Headline = fmt.Sprintf("Completed with %d artifact(s) ready for review.", len(envelope.ArtifactPaths))
		} else {
			envelope.Headline = "Completed and persisted to the runtime."
		}
	case airuntime.TaskStateAwaitingApproval:
		envelope.Headline = "Waiting for approval before execution can continue."
	case airuntime.TaskStateBlocked:
		envelope.Headline = "Blocked and waiting for intervention."
	case airuntime.TaskStateFailed:
		envelope.Headline = "Failed during execution and needs review."
	}
	return envelope
}

func updateStateForTaskStart(state SessionState, task airuntime.Task, ctx *Context, dialogue DialogueDecision) SessionState {
	state.SessionID = firstNonEmpty(state.SessionID, task.Metadata["chat_session_id"])
	state.Topic = firstNonEmpty(task.Title, state.Topic)
	state.FocusTaskID = task.TaskID
	state.LastUserAct = dialogue.Act
	state.LastAssistantAct = "task_started"
	state.PendingQuestion = ""
	state.PendingAction = ""
	if ctx != nil {
		state.WorkingSet = workingSetFromContext(ctx)
	}
	return state
}

func (s *Service) finalizeSessionState(sessionID string, task airuntime.Task, runResult *skills.RunResult, assistantContent, userAct string, ctx *Context) {
	if s == nil || s.store == nil {
		return
	}
	state := s.loadSessionState(sessionID)
	state.SessionID = sessionID
	state.Topic = firstNonEmpty(task.Title, state.Topic)
	state.FocusTaskID = task.TaskID
	state.LastUserAct = firstNonEmpty(userAct, state.LastUserAct)
	state.LastAssistantAct = finalStage(task)
	state.WorkingSet = mergeWorkingSet(state.WorkingSet, workingSetFromRunResult(runResult, ctx))
	state.PendingQuestion = extractPendingQuestion(assistantContent)
	state.PendingAction = inferPendingAction(task, state.PendingQuestion)
	_ = s.store.SaveSessionState(state)
}

func workingSetFromContext(ctx *Context) SessionWorkset {
	if ctx == nil {
		return SessionWorkset{}
	}
	out := SessionWorkset{}
	for _, hit := range ctx.RecallHits {
		out.RecallCardIDs = append(out.RecallCardIDs, hit.CardID)
		out.SourceRefs = append(out.SourceRefs, hit.Source)
	}
	return out
}

func workingSetFromRunResult(runResult *skills.RunResult, ctx *Context) SessionWorkset {
	out := workingSetFromContext(ctx)
	if runResult != nil {
		out.ArtifactPaths = append(out.ArtifactPaths, runResult.ArtifactPaths...)
	}
	return dedupeWorkingSet(out)
}

func mergeWorkingSet(left, right SessionWorkset) SessionWorkset {
	out := SessionWorkset{}
	out.ArtifactPaths = append(out.ArtifactPaths, left.ArtifactPaths...)
	out.ArtifactPaths = append(out.ArtifactPaths, right.ArtifactPaths...)
	out.RecallCardIDs = append(out.RecallCardIDs, left.RecallCardIDs...)
	out.RecallCardIDs = append(out.RecallCardIDs, right.RecallCardIDs...)
	out.SourceRefs = append(out.SourceRefs, left.SourceRefs...)
	out.SourceRefs = append(out.SourceRefs, right.SourceRefs...)
	return dedupeWorkingSet(out)
}

func dedupeWorkingSet(in SessionWorkset) SessionWorkset {
	in.ArtifactPaths = dedupeStrings(in.ArtifactPaths)
	in.RecallCardIDs = dedupeStrings(in.RecallCardIDs)
	in.SourceRefs = dedupeStrings(in.SourceRefs)
	return in
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func extractPendingQuestion(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.Contains(line, "?") || strings.Contains(line, "？") {
			return line
		}
	}
	return ""
}

func inferPendingAction(task airuntime.Task, pendingQuestion string) string {
	if pendingQuestion == "" {
		return ""
	}
	switch task.SelectedSkill {
	case SkillWebSearch, SkillEmailInbox:
		return "summarize_focus_task"
	default:
		return ""
	}
}

func (s *Service) recentTasks(limit int) []TaskRef {
	if s.runtimeStore == nil || limit <= 0 {
		return nil
	}
	tasks, err := s.runtimeStore.ListTasks()
	if err != nil {
		return nil
	}
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	refs := make([]TaskRef, 0, len(tasks))
	for _, task := range tasks {
		refs = append(refs, TaskRef{
			TaskID:        task.TaskID,
			Title:         task.Title,
			State:         task.State,
			SelectedSkill: task.SelectedSkill,
		})
	}
	return refs
}

func (s *Service) recallHits(query string, limit int) []RecallRef {
	if s.recall == nil || strings.TrimSpace(query) == "" || limit <= 0 {
		return nil
	}
	resp := s.recall.Recall(recall.Request{
		Query: query,
		Limit: limit,
	})
	if len(resp.Hits) == 0 {
		return nil
	}
	refs := make([]RecallRef, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		refs = append(refs, RecallRef{
			Source:   hit.Source,
			CardID:   hit.CardID,
			CardType: hit.CardType,
			Snippet:  hit.Snippet,
		})
	}
	return refs
}

func buildLinks(task airuntime.Task, runResult *skills.RunResult) []Link {
	links := []Link{
		{Label: "Task", Href: "/ui/tasks?task_id=" + task.TaskID},
	}
	if approvalID := strings.TrimSpace(task.Metadata["root_approval_id"]); approvalID != "" {
		links = append(links, Link{
			Label: "Approval",
			Href:  "/ui/approvals?approval_id=" + approvalID,
		})
	}

	artifactPaths := make([]string, 0)
	if runResult != nil {
		artifactPaths = append(artifactPaths, runResult.ArtifactPaths...)
	}
	for key, value := range task.Metadata {
		if strings.HasSuffix(key, "_artifact") && strings.TrimSpace(value) != "" {
			artifactPaths = append(artifactPaths, value)
		}
	}
	if len(artifactPaths) > 0 {
		seen := make(map[string]struct{})
		sort.Strings(artifactPaths)
		for _, path := range artifactPaths {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			links = append(links, Link{
				Label: "Artifact: " + filepath.Base(path),
				Href:  "/ui/artifacts/view?path=" + url.QueryEscape(path),
			})
		}
	}
	return links
}

func summarizeRunResult(runResult *skills.RunResult, artifactLimit, maxChars int) []string {
	if runResult == nil || len(runResult.ArtifactPaths) == 0 {
		return nil
	}
	limit := artifactLimit
	if limit <= 0 || limit > len(runResult.ArtifactPaths) {
		limit = len(runResult.ArtifactPaths)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		path := strings.TrimSpace(runResult.ArtifactPaths[i])
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(raw))
		if content == "" {
			continue
		}
		if len(content) > maxChars {
			content = content[:maxChars] + "..."
		}
		parts = append(parts, fmt.Sprintf("Artifact %d (%s):\n%s", i+1, filepath.Base(path), content))
	}
	return parts
}

func buildActions(task airuntime.Task) []Action {
	actions := make([]Action, 0)
	switch task.State {
	case airuntime.TaskStateAwaitingApproval:
		if approvalID := strings.TrimSpace(task.Metadata["root_approval_id"]); approvalID != "" {
			actions = append(actions, Action{
				Label:  "Approve and Continue",
				Href:   "/ui/chat/approvals/" + approvalID + "/approve-run",
				Method: "post",
			})
		} else {
			actions = append(actions, Action{
				Label:  "Approve and Continue",
				Href:   "/ui/chat/tasks/" + task.TaskID + "/approve-run",
				Method: "post",
			})
		}
	case airuntime.TaskStatePlanned, airuntime.TaskStateBlocked:
		actions = append(actions, Action{
			Label:  "Run Task",
			Href:   "/ui/chat/tasks/" + task.TaskID + "/run",
			Method: "post",
		})
	}
	return actions
}

func applyChatContextMetadata(req *airuntime.CreateTaskRequest, ctx *Context) {
	if req == nil || ctx == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	if len(ctx.RecentTasks) > 0 {
		ids := make([]string, 0, len(ctx.RecentTasks))
		for _, task := range ctx.RecentTasks {
			ids = append(ids, task.TaskID)
		}
		req.Metadata["chat_recent_tasks"] = strings.Join(ids, ",")
	}
	if len(ctx.RecallHits) > 0 {
		ids := make([]string, 0, len(ctx.RecallHits))
		for _, hit := range ctx.RecallHits {
			ids = append(ids, hit.CardID)
		}
		req.Metadata["chat_recall_cards"] = strings.Join(ids, ",")
	}
}

func summarizeTitle(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "Chat task"
	}
	message = strings.ReplaceAll(message, "\n", " ")
	if len(message) > 72 {
		return strings.TrimSpace(message[:72]) + "..."
	}
	return message
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stageMessage(task airuntime.Task, stage string) string {
	skill := strings.TrimSpace(task.SelectedSkill)
	switch stage {
	case "routing":
		return "Routing the request and selecting the runtime path..."
	case "queued":
		switch skill {
		case "web-search":
			return "Queued. Preparing search query and recall context."
		case "github-issue-search":
			return "Queued. Preparing GitHub issue search."
		case "email-inbox":
			return "Queued. Preparing inbox scan."
		case "file-read":
			return "Queued. Preparing file read."
		case "file-edit":
			return "Queued. Preparing file update."
		case "shell-command":
			return "Queued. Preparing command execution."
		case "memory-consolidate":
			return "Queued. Preparing memory consolidation."
		default:
			return "Queued. Classifying and preparing the request."
		}
	case "searching":
		return "Searching the web and collecting sources..."
	case "planning":
		return "Planning the task and drafting the next steps..."
	case "reading":
		return "Reading file content and extracting the relevant parts..."
	case "writing":
		return "Writing file changes and persisting the update..."
	case "executing":
		return "Running command and waiting for execution output..."
	case "triaging_email":
		return "Reading inbox messages and grouping threads..."
	case "searching_github":
		return "Searching GitHub issues and collecting matches..."
	case "consolidating":
		return "Consolidating memory and building the summary..."
	case "summarizing":
		return "Summarizing the result for the conversation..."
	case "running":
		switch skill {
		case "web-search":
			return "Searching the web and collecting sources..."
		case "github-issue-search":
			return "Searching GitHub issues and collecting matches..."
		case "email-inbox":
			return "Reading inbox messages and grouping threads..."
		case "file-read":
			return "Reading file content..."
		case "file-edit":
			return "Writing file changes..."
		case "shell-command":
			return "Running command and waiting for output..."
		case "memory-consolidate":
			return "Consolidating memory and building summary..."
		case "task-plan":
			return "Planning the task and drafting the next steps..."
		}
		return "Running task..."
	default:
		return "Working on it..."
	}
}

func activeTaskStage(task airuntime.Task) string {
	switch strings.TrimSpace(task.SelectedSkill) {
	case "web-search":
		return "searching"
	case "task-plan":
		return "planning"
	case "file-read":
		return "reading"
	case "file-edit":
		return "writing"
	case "shell-command":
		return "executing"
	case "email-inbox":
		return "triaging_email"
	case "github-issue-search":
		return "searching_github"
	case "memory-consolidate":
		return "consolidating"
	default:
		return "running"
	}
}

func normalizeProgressStage(task airuntime.Task, stage string) string {
	stage = strings.TrimSpace(stage)
	switch stage {
	case "planning.generate":
		return "planning"
	case "planning.persist", "persist.artifacts":
		return "persisting"
	case "connector.request":
		switch strings.TrimSpace(task.SelectedSkill) {
		case "web-search":
			return "searching"
		case "github-issue-search":
			return "searching_github"
		case "email-inbox":
			return "triaging_email"
		default:
			return "running"
		}
	case "persist.memory":
		return "writing_memory"
	case "execution.dispatch":
		return activeTaskStage(task)
	case "execution.persist":
		return "persisting"
	case "consolidate.generate":
		return "consolidating"
	default:
		if stage != "" {
			return stage
		}
		return activeTaskStage(task)
	}
}

func finalStage(task airuntime.Task) string {
	switch task.State {
	case airuntime.TaskStateDone:
		return "done"
	case airuntime.TaskStateAwaitingApproval:
		return "awaiting_approval"
	case airuntime.TaskStateBlocked:
		return "blocked"
	case airuntime.TaskStateFailed:
		return "failed"
	default:
		return "responded"
	}
}

func fallbackDirectReply(userText string) string {
	text := strings.TrimSpace(strings.ToLower(userText))
	switch {
	case text == "hi" || text == "hello" || text == "hey" || text == "你好" || text == "您好" || text == "嗨":
		return "你好。我在这里。你可以直接聊天，也可以明确告诉我需要我去搜索、检查、规划或执行什么。"
	case strings.Contains(text, "thank") || strings.Contains(text, "谢谢"):
		return "收到。需要继续的话，直接说下一步。"
	default:
		return "我在。普通对话我会直接回复；只有在你明确提出操作请求时，我才会创建任务。你可以继续说。"
	}
}
