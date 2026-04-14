package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/chat"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/model"
	"mnemosyneos/internal/recall"
	"mnemosyneos/internal/skills"
)

func (s *Server) registerWebRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /", s.handleHome)
	mux.HandleFunc("GET /dashboard", s.handleDashboard)
	mux.HandleFunc("POST /ui/chat/sessions", s.handleWebChatCreateSession)
	mux.HandleFunc("POST /ui/chat/sessions/", s.handleWebChatSessionAction)
	mux.HandleFunc("GET /ui/chat", s.handleWebChat)
	mux.HandleFunc("GET /ui/chat/events", s.handleWebChatEvents)
	mux.HandleFunc("GET /ui/chat/messages", s.handleWebChatMessages)
	mux.HandleFunc("POST /ui/chat", s.handleWebChatSend)
	mux.HandleFunc("POST /ui/chat/tasks/", s.handleWebChatTaskAction)
	mux.HandleFunc("POST /ui/chat/approvals/", s.handleWebChatApprovalAction)
	mux.HandleFunc("GET /ui/tasks", s.handleWebTasks)
	mux.HandleFunc("POST /ui/tasks", s.handleWebCreateTask)
	mux.HandleFunc("POST /ui/tasks/", s.handleWebTaskAction)
	mux.HandleFunc("GET /ui/approvals", s.handleWebApprovals)
	mux.HandleFunc("POST /ui/approvals/", s.handleWebApprovalAction)
	mux.HandleFunc("GET /ui/recall", s.handleWebRecall)
	mux.HandleFunc("GET /ui/memory", s.handleWebMemory)
	mux.HandleFunc("GET /ui/skills", s.handleWebSkills)
	mux.HandleFunc("POST /ui/skills/reload", s.handleWebReloadSkills)
	mux.HandleFunc("POST /ui/skills/manifests", s.handleWebSaveSkillManifest)
	mux.HandleFunc("POST /ui/skills/", s.handleWebSkillAction)
	mux.HandleFunc("GET /ui/models", s.handleWebModels)
	mux.HandleFunc("POST /ui/models", s.handleWebUpdateModels)
	mux.HandleFunc("POST /ui/models/test", s.handleWebTestModels)
	mux.HandleFunc("GET /ui/preview", s.handleWebPreview)
	mux.HandleFunc("GET /ui/artifacts/view", s.handleWebArtifactView)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	state, err := s.runtimeStore.LoadState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tasks, err := s.runtimeStore.ListTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	approvals := []approval.Request{}
	if s.approvalStore != nil {
		approvals, err = s.approvalStore.List(approval.StatusPending)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	actions, err := s.executor.ListActions(8)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var activeTask *airuntime.Task
	if state.ActiveTaskID != nil {
		for i := range tasks {
			if tasks[i].TaskID == *state.ActiveTaskID {
				activeTask = &tasks[i]
				break
			}
		}
	}
	if activeTask == nil && len(tasks) > 0 {
		activeTask = &tasks[0]
	}
	var pendingApproval *approval.Request
	if len(approvals) > 0 {
		pendingApproval = &approvals[0]
	}
	var latestAction *execution.ActionRecord
	if len(actions) > 0 {
		latestAction = &actions[0]
	}

	data := dashboardPageData{
		PageData: PageData{
			Title: "Dashboard",
			Nav:   navItems("dashboard"),
		},
		Runtime:         state,
		Tasks:           truncateTasks(tasks, 8),
		Approvals:       approvals,
		Actions:         actions,
		Summary:         summarizeDashboard(tasks, approvals, actions),
		ActiveTask:      activeTask,
		PendingApproval: pendingApproval,
		LatestAction:    latestAction,
	}
	renderTemplate(w, "dashboard", data)
}

func (s *Server) handleWebModels(w http.ResponseWriter, r *http.Request) {
	cfg := model.Config{}
	if s.modelConfig != nil {
		cfg = s.modelConfig.Get()
	}
	cfg = defaultModelConfig(cfg)
	data := modelSettingsPageData{
		PageData: PageData{
			Title: "Models",
			Nav:   navItems("models"),
		},
		Config:         cfg,
		Providers:      []string{"none", "deepseek", "siliconflow", "openai", "openai-compatible"},
		HasAPIKey:      strings.TrimSpace(cfg.APIKey) != "",
		MaskedAPIKey:   maskSecret(cfg.APIKey),
		SuccessMessage: strings.TrimSpace(r.URL.Query().Get("success")),
		TestMessage:    strings.TrimSpace(r.URL.Query().Get("test")),
		ErrorMessage:   strings.TrimSpace(r.URL.Query().Get("error")),
	}
	renderTemplate(w, "models", data)
}

