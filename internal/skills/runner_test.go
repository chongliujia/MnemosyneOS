package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/connectors"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/model"
)

func TestRunTaskPlanCompletesTask(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Plan next step",
		Goal:  "Plan the repository work",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	if len(result.ArtifactPaths) != 1 {
		t.Fatalf("expected one artifact, got %d", len(result.ArtifactPaths))
	}
}

func TestRegisterSkillDispatchesCustomHandler(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	if err := runner.RegisterSkill(Definition{
		Name:        "custom-skill",
		Description: "A test-only custom skill.",
		Enabled:     true,
		Handler: func(r *Runner, task airuntime.Task, _ func(ProgressEvent)) (RunResult, error) {
			updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
				ensureMetadata(t)
				t.NextAction = "custom skill completed"
			})
			if err != nil {
				return RunResult{}, err
			}
			if err := r.clearActiveTask(updated.TaskID); err != nil {
				return RunResult{}, err
			}
			return RunResult{Task: updated}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterSkill returned error: %v", err)
	}
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:         "Run custom skill",
		Goal:          "Verify registry dispatch",
		SelectedSkill: "custom-skill",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
}

func TestRegisterSkillRejectsDuplicateNames(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	err = runner.RegisterSkill(Definition{
		Name:    "task-plan",
		Enabled: true,
		Handler: func(r *Runner, task airuntime.Task, _ func(ProgressEvent)) (RunResult, error) {
			return RunResult{Task: task}, nil
		},
	})
	if err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}
}

