package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/memoryorchestrator"
	"mnemosyneos/internal/model"
	"mnemosyneos/internal/recall"
	"mnemosyneos/internal/skills"
)

// SystemInfo provides basic facts about the running MnemosyneOS instance
// so that the conversation model can answer questions about the system.
type SystemInfo struct {
	RuntimeRoot   string
	WorkspaceRoot string
	Version       string
	Addr          string
}

type Service struct {
	store         *Store
	orchestrator  *airuntime.Orchestrator
	runtimeStore  *airuntime.Store
	memoryStore   *memory.Store
	recall        *recall.Service
	skillRunner   *skills.Runner
	textModel     model.TextGateway
	systemInfo    SystemInfo
	routeAgent    *RouteAgent
	dialogueAgent *DialogueAgent
	intentAgent   *IntentAgent
	skillAgent    *SkillAgent
	agentLoop     *AgentLoop
}

func NewService(store *Store, orchestrator *airuntime.Orchestrator, runtimeStore *airuntime.Store, recallService *recall.Service, skillRunner *skills.Runner, textModel model.TextGateway, memStore *memory.Store) *Service {
	return NewServiceWithSystemInfo(store, orchestrator, runtimeStore, recallService, skillRunner, textModel, memStore, SystemInfo{})
}

func NewServiceWithSystemInfo(store *Store, orchestrator *airuntime.Orchestrator, runtimeStore *airuntime.Store, recallService *recall.Service, skillRunner *skills.Runner, textModel model.TextGateway, memStore *memory.Store, info SystemInfo) *Service {
	return &Service{
		store:         store,
		orchestrator:  orchestrator,
		runtimeStore:  runtimeStore,
		memoryStore:   memStore,
		recall:        recallService,
		skillRunner:   skillRunner,
		textModel:     textModel,
		systemInfo:    info,
		routeAgent:    NewRouteAgent(textModel),
		dialogueAgent: NewDialogueAgent(textModel),
		intentAgent:   NewIntentAgent(textModel),
		skillAgent:    NewSkillAgent(textModel),
	}
}

