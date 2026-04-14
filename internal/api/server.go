package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
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

type Server struct {
	store         *memory.Store
	runtimeStore  *airuntime.Store
	approvalStore *approval.Store
	chatService   *chat.Service
	recallService *recall.Service
	orchestrator  *airuntime.Orchestrator
	executor      *execution.Executor
	skillRunner   *skills.Runner
	modelConfig   *model.ConfigStore
}

func NewServer(store *memory.Store, runtimeStore *airuntime.Store, approvalStore *approval.Store, chatService *chat.Service, recallService *recall.Service, orchestrator *airuntime.Orchestrator, executor *execution.Executor, skillRunner *skills.Runner, modelConfig *model.ConfigStore) *Server {
	return &Server{
		store:         store,
		runtimeStore:  runtimeStore,
		approvalStore: approvalStore,
		chatService:   chatService,
		recallService: recallService,
		orchestrator:  orchestrator,
		executor:      executor,
		skillRunner:   skillRunner,
		modelConfig:   modelConfig,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	s.registerWebRoutes(mux)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /runtime/state", s.handleRuntimeState)
	mux.HandleFunc("GET /skills", s.handleListSkills)
	mux.HandleFunc("GET /skills/schema", s.handleSkillSchema)
	mux.HandleFunc("POST /skills/reload", s.handleReloadSkills)
	mux.HandleFunc("PATCH /skills/", s.handleSkillRoute)
	mux.HandleFunc("PUT /skills/manifests/", s.handleSkillManifestRoute)
	mux.HandleFunc("GET /tasks", s.handleListTasks)
	mux.HandleFunc("POST /tasks", s.handleCreateTask)
	mux.HandleFunc("GET /tasks/", s.handleTaskRoute)
	mux.HandleFunc("POST /tasks/", s.handleTaskRoute)
	mux.HandleFunc("POST /actions/shell", s.handleExecuteShell)
	mux.HandleFunc("POST /actions/file-read", s.handleExecuteFileRead)
	mux.HandleFunc("POST /actions/file-write", s.handleExecuteFileWrite)
	mux.HandleFunc("GET /actions/", s.handleGetAction)
	mux.HandleFunc("PUT /cards", s.handleCreateCard)
	mux.HandleFunc("PATCH /cards/", s.handleUpdateCard)
	mux.HandleFunc("POST /edges", s.handleCreateEdge)
	mux.HandleFunc("GET /query", s.handleQuery)
	mux.HandleFunc("GET /recall", s.handleRecall)
	mux.HandleFunc("GET /chat/messages", s.handleListChatMessages)
	mux.HandleFunc("POST /chat", s.handleSendChatMessage)
	mux.HandleFunc("GET /approvals", s.handleListApprovals)
	mux.HandleFunc("POST /approvals/root-actions", s.handleCreateRootApproval)
	mux.HandleFunc("GET /approvals/", s.handleApprovalRoute)
	mux.HandleFunc("POST /approvals/", s.handleApprovalRoute)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRuntimeState(w http.ResponseWriter, _ *http.Request) {
	state, err := s.runtimeStore.LoadState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleListSkills(w http.ResponseWriter, _ *http.Request) {
	if s.skillRunner == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"skills":            []skills.Definition{},
			"manifest_statuses": []skills.ManifestStatus{},
			"health":            skillHealth(nil, nil),
			"schema":            skillSchema(),
		})
		return
	}
	skillsList := s.skillRunner.ListSkills()
	statuses := s.skillRunner.ListManifestStatuses()
	writeJSON(w, http.StatusOK, map[string]any{
		"skills":            skillsList,
		"manifest_statuses": statuses,
		"health":            skillHealth(skillsList, statuses),
		"schema":            skillSchema(),
	})
}

func (s *Server) handleSkillSchema(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"schema": skillSchema()})
}