func TestLoadSkillManifestRegistersAliasSkill(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	manifestDir := filepath.Join(runtimeRoot, "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	raw := []byte("{\n  \"name\": \"web-research\",\n  \"description\": \"Manifest alias for web search.\",\n  \"uses\": \"web-search\",\n  \"maintenance_policy\": {\n    \"enabled\": true,\n    \"scope\": \"project\",\n    \"allowed_card_types\": [\"web_result\"],\n    \"min_candidates\": 1\n  }\n}\n")
	if err := os.WriteFile(filepath.Join(manifestDir, "web-research.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := runner.LoadSkillManifests(manifestDir); err != nil {
		t.Fatalf("LoadSkillManifests returned error: %v", err)
	}

	def, ok := runner.registry.Resolve("web-research")
	if !ok {
		t.Fatalf("expected manifest skill to be registered")
	}
	if def.Source != "manifest" || def.Uses != "web-search" {
		t.Fatalf("expected manifest metadata to be preserved, got %+v", def)
	}
	if def.MaintenancePolicy == nil || len(def.MaintenancePolicy.AllowedCardTypes) != 1 || def.MaintenancePolicy.AllowedCardTypes[0] != "web_result" {
		t.Fatalf("expected maintenance policy override, got %+v", def.MaintenancePolicy)
	}
}

func TestSetSkillEnabledPersistsDisabledManifestSkill(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	manifestDir := filepath.Join(runtimeRoot, "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	raw := []byte("{\n  \"name\": \"web-research\",\n  \"description\": \"Manifest alias for web search.\",\n  \"uses\": \"web-search\"\n}\n")
	if err := os.WriteFile(filepath.Join(manifestDir, "web-research.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := runner.ReloadSkills(); err != nil {
		t.Fatalf("ReloadSkills returned error: %v", err)
	}
	if err := runner.SetSkillEnabled("web-research", false); err != nil {
		t.Fatalf("SetSkillEnabled returned error: %v", err)
	}
	def, ok := runner.registry.Resolve("web-research")
	if !ok || def.Enabled {
		t.Fatalf("expected manifest skill to remain registered but disabled, got %+v", def)
	}
	stateData, err := os.ReadFile(filepath.Join(runtimeRoot, "state", "skills.json"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(stateData), "\"web-research\": false") {
		t.Fatalf("expected persisted disabled state, got %q", string(stateData))
	}
}

func TestSetSkillEnabledPersistsBuiltinSkill(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)

	if err := runner.SetSkillEnabled("web-search", false); err != nil {
		t.Fatalf("SetSkillEnabled returned error: %v", err)
	}
	def, ok := runner.registry.Resolve("web-search")
	if !ok || def.Enabled {
		t.Fatalf("expected builtin skill to remain registered but disabled, got %+v", def)
	}
	if err := runner.ReloadSkills(); err != nil {
		t.Fatalf("ReloadSkills returned error: %v", err)
	}
	def, ok = runner.registry.Resolve("web-search")
	if !ok || def.Enabled {
		t.Fatalf("expected builtin disabled state to survive reload, got %+v", def)
	}
}

func TestReloadSkillsCapturesManifestErrors(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	manifestDir := filepath.Join(runtimeRoot, "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "good.json"), []byte("{\n  \"name\": \"web-research\",\n  \"uses\": \"web-search\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile good returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "bad.json"), []byte("{\n  \"name\": \"Bad Skill\",\n  \"uses\": \"web-search\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile bad returned error: %v", err)
	}

	err = runner.ReloadSkills()
	if err == nil {
		t.Fatalf("expected reload to report manifest failure")
	}
	if _, ok := runner.registry.Resolve("web-research"); !ok {
		t.Fatalf("expected valid manifest skill to remain loaded")
	}
	statuses := runner.ListManifestStatuses()
	if len(statuses) != 2 {
		t.Fatalf("expected two manifest statuses, got %d", len(statuses))
	}
	foundError := false
	for _, status := range statuses {
		if strings.Contains(status.Path, "bad.json") && status.Error != "" {
			foundError = true
		}
	}
	if !foundError {
		t.Fatalf("expected invalid manifest status to capture error, got %+v", statuses)
	}
}

func TestReloadSkillsRejectsUnsupportedManifestVersion(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	manifestDir := filepath.Join(runtimeRoot, "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	raw := []byte("{\n  \"version\": 2,\n  \"name\": \"web-research\",\n  \"uses\": \"web-search\"\n}\n")
	if err := os.WriteFile(filepath.Join(manifestDir, "web-research.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	err = runner.ReloadSkills()
	if err == nil || !strings.Contains(err.Error(), "unsupported manifest version 2") {
		t.Fatalf("expected unsupported version error, got %v", err)
	}
}

func TestLoadExternalManifestRunsCommandAdapter(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)
	manifestDir := filepath.Join(runtimeRoot, "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	scriptPath := filepath.Join(manifestDir, "external-skill.sh")
	script := "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"state\":\"done\",\"next_action\":\"external complete\",\"metadata\":{\"external\":\"true\"},\"artifacts\":[{\"kind\":\"reports\",\"name\":\"external.md\",\"content\":\"# External Report\"}],\"observations\":[{\"kind\":\"os\",\"name\":\"external.json\",\"payload\":{\"summary\":\"ok\"}}]}'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile script returned error: %v", err)
	}
	manifest := "{\n  \"name\": \"external-skill\",\n  \"description\": \"External command skill.\",\n  \"external\": {\n    \"kind\": \"command\",\n    \"command\": \"./external-skill.sh\",\n    \"timeout_ms\": 1500\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(manifestDir, "external-skill.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	if err := runner.ReloadSkills(); err != nil {
		t.Fatalf("ReloadSkills returned error: %v", err)
	}

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:         "Run external skill",
		Goal:          "Verify external command adapter",
		SelectedSkill: "external-skill",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	if len(result.ArtifactPaths) != 1 || len(result.ObservationPaths) < 2 {
		t.Fatalf("expected one artifact and telemetry-backed observations, got artifacts=%d observations=%d", len(result.ArtifactPaths), len(result.ObservationPaths))
	}
	artifactData, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile artifact returned error: %v", err)
	}
	if !strings.Contains(string(artifactData), "External Report") {
		t.Fatalf("expected artifact content from external skill, got %q", string(artifactData))
	}
	stored, err := runtimeStore.GetTask(task.TaskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if stored.Metadata["external"] != "true" {
		t.Fatalf("expected external metadata on task, got %+v", stored.Metadata)
	}
	if stored.Metadata["external_duration_ms"] == "" || stored.Metadata["external_telemetry_observation"] == "" {
		t.Fatalf("expected external telemetry metadata, got %+v", stored.Metadata)
	}
}

func TestLoadExternalManifestRejectsAbsoluteCommand(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	manifestDir := filepath.Join(runtimeRoot, "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	commandPath := "/bin/echo"
	if runtime.GOOS == "windows" {
		commandPath = `C:\Windows\System32\cmd.exe`
	}
	manifest := "{\n  \"name\": \"external-skill\",\n  \"external\": {\n    \"kind\": \"command\",\n    \"command\": \"" + strings.ReplaceAll(commandPath, `\`, `\\`) + "\"\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(manifestDir, "external-skill.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}

	err = runner.ReloadSkills()
	if err == nil || !strings.Contains(err.Error(), "external.command must be relative") {
		t.Fatalf("expected absolute command validation error, got %v", err)
	}
}

func TestExternalSkillRootExecutionRequiresApprovalFlag(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)
	manifestDir := filepath.Join(runtimeRoot, "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	scriptPath := filepath.Join(manifestDir, "external-skill.sh")
	script := "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"state\":\"done\"}'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile script returned error: %v", err)
	}
	manifest := "{\n  \"name\": \"external-skill\",\n  \"external\": {\n    \"kind\": \"command\",\n    \"command\": \"./external-skill.sh\"\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(manifestDir, "external-skill.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	if err := runner.ReloadSkills(); err != nil {
		t.Fatalf("ReloadSkills returned error: %v", err)
	}

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:            "Run external skill as root",
		Goal:             "Verify approval boundary",
		SelectedSkill:    "external-skill",
		ExecutionProfile: "root",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateFailed {
		t.Fatalf("expected failed task, got %s", result.Task.State)
	}
	if !strings.Contains(result.Task.FailureReason, "require_approval=true") {
		t.Fatalf("expected approval guard failure, got %q", result.Task.FailureReason)
	}
}

func TestExternalSkillDoesNotExposeRuntimeRootByDefault(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)
	manifestDir := filepath.Join(runtimeRoot, "skills")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	scriptPath := filepath.Join(manifestDir, "external-skill.sh")
	script := "#!/bin/sh\ninput=$(cat)\ncase \"$input\" in\n  *'\"runtime_root\":\"\"'*) printf '%s' '{\"state\":\"done\",\"metadata\":{\"runtime_root_hidden\":\"true\"}}' ;;\n  *) printf '%s' '{\"state\":\"failed\",\"failure_reason\":\"runtime root leaked\"}' ;;\nesac\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile script returned error: %v", err)
	}
	manifest := "{\n  \"name\": \"external-skill\",\n  \"external\": {\n    \"kind\": \"command\",\n    \"command\": \"./external-skill.sh\"\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(manifestDir, "external-skill.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	if err := runner.ReloadSkills(); err != nil {
		t.Fatalf("ReloadSkills returned error: %v", err)
	}

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:         "Run external skill",
		Goal:          "Verify runtime root is hidden",
		SelectedSkill: "external-skill",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	stored, err := runtimeStore.GetTask(task.TaskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if stored.Metadata["runtime_root_hidden"] != "true" {
		t.Fatalf("expected runtime root to stay hidden, got %+v", stored.Metadata)
	}
}

func TestRunTaskPlanPersistsProcedureEvidence(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:         "Plan expense audit",
		Goal:          "Create an expense audit workflow plan",
		SelectedSkill: "task-plan",
		Metadata: map[string]string{
			"task_class":               "expense_audit",
			"procedure_steps":          "extract_fields\nvalidate_policy\nflag_missing_evidence",
			"procedure_guardrails":     "never invent invoice ids",
			"procedure_summary":        "Audit reimbursements with explicit policy validation.",
			"procedure_success_signal": "exceptions enumerated",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if len(result.ArtifactPaths) != 1 || len(result.ObservationPaths) != 1 {
		t.Fatalf("expected one artifact and one observation, got artifacts=%d observations=%d", len(result.ArtifactPaths), len(result.ObservationPaths))
	}
	artifactData, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile artifact returned error: %v", err)
	}
	if !strings.Contains(string(artifactData), "## Procedure Candidate") || !strings.Contains(string(artifactData), "validate_policy") {
		t.Fatalf("expected procedure hints in artifact, got %q", string(artifactData))
	}
	observationData, err := os.ReadFile(result.ObservationPaths[0])
	if err != nil {
		t.Fatalf("ReadFile observation returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(observationData, &payload); err != nil {
		t.Fatalf("Unmarshal observation returned error: %v", err)
	}
	if payload["procedure_steps"] != "extract_fields\nvalidate_policy\nflag_missing_evidence" {
		t.Fatalf("expected procedure steps in observation, got %+v", payload)
	}
	stored, err := runtimeStore.GetTask(task.TaskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if strings.TrimSpace(stored.Metadata["plan_observation"]) == "" {
		t.Fatalf("expected plan_observation metadata to be set, got %+v", stored.Metadata)
	}
}

func TestSplitArgsHonorsQuotedSegments(t *testing.T) {
	args := splitArgs(`-c "import pathlib,time,sys;p=pathlib.Path('retryable-timeout-flag');sys.stdout.write('recovered\n') if p.exists() else (p.write_text('1'),time.sleep(0.12))"`)
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %#v", args)
	}
	if args[0] != "-c" {
		t.Fatalf("expected first arg to be -c, got %q", args[0])
	}
	if !strings.Contains(args[1], "retryable-timeout-flag") || !strings.Contains(args[1], "recovered\\n") {
		t.Fatalf("expected quoted script to stay intact, got %q", args[1])
	}
}

func TestProcedureEvidenceForTaskScansGenericMetadataKeys(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)

	observationPath := filepath.Join(runtimeRoot, "observations", "os", "custom-procedure.json")
	if err := os.MkdirAll(filepath.Dir(observationPath), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	payload := map[string]any{
		"procedure_steps":          "fetch_policy\nvalidate_policy\nrecord_exceptions",
		"procedure_guardrails":     "do not skip policy validation",
		"procedure_summary":        "Validate policy before recording exceptions.",
		"procedure_success_signal": "exceptions recorded with policy evidence",
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(observationPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	evidence := runner.procedureEvidenceForTask(airuntime.Task{
		TaskID: "task-generic-evidence",
		Metadata: map[string]string{
			"expense_observation": observationPath,
		},
	})
	if evidence.Steps != "fetch_policy\nvalidate_policy\nrecord_exceptions" {
		t.Fatalf("expected generic observation steps, got %+v", evidence)
	}
	if evidence.ObservationPath != observationPath {
		t.Fatalf("expected generic observation path, got %+v", evidence)
	}
}

func TestProcedureEvidenceForTaskPrefersObservationOverArtifactAndMetadata(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)

	artifactPath := filepath.Join(runtimeRoot, "artifacts", "reports", "procedure.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("MkdirAll artifact returned error: %v", err)
	}
	artifactBody := "# Procedure\n\nSteps:\n- artifact_step\n"
	if err := os.WriteFile(artifactPath, []byte(artifactBody), 0o644); err != nil {
		t.Fatalf("WriteFile artifact returned error: %v", err)
	}

	observationPath := filepath.Join(runtimeRoot, "observations", "os", "procedure.json")
	if err := os.MkdirAll(filepath.Dir(observationPath), 0o755); err != nil {
		t.Fatalf("MkdirAll observation returned error: %v", err)
	}
	payload := map[string]any{
		"procedure_steps": "observation_step\nvalidate_policy",
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(observationPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile observation returned error: %v", err)
	}

	evidence := runner.procedureEvidenceForTask(airuntime.Task{
		TaskID: "task-priority",
		Metadata: map[string]string{
			"custom_artifact":    artifactPath,
			"custom_observation": observationPath,
			"procedure_steps":    "metadata_step",
		},
	})
	if evidence.Steps != "observation_step\nvalidate_policy" {
		t.Fatalf("expected observation evidence to win, got %+v", evidence)
	}
}

func TestRunMemoryConsolidatePersistsProcedureEvidence(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:         "Consolidate expense audit memory",
		Goal:          "Summarize reusable expense audit memory",
		SelectedSkill: "memory-consolidate",
		Metadata: map[string]string{
			"task_class":               "expense_audit_memory",
			"card_type":                "procedure",
			"scope":                    "project",
			"procedure_steps":          "collect_runs\nvalidate_policy\npromote_fact",
			"procedure_guardrails":     "never promote unsupported claims",
			"procedure_summary":        "Consolidate repeated expense audit evidence before promotion.",
			"procedure_success_signal": "promotions recorded with evidence",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if len(result.ArtifactPaths) != 1 || len(result.ObservationPaths) != 1 {
		t.Fatalf("expected one artifact and one observation, got artifacts=%d observations=%d", len(result.ArtifactPaths), len(result.ObservationPaths))
	}
	artifactData, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile artifact returned error: %v", err)
	}
	if !strings.Contains(string(artifactData), "## Procedure Candidate") || !strings.Contains(string(artifactData), "validate_policy") {
		t.Fatalf("expected procedure hints in memory artifact, got %q", string(artifactData))
	}
	observationData, err := os.ReadFile(result.ObservationPaths[0])
	if err != nil {
		t.Fatalf("ReadFile observation returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(observationData, &payload); err != nil {
		t.Fatalf("Unmarshal observation returned error: %v", err)
	}
	if payload["procedure_steps"] != "collect_runs\nvalidate_policy\npromote_fact" {
		t.Fatalf("expected procedure steps in memory observation, got %+v", payload)
	}
	stored, err := runtimeStore.GetTask(task.TaskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if strings.TrimSpace(stored.Metadata["memory_observation"]) == "" {
		t.Fatalf("expected memory_observation metadata to be set, got %+v", stored.Metadata)
	}
}

func TestRunMemoryConsolidateDoesNotRescheduleItself(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "candidate:web:1",
		CardType: "web_result",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"snippet": "candidate"},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:         "Consolidate procedure memory",
		Goal:          "Consolidate procedure memory",
		SelectedSkill: "memory-consolidate",
		Metadata: map[string]string{
			"card_type": "procedure",
			"scope":     memory.ScopeProject,
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	if _, err := runner.RunTask(task.TaskID); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	maintenanceTasks := 0
	for _, scheduled := range tasks {
		if scheduled.SelectedSkill == "memory-consolidate" {
			maintenanceTasks++
		}
	}
	if maintenanceTasks != 1 {
		t.Fatalf("expected exactly the original memory-consolidate task, got %d", maintenanceTasks)
	}
}

func TestRunWebSearchCompletesTask(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(fakeSearchClient{
		resp: connectors.SearchResponse{
			Query:    "Search the web for docs",
			Provider: "fake-search",
			Results: []connectors.SearchResult{
				{Title: "Docs", URL: "https://example.com/a", Snippet: "Alpha"},
			},
		},
	}, fakeGitHubClient{}, fakeEmailClient{}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Search docs",
		Goal:  "Search the web for docs",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	if len(result.ObservationPaths) != 1 || len(result.ArtifactPaths) != 1 {
		t.Fatalf("expected one observation and one artifact, got obs=%d artifacts=%d", len(result.ObservationPaths), len(result.ArtifactPaths))
	}
	data, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "fake-search") {
		t.Fatalf("expected provider in artifact, got %q", string(data))
	}

	rootCard := memoryStore.Query(memory.QueryRequest{CardID: searchMemoryCardID(task.TaskID)})
	if len(rootCard.Cards) != 1 {
		t.Fatalf("expected one root memory card, got %d", len(rootCard.Cards))
	}
	if got := rootCard.Cards[0].Content["provider"]; got != "fake-search" {
		t.Fatalf("unexpected memory provider content: %#v", got)
	}
	if len(rootCard.Edges) != 2 {
		t.Fatalf("expected summary edge plus one search result edge, got %d", len(rootCard.Edges))
	}

	summaryCard := memoryStore.Query(memory.QueryRequest{CardID: searchSummaryCardID(task.TaskID)})
	if len(summaryCard.Cards) != 1 {
		t.Fatalf("expected one summary card, got %d", len(summaryCard.Cards))
	}

	resultCard := memoryStore.Query(memory.QueryRequest{CardID: canonicalSearchResultCardID("https://example.com/a")})
	if len(resultCard.Cards) != 1 {
		t.Fatalf("expected one canonical result card, got %d", len(resultCard.Cards))
	}
}

func TestRunWebSearchSchedulesMemoryMaintenance(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(fakeSearchClient{
		resp: connectors.SearchResponse{
			Query:    "Search the web for docs",
			Provider: "fake-search",
			Results: []connectors.SearchResult{
				{Title: "Docs", URL: "https://example.com/a", Snippet: "Alpha"},
			},
		},
	}, fakeGitHubClient{}, fakeEmailClient{}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Search docs",
		Goal:  "Search the web for docs",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	if _, err := runner.RunTask(task.TaskID); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	maintenanceTasks := 0
	for _, scheduled := range tasks {
		if scheduled.SelectedSkill != "memory-consolidate" {
			continue
		}
		maintenanceTasks++
		if scheduled.Metadata["scheduled_reason"] != "task_completion" {
			t.Fatalf("expected scheduled_reason=task_completion, got %+v", scheduled.Metadata)
		}
	}
	if maintenanceTasks != 1 {
		t.Fatalf("expected one scheduled memory maintenance task, got %d", maintenanceTasks)
	}
}

func TestRunWebSearchDoesNotScheduleFromEmailOnlyCandidates(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "candidate:email:1",
		CardType: "email_thread",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"subject": "email only"},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(fakeSearchClient{
		resp: connectors.SearchResponse{
			Query:    "Search the web for docs",
			Provider: "fake-search",
			Results:  nil,
		},
	}, fakeGitHubClient{}, fakeEmailClient{}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Search docs",
		Goal:  "Search the web for docs",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	if _, err := runner.RunTask(task.TaskID); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	maintenanceTasks := 0
	for _, scheduled := range tasks {
		if scheduled.SelectedSkill == "memory-consolidate" {
			maintenanceTasks++
		}
	}
	if maintenanceTasks != 0 {
		t.Fatalf("expected no scheduled memory maintenance task from email-only candidates, got %d", maintenanceTasks)
	}
}

func TestRunFileReadDoesNotScheduleMemoryMaintenance(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "notes.txt"), []byte("read me"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "candidate:web:1",
		CardType: "web_result",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"snippet": "candidate"},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Read a file",
		Goal:  "Read a file in the workspace",
		Metadata: map[string]string{
			"path": "notes.txt",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	if _, err := runner.RunTask(task.TaskID); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	maintenanceTasks := 0
	for _, scheduled := range tasks {
		if scheduled.SelectedSkill == "memory-consolidate" {
			maintenanceTasks++
		}
	}
	if maintenanceTasks != 0 {
		t.Fatalf("expected no scheduled memory maintenance task for file-read, got %d", maintenanceTasks)
	}
}

func TestRunEmailInboxSchedulesMemoryMaintenanceFromEmailCandidates(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(nil, fakeGitHubClient{}, fakeEmailClient{
		resp: connectors.EmailResponse{
			Provider: "maildir",
			Results: []connectors.EmailMessage{
				{MessageID: "<msg-1>", Subject: "Root approval required", From: "agent@example.com", Snippet: "Please approve root action", Unread: true, Date: "2026-03-23T10:00:00Z"},
			},
		},
	}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Check email inbox",
		Goal:  "Check email inbox",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	if _, err := runner.RunTask(task.TaskID); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	tasks, err := runtimeStore.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	maintenanceTasks := 0
	for _, scheduled := range tasks {
		if scheduled.SelectedSkill != "memory-consolidate" {
			continue
		}
		maintenanceTasks++
	}
	if maintenanceTasks != 1 {
		t.Fatalf("expected one scheduled memory maintenance task from email candidates, got %d", maintenanceTasks)
	}
}

func TestRunWebSearchBlocksWhenSearchClientMissing(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Search docs",
		Goal:  "Search the web for docs",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateBlocked {
		t.Fatalf("expected blocked task, got %s", result.Task.State)
	}
	if len(result.ObservationPaths) != 1 {
		t.Fatalf("expected one observation, got %d", len(result.ObservationPaths))
	}
}

func TestRunFileEditWritesFile(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Edit a file",
		Goal:  "Edit a file in the workspace",
		Metadata: map[string]string{
			"path":    "notes/todo.txt",
			"content": "ship runtime MVP",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	if result.Action == nil || result.Action.Status != execution.ActionStatusCompleted {
		t.Fatalf("expected completed action, got %#v", result.Action)
	}
}

func TestRunRootFileEditRequestsApproval(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:            "Edit a root file",
		Goal:             "Edit a root-owned file",
		ExecutionProfile: "root",
		Metadata: map[string]string{
			"path":    "notes/root.txt",
			"content": "needs approval",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateAwaitingApproval {
		t.Fatalf("expected awaiting approval task, got %s", result.Task.State)
	}
	approvals, err := approvalStore.List(approval.StatusPending)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("expected one pending approval, got %d", len(approvals))
	}
}

func TestRunTaskPlanUsesModelWhenAvailable(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, fakeTextModel{
		resp: model.TextResponse{
			Provider: "fake",
			Model:    "fake-model",
			Text:     "# Task Plan\n\nModel-generated plan.\n",
		},
	})
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Plan next step with model",
		Goal:  "Plan with actual model output",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	data, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "Model-generated plan.") {
		t.Fatalf("expected model-generated artifact, got %q", string(data))
	}
}

func TestRunRootFileReadRequestsApproval(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "notes.txt"), []byte("read me"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:            "Read a root file",
		Goal:             "Read a file in the workspace",
		ExecutionProfile: "root",
		Metadata: map[string]string{
			"path": "notes.txt",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateAwaitingApproval {
		t.Fatalf("expected awaiting approval task, got %s", result.Task.State)
	}
}

func TestRunRootShellRequestsApproval(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:            "Run a root shell command",
		Goal:             "Run a shell command in the workspace",
		ExecutionProfile: "root",
		Metadata: map[string]string{
			"command": "pwd",
			"workdir": ".",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateAwaitingApproval {
		t.Fatalf("expected awaiting approval task, got %s", result.Task.State)
	}
}

func TestRunGitHubIssueSearchCompletesTask(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(nil, fakeGitHubClient{
		resp: connectors.GitHubIssueResponse{
			Query:    "approval flow",
			Provider: "github",
			Results: []connectors.GitHubIssue{
				{Number: 12, Title: "Approval flow", URL: "https://example.com/issues/12", State: "open", Body: "Need root approval flow", Repo: "mnemosyne/agentos"},
			},
		},
	}, fakeEmailClient{}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Search GitHub issues",
		Goal:  "Search github issues for approval flow",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	data, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "Approval flow") {
		t.Fatalf("expected github issue in artifact, got %q", string(data))
	}

	rootCard := memoryStore.Query(memory.QueryRequest{CardID: githubIssueMemoryCardID(task.TaskID)})
	if len(rootCard.Cards) != 1 {
		t.Fatalf("expected one github root card, got %d", len(rootCard.Cards))
	}
	if len(rootCard.Edges) != 2 {
		t.Fatalf("expected summary edge plus one issue edge, got %d", len(rootCard.Edges))
	}

	summaryCard := memoryStore.Query(memory.QueryRequest{CardID: githubIssueSummaryCardID(task.TaskID)})
	if len(summaryCard.Cards) != 1 {
		t.Fatalf("expected one github summary card, got %d", len(summaryCard.Cards))
	}

	issueCard := memoryStore.Query(memory.QueryRequest{CardID: canonicalGitHubIssueCardID(connectors.GitHubIssue{
		Number: 12,
		Title:  "Approval flow",
		URL:    "https://example.com/issues/12",
		State:  "open",
		Body:   "Need root approval flow",
		Repo:   "mnemosyne/agentos",
	})})
	if len(issueCard.Cards) != 1 {
		t.Fatalf("expected one canonical github issue card, got %d", len(issueCard.Cards))
	}
}

func TestRunEmailInboxCompletesTask(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(nil, fakeGitHubClient{}, fakeEmailClient{
		resp: connectors.EmailResponse{
			Provider: "maildir",
			Results: []connectors.EmailMessage{
				{MessageID: "<msg-1>", Subject: "Root approval required", From: "agent@example.com", Snippet: "Please approve root action", Unread: true, Date: "2026-03-23T10:00:00Z"},
				{MessageID: "<msg-2>", Subject: "Re: Root approval required", From: "reviewer@example.com", Snippet: "Approved, rerun the task", Unread: false, Date: "2026-03-23T10:05:00Z"},
			},
		},
	}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Check email inbox",
		Goal:  "Check email inbox",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	data, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "Root approval required") {
		t.Fatalf("expected email subject in artifact, got %q", string(data))
	}

	rootCard := memoryStore.Query(memory.QueryRequest{CardID: emailMemoryCardID(task.TaskID)})
	if len(rootCard.Cards) != 1 {
		t.Fatalf("expected one email root card, got %d", len(rootCard.Cards))
	}
	if got := rootCard.Cards[0].Content["thread_count"]; got != 1 {
		t.Fatalf("expected one thread, got %#v", got)
	}
	if len(rootCard.Edges) != 4 {
		t.Fatalf("expected summary edge, thread edge, and two message edges, got %d", len(rootCard.Edges))
	}

	summaryCard := memoryStore.Query(memory.QueryRequest{CardID: emailSummaryCardID(task.TaskID)})
	if len(summaryCard.Cards) != 1 {
		t.Fatalf("expected one email summary card, got %d", len(summaryCard.Cards))
	}
	threadCard := memoryStore.Query(memory.QueryRequest{CardID: canonicalEmailThreadCardID("root approval required")})
	if len(threadCard.Cards) != 1 {
		t.Fatalf("expected one email thread card, got %d", len(threadCard.Cards))
	}
	threadMessageEdges := 0
	for _, edge := range threadCard.Edges {
		if edge.EdgeType == "thread_message" {
			threadMessageEdges++
		}
	}
	if threadMessageEdges != 2 {
		t.Fatalf("expected two thread_message edges, got %d", threadMessageEdges)
	}

	messageCard := memoryStore.Query(memory.QueryRequest{CardID: canonicalEmailMessageCardID(connectors.EmailMessage{
		MessageID: "<msg-1>",
		Subject:   "Root approval required",
		From:      "agent@example.com",
		Snippet:   "Please approve root action",
		Unread:    true,
	})})
	if len(messageCard.Cards) != 1 {
		t.Fatalf("expected one canonical email message card, got %d", len(messageCard.Cards))
	}
}

type fakeTextModel struct {
	resp model.TextResponse
	err  error
}

func (f fakeTextModel) GenerateText(_ context.Context, _ model.TextRequest) (model.TextResponse, error) {
	return f.resp, f.err
}

func (f fakeTextModel) StreamText(_ context.Context, _ model.TextRequest, onDelta func(model.TextDelta) error) (model.TextResponse, error) {
	if f.err != nil {
		return model.TextResponse{}, f.err
	}
	if onDelta != nil && strings.TrimSpace(f.resp.Text) != "" {
		if err := onDelta(model.TextDelta{Text: f.resp.Text}); err != nil {
			return model.TextResponse{}, err
		}
	}
	return f.resp, nil
}

type fakeSearchClient struct {
	resp connectors.SearchResponse
	err  error
}

func (f fakeSearchClient) Search(_ context.Context, _ connectors.SearchRequest) (connectors.SearchResponse, error) {
	return f.resp, f.err
}

type fakeGitHubClient struct {
	resp connectors.GitHubIssueResponse
	err  error
}

func (f fakeGitHubClient) SearchIssues(_ context.Context, _ connectors.GitHubIssueRequest) (connectors.GitHubIssueResponse, error) {
	return f.resp, f.err
}

type fakeEmailClient struct {
	resp connectors.EmailResponse
	err  error
}

func (f fakeEmailClient) ListMessages(_ context.Context, _ connectors.EmailRequest) (connectors.EmailResponse, error) {
	return f.resp, f.err
}

func tempSkillRuntimeRoot(t *testing.T) string {
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
		RuntimeID:        "test-runtime",
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