func (s *Server) handleWebUpdateModels(w http.ResponseWriter, r *http.Request) {
	if s.modelConfig == nil {
		writeError(w, http.StatusNotImplemented, "model config is not configured")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/ui/models?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	current := s.modelConfig.Get()
	cfg, err := modelConfigFromForm(r, current)
	if err != nil {
		http.Redirect(w, r, "/ui/models?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := s.modelConfig.Save(cfg); err != nil {
		http.Redirect(w, r, "/ui/models?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/models?success="+url.QueryEscape("model settings updated"), http.StatusSeeOther)
}

func (s *Server) handleWebTestModels(w http.ResponseWriter, r *http.Request) {
	if s.modelConfig == nil {
		writeError(w, http.StatusNotImplemented, "model config is not configured")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/ui/models?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	cfg, err := modelConfigFromForm(r, s.modelConfig.Get())
	if err != nil {
		http.Redirect(w, r, "/ui/models?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	gateway, ok := model.NewRuntimeGateway(nil).(*model.RuntimeGateway)
	if !ok {
		http.Redirect(w, r, "/ui/models?error="+url.QueryEscape("model test gateway is unavailable"), http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	resp, err := gateway.TestConfig(ctx, cfg)
	if err != nil {
		http.Redirect(w, r, "/ui/models?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	message := fmt.Sprintf("model connection ok: %s / %s", resp.Provider, resp.Model)
	http.Redirect(w, r, "/ui/models?test="+url.QueryEscape(message), http.StatusSeeOther)
}

func (s *Server) activeChatSession(r *http.Request) string {
	if r == nil {
		return "default"
	}
	if sessionID := strings.TrimSpace(r.URL.Query().Get("session")); sessionID != "" {
		return sessionID
	}
	return "default"
}

func (s *Server) handleWebChat(w http.ResponseWriter, r *http.Request) {
	sessionID := s.activeChatSession(r)
	if s.chatService != nil {
		if _, err := s.chatService.EnsureSession(sessionID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	messages, err := s.loadChatMessages(sessionID, 40)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sessions, err := s.loadChatSessions(20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	archived, err := s.loadArchivedChatSessions(20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activeTitle := sessionID
	for _, summary := range sessions {
		if summary.SessionID == sessionID {
			activeTitle = firstNonEmpty(summary.Title, sessionID)
			break
		}
	}
	sessionState := chat.SessionState{SessionID: sessionID}
	if s.chatService != nil {
		if state, err := s.chatService.SessionState(sessionID); err == nil {
			sessionState = state
		}
	}
	data := chatPageData{
		PageData: PageData{
			Title:     "Chat",
			Nav:       navItems("chat"),
			BodyClass: "chat-body",
			MainClass: "chat-main-shell",
		},
		Sessions:        sessions,
		Archived:        archived,
		ActiveSessionID: sessionID,
		ActiveTitle:     activeTitle,
		SessionState:    sessionState,
		Error:           strings.TrimSpace(r.URL.Query().Get("error")),
		Messages:        messages,
		Form: chatFormData{
			SessionID:   sessionID,
			RequestedBy: "web-chat",
			Source:      "web-chat",
			Profile:     "user",
		},
	}
	renderTemplate(w, "chat", data)
}

func (s *Server) handleWebChatCreateSession(w http.ResponseWriter, r *http.Request) {
	if s.chatService == nil {
		writeError(w, http.StatusNotImplemented, "chat service is not configured")
		return
	}
	sessionID := fmt.Sprintf("session-%d", time.Now().UTC().UnixNano())
	if _, err := s.chatService.EnsureSession(sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID), http.StatusSeeOther)
}

func (s *Server) handleWebChatSessionAction(w http.ResponseWriter, r *http.Request) {
	if s.chatService == nil {
		writeError(w, http.StatusNotImplemented, "chat service is not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/ui/chat/sessions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	sessionID := strings.TrimSpace(parts[0])
	switch parts[1] {
	case "rename":
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID)+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		if err := s.chatService.RenameSession(sessionID, r.FormValue("title")); err != nil {
			http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID)+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID), http.StatusSeeOther)
	case "archive":
		if err := s.chatService.ArchiveSession(sessionID); err != nil {
			http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID)+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		active := "default"
		if _, err := s.chatService.EnsureSession(active); err != nil {
			http.Redirect(w, r, "/ui/chat?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(active), http.StatusSeeOther)
	case "restore":
		if err := s.chatService.RestoreSession(sessionID); err != nil {
			http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(firstNonEmpty(sessionID, "default"))+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID), http.StatusSeeOther)
	case "delete":
		if err := s.chatService.DeleteSession(sessionID); err != nil {
			http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID)+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		active := "default"
		if _, err := s.chatService.EnsureSession(active); err != nil {
			http.Redirect(w, r, "/ui/chat?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(active), http.StatusSeeOther)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleWebChatMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := s.activeChatSession(r)
	messages, err := s.loadChatMessages(sessionID, 40)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	renderTemplate(w, "chat_messages", chatMessagesData{ActiveSessionID: sessionID, Messages: messages})
}

func (s *Server) handleWebChatEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	lastPayload := ""
	lastFingerprints := map[string]string{}
	lastMessages := map[string]chat.Message{}
	for {
		sessionID := s.activeChatSession(r)
		messages, err := s.loadChatMessages(sessionID, 40)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(err.Error(), "\n", " "))
			flusher.Flush()
			return
		}
		payload, err := s.renderChatMessagesPayload(sessionID, messages)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(err.Error(), "\n", " "))
			flusher.Flush()
			return
		}
		fingerprints := chatMessageFingerprints(messages)
		if payload != lastPayload {
			if lastPayload == "" || len(fingerprints) < len(lastFingerprints) {
				writeSSEEvent(w, "full", payload)
				flusher.Flush()
			} else {
				changed := 0
				for _, message := range messages {
					fp := fingerprints[message.MessageID]
					if lastFingerprints[message.MessageID] == fp {
						continue
					}
					if prev, ok := lastMessages[message.MessageID]; ok && canEmitChatDelta(prev, message) {
						sessionStateHTML, stateErr := s.renderChatSessionStateHTML(sessionID)
						if stateErr != nil {
							fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(stateErr.Error(), "\n", " "))
							flusher.Flush()
							return
						}
						deltaPayload, marshalErr := json.Marshal(map[string]string{
							"message_id":         message.MessageID,
							"delta":              strings.TrimPrefix(message.Content, prev.Content),
							"class_name":         renderChatMessageClass(message),
							"stage":              message.Stage,
							"intent_kind":        message.IntentKind,
							"selected_skill":     message.SelectedSkill,
							"task_state":         string(message.TaskState),
							"session_state_html": sessionStateHTML,
						})
						if marshalErr != nil {
							fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(marshalErr.Error(), "\n", " "))
							flusher.Flush()
							return
						}
						writeSSEEvent(w, "delta", string(deltaPayload))
						changed++
						continue
					}
					itemHTML, renderErr := s.renderChatMessageHTML(sessionID, message)
					if renderErr != nil {
						fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(renderErr.Error(), "\n", " "))
						flusher.Flush()
						return
					}
					innerHTML, innerErr := s.renderChatMessageInnerHTML(sessionID, message)
					if innerErr != nil {
						fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(innerErr.Error(), "\n", " "))
						flusher.Flush()
						return
					}
					sessionStateHTML, stateErr := s.renderChatSessionStateHTML(sessionID)
					if stateErr != nil {
						fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(stateErr.Error(), "\n", " "))
						flusher.Flush()
						return
					}
					patch, marshalErr := json.Marshal(map[string]string{
						"message_id":         message.MessageID,
						"html":               itemHTML,
						"inner_html":         innerHTML,
						"class_name":         renderChatMessageClass(message),
						"session_state_html": sessionStateHTML,
					})
					if marshalErr != nil {
						fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(marshalErr.Error(), "\n", " "))
						flusher.Flush()
						return
					}
					writeSSEEvent(w, "patch", string(patch))
					changed++
				}
				if changed == 0 {
					writeSSEEvent(w, "noop", "{}")
				}
				flusher.Flush()
			}
			lastPayload = payload
			lastFingerprints = fingerprints
			lastMessages = chatMessageMap(messages)
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (s *Server) handleWebChatSend(w http.ResponseWriter, r *http.Request) {
	if s.chatService == nil {
		writeError(w, http.StatusNotImplemented, "chat service is not configured")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/ui/chat?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	sessionID := firstNonEmpty(strings.TrimSpace(r.FormValue("session_id")), "default")
	resp, err := s.chatService.Send(chat.SendRequest{
		SessionID:        sessionID,
		Message:          strings.TrimSpace(r.FormValue("message")),
		RequestedBy:      firstNonEmpty(strings.TrimSpace(r.FormValue("requested_by")), "web-chat"),
		Source:           firstNonEmpty(strings.TrimSpace(r.FormValue("source")), "web-chat"),
		ExecutionProfile: firstNonEmpty(strings.TrimSpace(r.FormValue("execution_profile")), "user"),
		Async:            wantsJSON(r),
	})
	if err != nil {
		if wantsJSON(r) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID)+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"ok":                true,
			"session_id":        sessionID,
			"user_message":      resp.UserMessage,
			"assistant_message": resp.AssistantMessage,
		})
		return
	}
	http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(sessionID), http.StatusSeeOther)
}

func (s *Server) loadChatMessages(sessionID string, limit int) ([]chat.Message, error) {
	if s.chatService == nil {
		return []chat.Message{}, nil
	}
	return s.chatService.Messages(sessionID, limit)
}

func (s *Server) loadChatSessions(limit int) ([]chat.SessionSummary, error) {
	if s.chatService == nil {
		return nil, nil
	}
	return s.chatService.Sessions(limit)
}

func (s *Server) loadArchivedChatSessions(limit int) ([]chat.SessionSummary, error) {
	if s.chatService == nil {
		return nil, nil
	}
	return s.chatService.ArchivedSessions(limit)
}

func (s *Server) renderChatMessagesHTML(sessionID string, limit int) (string, error) {
	messages, err := s.loadChatMessages(sessionID, limit)
	if err != nil {
		return "", err
	}
	return s.renderChatMessagesPayload(sessionID, messages)
}

func (s *Server) renderChatMessagesPayload(sessionID string, messages []chat.Message) (string, error) {
	var buf bytes.Buffer
	if err := webTemplates.ExecuteTemplate(&buf, "chat_messages", chatMessagesData{ActiveSessionID: sessionID, Messages: messages}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Server) renderChatMessageHTML(sessionID string, message chat.Message) (string, error) {
	var buf bytes.Buffer
	if err := webTemplates.ExecuteTemplate(&buf, "chat_message", chatMessageData{
		ActiveSessionID: sessionID,
		Message:         message,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Server) renderChatSessionStateHTML(sessionID string) (string, error) {
	state := chat.SessionState{SessionID: sessionID}
	if s.chatService != nil {
		current, err := s.chatService.SessionState(sessionID)
		if err != nil {
			return "", err
		}
		state = current
	}
	var buf bytes.Buffer
	if err := webTemplates.ExecuteTemplate(&buf, "chat_session_state", state); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Server) renderChatMessageInnerHTML(sessionID string, message chat.Message) (string, error) {
	var buf bytes.Buffer
	if err := webTemplates.ExecuteTemplate(&buf, "chat_message_inner", chatMessageData{
		ActiveSessionID: sessionID,
		Message:         message,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderChatMessageClass(message chat.Message) string {
	className := "message " + message.Role
	if isPendingChatStage(message.Stage) {
		className += " pending"
	}
	return className
}

func chatMessageFingerprints(messages []chat.Message) map[string]string {
	out := make(map[string]string, len(messages))
	for _, message := range messages {
		raw, err := json.Marshal(message)
		if err != nil {
			out[message.MessageID] = fmt.Sprintf("%s/%s/%s", message.MessageID, message.TaskState, message.Stage)
			continue
		}
		out[message.MessageID] = string(raw)
	}
	return out
}

func chatMessageMap(messages []chat.Message) map[string]chat.Message {
	out := make(map[string]chat.Message, len(messages))
	for _, message := range messages {
		out[message.MessageID] = message
	}
	return out
}

func canEmitChatDelta(prev, current chat.Message) bool {
	if !strings.EqualFold(current.Role, "assistant") {
		return false
	}
	if strings.TrimSpace(prev.Content) == "" || strings.TrimSpace(current.Content) == "" {
		return false
	}
	if !isPendingChatStage(current.Stage) {
		return false
	}
	return strings.HasPrefix(current.Content, prev.Content)
}

func isPendingChatStage(stage string) bool {
	switch strings.TrimSpace(stage) {
	case "routing", "queued", "running", "working", "searching", "planning", "reading", "writing", "executing", "triaging_email", "searching_github", "consolidating", "summarizing", "persisting", "writing_memory":
		return true
	default:
		return false
	}
}

func writeSSEEvent(w http.ResponseWriter, eventName, payload string) {
	if eventName != "" {
		fmt.Fprintf(w, "event: %s\n", eventName)
	}
	for _, line := range strings.Split(payload, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func (s *Server) handleWebChatTaskAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/ui/chat/tasks/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	taskID := parts[0]
	switch parts[1] {
	case "run":
		if task, err := s.runtimeStore.GetTask(taskID); err == nil {
			switch task.State {
			case airuntime.TaskStateFailed, airuntime.TaskStateBlocked:
				if _, err := s.orchestrator.RequeueTask(taskID, "web_chat_run", nil); err != nil {
					redirectChatSessionWithError(w, r, chatSessionFromForm(r), err)
					return
				}
			}
		}
		if _, err := s.skillRunner.RunTask(taskID); err != nil {
			redirectChatSessionWithError(w, r, chatSessionFromForm(r), err)
			return
		}
		redirectChatSession(w, r, chatSessionFromForm(r))
	case "approve-run":
		if _, err := s.orchestrator.ApproveTask(taskID, "web-chat"); err != nil {
			redirectChatSessionWithError(w, r, chatSessionFromForm(r), err)
			return
		}
		if _, err := s.skillRunner.RunTask(taskID); err != nil {
			redirectChatSessionWithError(w, r, chatSessionFromForm(r), err)
			return
		}
		redirectChatSession(w, r, chatSessionFromForm(r))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleWebChatApprovalAction(w http.ResponseWriter, r *http.Request) {
	if s.approvalStore == nil {
		writeError(w, http.StatusNotImplemented, "approval flow is not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/ui/chat/approvals/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	approvalID := parts[0]
	switch parts[1] {
	case "approve-run":
		record, err := s.approvalStore.Approve(approvalID, "web-chat")
		if err != nil {
			redirectChatSessionWithError(w, r, chatSessionFromForm(r), err)
			return
		}
		if record.TaskID != "" {
			if _, err := s.orchestrator.RequeueTask(record.TaskID, "approval_granted", func(task *airuntime.Task) {
				if task.Metadata == nil {
					task.Metadata = map[string]string{}
				}
				task.Metadata["root_approval_id"] = approvalID
				task.NextAction = "root approval granted; rerun task"
			}); err == nil {
				_, _ = s.skillRunner.RunTask(record.TaskID)
			}
		}
		redirectChatSession(w, r, chatSessionFromForm(r))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleWebTasks(w http.ResponseWriter, r *http.Request) {
	allTasks, err := s.runtimeStore.ListTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	filter := taskFilter{
		State:   strings.TrimSpace(r.URL.Query().Get("state")),
		Skill:   strings.TrimSpace(r.URL.Query().Get("skill")),
		Profile: strings.TrimSpace(r.URL.Query().Get("profile")),
		Query:   strings.TrimSpace(r.URL.Query().Get("query")),
	}
	tasks := filterTasks(allTasks, filter)
	form := taskFormData{
		RequestedBy: "web-console",
		Source:      "web-console",
		Profile:     "user",
	}
	var selected *airuntime.Task
	if taskID := strings.TrimSpace(r.URL.Query().Get("task_id")); taskID != "" {
		task, err := s.runtimeStore.GetTask(taskID)
		if err == nil {
			selected = &task
		}
	} else if len(tasks) > 0 {
		task := tasks[0]
		selected = &task
	}

	data := tasksPageData{
		PageData: PageData{
			Title: "Tasks",
			Nav:   navItems("tasks"),
		},
		Tasks:     tasks,
		Selected:  selected,
		Form:      form,
		Filter:    filter,
		Summary:   summarizeTasks(allTasks),
		Skills:    uniqueTaskSkills(allTasks),
		Profiles:  []string{"user", "root"},
		HasFilter: filter != (taskFilter{}),
	}
	renderTemplate(w, "tasks", data)
}

func (s *Server) handleWebCreateTask(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/ui/tasks?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	req := airuntime.CreateTaskRequest{
		Title:            strings.TrimSpace(r.FormValue("title")),
		Goal:             strings.TrimSpace(r.FormValue("goal")),
		RequestedBy:      firstNonEmpty(strings.TrimSpace(r.FormValue("requested_by")), "web-console"),
		Source:           firstNonEmpty(strings.TrimSpace(r.FormValue("source")), "web-console"),
		ExecutionProfile: firstNonEmpty(strings.TrimSpace(r.FormValue("execution_profile")), "user"),
		RequiresApproval: r.FormValue("requires_approval") != "",
		Metadata:         map[string]string{},
	}
	if req.ExecutionProfile == "root" {
		req.RequiresApproval = true
	}
	if path := strings.TrimSpace(r.FormValue("path")); path != "" {
		req.Metadata["path"] = path
	}
	if content := r.FormValue("content"); strings.TrimSpace(content) != "" {
		req.Metadata["content"] = content
	}
	if command := strings.TrimSpace(r.FormValue("command")); command != "" {
		req.Metadata["command"] = command
	}
	if args := strings.TrimSpace(r.FormValue("args")); args != "" {
		req.Metadata["args"] = args
	}
	if query := strings.TrimSpace(r.FormValue("query")); query != "" {
		req.Metadata["query"] = query
	}
	task, err := s.orchestrator.SubmitTask(req)
	if err != nil {
		http.Redirect(w, r, "/ui/tasks?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/tasks?task_id="+url.QueryEscape(task.TaskID), http.StatusSeeOther)
}

func (s *Server) handleWebTaskAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/ui/tasks/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	taskID := parts[0]
	switch parts[1] {
	case "run":
		_, err := s.skillRunner.RunTask(taskID)
		if err != nil {
			http.Redirect(w, r, "/ui/tasks?task_id="+url.QueryEscape(taskID)+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/ui/tasks?task_id="+url.QueryEscape(taskID), http.StatusSeeOther)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleWebApprovals(w http.ResponseWriter, r *http.Request) {
	if s.approvalStore == nil {
		writeError(w, http.StatusNotImplemented, "approval flow is not configured")
		return
	}
	allRecords, err := s.approvalStore.List("")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	filter := approvalFilter{
		Status:  strings.TrimSpace(r.URL.Query().Get("status")),
		Action:  strings.TrimSpace(r.URL.Query().Get("action")),
		Profile: strings.TrimSpace(r.URL.Query().Get("profile")),
		Query:   strings.TrimSpace(r.URL.Query().Get("query")),
	}
	records := filterApprovals(allRecords, filter)
	var selected *approval.Request
	var selectedTask *airuntime.Task
	if approvalID := strings.TrimSpace(r.URL.Query().Get("approval_id")); approvalID != "" {
		record, err := s.approvalStore.Get(approvalID)
		if err == nil {
			selected = &record
			if record.TaskID != "" {
				task, taskErr := s.runtimeStore.GetTask(record.TaskID)
				if taskErr == nil {
					selectedTask = &task
				}
			}
		}
	} else if len(records) > 0 {
		record := records[0]
		selected = &record
		if record.TaskID != "" {
			task, taskErr := s.runtimeStore.GetTask(record.TaskID)
			if taskErr == nil {
				selectedTask = &task
			}
		}
	}
	data := approvalsPageData{
		PageData: PageData{
			Title: "Approvals",
			Nav:   navItems("approvals"),
		},
		Status:       filter.Status,
		Approvals:    records,
		Selected:     selected,
		SelectedTask: selectedTask,
		Filter:       filter,
		Summary:      summarizeApprovals(allRecords),
		Actions:      uniqueApprovalActions(allRecords),
		Profiles:     []string{"user", "root"},
		HasFilter:    filter != (approvalFilter{}),
	}
	renderTemplate(w, "approvals", data)
}

func (s *Server) handleWebApprovalAction(w http.ResponseWriter, r *http.Request) {
	if s.approvalStore == nil {
		writeError(w, http.StatusNotImplemented, "approval flow is not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/ui/approvals/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	approvalID := parts[0]
	switch parts[1] {
	case "approve":
		_, err := s.approvalStore.Approve(approvalID, "web-console")
		if err != nil {
			http.Redirect(w, r, "/ui/approvals?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		record, _ := s.approvalStore.Get(approvalID)
		if record.TaskID != "" {
			_, _ = s.orchestrator.RequeueTask(record.TaskID, "approval_granted", func(task *airuntime.Task) {
				if task.Metadata == nil {
					task.Metadata = map[string]string{}
				}
				task.Metadata["root_approval_id"] = approvalID
				task.NextAction = "root approval granted; rerun task"
			})
		}
	case "deny":
		reason := strings.TrimSpace(r.FormValue("reason"))
		record, err := s.approvalStore.Deny(approvalID, "web-console", reason)
		if err != nil {
			http.Redirect(w, r, "/ui/approvals?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		if record.TaskID != "" {
			_, _ = s.runtimeStore.MoveTask(record.TaskID, airuntime.TaskStateBlocked, func(task *airuntime.Task) {
				if task.Metadata == nil {
					task.Metadata = map[string]string{}
				}
				task.Metadata["root_approval_id"] = record.ApprovalID
				task.FailureReason = firstNonEmpty(record.DeniedReason, "root approval denied")
				task.NextAction = "root approval denied"
			})
		}
	default:
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/ui/approvals", http.StatusSeeOther)
}

func (s *Server) handleWebRecall(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	sources := make([]string, 0)
	for _, value := range r.URL.Query()["source"] {
		for _, source := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(source); trimmed != "" {
				sources = append(sources, trimmed)
			}
		}
	}
	resp := recall.Response{Query: query}
	if s.recallService != nil && (query != "" || len(sources) > 0) {
		resp = s.recallService.Recall(recall.Request{
			Query:   query,
			Sources: sources,
			Limit:   20,
		})
	}
	selected := selectRecallHit(resp.Hits, strings.TrimSpace(r.URL.Query().Get("card_id")))
	summary := summarizeRecall(resp.Hits)
	if selected != nil {
		summary.Selected = 1
	}
	data := recallPageData{
		PageData: PageData{
			Title: "Recall",
			Nav:   navItems("recall"),
		},
		Query:        query,
		Sources:      sources,
		Response:     resp,
		Summary:      summary,
		SourceCounts: summarizeRecallSources(resp.Hits),
		Selected:     selected,
		HasFilter:    query != "" || len(sources) > 0,
	}
	renderTemplate(w, "recall", data)
}

func (s *Server) handleWebMemory(w http.ResponseWriter, r *http.Request) {
	cards := []memory.Card{}
	if s.store != nil {
		cards = s.store.LatestCards()
	}
	var selected *memory.Card
	edges := []memory.Edge{}
	cardID := strings.TrimSpace(r.URL.Query().Get("card_id"))
	if cardID == "" && len(cards) > 0 {
		cardID = cards[0].CardID
	}
	if cardID != "" && s.store != nil {
		resp := s.store.Query(memory.QueryRequest{CardID: cardID})
		if len(resp.Cards) > 0 {
			selected = &resp.Cards[0]
			edges = sortEdges(resp.Edges)
		}
	}
	data := memoryPageData{
		PageData: PageData{
			Title: "Memory",
			Nav:   navItems("memory"),
		},
		Cards:         truncateCards(cards, 40),
		Selected:      selected,
		SelectedEdges: edges,
		Summary:       summarizeMemory(cards, edges),
		CardTypes:     summarizeMemoryTypes(cards),
	}
	renderTemplate(w, "memory", data)
}

func (s *Server) handleWebSkills(w http.ResponseWriter, r *http.Request) {
	page := skillsPageData{
		PageData: PageData{
			Title: "Skills",
			Nav:   navItems("skills"),
		},
		ManifestJSON:   defaultSkillManifestJSON(),
		ErrorMessage:   strings.TrimSpace(r.URL.Query().Get("error")),
		SuccessMessage: strings.TrimSpace(r.URL.Query().Get("success")),
	}
	if s.skillRunner != nil {
		page.Skills = s.skillRunner.ListSkills()
		page.ManifestStatuses = s.skillRunner.ListManifestStatuses()
		if name := strings.TrimSpace(r.URL.Query().Get("manifest")); name != "" {
			if raw, err := s.skillRunner.LoadManifestFile(name); err == nil {
				page.ManifestJSON = raw
			} else {
				page.ErrorMessage = firstNonEmpty(page.ErrorMessage, err.Error())
			}
		}
	}
	renderTemplate(w, "skills", page)
}

func (s *Server) handleWebReloadSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillRunner == nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape("skill runner is not configured"), http.StatusSeeOther)
		return
	}
	if err := s.skillRunner.ReloadSkills(); err != nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/skills?success="+url.QueryEscape("skills reloaded"), http.StatusSeeOther)
}

func (s *Server) handleWebSaveSkillManifest(w http.ResponseWriter, r *http.Request) {
	if s.skillRunner == nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape("skill runner is not configured"), http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	raw := strings.TrimSpace(r.FormValue("manifest_json"))
	if raw == "" {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape("manifest_json is required"), http.StatusSeeOther)
		return
	}
	var manifest skills.Manifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if _, err := s.skillRunner.SaveManifest(manifest); err != nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/skills?success="+url.QueryEscape(manifest.Name+" saved"), http.StatusSeeOther)
}

func (s *Server) handleWebSkillAction(w http.ResponseWriter, r *http.Request) {
	if s.skillRunner == nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape("skill runner is not configured"), http.StatusSeeOther)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/ui/skills/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "toggle" {
		writeError(w, http.StatusNotFound, "skill route not found")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	enabled := strings.EqualFold(strings.TrimSpace(r.FormValue("enabled")), "true")
	if err := s.skillRunner.SetSkillEnabled(parts[0], enabled); err != nil {
		http.Redirect(w, r, "/ui/skills?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	label := "disabled"
	if enabled {
		label = "enabled"
	}
	http.Redirect(w, r, "/ui/skills?success="+url.QueryEscape(parts[0]+" "+label), http.StatusSeeOther)
}

func defaultSkillManifestJSON() string {
	return "{\n  \"name\": \"web-research\",\n  \"description\": \"Alias for web search with tuned defaults.\",\n  \"uses\": \"web-search\",\n  \"enabled\": true,\n  \"default_metadata\": {\n    \"query_style\": \"research\"\n  },\n  \"maintenance_policy\": {\n    \"enabled\": true,\n    \"scope\": \"project\",\n    \"allowed_card_types\": [\"web_result\"],\n    \"min_candidates\": 1\n  }\n}\n"
}

func (s *Server) handleWebPreview(w http.ResponseWriter, r *http.Request) {
	html, err := s.renderResourcePreview(strings.TrimSpace(r.URL.Query().Get("href")), strings.TrimSpace(r.URL.Query().Get("method")))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleWebArtifactView(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		http.NotFound(w, r)
		return
	}
	clean := filepath.Clean(path)
	root := filepath.Clean(s.runtimeStore.RootDir())
	rel, err := filepath.Rel(root, clean)
	if err != nil || strings.HasPrefix(rel, "..") {
		writeError(w, http.StatusForbidden, "artifact path is outside runtime root")
		return
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if r.URL.Query().Get("raw") == "1" || r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", http.DetectContentType(data))
		if r.URL.Query().Get("download") == "1" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(clean)))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	page := artifactPageData{
		PageData: PageData{
			Title: "Artifact",
			Nav:   navItems("chat"),
		},
		Path:    clean,
		Content: string(data),
	}
	renderTemplate(w, "artifact", page)
}

type PageData struct {
	Title     string
	Nav       []navItem
	BodyClass string
	MainClass string
	BodyStyle string
	MainStyle string
}

type navItem struct {
	Name   string
	Short  string
	Href   string
	Active bool
}

type dashboardPageData struct {
	PageData
	Runtime         airuntime.RuntimeState
	Tasks           []airuntime.Task
	Approvals       []approval.Request
	Actions         []execution.ActionRecord
	Summary         dashboardSummary
	ActiveTask      *airuntime.Task
	PendingApproval *approval.Request
	LatestAction    *execution.ActionRecord
}

type chatPageData struct {
	PageData
	Sessions        []chat.SessionSummary
	Archived        []chat.SessionSummary
	ActiveSessionID string
	ActiveTitle     string
	SessionState    chat.SessionState
	Error           string
	Messages        []chat.Message
	Form            chatFormData
}

type chatMessagesData struct {
	ActiveSessionID string
	Messages        []chat.Message
}

type chatMessageData struct {
	ActiveSessionID string
	Message         chat.Message
}

type chatFormData struct {
	SessionID   string
	Message     string
	RequestedBy string
	Source      string
	Profile     string
}

type tasksPageData struct {
	PageData
	Tasks     []airuntime.Task
	Selected  *airuntime.Task
	Form      taskFormData
	Filter    taskFilter
	Summary   taskSummary
	Skills    []string
	Profiles  []string
	HasFilter bool
}

type approvalsPageData struct {
	PageData
	Status       string
	Approvals    []approval.Request
	Selected     *approval.Request
	SelectedTask *airuntime.Task
	Filter       approvalFilter
	Summary      approvalSummary
	Actions      []string
	Profiles     []string
	HasFilter    bool
}

type recallPageData struct {
	PageData
	Query        string
	Sources      []string
	Response     recall.Response
	Summary      recallSummary
	SourceCounts []recallSourceCount
	Selected     *recall.Hit
	HasFilter    bool
}

type memoryPageData struct {
	PageData
	Cards         []memory.Card
	Selected      *memory.Card
	SelectedEdges []memory.Edge
	Summary       memorySummary
	CardTypes     []memoryTypeCount
}

type skillsPageData struct {
	PageData
	Skills           []skills.Definition
	ManifestStatuses []skills.ManifestStatus
	ManifestJSON     string
	ErrorMessage     string
	SuccessMessage   string
}

type artifactPageData struct {
	PageData
	Path    string
	Content string
}

type modelSettingsPageData struct {
	PageData
	Config         model.Config
	Providers      []string
	HasAPIKey      bool
	MaskedAPIKey   string
	SuccessMessage string
	TestMessage    string
	ErrorMessage   string
}

type taskFormData struct {
	Title            string
	Goal             string
	RequestedBy      string
	Source           string
	Profile          string
	RequiresApproval bool
	Path             string
	Content          string
	Command          string
	Args             string
	Query            string
}

type taskFilter struct {
	State   string
	Skill   string
	Profile string
	Query   string
}

type taskSummary struct {
	Total            int
	InFlight         int
	AwaitingApproval int
	Blocked          int
	Done             int
	Failed           int
}

type approvalFilter struct {
	Status  string
	Action  string
	Profile string
	Query   string
}

type approvalSummary struct {
	Total    int
	Pending  int
	Approved int
	Denied   int
	Consumed int
}

type recallSummary struct {
	Total        int
	Selected     int
	TopSource    string
	MatchedCards int
}

type recallSourceCount struct {
	Source string
	Count  int
}

type memorySummary struct {
	Total     int
	CardTypes int
	Edges     int
}

type memoryTypeCount struct {
	CardType string
	Count    int
}

type dashboardSummary struct {
	OpenTasks        int
	PendingApprovals int
	FailedActions    int
}

func navItems(active string) []navItem {
	items := []navItem{
		{Name: "Dashboard", Short: "DB", Href: "/dashboard"},
		{Name: "Chat", Short: "CH", Href: "/ui/chat"},
		{Name: "Tasks", Short: "TK", Href: "/ui/tasks"},
		{Name: "Approvals", Short: "AP", Href: "/ui/approvals"},
		{Name: "Recall", Short: "RC", Href: "/ui/recall"},
		{Name: "Memory", Short: "MM", Href: "/ui/memory"},
		{Name: "Skills", Short: "SK", Href: "/ui/skills"},
		{Name: "Models", Short: "AI", Href: "/ui/models"},
	}
	for i := range items {
		items[i].Active = strings.EqualFold(active, strings.ToLower(items[i].Name))
	}
	return items
}

func truncateTasks(tasks []airuntime.Task, limit int) []airuntime.Task {
	if len(tasks) > limit {
		return tasks[:limit]
	}
	return tasks
}

func truncateCards(cards []memory.Card, limit int) []memory.Card {
	if len(cards) > limit {
		return cards[:limit]
	}
	return cards
}

func sortEdges(edges []memory.Edge) []memory.Edge {
	if len(edges) == 0 {
		return edges
	}
	sorted := append([]memory.Edge(nil), edges...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].EdgeID < sorted[j].EdgeID
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})
	return sorted
}

func filterTasks(tasks []airuntime.Task, filter taskFilter) []airuntime.Task {
	if filter == (taskFilter{}) {
		return tasks
	}
	out := make([]airuntime.Task, 0, len(tasks))
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	for _, task := range tasks {
		if filter.State != "" && !strings.EqualFold(task.State, filter.State) {
			continue
		}
		if filter.Skill != "" && !strings.EqualFold(task.SelectedSkill, filter.Skill) {
			continue
		}
		if filter.Profile != "" && !strings.EqualFold(task.ExecutionProfile, filter.Profile) {
			continue
		}
		if query != "" {
			hay := strings.ToLower(strings.Join([]string{
				task.TaskID, task.Title, task.Goal, task.SelectedSkill, task.State, task.NextAction, task.FailureReason,
			}, " "))
			if !strings.Contains(hay, query) {
				continue
			}
		}
		out = append(out, task)
	}
	return out
}

func summarizeTasks(tasks []airuntime.Task) taskSummary {
	out := taskSummary{Total: len(tasks)}
	for _, task := range tasks {
		switch task.State {
		case airuntime.TaskStateInbox, airuntime.TaskStatePlanned, airuntime.TaskStateActive:
			out.InFlight++
		case airuntime.TaskStateAwaitingApproval:
			out.AwaitingApproval++
		case airuntime.TaskStateBlocked:
			out.Blocked++
		case airuntime.TaskStateDone:
			out.Done++
		case airuntime.TaskStateFailed:
			out.Failed++
		}
	}
	return out
}

func uniqueTaskSkills(tasks []airuntime.Task) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, task := range tasks {
		value := strings.TrimSpace(task.SelectedSkill)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func filterApprovals(records []approval.Request, filter approvalFilter) []approval.Request {
	if filter == (approvalFilter{}) {
		return records
	}
	out := make([]approval.Request, 0, len(records))
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	for _, record := range records {
		if filter.Status != "" && !strings.EqualFold(record.Status, filter.Status) {
			continue
		}
		if filter.Action != "" && !strings.EqualFold(record.ActionKind, filter.Action) {
			continue
		}
		if filter.Profile != "" && !strings.EqualFold(record.ExecutionProfile, filter.Profile) {
			continue
		}
		if query != "" {
			hay := strings.ToLower(strings.Join([]string{
				record.ApprovalID, record.TaskID, record.Summary, record.ActionKind, record.Status, record.RequestedBy, metadataPreview(record.Metadata),
			}, " "))
			if !strings.Contains(hay, query) {
				continue
			}
		}
		out = append(out, record)
	}
	return out
}

func summarizeApprovals(records []approval.Request) approvalSummary {
	out := approvalSummary{Total: len(records)}
	for _, record := range records {
		switch record.Status {
		case approval.StatusPending:
			out.Pending++
		case approval.StatusApproved:
			out.Approved++
		case approval.StatusDenied:
			out.Denied++
		case approval.StatusConsumed:
			out.Consumed++
		}
	}
	return out
}

func uniqueApprovalActions(records []approval.Request) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, record := range records {
		value := strings.TrimSpace(record.ActionKind)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func summarizeRecall(hits []recall.Hit) recallSummary {
	out := recallSummary{Total: len(hits), MatchedCards: len(hits)}
	if len(hits) == 0 {
		return out
	}
	counts := map[string]int{}
	for _, hit := range hits {
		counts[hit.Source]++
	}
	for source, count := range counts {
		if count > 0 && (out.TopSource == "" || count > counts[out.TopSource] || (count == counts[out.TopSource] && source < out.TopSource)) {
			out.TopSource = source
		}
	}
	return out
}

func summarizeRecallSources(hits []recall.Hit) []recallSourceCount {
	if len(hits) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, hit := range hits {
		counts[hit.Source]++
	}
	out := make([]recallSourceCount, 0, len(counts))
	for source, count := range counts {
		out = append(out, recallSourceCount{Source: source, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Source < out[j].Source
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func selectRecallHit(hits []recall.Hit, cardID string) *recall.Hit {
	if len(hits) == 0 {
		return nil
	}
	if cardID != "" {
		for i := range hits {
			if hits[i].CardID == cardID {
				return &hits[i]
			}
		}
	}
	return &hits[0]
}

func recallDetailFields(card memory.Card) [][2]string {
	if len(card.Content) == 0 {
		return nil
	}
	keys := make([]string, 0, len(card.Content))
	for key := range card.Content {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([][2]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(strings.ReplaceAll(fmt.Sprintf("%v", card.Content[key]), "\n", " "))
		fields = append(fields, [2]string{key, value})
	}
	return fields
}

func summarizeMemory(cards []memory.Card, selectedEdges []memory.Edge) memorySummary {
	types := map[string]struct{}{}
	for _, card := range cards {
		if card.CardType != "" {
			types[card.CardType] = struct{}{}
		}
	}
	return memorySummary{
		Total:     len(cards),
		CardTypes: len(types),
		Edges:     len(selectedEdges),
	}
}

func summarizeMemoryTypes(cards []memory.Card) []memoryTypeCount {
	if len(cards) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, card := range cards {
		counts[card.CardType]++
	}
	out := make([]memoryTypeCount, 0, len(counts))
	for cardType, count := range counts {
		out = append(out, memoryTypeCount{CardType: cardType, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].CardType < out[j].CardType
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func summarizeDashboard(tasks []airuntime.Task, approvals []approval.Request, actions []execution.ActionRecord) dashboardSummary {
	out := dashboardSummary{
		PendingApprovals: len(approvals),
	}
	for _, task := range tasks {
		switch task.State {
		case airuntime.TaskStateInbox, airuntime.TaskStatePlanned, airuntime.TaskStateActive, airuntime.TaskStateAwaitingApproval, airuntime.TaskStateBlocked:
			out.OpenTasks++
		}
	}
	for _, action := range actions {
		if action.Status == execution.ActionStatusFailed {
			out.FailedActions++
		}
	}
	return out
}

func memoryCardTitle(card memory.Card) string {
	for _, key := range []string{"title", "subject", "summary", "query", "goal"} {
		if value, ok := card.Content[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return card.CardType
}

func memoryEdgePeer(edge memory.Edge, selectedCardID string) string {
	if edge.FromCardID == selectedCardID {
		return edge.ToCardID
	}
	if edge.ToCardID == selectedCardID {
		return edge.FromCardID
	}
	return firstNonEmpty(edge.ToCardID, edge.FromCardID)
}

func metadataPreview(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return strings.Join(parts, ", ")
}

func (s *Server) renderResourcePreview(rawHref, method string) (string, error) {
	if rawHref == "" {
		return "", fmt.Errorf("preview href is required")
	}
	parsed, err := url.Parse(rawHref)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(parsed.Path)
	switch {
	case path == "/ui/tasks":
		taskID := strings.TrimSpace(parsed.Query().Get("task_id"))
		return s.renderTaskPreview(taskID)
	case strings.HasPrefix(path, "/ui/chat/tasks/"):
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) >= 4 {
			return s.renderTaskPreview(parts[2])
		}
	case path == "/ui/approvals":
		approvalID := strings.TrimSpace(parsed.Query().Get("approval_id"))
		return s.renderApprovalPreview(approvalID, method)
	case strings.HasPrefix(path, "/ui/chat/approvals/"):
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) >= 4 {
			return s.renderApprovalPreview(parts[2], firstNonEmpty(method, parts[3]))
		}
	case path == "/ui/artifacts/view":
		artifactPath := strings.TrimSpace(parsed.Query().Get("path"))
		return s.renderArtifactPreview(artifactPath)
	}
	return "", fmt.Errorf("unsupported preview target")
}

func (s *Server) renderTaskPreview(taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("task id is required")
	}
	task, err := s.runtimeStore.GetTask(taskID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("<strong>")
	b.WriteString(template.HTMLEscapeString(firstNonEmpty(task.Title, task.TaskID)))
	b.WriteString("</strong><br>")
	b.WriteString("State: ")
	b.WriteString(template.HTMLEscapeString(task.State))
	if task.SelectedSkill != "" {
		b.WriteString(" · Skill: ")
		b.WriteString(template.HTMLEscapeString(task.SelectedSkill))
	}
	b.WriteString("<br>")
	b.WriteString(template.HTMLEscapeString(previewText(firstNonEmpty(task.NextAction, task.Goal), 180)))
	return b.String(), nil
}

func (s *Server) renderApprovalPreview(approvalID, method string) (string, error) {
	if approvalID == "" || s.approvalStore == nil {
		return "", fmt.Errorf("approval id is required")
	}
	record, err := s.approvalStore.Get(approvalID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("<strong>")
	b.WriteString(template.HTMLEscapeString(firstNonEmpty(record.Summary, record.ApprovalID)))
	b.WriteString("</strong><br>")
	b.WriteString("Status: ")
	b.WriteString(template.HTMLEscapeString(record.Status))
	b.WriteString(" · Action: ")
	b.WriteString(template.HTMLEscapeString(record.ActionKind))
	if method != "" {
		b.WriteString(" · Request: ")
		b.WriteString(template.HTMLEscapeString(strings.ToUpper(method)))
	}
	if record.TaskID != "" {
		b.WriteString("<br>Task: ")
		b.WriteString(template.HTMLEscapeString(record.TaskID))
	}
	return b.String(), nil
}

func (s *Server) renderArtifactPreview(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("artifact path is required")
	}
	clean := filepath.Clean(path)
	root := filepath.Clean(s.runtimeStore.RootDir())
	rel, err := filepath.Rel(root, clean)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("artifact path is outside runtime root")
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", " "), "\n", " "))
	var b strings.Builder
	b.WriteString("<strong>")
	b.WriteString(template.HTMLEscapeString(filepath.Base(clean)))
	b.WriteString("</strong><br>")
	b.WriteString(template.HTMLEscapeString(previewText(content, 220)))
	return b.String(), nil
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := webTemplates.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, fmt.Sprintf("render %s: %v", name, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

var urlPattern = regexp.MustCompile(`https?://[^\s<]+`)

func renderChatContentHTML(role, content string) template.HTML {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if role == "user" {
		return template.HTML("<p>" + linkifyEscaped(strings.ReplaceAll(template.HTMLEscapeString(content), "\n", "<br>")) + "</p>")
	}
	lines := strings.Split(content, "\n")
	var out strings.Builder
	inUL := false
	inOL := false
	inPara := false
	closeBlocks := func() {
		if inPara {
			out.WriteString("</p>")
			inPara = false
		}
		if inUL {
			out.WriteString("</ul>")
			inUL = false
		}
		if inOL {
			out.WriteString("</ol>")
			inOL = false
		}
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			closeBlocks()
			continue
		}
		if ordered, item, ok := parseListItem(line); ok {
			if inPara {
				out.WriteString("</p>")
				inPara = false
			}
			if ordered {
				if inUL {
					out.WriteString("</ul>")
					inUL = false
				}
				if !inOL {
					out.WriteString("<ol>")
					inOL = true
				}
				out.WriteString("<li>" + linkifyEscaped(template.HTMLEscapeString(item)) + "</li>")
			} else {
				if inOL {
					out.WriteString("</ol>")
					inOL = false
				}
				if !inUL {
					out.WriteString("<ul>")
					inUL = true
				}
				out.WriteString("<li>" + linkifyEscaped(template.HTMLEscapeString(item)) + "</li>")
			}
			continue
		}
		if inUL {
			out.WriteString("</ul>")
			inUL = false
		}
		if inOL {
			out.WriteString("</ol>")
			inOL = false
		}
		escaped := linkifyEscaped(template.HTMLEscapeString(line))
		if !inPara {
			out.WriteString("<p>")
			inPara = true
		} else {
			out.WriteString("<br>")
		}
		out.WriteString(escaped)
	}
	closeBlocks()
	return template.HTML(out.String())
}

func parseListItem(line string) (ordered bool, item string, ok bool) {
	switch {
	case strings.HasPrefix(line, "• "):
		return false, strings.TrimSpace(strings.TrimPrefix(line, "• ")), true
	case strings.HasPrefix(line, "- "):
		return false, strings.TrimSpace(strings.TrimPrefix(line, "- ")), true
	}
	for i := 0; i < len(line); i++ {
		if line[i] == '.' && i > 0 {
			digits := line[:i]
			if strings.Trim(digits, "0123456789") == "" && len(line) > i+1 && line[i+1] == ' ' {
				return true, strings.TrimSpace(line[i+1:]), true
			}
			break
		}
		if line[i] < '0' || line[i] > '9' {
			break
		}
	}
	return false, "", false
}

func linkifyEscaped(escaped string) string {
	return urlPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		href := strings.TrimSuffix(match, ".")
		suffix := strings.TrimPrefix(match, href)
		return `<a href="` + href + `" target="_blank" rel="noreferrer">` + href + `</a>` + suffix
	})
}

func previewText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func resourceKind(label, href string) string {
	lowerLabel := strings.ToLower(strings.TrimSpace(label))
	lowerHref := strings.ToLower(strings.TrimSpace(href))
	switch {
	case strings.Contains(lowerLabel, "artifact") || strings.Contains(lowerHref, "/ui/artifacts/view"):
		return "artifact"
	case strings.Contains(lowerLabel, "approval") || strings.Contains(lowerHref, "/ui/approvals"):
		return "approval"
	case strings.Contains(lowerLabel, "task") || strings.Contains(lowerHref, "/ui/tasks"):
		return "task"
	default:
		return "link"
	}
}

func resourceIcon(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "artifact":
		return "DOC"
	case "approval":
		return "KEY"
	case "task":
		return "JOB"
	case "action":
		return "RUN"
	default:
		return "LINK"
	}
}

func resourcePreview(label, href string) string {
	kind := resourceKind(label, href)
	switch kind {
	case "artifact":
		return "Open the generated artifact in-place without leaving the conversation. Use this to inspect the actual output before asking for a follow-up."
	case "approval":
		return "Jump to the approval context for a gated root or privileged action. Review before continuing execution."
	case "task":
		return "Open the underlying runtime task with state, skill, metadata, artifacts, and execution history."
	default:
		return "Open the linked runtime resource in a separate management view."
	}
}

func actionPreview(label, href, method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "POST"
	}
	switch {
	case strings.Contains(strings.ToLower(label), "approve"):
		return method + " request that approves the pending action and continues execution."
	case strings.Contains(strings.ToLower(label), "run"):
		return method + " request that asks AgentOS to continue or rerun the selected task."
	default:
		return method + " request against " + href
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func previewURL(href, method string) string {
	values := url.Values{}
	values.Set("href", href)
	if strings.TrimSpace(method) != "" {
		values.Set("method", method)
	}
	return "/ui/preview?" + values.Encode()
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "configured"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func defaultModelConfig(cfg model.Config) model.Config {
	cfg = normalizeProviderPreset(cfg)
	if cfg.Provider == "none" {
		cfg.Provider = "deepseek"
	}
	if cfg.Conversation.MaxTokens <= 0 {
		cfg.Conversation.MaxTokens = 1600
	}
	if cfg.Routing.MaxTokens <= 0 {
		cfg.Routing.MaxTokens = 220
	}
	if cfg.Skills.MaxTokens <= 0 {
		cfg.Skills.MaxTokens = 1200
	}
	if cfg.Conversation.Temperature == 0 {
		cfg.Conversation.Temperature = 0.2
	}
	if cfg.Skills.Temperature == 0 {
		cfg.Skills.Temperature = 0.2
	}
	return cfg
}

func normalizeProviderPreset(cfg model.Config) model.Config {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.Preset = ""
	if cfg.Provider == "" {
		cfg.Provider = "none"
	}
	return cfg
}

func modelConfigFromForm(r *http.Request, current model.Config) (model.Config, error) {
	cfg := defaultModelConfig(current)
	cfg.Provider = strings.ToLower(strings.TrimSpace(r.FormValue("provider")))
	cfg.BaseURL = strings.TrimSpace(r.FormValue("base_url"))
	cfg.APIKey = strings.TrimSpace(r.FormValue("api_key"))
	if cfg.APIKey == "" {
		cfg.APIKey = current.APIKey
	}
	cfg.Conversation.Model = strings.TrimSpace(r.FormValue("conversation_model"))
	cfg.Routing.Model = strings.TrimSpace(r.FormValue("routing_model"))
	cfg.Skills.Model = strings.TrimSpace(r.FormValue("skills_model"))

	var err error
	if cfg.Conversation.MaxTokens, err = parseModelIntField(r, "conversation_max_tokens", cfg.Conversation.MaxTokens); err != nil {
		return model.Config{}, err
	}
	if cfg.Routing.MaxTokens, err = parseModelIntField(r, "routing_max_tokens", cfg.Routing.MaxTokens); err != nil {
		return model.Config{}, err
	}
	if cfg.Skills.MaxTokens, err = parseModelIntField(r, "skills_max_tokens", cfg.Skills.MaxTokens); err != nil {
		return model.Config{}, err
	}
	if cfg.Conversation.Temperature, err = parseModelFloatField(r, "conversation_temperature", cfg.Conversation.Temperature); err != nil {
		return model.Config{}, err
	}
	if cfg.Skills.Temperature, err = parseModelFloatField(r, "skills_temperature", cfg.Skills.Temperature); err != nil {
		return model.Config{}, err
	}

	cfg = normalizeProviderPreset(cfg)
	if cfg.Provider == "none" {
		return model.Config{Provider: "none"}, nil
	}
	return cfg, nil
}

func parseModelIntField(r *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.FormValue(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", strings.ReplaceAll(key, "_", " "))
	}
	return value, nil
}

func parseModelFloatField(r *http.Request, key string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(r.FormValue(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", strings.ReplaceAll(key, "_", " "))
	}
	return value, nil
}

func recallHitTitle(hit recall.Hit) string {
	for _, key := range []string{"title", "subject", "summary", "query"} {
		if value, ok := hit.Card.Content[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return hit.CardType
}

func chatStageLabel(stage string) string {
	switch strings.TrimSpace(stage) {
	case "routing":
		return "Routing"
	case "queued":
		return "Queued"
	case "searching":
		return "Searching"
	case "planning":
		return "Planning"
	case "reading":
		return "Reading"
	case "writing":
		return "Writing"
	case "executing":
		return "Executing"
	case "triaging_email":
		return "Triaging Email"
	case "searching_github":
		return "Searching GitHub"
	case "consolidating":
		return "Consolidating"
	case "summarizing":
		return "Summarizing"
	case "persisting":
		return "Persisting"
	case "writing_memory":
		return "Writing Memory"
	case "running":
		return "Running"
	case "awaiting_approval":
		return "Approval Needed"
	case "blocked":
		return "Blocked"
	case "failed":
		return "Failed"
	case "done":
		return "Done"
	case "responded":
		return "Responded"
	case "working":
		return "Working"
	default:
		if strings.TrimSpace(stage) == "" {
			return ""
		}
		return strings.ReplaceAll(strings.Title(strings.ReplaceAll(stage, "_", " ")), "_", " ")
	}
}

func chatStageClass(stage string) string {
	switch strings.TrimSpace(stage) {
	case "routing", "queued":
		return "stage-warn"
	case "running", "working", "searching", "planning", "reading", "writing", "executing", "triaging_email", "searching_github", "consolidating", "summarizing", "persisting", "writing_memory":
		return "stage-live"
	case "awaiting_approval", "blocked":
		return "stage-alert"
	case "failed":
		return "stage-danger"
	case "done", "responded":
		return "stage-ok"
	default:
		return ""
	}
}

func chatStageStatusText(stage string) string {
	label := chatStageLabel(stage)
	if label == "" {
		return "Ready"
	}
	return label
}

var webTemplates = template.Must(template.New("web").Funcs(template.FuncMap{
	"join":        strings.Join,
	"lower":       strings.ToLower,
	"queryEscape": url.QueryEscape,
	"dict": func(values ...any) map[string]any {
		out := map[string]any{}
		for i := 0; i+1 < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				continue
			}
			out[key] = values[i+1]
		}
		return out
	},
	"hasConfidence": func(value float64) bool {
		return value > 0
	},
	"containsSource": func(values []string, target string) bool {
		for _, value := range values {
			if strings.EqualFold(value, target) {
				return true
			}
		}
		return false
	},
	"preview": func(value string, max int) string {
		value = strings.TrimSpace(value)
		if len(value) <= max {
			return value
		}
		if max <= 3 {
			return value[:max]
		}
		return value[:max-3] + "..."
	},
	"dictPreview": func(values map[string]string) string {
		if len(values) == 0 {
			return ""
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", key, values[key]))
		}
		return strings.Join(parts, ", ")
	},
	"eqFold":                strings.EqualFold,
	"renderChatContentHTML": renderChatContentHTML,
	"resourceKind":          resourceKind,
	"resourceIcon":          resourceIcon,
	"resourcePreview":       resourcePreview,
	"actionPreview":         actionPreview,
	"derefString":           derefString,
	"previewURL":            previewURL,
	"recallHitTitle":        recallHitTitle,
	"chatStageLabel":        chatStageLabel,
	"chatStageClass":        chatStageClass,
	"chatStageStatusText":   chatStageStatusText,
	"recallDetailFields":    recallDetailFields,
	"memoryCardTitle":       memoryCardTitle,
	"memoryEdgePeer":        memoryEdgePeer,
}).Parse(webTemplateHTML))

func wantsJSON(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json")
}

func chatSessionFromForm(r *http.Request) string {
	if r == nil {
		return "default"
	}
	if err := r.ParseForm(); err == nil {
		if sessionID := strings.TrimSpace(r.FormValue("session_id")); sessionID != "" {
			return sessionID
		}
	}
	if sessionID := strings.TrimSpace(r.URL.Query().Get("session")); sessionID != "" {
		return sessionID
	}
	return "default"
}

func redirectChatSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(firstNonEmpty(sessionID, "default")), http.StatusSeeOther)
}

func redirectChatSessionWithError(w http.ResponseWriter, r *http.Request, sessionID string, err error) {
	http.Redirect(w, r, "/ui/chat?session="+url.QueryEscape(firstNonEmpty(sessionID, "default"))+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
}