func (s *Server) handleReloadSkills(w http.ResponseWriter, _ *http.Request) {
	if s.skillRunner == nil {
		writeError(w, http.StatusNotImplemented, "skill runner is not configured")
		return
	}
	var reloadErr string
	if err := s.skillRunner.ReloadSkills(); err != nil {
		reloadErr = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"skills":            s.skillRunner.ListSkills(),
		"manifest_statuses": s.skillRunner.ListManifestStatuses(),
		"health":            skillHealth(s.skillRunner.ListSkills(), s.skillRunner.ListManifestStatuses()),
		"schema":            skillSchema(),
		"error":             reloadErr,
	})
}

func (s *Server) handleSkillRoute(w http.ResponseWriter, r *http.Request) {
	if s.skillRunner == nil {
		writeError(w, http.StatusNotImplemented, "skill runner is not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/skills/")
	name := strings.Trim(strings.TrimSuffix(path, "/"), "/")
	if name == "" {
		writeError(w, http.StatusNotFound, "skill route not found")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.skillRunner.SetSkillEnabled(name, req.Enabled); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"skills":            s.skillRunner.ListSkills(),
		"manifest_statuses": s.skillRunner.ListManifestStatuses(),
		"health":            skillHealth(s.skillRunner.ListSkills(), s.skillRunner.ListManifestStatuses()),
		"schema":            skillSchema(),
	})
}

func (s *Server) handleSkillManifestRoute(w http.ResponseWriter, r *http.Request) {
	if s.skillRunner == nil {
		writeError(w, http.StatusNotImplemented, "skill runner is not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/skills/manifests/")
	name := strings.Trim(strings.TrimSuffix(path, "/"), "/")
	if name == "" {
		writeError(w, http.StatusNotFound, "skill manifest route not found")
		return
	}
	var manifest skills.Manifest
	if err := decodeJSON(r, &manifest); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(manifest.Name) == "" {
		manifest.Name = name
	}
	if strings.TrimSpace(manifest.Name) != name {
		writeError(w, http.StatusBadRequest, "manifest name does not match path")
		return
	}
	pathSaved, err := s.skillRunner.SaveManifest(manifest)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	skillsList := s.skillRunner.ListSkills()
	statuses := s.skillRunner.ListManifestStatuses()
	writeJSON(w, http.StatusOK, map[string]any{
		"path":              pathSaved,
		"skills":            skillsList,
		"manifest_statuses": statuses,
		"health":            skillHealth(skillsList, statuses),
		"schema":            skillSchema(),
	})
}

func skillHealth(defs []skills.Definition, statuses []skills.ManifestStatus) map[string]any {
	bySource := map[string]int{}
	enabled := 0
	disabled := 0
	for _, def := range defs {
		bySource[firstNonEmpty(def.Source, "runtime")]++
		if def.Enabled {
			enabled++
		} else {
			disabled++
		}
	}
	manifestLoaded := 0
	manifestErrors := 0
	for _, status := range statuses {
		if status.Loaded {
			manifestLoaded++
		}
		if strings.TrimSpace(status.Error) != "" {
			manifestErrors++
		}
	}
	return map[string]any{
		"total_skills":    len(defs),
		"enabled_skills":  enabled,
		"disabled_skills": disabled,
		"by_source":       bySource,
		"manifest_loaded": manifestLoaded,
		"manifest_errors": manifestErrors,
		"manifest_total":  len(statuses),
	}
}

func skillSchema() map[string]any {
	return map[string]any{
		"execution_profiles": []string{"user", "root"},
		"maintenance_scopes": []string{"project", "user"},
		"external_kinds":     []string{"command"},
		"manifest_fields": []map[string]any{
			{"name": "name", "required": true, "type": "string"},
			{"name": "description", "required": false, "type": "string"},
			{"name": "uses", "required": false, "type": "string", "notes": "mutually exclusive with external"},
			{"name": "enabled", "required": false, "type": "bool"},
			{"name": "default_metadata", "required": false, "type": "map[string]string"},
			{"name": "execution_profile", "required": false, "type": "enum", "values": []string{"user", "root"}},
			{"name": "maintenance_policy", "required": false, "type": "object"},
			{"name": "external", "required": false, "type": "object", "notes": "mutually exclusive with uses"},
		},
		"external_fields": []map[string]any{
			{"name": "kind", "required": true, "type": "enum", "values": []string{"command"}},
			{"name": "command", "required": true, "type": "relative-path"},
			{"name": "args", "required": false, "type": "[]string"},
			{"name": "env", "required": false, "type": "map[string]string"},
			{"name": "workdir", "required": false, "type": "relative-path"},
			{"name": "timeout_ms", "required": false, "type": "int"},
			{"name": "allow_write_root", "required": false, "type": "bool"},
			{"name": "require_approval", "required": false, "type": "bool"},
		},
	}
}

func (s *Server) handleListTasks(w http.ResponseWriter, _ *http.Request) {
	tasks, err := s.runtimeStore.ListTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req airuntime.CreateTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err := s.orchestrator.SubmitTask(req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, airuntime.ErrRuntimeInvalidInput) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (s *Server) handleTaskRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/tasks/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "task route not found")
		return
	}

	taskID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		task, err := s.runtimeStore.GetTask(taskID)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, airuntime.ErrTaskNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, task)
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "approve":
			s.handleApproveTask(w, r, taskID)
			return
		case "deny":
			s.handleDenyTask(w, r, taskID)
			return
		case "run":
			s.handleRunTask(w, r, taskID)
			return
		}
	}

	writeError(w, http.StatusNotFound, "task route not found")
}