// SetAgentLoop injects the agent loop for direct conversational replies.
// Task-like requests still go through the runtime pipeline.
func (s *Service) SetAgentLoop(loop *AgentLoop) {
	if s != nil {
		s.agentLoop = loop
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
	if s == nil || s.store == nil {
		return SendResponse{}, fmt.Errorf("chat service is not configured")
	}
	text := strings.TrimSpace(req.Message)
	if text == "" {
		return SendResponse{}, fmt.Errorf("message is required")
	}
	turnLocale := detectTurnLocale(text)

	if s.orchestrator == nil || s.skillRunner == nil {
		return SendResponse{}, fmt.Errorf("chat service is not configured")
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

	if followupMessage, handled := s.tryHandleSessionFollowup(sessionID, text, dialogue, sessionState, fastContext, req.ExecutionProfile, turnLocale); handled {
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
		s.recordMemoryUse(fastContext, "followup_reply", "responded")
		s.recordChatEvent(sessionID, text, assistantMessage.Content, IntentKindDirect, "")
		_ = s.store.SaveSessionState(sessionState)
		return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, nil
	}

	if followupMessage, handled := s.tryHandleFocusedTaskContinuation(sessionID, text, route, sessionState, fastContext, turnLocale); handled {
		assistantMessage := Message{
			MessageID:        fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano()),
			SessionID:        sessionID,
			Role:             "assistant",
			Content:          "",
			DialogueAct:      dialogue.Act,
			IntentKind:       IntentKindDirect,
			IntentReason:     "continued focused task context",
			IntentConfidence: route.Confidence,
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
		sessionState.LastAssistantAct = "focused_task_followup"
		s.recordMemoryUse(fastContext, "focused_task_followup", "responded")
		s.recordChatEvent(sessionID, text, assistantMessage.Content, IntentKindDirect, "")
		_ = s.store.SaveSessionState(sessionState)
		return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, nil
	}

	if reply, ok := s.tryHandleIntentConfirmation(sessionID, text, req, sessionState, userMessage, dialogue, fastContext, turnLocale); ok {
		return reply, nil
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
		if s.agentLoop != nil && s.textModel != nil {
			assistantMessage = s.composeAgentDirectReply(sessionID, text, fastContext, conversationContext, assistantMessage, turnLocale)
		} else {
			assistantMessage = s.composeDirectReply(text, fastContext, conversationContext, assistantMessage, turnLocale)
		}
		sessionState.WorkingSet.FocusPaths = mergeFocusPaths(sessionState.WorkingSet.FocusPaths, extractChatFilesystemPaths(assistantMessage.Content), 24)
		sessionState.LastUserAct = dialogue.Act
		sessionState.LastAssistantAct = "direct_reply"
		if pq := extractPendingQuestion(assistantMessage.Content); pq != "" {
			sessionState.PendingQuestion = pq
			sessionState.FocusTaskID = ""
			sessionState.PendingAction = ""
		}
		s.recordMemoryUse(fastContext, "direct_reply", "responded")
		s.recordChatEvent(sessionID, text, assistantMessage.Content, intent.Kind, "")
		_ = s.store.SaveSessionState(sessionState)
		return SendResponse{
			UserMessage:      userMessage,
			AssistantMessage: assistantMessage,
		}, nil
	}

	if shouldConfirmTaskIntent(route, text, req.Source, dialogue.Act, req.ExecutionProfile) {
		question := buildTaskConfirmationMessage(turnLocale, text, route, firstNonEmpty(req.ExecutionProfile, "user"))
		assistantMessage := Message{
			MessageID:        fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano()),
			SessionID:        sessionID,
			Role:             "assistant",
			Content:          normalizeAssistantText(question),
			DialogueAct:      DialogueActClarify,
			IntentKind:       IntentKindDirect,
			IntentReason:     "task intent needs confirmation",
			IntentConfidence: route.Confidence,
			Stage:            "awaiting_confirmation",
			ExecutionProfile: firstNonEmpty(req.ExecutionProfile, "user"),
			Context:          fastContext,
			CreatedAt:        time.Now().UTC(),
		}
		if err := s.store.Append(sessionID, assistantMessage); err != nil {
			return SendResponse{}, err
		}
		sessionState.PendingQuestion = question
		sessionState.PendingAction = "confirm_task_intent"
		if sessionState.WorkingSet.SourceRefs == nil {
			sessionState.WorkingSet.SourceRefs = []string{}
		}
		sessionState.WorkingSet.SourceRefs = append(sessionState.WorkingSet.SourceRefs,
			"confirm_goal:"+text,
			"confirm_skill:"+route.Skill,
			"confirm_scope:"+route.TargetScope,
			"confirm_profile:"+firstNonEmpty(req.ExecutionProfile, "user"),
			"confirm_requested_by:"+firstNonEmpty(req.RequestedBy, "web-chat"),
			"confirm_source:"+firstNonEmpty(req.Source, "web-chat"),
		)
		sessionState.LastUserAct = dialogue.Act
		sessionState.LastAssistantAct = "clarify"
		_ = s.store.SaveSessionState(sessionState)
		return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, nil
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
	contextSnapshot := s.buildTaskContextSnapshot(effectiveQuery, sessionState, createReq.SelectedSkill)
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
			Content:   normalizeAssistantText(localizedText(turnLocale, "我没能为这个请求创建任务："+err.Error(), "I could not create a task for that request: "+err.Error())),
			CreatedAt: time.Now().UTC(),
		}
		_ = s.store.Append(sessionID, assistantMessage)
		return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, nil
	}

	assistantMessage := Message{
		MessageID:        fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano()),
		SessionID:        sessionID,
		Role:             "assistant",
		Content:          stageMessage(task, "queued", turnLocale),
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
		assistantMessage.Content = stageMessage(task, assistantMessage.Stage, turnLocale)
		_ = s.store.Upsert(sessionID, assistantMessage)
		result, runErr := s.skillRunner.RunTaskWithProgress(task.TaskID, func(event skills.ProgressEvent) {
			assistantMessage.Stage = normalizeProgressStage(task, event.Stage)
			assistantMessage.Content = firstNonEmpty(strings.TrimSpace(event.Message), stageMessage(task, assistantMessage.Stage, turnLocale))
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
	assistantMessage.Content = stageMessage(task, assistantMessage.Stage, turnLocale)
	_ = s.store.Upsert(sessionID, assistantMessage)
	assistantMessage.Content = s.composeAssistantMessage(task, runResult, contextSnapshot, intent, sessionID, assistantMessage.MessageID, assistantMessage.CreatedAt, turnLocale)
	if err := s.store.Upsert(sessionID, assistantMessage); err != nil {
		return SendResponse{}, err
	}
	s.finalizeSessionState(sessionID, task, runResult, assistantMessage.Content, dialogue.Act, contextSnapshot)
	s.recordChatEvent(sessionID, text, assistantMessage.Content, intent.Kind, task.SelectedSkill)

	return SendResponse{
		UserMessage:      userMessage,
		AssistantMessage: assistantMessage,
	}, nil
}

func agentDirectReplyUserContent(locale, userText, structuredFocus, transcript string) string {
	userText = strings.TrimSpace(userText)
	structuredFocus = strings.TrimSpace(structuredFocus)
	transcript = strings.TrimSpace(transcript)
	if structuredFocus == "" && transcript == "" {
		return userText
	}
	var blocks []string
	if structuredFocus != "" {
		blocks = append(blocks, structuredFocus)
	}
	if transcript != "" {
		blocks = append(blocks, transcript)
	}
	combined := strings.Join(blocks, "\n\n")
	if isEnglishLocale(locale) {
		return "You are given (1) optional session working-memory paths and (2) a recent transcript. Resolve phrases like \"this directory\" using BOTH — when listing files you MUST pass the correct absolute path as list_directory.path. Omitting path lists only the configured project workspace.\n\n" + combined + "\n\nCurrent user message:\n" + userText
	}
	return "下面可能包含两类信息：（1）会话工作记忆中的绝对路径；（2）近期对话摘要。用户说「这个目录」「该文件夹」等时，请同时结合二者，并在调用 list_directory 时传入正确的 path；省略 path 只会列出当前项目工作区根目录。\n\n" + combined + "\n\n当前用户消息：\n" + userText
}

var chatAbsoluteUnixPathRE = regexp.MustCompile(`(/[A-Za-z0-9_.+~-]+(?:/[A-Za-z0-9_.+~/@-]+)+/?)`)

func extractChatFilesystemPaths(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	raw := chatAbsoluteUnixPathRE.FindAllString(text, -1)
	seen := make(map[string]struct{})
	var out []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		s = strings.TrimRight(s, ".,;:!?）】〉、\"'")
		if s == "" || !filepath.IsAbs(s) {
			continue
		}
		s = filepath.Clean(s)
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func mergeFocusPaths(existing, discovered []string, maxKeep int) []string {
	if maxKeep <= 0 {
		maxKeep = 24
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || !filepath.IsAbs(p) {
			return
		}
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, p := range existing {
		add(p)
	}
	for _, p := range discovered {
		add(p)
	}
	if len(out) > maxKeep {
		out = out[len(out)-maxKeep:]
	}
	return out
}

func formatFocusPathsForAgent(paths []string, locale string) string {
	paths = dedupeStrings(paths)
	if len(paths) == 0 {
		return ""
	}
	lines := strings.Join(paths, "\n")
	if isEnglishLocale(locale) {
		return "Session working memory (absolute paths surfaced earlier in this chat; align with transcript when resolving \"this directory\"):\n" + lines
	}
	return "会话工作记忆（本会话中已出现过的绝对路径；与「这个目录」等指代对齐）：\n" + lines
}

func (s *Service) composeAgentDirectReply(sessionID, text string, fastContext *Context, conversationContext string, assistantMessage Message, locale string) Message {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	st := s.loadSessionState(sessionID)
	focusBlock := formatFocusPathsForAgent(st.WorkingSet.FocusPaths, locale)

	// Enough turns + generous per-line preview (head+tail) so paths at the end
	// of long assistant replies are not truncated away.
	agentTranscript := s.recentConversationContextWithPreview(sessionID, 16, 2500)
	userTurn := agentDirectReplyUserContent(locale, text, focusBlock, agentTranscript)

	// Build the tool-call trace as events stream in so the final assistant
	// message can surface "used: search_files → 3 matches" in CLI/Web.
	// Each tool_call appends an entry; the matching tool_result fills in the
	// preview. Ordering is guaranteed by the agent loop (call before result).
	trace := []ToolTraceEntry{}
	replyText, err := s.agentLoop.Run(ctx, s.agentSystemPrompt(locale), userTurn, locale, func(event AgentLoopEvent) {
		switch event.Kind {
		case "tool_call":
			assistantMessage.Stage = "tool_call"
			assistantMessage.Content = localizedText(locale, fmt.Sprintf("正在调用 %s ...", event.ToolName), fmt.Sprintf("Calling %s ...", event.ToolName))
			trace = append(trace, ToolTraceEntry{
				ToolName:  event.ToolName,
				Arguments: event.ToolArgs,
			})
			assistantMessage.ToolTrace = append([]ToolTraceEntry(nil), trace...)
			_ = s.store.Upsert(sessionID, assistantMessage)
		case "tool_result":
			assistantMessage.Stage = "tool_result"
			assistantMessage.Content = localizedText(locale, fmt.Sprintf("%s 完成。", event.ToolName), fmt.Sprintf("%s finished.", event.ToolName))
			if len(trace) > 0 {
				last := &trace[len(trace)-1]
				last.ResultPreview = summarizeToolResult(event.Result)
				if strings.TrimSpace(event.Error) != "" {
					last.Error = event.Error
				}
			}
			assistantMessage.ToolTrace = append([]ToolTraceEntry(nil), trace...)
			_ = s.store.Upsert(sessionID, assistantMessage)
		}
	})
	if err != nil {
		return s.composeDirectReply(text, fastContext, conversationContext, assistantMessage, locale)
	}

	assistantMessage.Content = normalizeAssistantText(strings.TrimSpace(replyText))
	if assistantMessage.Content == "" {
		assistantMessage.Content = normalizeAssistantText(fallbackDirectReply(text, locale))
	}
	assistantMessage.Stage = "responded"
	if len(trace) > 0 {
		assistantMessage.ToolTrace = append([]ToolTraceEntry(nil), trace...)
	}
	_ = s.store.Upsert(sessionID, assistantMessage)
	return assistantMessage
}

// summarizeToolResult trims a tool response for on-screen display. The full
// payload already went back to the LLM; here we only keep enough to let a
// human verify something real happened (e.g. "Found 3 matching files: ...").
func summarizeToolResult(result string) string {
	result = strings.TrimSpace(result)
	if result == "" {
		return ""
	}
	const maxLen = 280
	if firstNL := strings.Index(result, "\n"); firstNL > 0 && firstNL < maxLen {
		head := result[:firstNL]
		// Keep first line (usually a count / summary) plus up to 2 follow-up
		// lines so list-style results stay informative without dominating the
		// chat card.
		rest := result[firstNL+1:]
		if nl2 := strings.IndexAny(rest, "\n"); nl2 >= 0 {
			rest = rest[:nl2]
		}
		combined := head
		if strings.TrimSpace(rest) != "" {
			combined = head + " | " + strings.TrimSpace(rest)
		}
		if len(combined) > maxLen {
			combined = combined[:maxLen-1] + "…"
		}
		return combined
	}
	if len(result) > maxLen {
		return result[:maxLen-1] + "…"
	}
	return result
}

func (s *Service) agentSystemPrompt(locale string) string {
	if isEnglishLocale(locale) {
		return `You are MnemosyneOS, an Agent OS running on the user's computer. You can use tools to help with real tasks.

Core rules:
1. Reply directly for simple greetings and casual conversation.
2. Use get_system_info/list_directory for real system or workspace facts.
2b. The user turn may include "Session working memory" paths plus a recent transcript. When they say "this directory" or similar, resolve the path from BOTH blocks and call list_directory with that absolute path. Omitting path lists the configured project workspace only — do not use that default when the user clearly meant another directory.
3. Use web_search for web information requests.
4. Use read_file for file reading requests.
5. When the user asks to FIND or LOCATE a file or directory by name (e.g. "where is lab?", "find my notes folder"), you MUST call search_files. Never answer "not found" from memory. If the first search (workspace) returns 0 matches, call search_files again with directory="~" before giving up. Only claim "not found" after BOTH the project workspace and the home directory have been searched.
6. Do not execute file edits/commands directly in this loop; explain these requests should go through the task runtime and approval/execution plane.
7. Use list_tasks for runtime task history.
8. Use recall_memory for remembered facts/procedures/events.
9. If a tool returns "[功能未配置]", clearly pass the setup steps to the user instead of bypassing it.

Always reply in the user's current message language. Keep it concise and practical.`
	}
	prompt := `你是 MnemosyneOS，一个运行在用户电脑上的 Agent OS（智能操作系统管家）。你可以通过工具来帮助用户完成各种任务。

核心原则：
1. 对于简单的问候和闲聊，直接回复，不需要调用任何工具。
2. 当用户询问关于系统、文件、项目位置等信息时，使用 get_system_info 或 list_directory 等工具获取真实数据后再回答。
2b. 用户消息中可能同时带有「会话工作记忆」里的绝对路径和「近期对话」摘要。用户说「这个目录」等而未写路径时，必须结合这两块信息解析路径，并在 list_directory 中传入 path；省略 path 只会列出当前项目工作区根目录。
3. 当用户要求搜索信息时，使用 web_search 工具。
4. 当用户要求查看文件时，使用 read_file 工具。
5. 当用户要求"查找/定位/locate/find"某个文件或目录时（例如「帮我查找一个名叫 lab 的目录」「where is my notes folder」），必须调用 search_files，不能凭记忆回答。如果第一次在项目工作区（workspace）下返回 0 个匹配，必须再调用一次 search_files 并把 directory 设成 "~" 搜用户 home。只有在**工作区和 home 都**搜过之后，才能告诉用户"没找到"。
6. 当用户要求执行命令、修改文件或创建任务时，不要直接执行；说明这类请求会通过 MnemosyneOS task runtime 和 approval/execution plane 处理。
7. 当用户询问历史任务时，使用 list_tasks 工具。
8. 当用户询问以前记住的内容时，使用 recall_memory 工具。
9. 如果工具返回"[功能未配置]"的提示，请把配置方法直接转达给用户，不要试图绕过或忽略。用友好的语气说明哪个功能需要配置、具体步骤是什么。

回复时使用用户使用的语言。保持简洁、自然、有帮助。`

	if s.systemInfo.WorkspaceRoot != "" || s.systemInfo.RuntimeRoot != "" {
		prompt += "\n\n系统信息："
		if s.systemInfo.WorkspaceRoot != "" {
			prompt += "\n- 项目/工作区位置: " + s.systemInfo.WorkspaceRoot
		}
		if s.systemInfo.RuntimeRoot != "" {
			prompt += "\n- 运行时数据位置: " + s.systemInfo.RuntimeRoot
		}
		if s.systemInfo.Addr != "" {
			prompt += "\n- 服务器地址: " + s.systemInfo.Addr
		}
		if s.systemInfo.Version != "" {
			prompt += "\n- 版本: " + s.systemInfo.Version
		}
	}
	return prompt
}

func (s *Service) completeTaskReply(sessionID string, assistantMessage Message, task airuntime.Task, contextSnapshot *Context, intent IntentDecision) {
	locale := detectTurnLocale(task.Goal)
	assistantMessage.Stage = activeTaskStage(task)
	assistantMessage.Content = stageMessage(task, assistantMessage.Stage, locale)
	_ = s.store.Upsert(sessionID, assistantMessage)

	var runResult *skills.RunResult
	if task.State == airuntime.TaskStateActive || task.State == airuntime.TaskStatePlanned {
		result, runErr := s.skillRunner.RunTaskWithProgress(task.TaskID, func(event skills.ProgressEvent) {
			assistantMessage.Stage = normalizeProgressStage(task, event.Stage)
			assistantMessage.Content = firstNonEmpty(strings.TrimSpace(event.Message), stageMessage(task, assistantMessage.Stage, locale))
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
		assistantMessage.Content = stageMessage(task, assistantMessage.Stage, locale)
		_ = s.store.Upsert(sessionID, assistantMessage)
		assistantMessage.Stage = finalStage(task)
		assistantMessage.Content = s.composeAssistantMessage(task, runResult, contextSnapshot, intent, sessionID, assistantMessage.MessageID, assistantMessage.CreatedAt, locale)
	}
	_ = s.store.Upsert(sessionID, assistantMessage)
	s.finalizeSessionState(sessionID, task, runResult, assistantMessage.Content, assistantMessage.DialogueAct, contextSnapshot)
	s.recordChatEvent(sessionID, task.Goal, assistantMessage.Content, intent.Kind, task.SelectedSkill)
}

func (s *Service) normalizeSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "default"
	}
	return sessionID
}

func (s *Service) composeAssistantMessage(task airuntime.Task, runResult *skills.RunResult, contextSnapshot *Context, intent IntentDecision, sessionID, messageID string, createdAt time.Time, locale string) string {
	envelope := buildTaskResultEnvelope(task, runResult)
	fallback := composeAssistantReply(task, runResult, contextSnapshot, locale)
	if s == nil || s.textModel == nil {
		return normalizeAssistantText(fallback)
	}

	req := model.TextRequest{
		SystemPrompt: localizedText(locale, "你是 MnemosyneOS 的聊天结果层。请基于真实运行状态，用简洁自然的中文回复。需要时说明审批、阻塞、产物和下一步，不要杜撰。仅输出纯文本。", "You are the chat surface of MnemosyneOS. Reply like an operator-facing assistant: concise, conversational, and grounded in the actual runtime state. Mention approvals, blockers, artifacts, or next actions when they matter. Do not invent actions or results. Output plain text only."),
		UserPrompt:   s.assistantPrompt(task, envelope, contextSnapshot, fallback, locale),
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

func (s *Service) assistantPrompt(task airuntime.Task, envelope TaskResultEnvelope, contextSnapshot *Context, fallback, locale string) string {
	if !isEnglishLocale(locale) {
		parts := []string{
			"任务执行摘要：",
			fmt.Sprintf("任务 ID: %s", task.TaskID),
			fmt.Sprintf("任务标题: %s", task.Title),
			fmt.Sprintf("任务状态: %s", task.State),
			fmt.Sprintf("选择技能: %s", firstNonEmpty(task.SelectedSkill, "task-plan")),
			fmt.Sprintf("执行权限: %s", firstNonEmpty(task.ExecutionProfile, "user")),
			"User request execution summary:",
			fmt.Sprintf("Task ID: %s", task.TaskID),
			fmt.Sprintf("Task title: %s", task.Title),
			fmt.Sprintf("Task state: %s", task.State),
			fmt.Sprintf("Selected skill: %s", firstNonEmpty(task.SelectedSkill, "task-plan")),
			fmt.Sprintf("Execution profile: %s", firstNonEmpty(task.ExecutionProfile, "user")),
		}
		if strings.TrimSpace(envelope.Headline) != "" {
			parts = append(parts, "结果概览: "+envelope.Headline, "Outcome headline: "+envelope.Headline)
		}
		if strings.TrimSpace(envelope.NextAction) != "" {
			parts = append(parts, "下一步: "+envelope.NextAction, "Next action: "+envelope.NextAction)
		}
		if strings.TrimSpace(envelope.FailureReason) != "" {
			parts = append(parts, "失败原因: "+envelope.FailureReason, "Failure reason: "+envelope.FailureReason)
		}
		if len(envelope.ArtifactPaths) > 0 {
			parts = append(parts, fmt.Sprintf("Artifacts: %d", len(envelope.ArtifactPaths)))
		}
		if len(envelope.ObservationPaths) > 0 {
			parts = append(parts, fmt.Sprintf("Observations: %d", len(envelope.ObservationPaths)))
		}
		parts = append(parts, "回退摘要：", fallback, "Deterministic fallback summary:", fallback)
		return strings.Join(parts, "\n")
	}
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
		if len(contextSnapshot.WorkingNotes) > 0 {
			parts = append(parts, "Working memory:")
			for _, note := range contextSnapshot.WorkingNotes {
				parts = append(parts, "- "+note)
			}
		}
		if len(contextSnapshot.ProcedureHits) > 0 {
			parts = append(parts, "Relevant procedure guidance:")
			for _, hit := range contextSnapshot.ProcedureHits {
				parts = append(parts, fmt.Sprintf("- %s / %s", hit.CardID, hit.Snippet))
			}
		}
		if len(contextSnapshot.SemanticHits) > 0 {
			parts = append(parts, "Relevant long-term facts:")
			for _, hit := range contextSnapshot.SemanticHits {
				parts = append(parts, fmt.Sprintf("- %s / %s / %s", hit.Source, hit.CardType, hit.Snippet))
			}
		}
		if len(contextSnapshot.RecallHits) > 0 {
			parts = append(parts, "Relevant episodic fallback:")
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

func (s *Service) composeDirectReply(userText string, contextSnapshot *Context, conversationContext string, message Message, locale string) Message {
	fallback := fallbackDirectReply(userText, locale)
	if s == nil || s.textModel == nil {
		message.Content = normalizeAssistantText(fallback)
		message.Stage = "responded"
		_ = s.store.Upsert(message.SessionID, message)
		return message
	}

	req := model.TextRequest{
		SystemPrompt: s.directReplySystemPrompt(locale),
		UserPrompt:   s.directReplyPrompt(userText, contextSnapshot, conversationContext, fallback, locale),
		MaxTokens:    directReplyMaxTokens(userText),
		Temperature:  0.3,
		Profile:      model.ProfileConversation,
	}
	return s.streamConversationReply(message, req, fallback, locale)
}

func (s *Service) directReplySystemPrompt(locale string) string {
	if !isEnglishLocale(locale) {
		return `你是 MnemosyneOS，一个运行在用户电脑上的 Agent OS。

在普通对话中，请自然、简洁、友好地回答。对于系统信息问题可直接回答。除非用户明确要求执行动作，否则不要创建任务。仅输出纯文本。`
	}
	prompt := `You are MnemosyneOS, an Agent OS that runs as a background service on the user's computer. You can observe, plan, act, remember, and resume work across sessions.

For ordinary conversation, respond naturally and helpfully. Answer questions about yourself and the system directly using the system info below. Do not create tasks unless the user explicitly asks for an action. Output plain text only.`

	if s.systemInfo.WorkspaceRoot != "" || s.systemInfo.RuntimeRoot != "" {
		prompt += "\n\nSystem info:"
		if s.systemInfo.WorkspaceRoot != "" {
			prompt += "\n- Project/workspace location: " + s.systemInfo.WorkspaceRoot
		}
		if s.systemInfo.RuntimeRoot != "" {
			prompt += "\n- Runtime data location: " + s.systemInfo.RuntimeRoot
		}
		if s.systemInfo.Addr != "" {
			prompt += "\n- Server address: " + s.systemInfo.Addr
		}
		if s.systemInfo.Version != "" {
			prompt += "\n- Version: " + s.systemInfo.Version
		}
	}
	return prompt
}

func (s *Service) directReplyPrompt(userText string, contextSnapshot *Context, conversationContext, fallback, locale string) string {
	if !isEnglishLocale(locale) {
		parts := []string{
			"用户消息：",
			userText,
			"User message:",
			userText,
		}
		if strings.TrimSpace(conversationContext) != "" {
			parts = append(parts, "最近对话上下文：", conversationContext, "Recent conversation context:", conversationContext)
		}
		parts = append(parts,
			"回退回复：",
			fallback,
			"Fallback reply:",
			fallback,
		)
		return strings.Join(parts, "\n")
	}
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
	if contextSnapshot != nil {
		if len(contextSnapshot.WorkingNotes) > 0 {
			parts = append(parts, "Working memory:")
			parts = append(parts, contextSnapshot.WorkingNotes...)
		}
		if len(contextSnapshot.SemanticHits) > 0 {
			parts = append(parts, "Relevant long-term facts:")
			for _, hit := range contextSnapshot.SemanticHits {
				parts = append(parts, "- "+firstNonEmpty(strings.TrimSpace(hit.Snippet), hit.CardID))
			}
		}
		if len(contextSnapshot.ProcedureHits) > 0 {
			parts = append(parts, "Relevant procedure guidance:")
			for _, hit := range contextSnapshot.ProcedureHits {
				parts = append(parts, "- "+firstNonEmpty(strings.TrimSpace(hit.Snippet), hit.CardID))
			}
		}
		if len(contextSnapshot.RecentTasks) > 0 {
			parts = append(parts, "Recent runtime context is available, but only mention it if directly relevant.")
		}
	}
	return strings.Join(parts, "\n")
}

func composeAssistantReply(task airuntime.Task, runResult *skills.RunResult, contextSnapshot *Context, locale string) string {
	// Try to include actual artifact content so the user sees a real
	// answer instead of just metadata like "Task started... Artifacts: 1".
	if runResult != nil && len(runResult.ArtifactPaths) > 0 {
		for _, p := range runResult.ArtifactPaths {
			data, err := os.ReadFile(p)
			if err != nil || len(data) == 0 {
				continue
			}
			content := strings.TrimSpace(string(data))
			if len(content) > 2000 {
				content = content[:2000] + "\n\n(truncated)"
			}
			if content != "" {
				return content
			}
		}
	}

	lines := []string{
		localizedText(locale, fmt.Sprintf("任务 %s 已由技能 %s 执行。", task.TaskID, firstNonEmpty(task.SelectedSkill, "task-plan")), fmt.Sprintf("Task %s completed with skill %s.", task.TaskID, firstNonEmpty(task.SelectedSkill, "task-plan"))),
	}
	if strings.TrimSpace(task.NextAction) != "" {
		lines = append(lines, "Next: "+task.NextAction+".")
	}
	if strings.TrimSpace(task.FailureReason) != "" {
		lines = append(lines, "Failure: "+task.FailureReason+".")
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
		message.Content = composeAssistantReply(task, nil, message.Context, detectTurnLocale(task.Goal))
	}
	return message
}

func (s *Service) buildTaskContextSnapshot(query string, state SessionState, selectedSkill string) *Context {
	orchestrator := memoryorchestrator.New(s.recall, s.runtimeStore)
	packet := orchestrator.BuildTaskPacket(query, selectedSkill, sessionViewFromState(state), taskRefsToPacketRefs(s.recentTasks(3)))
	return contextFromPacket(packet)
}

func (s *Service) buildFastContextSnapshot(state SessionState) *Context {
	orchestrator := memoryorchestrator.New(s.recall, s.runtimeStore)
	packet := orchestrator.BuildFastPacket(sessionViewFromState(state))
	return contextFromPacket(packet)
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
	return s.recentConversationContextWithPreview(sessionID, limit, 240)
}

func (s *Service) recentConversationContextWithPreview(sessionID string, limit int, previewRunes int) string {
	if s == nil || s.store == nil || limit <= 0 {
		return ""
	}
	if previewRunes <= 0 {
		previewRunes = 240
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
		parts = append(parts, fmt.Sprintf("%s: %s", msg.Role, previewForTranscriptLine(content, previewRunes)))
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

func previewStringRunes(input string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

// previewForTranscriptLine picks a preview strategy: short lines (routing) keep
// a head-only window; long lines (agent transcript) keep head+tail so late
// paths like "/Users/.../Lab/" survive truncation.
func previewForTranscriptLine(content string, previewRunes int) string {
	if previewRunes < 512 {
		return previewStringRunes(content, previewRunes)
	}
	return previewStringRunesHeadTail(content, previewRunes)
}

func previewStringRunesHeadTail(input string, maxRunes int) string {
	if maxRunes <= 32 {
		return previewStringRunes(input, maxRunes)
	}
	rs := []rune(strings.TrimSpace(input))
	n := len(rs)
	if n <= maxRunes {
		return string(rs)
	}
	sep := []rune("\n...\n")
	avail := maxRunes - len(sep)
	if avail < 8 {
		return previewStringRunes(input, maxRunes)
	}
	head := avail / 2
	tail := avail - head
	if head < 1 {
		head = 1
		tail = avail - 1
	}
	if tail < 1 {
		tail = 1
		head = avail - 1
	}
	return string(rs[:head]) + string(sep) + string(rs[n-tail:])
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

func (s *Service) tryHandleSessionFollowup(sessionID, userText string, dialogue DialogueDecision, state SessionState, contextSnapshot *Context, executionProfile string, locale string) (func(Message) Message, bool) {
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
			return s.composeFollowupArtifactReply(userText, task, state, contextSnapshot, message, locale)
		}, true
	default:
		return nil, false
	}
}

func (s *Service) tryHandleFocusedTaskContinuation(sessionID, userText string, route RouteDecision, state SessionState, contextSnapshot *Context, locale string) (func(Message) Message, bool) {
	if strings.TrimSpace(route.TargetScope) != "focused_task" || s.runtimeStore == nil || strings.TrimSpace(state.FocusTaskID) == "" {
		return nil, false
	}
	switch route.DialogueAct {
	case DialogueActContinueTask, DialogueActAnswer, DialogueActConfirm:
	default:
		return nil, false
	}
	task, err := s.runtimeStore.GetTask(state.FocusTaskID)
	if err != nil || task.State != airuntime.TaskStateDone {
		return nil, false
	}
	switch task.SelectedSkill {
	case SkillWebSearch, SkillEmailInbox, SkillGitHubIssueSearch, SkillTaskPlan:
		return func(message Message) Message {
			return s.composeFollowupArtifactReply(userText, task, state, contextSnapshot, message, locale)
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

func (s *Service) composeFollowupArtifactReply(userText string, task airuntime.Task, state SessionState, contextSnapshot *Context, message Message, locale string) Message {
	excerpts := s.focusArtifactExcerpts(task, state, 2, 1600)
	fallback := composeFollowupFallback(task, excerpts, locale)
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
	return s.streamConversationReply(message, req, fallback, locale)
}

func (s *Service) streamConversationReply(message Message, req model.TextRequest, fallback, locale string) Message {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var builder strings.Builder
	lastFlush := time.Now()
	resp, streamErr := s.textModel.StreamText(ctx, req, func(delta model.TextDelta) error {
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
	if streamErr == nil {
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

	resp2, genErr := s.textModel.GenerateText(ctx, req)
	if genErr == nil && strings.TrimSpace(resp2.Text) != "" {
		message.Content = normalizeAssistantText(strings.TrimSpace(resp2.Text))
		message.Stage = "responded"
		_ = s.store.Upsert(message.SessionID, message)
		return message
	}
	lastErr := streamErr
	if genErr != nil {
		lastErr = genErr
	}
	if lastErr != nil {
		message.Content = normalizeAssistantText(modelReplyFailureMessage(locale, lastErr))
	} else {
		message.Content = normalizeAssistantText(modelReplyFailureMessage(locale, fmt.Errorf("model returned empty content")))
	}
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

func composeFollowupFallback(task airuntime.Task, excerpts []string, locale string) string {
	if len(excerpts) == 0 {
		return localizedText(locale, "我可以继续，但当前会话里没有可复用的任务产出。你可以明确告诉我下一步要我做什么。", "I can continue, but this session does not yet have reusable task output. Tell me exactly what to do next.")
	}
	if task.SelectedSkill == SkillWebSearch {
		return localizedText(locale, "基于刚才搜索到的资料，OpenClaw 的 memory 设计核心是把记忆沉淀为可持久化内容，再通过检索层按相关性和时间性召回。", "Based on the previous search results, OpenClaw memory focuses on persistent memory artifacts plus relevance- and time-aware retrieval.")
	}
	return localizedText(locale, "我可以继续基于上一轮任务结果展开，当前会话已经保留了相关 artifact，可继续总结或细化。", "I can continue from the previous task result. This session already has artifacts we can summarize or refine.")
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
		Headline:      stageMessage(task, finalStage(task), localeEN),
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
	prevFocus := append([]string{}, state.WorkingSet.FocusPaths...)
	if ctx != nil {
		state.WorkingSet = workingSetFromContext(ctx)
	}
	state.WorkingSet.FocusPaths = mergeFocusPaths(state.WorkingSet.FocusPaths, prevFocus, 24)
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
	s.recordMemoryUse(ctx, task.SelectedSkill, task.State)
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
	out.FocusPaths = mergeFocusPaths(left.FocusPaths, right.FocusPaths, 24)
	return dedupeWorkingSet(out)
}

func dedupeWorkingSet(in SessionWorkset) SessionWorkset {
	in.ArtifactPaths = dedupeStrings(in.ArtifactPaths)
	in.RecallCardIDs = dedupeStrings(in.RecallCardIDs)
	in.SourceRefs = dedupeStrings(in.SourceRefs)
	in.FocusPaths = dedupeStrings(in.FocusPaths)
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

func sessionViewFromState(state SessionState) memoryorchestrator.SessionView {
	return memoryorchestrator.SessionView{
		Topic:            state.Topic,
		FocusTaskID:      state.FocusTaskID,
		PendingQuestion:  state.PendingQuestion,
		PendingAction:    state.PendingAction,
		LastAssistantAct: state.LastAssistantAct,
		WorkingRecallIDs: append([]string{}, state.WorkingSet.RecallCardIDs...),
		WorkingSources:   append([]string{}, state.WorkingSet.SourceRefs...),
		FocusPaths:       append([]string{}, state.WorkingSet.FocusPaths...),
	}
}

func taskRefsToPacketRefs(in []TaskRef) []memoryorchestrator.TaskRef {
	out := make([]memoryorchestrator.TaskRef, 0, len(in))
	for _, task := range in {
		out = append(out, memoryorchestrator.TaskRef{
			TaskID:        task.TaskID,
			Title:         task.Title,
			State:         task.State,
			SelectedSkill: task.SelectedSkill,
		})
	}
	return out
}

func contextFromPacket(packet *memoryorchestrator.Packet) *Context {
	if packet == nil || packet.IsEmpty() {
		return nil
	}
	return &Context{
		RecentTasks:   packetTaskRefsToContext(packet.RecentTasks),
		WorkingNotes:  append([]string{}, packet.WorkingNotes...),
		SemanticHits:  packetRecallRefsToContext(packet.SemanticHits),
		ProcedureHits: packetRecallRefsToContext(packet.ProcedureHits),
		RecallHits:    packetRecallRefsToContext(packet.RecallHits),
	}
}

func packetTaskRefsToContext(in []memoryorchestrator.TaskRef) []TaskRef {
	out := make([]TaskRef, 0, len(in))
	for _, task := range in {
		out = append(out, TaskRef{
			TaskID:        task.TaskID,
			Title:         task.Title,
			State:         task.State,
			SelectedSkill: task.SelectedSkill,
		})
	}
	return out
}

func packetRecallRefsToContext(in []memoryorchestrator.RecallRef) []RecallRef {
	out := make([]RecallRef, 0, len(in))
	for _, hit := range in {
		out = append(out, RecallRef{
			Source:   hit.Source,
			CardID:   hit.CardID,
			CardType: hit.CardType,
			Snippet:  hit.Snippet,
		})
	}
	return out
}

// recordChatEvent creates an episodic "event" memory card from a chat exchange.
// Only records when there's substantive content (not greetings/acks).
func (s *Service) recordChatEvent(sessionID, userText, assistantText, intentKind, skill string) {
	if s == nil || s.memoryStore == nil {
		return
	}
	userText = strings.TrimSpace(userText)
	assistantText = strings.TrimSpace(assistantText)
	if len(userText) < 10 || len(assistantText) < 20 {
		return
	}
	// Skip trivial greetings
	lower := strings.ToLower(userText)
	for _, skip := range []string{"hi", "hello", "hey", "你好", "谢谢", "thanks", "ok", "好的"} {
		if lower == skip {
			return
		}
	}

	cardID := fmt.Sprintf("chat:event:%s:%d", sessionID, time.Now().UTC().UnixNano())
	topic := previewString(userText, 120)
	summary := previewString(assistantText, 300)

	content := map[string]any{
		"topic":      topic,
		"summary":    summary,
		"user_query": previewString(userText, 500),
		"session_id": sessionID,
	}
	if skill != "" {
		content["skill"] = skill
	}

	_, _ = s.memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   cardID,
		CardType: "event",
		Scope:    memory.ScopeSession,
		Status:   memory.CardStatusActive,
		Content:  content,
		Provenance: memory.Provenance{
			Source:     "chat-" + firstNonEmpty(intentKind, "direct"),
			Confidence: 0.7,
		},
		Activation: &memory.ActivationState{
			Score:       0.9,
			DecayPolicy: "session_use",
		},
	})
}

func (s *Service) recordMemoryUse(ctx *Context, taskClass, outcome string) {
	if s == nil || ctx == nil {
		return
	}
	orchestrator := memoryorchestrator.New(s.recall, s.runtimeStore)
	packet := &memoryorchestrator.Packet{
		ProcedureHits: contextRecallRefsToPacket(ctx.ProcedureHits),
		SemanticHits:  contextRecallRefsToPacket(ctx.SemanticHits),
	}
	_ = orchestrator.RecordPacketUse(packet, memoryorchestrator.UsageContext{
		TaskClass: taskClass,
		Outcome:   outcome,
	})
}

func contextRecallRefsToPacket(in []RecallRef) []memoryorchestrator.RecallRef {
	out := make([]memoryorchestrator.RecallRef, 0, len(in))
	for _, hit := range in {
		out = append(out, memoryorchestrator.RecallRef{
			Source:   hit.Source,
			CardID:   hit.CardID,
			CardType: hit.CardType,
			Snippet:  hit.Snippet,
		})
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

// shouldConfirmTaskIntent gates every task_request turn behind an explicit
// "preview + approve" step so the runtime never runs work the user did not
// green-light. The only bypasses are:
//   - follow-ups that are already an answer/confirm/continue dialogue act
//     (they are themselves the user's approval to keep going),
//   - resubmissions tagged with Source="intent-confirmation" (the internal
//     tail-call made after the user typed "开始/yes"),
//   - root execution profile (the orchestrator already owns its approval flow
//     for root; chat-level confirmation would double-gate),
//   - empty text (nothing to confirm).
func shouldConfirmTaskIntent(route RouteDecision, text, source, dialogueAct, executionProfile string) bool {
	if route.IntentKind != IntentKindTask {
		return false
	}
	switch dialogueAct {
	case DialogueActContinueTask, DialogueActConfirm, DialogueActAnswer:
		return false
	}
	if strings.EqualFold(strings.TrimSpace(source), "intent-confirmation") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(executionProfile), "root") {
		return false
	}
	if strings.TrimSpace(text) == "" {
		return false
	}
	return true
}

// buildTaskConfirmationMessage renders the preview shown to the user before
// the runtime starts a task. Users see goal/skill/profile so they know what
// will actually run and can answer "开始/取消" or refine the goal.
func buildTaskConfirmationMessage(locale, goal string, route RouteDecision, executionProfile string) string {
	goalPreview := previewStringRunes(strings.TrimSpace(goal), 200)
	if goalPreview == "" {
		goalPreview = strings.TrimSpace(goal)
	}
	skillLabel := firstNonEmpty(route.Skill, "task-plan")
	profile := firstNonEmpty(executionProfile, "user")
	if isEnglishLocale(locale) {
		lines := []string{
			"Task ready. Please confirm before I start:",
			"• Goal: " + goalPreview,
			"• Skill: " + skillLabel,
			"• Profile: " + profile,
			"",
			"Reply \"start\" to begin, \"cancel\" to abandon, or send a refined goal.",
		}
		return strings.Join(lines, "\n")
	}
	lines := []string{
		"任务已准备好，开始前先跟你确认一下：",
		"• 目标: " + goalPreview,
		"• Skill: " + skillLabel,
		"• 执行权限: " + profile,
		"",
		"回复「开始」开始执行，「取消」放弃，或直接告诉我更具体的目标。",
	}
	return strings.Join(lines, "\n")
}

func parseConfirmationFromState(state SessionState, key string) string {
	prefix := key + ":"
	for _, ref := range state.WorkingSet.SourceRefs {
		if strings.HasPrefix(ref, prefix) {
			return strings.TrimPrefix(ref, prefix)
		}
	}
	return ""
}

func isAffirmativeConfirmation(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "是", "是的", "要", "好的", "好", "可以", "确认", "确定",
		"开始", "执行", "开始执行", "开始吧", "继续", "跑", "干",
		"yes", "y", "ok", "okay", "sure", "confirm",
		"start", "go", "run", "proceed", "do it":
		return true
	default:
		return false
	}
}

func isNegativeConfirmation(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "不", "不是", "不要", "否", "算了", "不用", "不用了", "取消", "停", "停下", "别",
		"no", "n", "cancel", "stop", "abort", "nope", "nvm", "nevermind":
		return true
	default:
		return false
	}
}

func (s *Service) tryHandleIntentConfirmation(sessionID, text string, req SendRequest, state SessionState, userMessage Message, dialogue DialogueDecision, ctx *Context, locale string) (SendResponse, bool) {
	if state.PendingAction != "confirm_task_intent" {
		return SendResponse{}, false
	}
	assistantMessage := Message{
		MessageID:        fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano()),
		SessionID:        sessionID,
		Role:             "assistant",
		DialogueAct:      dialogue.Act,
		IntentKind:       IntentKindDirect,
		IntentReason:     "intent confirmation follow-up",
		IntentConfidence: 0.95,
		ExecutionProfile: firstNonEmpty(req.ExecutionProfile, "user"),
		Context:          ctx,
		CreatedAt:        time.Now().UTC(),
	}
	if isAffirmativeConfirmation(text) {
		goal := parseConfirmationFromState(state, "confirm_goal")
		if strings.TrimSpace(goal) == "" {
			goal = text
		}
		state.PendingAction = ""
		state.PendingQuestion = ""
		state.WorkingSet.SourceRefs = nil
		_ = s.store.SaveSessionState(state)
		req.Message = goal
		req.Source = "intent-confirmation"
		resp, err := s.Send(req)
		if err != nil {
			assistantMessage.Content = localizedText(locale, "确认后创建任务失败："+err.Error(), "Failed to create task after confirmation: "+err.Error())
			assistantMessage.Stage = "failed"
			_ = s.store.Append(sessionID, assistantMessage)
			return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, true
		}
		return resp, true
	}
	if isNegativeConfirmation(text) {
		assistantMessage.Content = localizedText(locale, "好的，这次不执行。你可以直接告诉我更具体的目标、范围或约束，我再确认一次。", "Understood, not running this one. Tell me a more specific goal, scope, or constraint and I will ask again before running.")
		assistantMessage.Stage = "responded"
		state.PendingAction = ""
		state.PendingQuestion = ""
		state.WorkingSet.SourceRefs = nil
		_ = s.store.Append(sessionID, assistantMessage)
		_ = s.store.SaveSessionState(state)
		return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, true
	}
	// Ambiguous reply (not yes, not no). Treat the text as a refined goal
	// instead of re-asking: clear the pending state, then recurse through
	// Send() so intent/route are recomputed against the freshly-cleared state
	// — typically that classifies it as a fresh task_request and produces a
	// new confirmation card with the refined goal/skill preview.
	state.PendingAction = ""
	state.PendingQuestion = ""
	state.WorkingSet.SourceRefs = nil
	state.LastAssistantAct = ""
	_ = s.store.SaveSessionState(state)
	refined := req
	refined.Message = text
	resp, err := s.Send(refined)
	if err != nil {
		assistantMessage.Content = localizedText(locale, "收到，但我没能处理这条消息："+err.Error(), "Got it, but I could not process this message: "+err.Error())
		assistantMessage.Stage = "failed"
		_ = s.store.Append(sessionID, assistantMessage)
		return SendResponse{UserMessage: userMessage, AssistantMessage: assistantMessage}, true
	}
	// The refined Send() already appended its own user/assistant messages, so
	// we thread its response back untouched but keep the original userMessage
	// as the "turn's user message" for caller metadata consistency.
	resp.UserMessage = userMessage
	return resp, true
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
	case airuntime.TaskStatePlanned, airuntime.TaskStateBlocked, airuntime.TaskStateFailed:
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

func stageMessage(task airuntime.Task, stage, locale string) string {
	skill := strings.TrimSpace(task.SelectedSkill)
	zh := !isEnglishLocale(locale)
	switch stage {
	case "routing":
		if zh {
			return "正在路由请求并选择执行路径..."
		}
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
		if zh {
			return "正在处理中..."
		}
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

func fallbackDirectReply(userText, locale string) string {
	text := strings.TrimSpace(strings.ToLower(userText))
	switch {
	case text == "hi" || text == "hello" || text == "hey" || text == "你好" || text == "您好" || text == "嗨":
		return localizedText(locale, "你好！有什么我能帮你的吗？你可以问我任何问题，也可以让我帮你搜索、查看邮件、执行命令等。", "Hi! How can I help? You can ask questions or ask me to search, check email, or run tasks.")
	case strings.Contains(text, "thank") || strings.Contains(text, "谢谢"):
		return localizedText(locale, "不客气！还需要什么尽管说。", "You're welcome. Let me know what else you need.")
	default:
		return localizedText(locale, "收到你的消息。你可以继续问我问题，或者告诉我你需要我做什么。", "Got your message. Ask me anything, or tell me what you want me to do.")
	}
}

// modelReplyFailureMessage is shown when the LLM is configured but a request
// fails, instead of a generic fallback that looks like "normal chat" works.
func modelReplyFailureMessage(locale string, err error) string {
	if err == nil {
		return localizedText(locale, "模型没有返回可用内容。", "The model did not return usable text.")
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		reason = localizedText(locale, "未知错误", "unknown error")
	}
	tlsHint := tlsFailureExtraHint(locale, reason)
	if isEnglishLocale(locale) {
		return fmt.Sprintf("The language model could not answer this turn.\nReason: %s\n\nCheck API key, model id, and base URL in runtime/model/config.json, then run: mnemosynectl doctor --test-model%s", reason, tlsHint)
	}
	return fmt.Sprintf("大模型这一轮流调用失败，所以你看不到正常对话回复。\n原因：%s\n\n请检查 runtime/model/config.json 里的 API Key、模型名、Base URL（SiliconFlow 等需与供应商一致），或运行：mnemosynectl doctor --test-model%s", reason, tlsHint)
}

func tlsFailureExtraHint(locale, reason string) string {
	low := strings.ToLower(reason)
	if !strings.Contains(low, "tls:") && !strings.Contains(low, "certificate") && !strings.Contains(reason, "SecPolicy") && !strings.Contains(low, "x509") {
		return ""
	}
	if isEnglishLocale(locale) {
		return "\n\nTLS: If a proxy/VPN intercepts HTTPS, install its root CA or fix the system trust store. For local debugging only, set MNEMOSYNE_TLS_INSECURE=true in the daemon environment (disables verification — insecure)."
	}
	return "\n\nTLS/证书：常见于代理、VPN 或本机钥匙串问题；可安装根证书或修复系统信任。仅为本机排障时，可在运行 daemon 的环境里设置 MNEMOSYNE_TLS_INSECURE=true（跳过校验，不安全，勿用于生产）。"
}
