package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mnemosyneos/internal/airuntime"
)

func TestRunInitBootstrapsRuntimeAndEnv(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	envFile := filepath.Join(root, ".env.local")
	runtimeRoot := filepath.Join(root, "runtime")

	result, err := runInit(initOptions{
		RuntimeRoot:       runtimeRoot,
		APIBase:           "http://127.0.0.1:8090",
		EnvFile:           envFile,
		Provider:          "openai-compatible",
		BaseURL:           "https://api.example.test/v1",
		APIKey:            "test-key",
		ConversationModel: "conv-model",
		RoutingModel:      "route-model",
		SkillsModel:       "skills-model",
	})
	if err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(runtimeRoot, "state", "runtime.json")); err != nil {
		t.Fatalf("expected runtime state file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeRoot, "state", "skills.json")); err != nil {
		t.Fatalf("expected skills state file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeRoot, "skills")); err != nil {
		t.Fatalf("expected skills dir: %v", err)
	}
	rawEnv, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("ReadFile env returned error: %v", err)
	}
	envText := string(rawEnv)
	if !strings.Contains(envText, "MNEMOSYNE_RUNTIME_ROOT="+runtimeRoot) {
		t.Fatalf("expected runtime root in env file, got %q", envText)
	}
	if !strings.Contains(envText, "MNEMOSYNE_API_BASE=http://127.0.0.1:8090") {
		t.Fatalf("expected api base in env file, got %q", envText)
	}

	if !strings.Contains(envText, "MNEMOSYNE_MODEL_CONFIG_PATH="+result.ModelConfigPath) {
		t.Fatalf("expected private model config path in env file, got %q", envText)
	}
	if filepath.Base(result.ModelConfigPath) != "local.config.json" {
		t.Fatalf("expected init with model flags to use private local config, got %s", result.ModelConfigPath)
	}

	rawCfg, err := os.ReadFile(result.ModelConfigPath)
	if err != nil {
		t.Fatalf("ReadFile config returned error: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		t.Fatalf("Unmarshal config returned error: %v", err)
	}
	if cfg["api_key"] != "test-key" {
		t.Fatalf("expected API key only in private model config")
	}
	if _, err := os.Stat(filepath.Join(runtimeRoot, "model", "config.json")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("unexpected default model config stat error: %v", err)
	}
	if result.ModelConfig.Provider != "openai-compatible" || result.ModelConfig.Conversation.Model != "conv-model" {
		t.Fatalf("unexpected init model config: %+v", result.ModelConfig)
	}
	redacted := redactedInitResult(result)
	if redacted.ModelConfig.APIKey != "<redacted>" {
		t.Fatalf("expected redacted API key in init output, got %q", redacted.ModelConfig.APIKey)
	}
	if result.ModelConfig.APIKey != "test-key" {
		t.Fatalf("expected runInit result to keep the persisted API key for internal use")
	}
}

func TestRunInitWritesWorkspaceAndFilesystemEnv(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	envFile := filepath.Join(root, ".env.local")
	runtimeRoot := filepath.Join(root, "runtime")
	ws := filepath.Join(root, "proj")

	_, err := runInit(initOptions{
		RuntimeRoot:            runtimeRoot,
		APIBase:                "http://127.0.0.1:8091",
		EnvFile:                envFile,
		Provider:               "none",
		WorkspaceRoot:          ws,
		FilesystemUnrestricted: "true",
	})
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}
	rawEnv, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(rawEnv)
	if !strings.Contains(text, "MNEMOSYNE_WORKSPACE_ROOT=") {
		t.Fatalf("missing workspace in env: %q", text)
	}
	if !strings.Contains(text, "MNEMOSYNE_FILESYSTEM_UNRESTRICTED=true") {
		t.Fatalf("missing filesystem flag: %q", text)
	}
}

func TestRunDoctorReportsHealthyRuntimeAndAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	envFile := filepath.Join(root, ".env.local")
	runtimeRoot := filepath.Join(root, "runtime")
	if _, err := runInit(initOptions{
		RuntimeRoot: runtimeRoot,
		APIBase:     "http://127.0.0.1:18080",
		EnvFile:     envFile,
	}); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	prev := newDoctorClient
	newDoctorClient = func(_ string) doctorClient {
		return stubDoctorClient{}
	}
	defer func() {
		newDoctorClient = prev
	}()

	report := runDoctor(doctorOptions{
		RuntimeRoot: runtimeRoot,
		APIBase:     "http://127.0.0.1:18080",
		EnvFile:     envFile,
	})
	if report.Status == "fail" {
		t.Fatalf("expected non-failing doctor report, got %+v", report)
	}
	hasAPIHealth := false
	for _, check := range report.Checks {
		if check.Name == "api_health" && check.Status == "ok" {
			hasAPIHealth = true
		}
	}
	if !hasAPIHealth {
		t.Fatalf("expected api_health ok, got %+v", report.Checks)
	}
}

type stubDoctorClient struct{}

func (stubDoctorClient) Health() (map[string]string, error) {
	return map[string]string{"status": "ok"}, nil
}

func (stubDoctorClient) RuntimeState() (airuntime.RuntimeState, error) {
	return airuntime.RuntimeState{
		RuntimeID:        "runtime-test",
		ActiveUserID:     "default-user",
		Status:           "idle",
		ExecutionProfile: "user",
	}, nil
}

func TestUpsertEnvFilePreservesCommentsAndUpdatesKeys(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(path, []byte("# comment\nMNEMOSYNE_API_BASE=http://old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	created, updated, err := upsertEnvFile(path, map[string]string{
		"MNEMOSYNE_API_BASE":     "http://new",
		"MNEMOSYNE_RUNTIME_ROOT": "runtime",
	})
	if err != nil {
		t.Fatalf("upsertEnvFile returned error: %v", err)
	}
	if created {
		t.Fatalf("expected existing env file")
	}
	if len(updated) != 2 {
		t.Fatalf("expected two updated keys, got %+v", updated)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "# comment") || !strings.Contains(text, "MNEMOSYNE_API_BASE=http://new") || !strings.Contains(text, "MNEMOSYNE_RUNTIME_ROOT=runtime") {
		t.Fatalf("unexpected env content: %q", text)
	}
}