func (s *Server) handleApproveTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req airuntime.ApproveTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err := s.orchestrator.ApproveTask(taskID, req.ApprovedBy)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, airuntime.ErrTaskNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleDenyTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req airuntime.DenyTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err := s.orchestrator.DenyTask(taskID, req.DeniedBy, req.Reason)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, airuntime.ErrTaskNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleRunTask(w http.ResponseWriter, r *http.Request, taskID string) {
	if task, err := s.runtimeStore.GetTask(taskID); err == nil {
		switch task.State {
		case airuntime.TaskStateFailed, airuntime.TaskStateBlocked:
			if _, err := s.orchestrator.RequeueTask(taskID, "api_run", nil); err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
		}
	}
	result, err := s.skillRunner.RunTask(taskID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, airuntime.ErrTaskNotFound) {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "awaiting approval") || strings.Contains(err.Error(), "not runnable") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleExecuteShell(w http.ResponseWriter, r *http.Request) {
	var req execution.ShellActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := s.executor.ExecuteShell(req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, execution.ErrExecutionInvalidInput) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, execution.ErrExecutionDenied) {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) handleExecuteFileRead(w http.ResponseWriter, r *http.Request) {
	var req execution.FileReadActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := s.executor.ExecuteFileRead(req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, execution.ErrExecutionInvalidInput) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, execution.ErrExecutionDenied) {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) handleExecuteFileWrite(w http.ResponseWriter, r *http.Request) {
	var req execution.FileWriteActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := s.executor.ExecuteFileWrite(req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, execution.ErrExecutionInvalidInput) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, execution.ErrExecutionDenied) {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) handleGetAction(w http.ResponseWriter, r *http.Request) {
	actionID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/actions/"), "/")
	if actionID == "" {
		writeError(w, http.StatusNotFound, "action route not found")
		return
	}
	record, err := s.executor.GetAction(actionID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, execution.ErrActionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleCreateCard(w http.ResponseWriter, r *http.Request) {
	var req memory.CreateCardRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	card, err := s.store.CreateCard(req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, card)
}

func (s *Server) handleUpdateCard(w http.ResponseWriter, r *http.Request) {
	cardID := strings.TrimPrefix(r.URL.Path, "/cards/")
	var req memory.UpdateCardRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	card, err := s.store.UpdateCard(cardID, req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func (s *Server) handleCreateEdge(w http.ResponseWriter, r *http.Request) {
	var req memory.CreateEdgeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	edge, err := s.store.CreateEdge(req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, edge)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	req := memory.QueryRequest{
		CardID:   r.URL.Query().Get("card_id"),
		CardType: r.URL.Query().Get("card_type"),
		Scope:    r.URL.Query().Get("scope"),
	}

	if asOfText := r.URL.Query().Get("as_of"); asOfText != "" {
		asOf, err := time.Parse(time.RFC3339, asOfText)
		if err != nil {
			writeError(w, http.StatusBadRequest, "as_of must be RFC3339")
			return
		}
		req.AsOf = &asOf
	}

	writeJSON(w, http.StatusOK, s.store.Query(req))
}

func (s *Server) handleRecall(w http.ResponseWriter, r *http.Request) {
	if s.recallService == nil {
		writeError(w, http.StatusNotImplemented, "recall service is not configured")
		return
	}
	limit := 10
	if text := strings.TrimSpace(r.URL.Query().Get("limit")); text != "" {
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
	}
	sources := make([]string, 0)
	for _, value := range r.URL.Query()["source"] {
		for _, source := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(source); trimmed != "" {
				sources = append(sources, trimmed)
			}
		}
	}
	resp := s.recallService.Recall(recall.Request{
		Query:   r.URL.Query().Get("query"),
		Sources: sources,
		Limit:   limit,
	})
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListChatMessages(w http.ResponseWriter, r *http.Request) {
	if s.chatService == nil {
		writeError(w, http.StatusNotImplemented, "chat service is not configured")
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session"))
	messages, err := s.chatService.Messages(sessionID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (s *Server) handleSendChatMessage(w http.ResponseWriter, r *http.Request) {
	if s.chatService == nil {
		writeError(w, http.StatusNotImplemented, "chat service is not configured")
		return
	}
	var req chat.SendRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.chatService.Send(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	if s.approvalStore == nil {
		writeError(w, http.StatusNotImplemented, "approval flow is not configured")
		return
	}
	records, err := s.approvalStore.List(r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": records})
}

func (s *Server) handleCreateRootApproval(w http.ResponseWriter, r *http.Request) {
	if s.approvalStore == nil {
		writeError(w, http.StatusNotImplemented, "approval flow is not configured")
		return
	}
	var req approval.CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.ExecutionProfile = "root"
	record, err := s.approvalStore.Create(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) handleApprovalRoute(w http.ResponseWriter, r *http.Request) {
	if s.approvalStore == nil {
		writeError(w, http.StatusNotImplemented, "approval flow is not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/approvals/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "approval route not found")
		return
	}
	approvalID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		record, err := s.approvalStore.Get(approvalID)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, approval.ErrApprovalNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, record)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "approve":
			s.handleApproveApproval(w, r, approvalID)
			return
		case "deny":
			s.handleDenyApproval(w, r, approvalID)
			return
		}
	}
	writeError(w, http.StatusNotFound, "approval route not found")
}

func (s *Server) handleApproveApproval(w http.ResponseWriter, r *http.Request, approvalID string) {
	var req struct {
		ApprovedBy string `json:"approved_by,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := s.approvalStore.Approve(approvalID, req.ApprovedBy)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, approval.ErrApprovalNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, approval.ErrApprovalDenied) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	if record.TaskID != "" {
		_, _ = s.orchestrator.RequeueTask(record.TaskID, "approval_granted", func(task *airuntime.Task) {
			ensureTaskMetadata(task)
			task.Metadata["root_approval_id"] = record.ApprovalID
			task.NextAction = "root approval granted; rerun task"
		})
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleDenyApproval(w http.ResponseWriter, r *http.Request, approvalID string) {
	var req struct {
		DeniedBy string `json:"denied_by,omitempty"`
		Reason   string `json:"reason,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := s.approvalStore.Deny(approvalID, req.DeniedBy, req.Reason)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, approval.ErrApprovalNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, approval.ErrApprovalDenied) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	if record.TaskID != "" {
		_, _ = s.runtimeStore.MoveTask(record.TaskID, airuntime.TaskStateBlocked, func(task *airuntime.Task) {
			ensureTaskMetadata(task)
			delete(task.Metadata, "approval_token")
			task.Metadata["root_approval_id"] = record.ApprovalID
			task.FailureReason = firstNonEmpty(record.DeniedReason, "root approval denied")
			task.NextAction = "root approval denied"
		})
	}
	writeJSON(w, http.StatusOK, record)
}

func ensureTaskMetadata(task *airuntime.Task) {
	if task.Metadata == nil {
		task.Metadata = map[string]string{}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
