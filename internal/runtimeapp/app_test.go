package runtimeapp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"mnemosyneos/internal/airuntime"
)

func TestBuildDisablesWebRoutesWhenRequested(t *testing.T) {
	t.Setenv("MNEMOSYNE_MODEL_PROVIDER", "none")
	runtimeRoot := tempRuntimeRoot(t)
	app, err := Build(Options{
		RuntimeRoot:   runtimeRoot,
		WorkspaceRoot: t.TempDir(),
		EnableWeb:     false,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	defer app.Shutdown()

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	app.Handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d", healthRec.Code)
	}

	webReq := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	webRec := httptest.NewRecorder()
	app.Handler.ServeHTTP(webRec, webReq)
	if webRec.Code != http.StatusNotFound {
		t.Fatalf("expected dashboard 404 when web disabled, got %d", webRec.Code)
	}
}

func tempRuntimeRoot(t *testing.T) string {
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
		filepath.Join(root, "approvals", "pending"),
		filepath.Join(root, "approvals", "approved"),
		filepath.Join(root, "approvals", "denied"),
		filepath.Join(root, "approvals", "consumed"),
		filepath.Join(root, "actions", "pending"),
		filepath.Join(root, "actions", "running"),
		filepath.Join(root, "actions", "completed"),
		filepath.Join(root, "actions", "failed"),
		filepath.Join(root, "artifacts", "reports"),
		filepath.Join(root, "observations", "filesystem"),
		filepath.Join(root, "observations", "os"),
		filepath.Join(root, "skills"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "state", "runtime.json"), []byte("{\n  \"runtime_id\": \"runtime-test\",\n  \"active_user_id\": \"default-user\",\n  \"status\": \"idle\",\n  \"execution_profile\": \"user\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile runtime state: %v", err)
	}
	return root
}
