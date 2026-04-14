package api

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestDashboardPageRenders(t *testing.T) {
	handler, runtimeStore, _, _, orchestrator, _, _, _ := newWebTestServer(t)

	if _, err := orchestrator.SubmitTask(airuntime.CreateTaskRequest{
		Title:       "Inspect dashboard",
		Goal:        "Plan the next repository step",
		RequestedBy: "web-test",
		Source:      "web-test",
	}); err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	task, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:       "Pending follow-up",
		Goal:        "Wait for review",
		RequestedBy: "web-test",
		Source:      "web-test",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if _, err := runtimeStore.MoveTask(task.TaskID, airuntime.TaskStatePlanned, func(task *airuntime.Task) {
		task.SelectedSkill = "task-plan"
		task.NextAction = "wait for rerun"
	}); err != nil {
		t.Fatalf("MoveTask returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "AgentOS Dashboard") {
		t.Fatalf("expected dashboard heading in response body")
	}
	if !strings.Contains(body, task.TaskID) {
		t.Fatalf("expected task id in dashboard response")
	}
	if !strings.Contains(body, "Runtime Focus") || !strings.Contains(body, "Task Queue") {
		t.Fatalf("expected dashboard overview panels in response body")
	}
}

func TestCreateTaskFormSubmitsTask(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	form := url.Values{
		"title":             {"Search approvals"},
		"goal":              {"Search the web for approval flow positioning"},
		"requested_by":      {"web-console"},
		"source":            {"web-console"},
		"execution_profile": {"user"},
		"query":             {"approval agentos"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/tasks", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after task creation, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "/ui/tasks?task_id=") {
		t.Fatalf("expected redirect to task detail, got %q", location)
	}

	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}
	if tasks[0].SelectedSkill != "web-search" {
		t.Fatalf("expected web-search skill, got %s", tasks[0].SelectedSkill)
	}
	if tasks[0].Metadata["query"] != "approval agentos" {
		t.Fatalf("expected task metadata query to be persisted")
	}
}

func TestListSkillsEndpointIncludesBuiltinAndManifestSkills(t *testing.T) {
	handler, runtimeStore, _, _, _, _, skillRunner, _ := newWebTestServer(t)

	manifestDir := filepath.Join(runtimeStore.RootDir(), "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	raw := []byte("{\n  \"name\": \"web-research\",\n  \"description\": \"Manifest alias for web search.\",\n  \"uses\": \"web-search\"\n}\n")
	if err := os.WriteFile(filepath.Join(manifestDir, "web-research.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := skillRunner.LoadSkillManifests(manifestDir); err != nil {
		t.Fatalf("LoadSkillManifests returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload struct {
		Skills []skills.Definition `json:"skills"`
		Schema map[string]any      `json:"schema"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Skills) == 0 {
		t.Fatalf("expected skills to be listed")
	}
	foundBuiltin := false
	foundManifest := false
	for _, skill := range payload.Skills {
		if skill.Name == "web-search" && skill.Source == "builtin" {
			foundBuiltin = true
		}
		if skill.Name == "web-research" && skill.Source == "manifest" && skill.Uses == "web-search" {
			foundManifest = true
		}
	}
	if !foundBuiltin || !foundManifest {
		t.Fatalf("expected builtin and manifest skills in payload, got %+v", payload.Skills)
	}
}

func TestReloadSkillsEndpointLoadsManifest(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	manifestDir := filepath.Join(runtimeStore.RootDir(), "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	raw := []byte("{\n  \"name\": \"web-research\",\n  \"description\": \"Manifest alias for web search.\",\n  \"uses\": \"web-search\"\n}\n")
	if err := os.WriteFile(filepath.Join(manifestDir, "web-research.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/skills/reload", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload struct {
		Skills []skills.Definition `json:"skills"`
		Schema map[string]any      `json:"schema"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	found := false
	for _, skill := range payload.Skills {
		if skill.Name == "web-research" && skill.Source == "manifest" && skill.Uses == "web-search" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected reloaded manifest skill in payload, got %+v", payload.Skills)
	}
	if payload.Schema["external_kinds"] == nil {
		t.Fatalf("expected schema in response, got %+v", payload.Schema)
	}
}

func TestPatchSkillEndpointPersistsEnableState(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	manifestDir := filepath.Join(runtimeStore.RootDir(), "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	raw := []byte("{\n  \"name\": \"web-research\",\n  \"description\": \"Manifest alias for web search.\",\n  \"uses\": \"web-search\"\n}\n")
	if err := os.WriteFile(filepath.Join(manifestDir, "web-research.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/skills/reload", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected reload status 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/skills/web-research", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected patch status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Skills []skills.Definition `json:"skills"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	foundDisabled := false
	for _, skill := range payload.Skills {
		if skill.Name == "web-research" && !skill.Enabled {
			foundDisabled = true
			break
		}
	}
	if !foundDisabled {
		t.Fatalf("expected manifest skill to be disabled, got %+v", payload.Skills)
	}

	stateData, err := os.ReadFile(filepath.Join(runtimeStore.RootDir(), "state", "skills.json"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(stateData), "\"web-research\": false") {
		t.Fatalf("expected persisted disabled state, got %q", string(stateData))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/skills/reload", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected reload status 200, got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	foundDisabled = false
	for _, skill := range payload.Skills {
		if skill.Name == "web-research" && !skill.Enabled {
			foundDisabled = true
			break
		}
	}
	if !foundDisabled {
		t.Fatalf("expected disabled state to survive reload, got %+v", payload.Skills)
	}
}

func TestListSkillsEndpointIncludesHealthAndSchema(t *testing.T) {
	handler, _, _, _, _, _, _, _ := newWebTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/skills", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload struct {
		Health map[string]any `json:"health"`
		Schema map[string]any `json:"schema"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Health["total_skills"] == nil {
		t.Fatalf("expected health payload, got %+v", payload.Health)
	}
	if payload.Schema["manifest_fields"] == nil {
		t.Fatalf("expected schema payload, got %+v", payload.Schema)
	}
}

func TestSaveSkillManifestEndpointCreatesManifest(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	body := `{"description":"Alias","uses":"web-search"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/skills/manifests/web-research", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(runtimeStore.RootDir(), "skills", "web-research.json")); err != nil {
		t.Fatalf("expected manifest file to be written: %v", err)
	}
}

func TestSkillsPageLoadsExistingManifestIntoEditor(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	manifestDir := filepath.Join(runtimeStore.RootDir(), "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	raw := "{\n  \"version\": 1,\n  \"name\": \"web-research\",\n  \"uses\": \"web-search\"\n}\n"
	if err := os.WriteFile(filepath.Join(manifestDir, "web-research.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/skills?manifest=web-research", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := html.UnescapeString(rec.Body.String())
	if !strings.Contains(body, "\"name\": \"web-research\"") || !strings.Contains(body, "\"version\": 1") {
		t.Fatalf("expected manifest content in editor, got %q", body)
	}
}

func TestSkillsPageRendersRegistryAndManifestHealth(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	manifestDir := filepath.Join(runtimeStore.RootDir(), "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "bad-skill.json"), []byte("{\n  \"name\": \"Bad Skill\",\n  \"uses\": \"web-search\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/skills/reload", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after reload, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.Contains(got, "/ui/skills?error=") {
		t.Fatalf("expected error redirect, got %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/skills", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Skills") || !strings.Contains(body, "Manifest Health") {
		t.Fatalf("expected skills management sections in page")
	}
	if !strings.Contains(body, "web-search") {
		t.Fatalf("expected builtin skill in page")
	}
	if !strings.Contains(body, "bad-skill.json") {
		t.Fatalf("expected manifest status path in page")
	}
}

func TestSkillsPageToggleDisablesBuiltinSkill(t *testing.T) {
	handler, _, _, _, _, _, runner, _ := newWebTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/skills/web-search/toggle", strings.NewReader("enabled=false"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after toggle, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.Contains(got, "/ui/skills?success=") {
		t.Fatalf("expected success redirect, got %q", got)
	}
	foundDisabled := false
	for _, skill := range runner.ListSkills() {
		if skill.Name == "web-search" && !skill.Enabled {
			foundDisabled = true
		}
	}
	if !foundDisabled {
		t.Fatalf("expected web-search to be disabled after toggle, got %+v", runner.ListSkills())
	}
}

func TestSkillsPageSavesManifest(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	form := url.Values{
		"manifest_json": {`{"name":"web-research","description":"Alias","uses":"web-search"}`},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/skills/manifests", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after save, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.Contains(got, "/ui/skills?success=") {
		t.Fatalf("expected success redirect, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(runtimeStore.RootDir(), "skills", "web-research.json")); err != nil {
		t.Fatalf("expected manifest file to exist: %v", err)
	}
}

func TestChatPageSendsMessage(t *testing.T) {
	handler, runtimeStore, _, memoryStore, _, _, _, _ := newWebTestServer(t)

	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "search:test:summary",
		CardType: "search_summary",
		Content: map[string]any{
			"summary": "Repository planning context",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	form := url.Values{
		"session_id":        {"default"},
		"message":           {"Plan the next repository step with repository planning context"},
		"requested_by":      {"web-chat"},
		"source":            {"web-chat"},
		"execution_profile": {"user"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/chat", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after chat send, got %d", rec.Code)
	}

	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task created by chat, got %d", len(tasks))
	}
	if tasks[0].SelectedSkill != "task-plan" {
		t.Fatalf("expected task-plan skill, got %s", tasks[0].SelectedSkill)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/chat", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Conversation") {
		t.Fatalf("expected chat page to render conversation")
	}
	if !strings.Contains(body, "Conversations") {
		t.Fatalf("expected conversations sidebar in chat page")
	}
	if !strings.Contains(body, `<body class="chat-body">`) {
		t.Fatalf("expected chat body class in chat page")
	}
	if !strings.Contains(body, `<main class="chat-main-shell">`) {
		t.Fatalf("expected chat main shell class in chat page")
	}
	if !strings.Contains(body, "Work completed.") {
		t.Fatalf("expected assistant completion reply in chat page")
	}
	if !strings.Contains(body, "task_request") {
		t.Fatalf("expected task intent indicator in chat page")
	}
	if !strings.Contains(body, "/ui/artifacts/view?path=") {
		t.Fatalf("expected artifact link in chat page")
	}
}

func TestQueryAPIHonorsScopeFilter(t *testing.T) {
	handler, _, _, memoryStore, _, _, _, _ := newWebTestServer(t)

	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "search:test:summary",
		CardType: "search_summary",
		Scope:    memory.ScopeProject,
		Content: map[string]any{
			"summary": "Project-scoped search memory",
		},
	}); err != nil {
		t.Fatalf("CreateCard project returned error: %v", err)
	}
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "email:test:summary",
		CardType: "email_summary",
		Scope:    memory.ScopeUser,
		Content: map[string]any{
			"summary": "User-scoped email memory",
		},
	}); err != nil {
		t.Fatalf("CreateCard user returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/query?scope=project", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp memory.QueryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal query response: %v", err)
	}
	if len(resp.Cards) != 1 {
		t.Fatalf("expected one project-scoped card, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Scope != memory.ScopeProject {
		t.Fatalf("expected project scope, got %q", resp.Cards[0].Scope)
	}
}

func TestChatPageRendersSessionState(t *testing.T) {
	handler, _, _, _, _, _, _, chatStore := newWebTestServer(t)

	if err := chatStore.SaveSessionState(chat.SessionState{
		SessionID:        "default",
		Topic:            "OpenClaw memory design",
		FocusTaskID:      "task-123",
		PendingQuestion:  "需要我继续总结吗？",
		PendingAction:    "summarize_focus_task",
		LastUserAct:      "confirm",
		LastAssistantAct: "ask_followup",
		WorkingSet: chat.SessionWorkset{
			ArtifactPaths: []string{"/tmp/artifacts/demo.md"},
			RecallCardIDs: []string{"search:test:summary"},
			SourceRefs:    []string{"web"},
		},
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveSessionState returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/chat?session=default", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Session State") {
		t.Fatalf("expected session state panel in chat page")
	}
	if !strings.Contains(body, "OpenClaw memory design") {
		t.Fatalf("expected topic in session state panel")
	}
	if !strings.Contains(body, "需要我继续总结吗？") {
		t.Fatalf("expected pending question in session state panel")
	}
}

func TestRenderChatContentHTMLFormatsAssistantText(t *testing.T) {
	got := string(renderChatContentHTML("assistant", "第一段\n\n1. 重点\n• 项目\nhttps://example.com"))
	if strings.Contains(got, "**") || strings.Contains(got, "```") {
		t.Fatalf("expected sanitized rich text, got %q", got)
	}
	if !strings.Contains(got, "<ol>") {
		t.Fatalf("expected ordered list in rendered html, got %q", got)
	}
	if !strings.Contains(got, "<ul>") {
		t.Fatalf("expected unordered list in rendered html, got %q", got)
	}
	if !strings.Contains(got, "<a href=\"https://example.com\"") {
		t.Fatalf("expected linkified url in rendered html, got %q", got)
	}
}

func TestChatSessionLifecycleRoutes(t *testing.T) {
	handler, _, _, _, _, _, _, _ := newWebTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/chat/sessions", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after session create, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "/ui/chat?session=") {
		t.Fatalf("expected redirect to chat session, got %q", location)
	}
	sessionID := strings.TrimPrefix(location, "/ui/chat?session=")
	sessionID, _ = url.QueryUnescape(sessionID)

	form := url.Values{"title": {"Renamed Session"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ui/chat/sessions/"+sessionID+"/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after session rename, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/chat?session="+url.QueryEscape(sessionID), nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 after rename, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Renamed Session") {
		t.Fatalf("expected renamed session title in chat page")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ui/chat/sessions/"+sessionID+"/archive", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after session archive, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/chat", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 after archive, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Archived Sessions") {
		t.Fatalf("expected archived session section in chat page")
	}
	if !strings.Contains(body, "/ui/chat/sessions/"+sessionID+"/restore") {
		t.Fatalf("expected archived session restore action in chat page")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ui/chat/sessions/"+sessionID+"/restore", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after restore, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/ui/chat?session="+url.QueryEscape(sessionID) {
		t.Fatalf("expected redirect back to restored session, got %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/chat?session="+url.QueryEscape(sessionID), nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 after restore, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Renamed Session") {
		t.Fatalf("expected restored session title in chat page")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ui/chat/sessions", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after second session create, got %d", rec.Code)
	}
	location = rec.Header().Get("Location")
	deleteID := strings.TrimPrefix(location, "/ui/chat?session=")
	deleteID, _ = url.QueryUnescape(deleteID)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ui/chat/sessions/"+deleteID+"/delete", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after session delete, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/ui/chat?session=default" {
		t.Fatalf("expected redirect to default after delete, got %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/chat?session=default", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 after delete, got %d", rec.Code)
	}
}

func TestModelsPageRendersAndUpdates(t *testing.T) {
	handler, _, _, _, _, _, _, _ := newWebTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/models", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Model Settings") {
		t.Fatalf("expected models page heading")
	}
	if !strings.Contains(body, "Conversation Model") {
		t.Fatalf("expected conversation profile fields")
	}

	form := url.Values{
		"provider":                 {"openai-compatible"},
		"base_url":                 {"https://api.openai.com/v1"},
		"api_key":                  {"test-key"},
		"conversation_model":       {"gpt-4.1-mini"},
		"conversation_max_tokens":  {"1200"},
		"conversation_temperature": {"0.55"},
		"routing_model":            {"gpt-4.1-mini"},
		"routing_max_tokens":       {"280"},
		"skills_model":             {"gpt-4.1-mini"},
		"skills_max_tokens":        {"800"},
		"skills_temperature":       {"0.35"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ui/models", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after model settings save, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.Contains(got, "/ui/models?success=") {
		t.Fatalf("expected success redirect, got %q", got)
	}
}

func TestModelsPageTestConnection(t *testing.T) {
	handler, _, _, _, _, _, _, _ := newWebTestServer(t)
	transport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if payload["model"] != "test-model" {
			t.Fatalf("expected model test-model, got %#v", payload["model"])
		}
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Type", "application/json")
		rec.WriteHeader(http.StatusOK)
		_, _ = rec.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"model connection ok"}}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`))
		return rec.Result(), nil
	})
	defer func() { http.DefaultTransport = transport }()

	form := url.Values{
		"provider":                 {"openai-compatible"},
		"base_url":                 {"https://model.test/v1"},
		"api_key":                  {"test-key"},
		"conversation_model":       {"test-model"},
		"conversation_max_tokens":  {"900"},
		"conversation_temperature": {"0.30"},
		"routing_model":            {"test-model"},
		"routing_max_tokens":       {"220"},
		"skills_model":             {"test-model"},
		"skills_max_tokens":        {"640"},
		"skills_temperature":       {"0.20"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/models/test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after model connectivity test, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.Contains(got, "/ui/models?test=") {
		t.Fatalf("expected test redirect, got %q", got)
	}
}

func TestChatApproveAndContinueRouteRunsTask(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	form := url.Values{
		"session_id":        {"default"},
		"message":           {"Plan the next repository step"},
		"requested_by":      {"web-chat"},
		"source":            {"web-chat"},
		"execution_profile": {"root"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/chat", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after chat send, got %d", rec.Code)
	}

	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one root task, got %d", len(tasks))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ui/chat/tasks/"+tasks[0].TaskID+"/approve-run", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after approve-run, got %d", rec.Code)
	}

	task, err := runtimeStore.GetTask(tasks[0].TaskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if task.State != airuntime.TaskStateDone {
		t.Fatalf("expected task done after approve-run, got %s", task.State)
	}
}

func TestApprovalsPageRendersPendingApproval(t *testing.T) {
	handler, _, approvalStore, _, _, _, _, _ := newWebTestServer(t)

	record, err := approvalStore.Create(approval.CreateRequest{
		TaskID:           "task-root-1",
		ExecutionProfile: "root",
		ActionKind:       "file_write",
		Summary:          "Root write to /etc/hosts",
		RequestedBy:      "web-test",
		Metadata: map[string]string{
			"path":  "/etc/hosts",
			"skill": "file-edit",
		},
	})
	if err != nil {
		t.Fatalf("Create approval returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/approvals?approval_id="+record.ApprovalID, nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Approvals") {
		t.Fatalf("expected approvals heading in response body")
	}
	if !strings.Contains(body, record.Summary) {
		t.Fatalf("expected approval summary in response body")
	}
	if !strings.Contains(body, "Approve") {
		t.Fatalf("expected approval action button in response body")
	}
	if !strings.Contains(body, "path=/etc/hosts") {
		t.Fatalf("expected approval metadata in response body")
	}
}

func TestRecallPageRendersHits(t *testing.T) {
	handler, _, _, memoryStore, _, _, _, _ := newWebTestServer(t)

	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "email:test:summary",
		CardType: "email_summary",
		Content: map[string]any{
			"summary": "Approval thread requires root review",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/recall?query=approval&source=email", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Recall") {
		t.Fatalf("expected recall heading in response body")
	}
	if !strings.Contains(body, "Total Hits") {
		t.Fatalf("expected recall summary cards in response body")
	}
	if !strings.Contains(strings.ToLower(body), "approval") {
		t.Fatalf("expected recall hit snippet in response body")
	}
	if !strings.Contains(body, "Hit Detail") {
		t.Fatalf("expected recall detail panel in response body")
	}
}

func TestRecallPageSelectsRequestedHit(t *testing.T) {
	handler, _, _, memoryStore, _, _, _, _ := newWebTestServer(t)

	for _, card := range []memory.CreateCardRequest{
		{
			CardID:   "email:test:summary",
			CardType: "email_summary",
			Content: map[string]any{
				"summary": "Approval thread requires root review",
			},
		},
		{
			CardID:   "github:test:issue",
			CardType: "github_issue",
			Content: map[string]any{
				"title":   "Approval flow issue",
				"summary": "Root review can be improved",
			},
		},
	} {
		if _, err := memoryStore.CreateCard(card); err != nil {
			t.Fatalf("CreateCard returned error: %v", err)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/recall?query=approval&card_id=github:test:issue", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "github:test:issue") {
		t.Fatalf("expected selected recall hit card id in detail panel")
	}
	if !strings.Contains(body, "Approval flow issue") {
		t.Fatalf("expected selected recall hit title in detail panel")
	}
}

func TestMemoryPageRendersCards(t *testing.T) {
	handler, _, _, memoryStore, _, _, _, _ := newWebTestServer(t)

	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "github:test:summary",
		CardType: "github_issue_summary",
		Content: map[string]any{
			"summary": "AgentOS control plane issue review",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "github:test:issue",
		CardType: "github_issue",
		Content: map[string]any{
			"title": "Control plane issue",
		},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	if _, err := memoryStore.CreateEdge(memory.CreateEdgeRequest{
		EdgeID:     "edge:github:test:summary",
		FromCardID: "github:test:summary",
		ToCardID:   "github:test:issue",
		EdgeType:   "github_issue",
	}); err != nil {
		t.Fatalf("CreateEdge returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/memory?card_id=github:test:summary", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Memory") {
		t.Fatalf("expected memory heading in response body")
	}
	if !strings.Contains(body, "github_issue_summary") {
		t.Fatalf("expected card type in memory response body")
	}
	if !strings.Contains(body, "github:test:summary") {
		t.Fatalf("expected card id in memory response body")
	}
	if !strings.Contains(body, "Card Queue") || !strings.Contains(body, "Card Detail") {
		t.Fatalf("expected memory queue and detail layout in response body")
	}
	if !strings.Contains(body, "edge:github:test:summary") && !strings.Contains(body, "github_issue") {
		t.Fatalf("expected edge context in memory response body")
	}
	if !strings.Contains(body, "Relationships") {
		t.Fatalf("expected relationship section in memory response body")
	}
}

func TestArtifactViewSupportsRawAndDownload(t *testing.T) {
	handler, runtimeStore, _, _, orchestrator, _, runner, _ := newWebTestServer(t)

	task, err := orchestrator.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Plan next step",
		Goal:  "Plan the next repository step",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if len(result.ArtifactPaths) == 0 {
		t.Fatalf("expected artifact path from task-plan")
	}
	artifactPath := result.ArtifactPaths[0]

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/artifacts/view?path="+url.QueryEscape(artifactPath)+"&raw=1", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected raw artifact status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Task Plan") {
		t.Fatalf("expected raw artifact content")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/artifacts/view?path="+url.QueryEscape(artifactPath)+"&download=1", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected download artifact status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), filepath.Base(artifactPath)) {
		t.Fatalf("expected content disposition filename")
	}

	_ = runtimeStore
}

func TestPreviewEndpointRendersTaskPreview(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	task, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:            "Inspect runtime preview",
		Goal:             "Review the active task preview payload",
		ExecutionProfile: "user",
		SelectedSkill:    "task-plan",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if _, err := runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateActive, func(task *airuntime.Task) {
		task.SelectedSkill = "task-plan"
		task.NextAction = "render hover preview"
	}); err != nil {
		t.Fatalf("MoveTask returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/preview?href="+url.QueryEscape("/ui/tasks?task_id="+task.TaskID), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Inspect runtime preview") || !strings.Contains(body, "render hover preview") {
		t.Fatalf("expected task preview content, got %q", body)
	}
}

func TestPreviewEndpointRendersArtifactPreview(t *testing.T) {
	handler, runtimeStore, _, _, _, _, _, _ := newWebTestServer(t)

	artifactPath := filepath.Join(runtimeStore.RootDir(), "artifacts", "reports", "preview.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("OpenClaw memory design summary preview content"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/preview?href="+url.QueryEscape("/ui/artifacts/view?path="+url.QueryEscape(artifactPath)), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "preview.md") || !strings.Contains(body, "OpenClaw memory design summary preview content") {
		t.Fatalf("expected artifact preview content, got %q", body)
	}
}

func newWebTestServer(t *testing.T) (http.Handler, *airuntime.Store, *approval.Store, *memory.Store, *airuntime.Orchestrator, *execution.Executor, *skills.Runner, *chat.Store) {
	t.Helper()

	runtimeRoot := tempWebRuntimeRoot(t)
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
	modelConfig, err := model.NewConfigStore(runtimeRoot)
	if err != nil {
		t.Fatalf("NewConfigStore returned error: %v", err)
	}
	if err := modelConfig.Save(model.Config{Provider: "none"}); err != nil {
		t.Fatalf("Save model config returned error: %v", err)
	}
	skillRunner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	recallService := recall.NewService(memoryStore)
	chatStore := chat.NewStore(runtimeRoot)
	chatService := chat.NewService(chatStore, orchestrator, runtimeStore, recallService, skillRunner, nil)
	handler := NewServer(memoryStore, runtimeStore, approvalStore, chatService, recallService, orchestrator, executor, skillRunner, modelConfig).Routes()
	return handler, runtimeStore, approvalStore, memoryStore, orchestrator, executor, skillRunner, chatStore
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func tempWebRuntimeRoot(t *testing.T) string {
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
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}

	state := airuntime.RuntimeState{
		RuntimeID:        "web-test-runtime",
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
