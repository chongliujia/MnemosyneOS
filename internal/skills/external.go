package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/execution"
)

type externalInvocation struct {
	Task        airuntime.Task `json:"task"`
	RuntimeRoot string         `json:"runtime_root"`
}

type externalResult struct {
	State         string                `json:"state,omitempty"`
	NextAction    string                `json:"next_action,omitempty"`
	FailureReason string                `json:"failure_reason,omitempty"`
	Metadata      map[string]string     `json:"metadata,omitempty"`
	Artifacts     []externalArtifact    `json:"artifacts,omitempty"`
	Observations  []externalObservation `json:"observations,omitempty"`
}

type externalArtifact struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type externalObservation struct {
	Kind    string         `json:"kind"`
	Name    string         `json:"name"`
	Payload map[string]any `json:"payload"`
}

func (r *Runner) externalHandler(manifestPath string, manifest Manifest) Handler {
	cfg := copyExternalConfig(manifest.External)
	return func(r *Runner, task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
		emitProgress(onProgress, "external.invoke", "Running external skill adapter...")
		if cfg == nil {
			return RunResult{}, fmt.Errorf("external configuration is missing")
		}
		timeout := 20 * time.Second
		if cfg.TimeoutMS > 0 {
			timeout = time.Duration(cfg.TimeoutMS) * time.Millisecond
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		commandPath, workDir, err := r.resolveExternalPaths(manifestPath, cfg)
		if err != nil {
			return r.failExternalTask(task, err.Error())
		}
		if task.ExecutionProfile == "root" {
			if !cfg.RequireApproval {
				return r.failExternalTask(task, "root external execution requires external.require_approval=true")
			}
			approvalToken, approvalResult, err := r.resolveRootApproval(task, execution.ActionKindShell, fmt.Sprintf("root external skill %s", task.SelectedSkill), map[string]string{
				"external_command": commandPath,
				"manifest_path":    manifestPath,
			})
			if approvalResult != nil {
				return *approvalResult, err
			}
			if err != nil {
				return RunResult{}, err
			}
			if _, err := r.approvals.Consume("root", approvalToken); err != nil {
				return r.failExternalTask(task, err.Error())
			}
		}
		cmd := exec.CommandContext(ctx, commandPath, cfg.Args...)
		cmd.Dir = workDir
		cmd.Env = append([]string(nil), cmd.Environ()...)
		for key, value := range cfg.Env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		cmd.Env = append(cmd.Env,
			"MNEMOSYNE_SKILL_NAME="+task.SelectedSkill,
			"MNEMOSYNE_TASK_ID="+task.TaskID,
		)
		if cfg.AllowWriteRoot {
			cmd.Env = append(cmd.Env, "MNEMOSYNE_RUNTIME_ROOT="+r.runtimeStore.RootDir())
		}

		input := externalInvocation{Task: task}
		if cfg.AllowWriteRoot {
			input.RuntimeRoot = r.runtimeStore.RootDir()
		}
		inputData, err := json.Marshal(input)
		if err != nil {
			return RunResult{}, err
		}
		cmd.Stdin = bytes.NewReader(inputData)

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		startedAt := time.Now().UTC()
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return r.failExternalTask(task, firstNonEmpty(strings.TrimSpace(stderr.String()), err.Error()))
		}
		finishedAt := time.Now().UTC()

		var result externalResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			return r.failExternalTask(task, fmt.Sprintf("decode external skill result: %v", err))
		}
		return r.applyExternalResult(task, result, externalTelemetry{
			ManifestPath:     manifestPath,
			CommandPath:      commandPath,
			WorkDir:          workDir,
			StartedAt:        startedAt,
			FinishedAt:       finishedAt,
			DurationMS:       finishedAt.Sub(startedAt).Milliseconds(),
			ApprovalRequired: cfg.RequireApproval,
			WriteRootExposed: cfg.AllowWriteRoot,
		})
	}
}

type externalTelemetry struct {
	ManifestPath     string
	CommandPath      string
	WorkDir          string
	StartedAt        time.Time
	FinishedAt       time.Time
	DurationMS       int64
	ApprovalRequired bool
	WriteRootExposed bool
}

func (r *Runner) resolveExternalPaths(manifestPath string, cfg *ExternalConfig) (string, string, error) {
	manifestDir := filepath.Dir(manifestPath)
	commandPath := filepath.Join(manifestDir, cfg.Command)
	commandPath = filepath.Clean(commandPath)
	rel, err := filepath.Rel(manifestDir, commandPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", "", fmt.Errorf("external.command escapes the manifest directory")
	}
	info, err := os.Stat(commandPath)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return "", "", fmt.Errorf("external.command must be a file")
	}

	workDir := manifestDir
	if strings.TrimSpace(cfg.WorkDir) != "" {
		workDir = filepath.Join(manifestDir, cfg.WorkDir)
		workDir = filepath.Clean(workDir)
		rel, err := filepath.Rel(manifestDir, workDir)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", "", fmt.Errorf("external.workdir escapes the manifest directory")
		}
	}
	return commandPath, workDir, nil
}

func (r *Runner) applyExternalResult(task airuntime.Task, result externalResult, telemetry externalTelemetry) (RunResult, error) {
	state := strings.TrimSpace(result.State)
	if state == "" {
		state = airuntime.TaskStateDone
	}
	switch state {
	case airuntime.TaskStateDone, airuntime.TaskStateBlocked, airuntime.TaskStateFailed:
	default:
		return RunResult{}, fmt.Errorf("unsupported external result state %q", state)
	}

	artifactPaths := make([]string, 0, len(result.Artifacts))
	for i, artifact := range result.Artifacts {
		name := strings.TrimSpace(artifact.Name)
		if name == "" {
			name = fmt.Sprintf("%s-external-%d.txt", task.TaskID, i+1)
		}
		kind := firstNonEmpty(strings.TrimSpace(artifact.Kind), "reports")
		path, err := r.writeArtifact(kind, name, artifact.Content)
		if err != nil {
			return RunResult{}, err
		}
		artifactPaths = append(artifactPaths, path)
	}

	observationPaths := make([]string, 0, len(result.Observations))
	for i, observation := range result.Observations {
		name := strings.TrimSpace(observation.Name)
		if name == "" {
			name = fmt.Sprintf("%s-external-%d.json", task.TaskID, i+1)
		}
		kind := firstNonEmpty(strings.TrimSpace(observation.Kind), "os")
		path, err := r.writeObservation(kind, name, observation.Payload)
		if err != nil {
			return RunResult{}, err
		}
		observationPaths = append(observationPaths, path)
	}
	telemetryPath, err := r.writeObservation("os", task.TaskID+"-external-telemetry.json", map[string]any{
		"type":               "external-skill-telemetry",
		"task_id":            task.TaskID,
		"selected_skill":     task.SelectedSkill,
		"manifest_path":      telemetry.ManifestPath,
		"command_path":       telemetry.CommandPath,
		"workdir":            telemetry.WorkDir,
		"started_at":         telemetry.StartedAt.Format(time.RFC3339Nano),
		"finished_at":        telemetry.FinishedAt.Format(time.RFC3339Nano),
		"duration_ms":        telemetry.DurationMS,
		"approval_required":  telemetry.ApprovalRequired,
		"write_root_exposed": telemetry.WriteRootExposed,
		"result_state":       firstNonEmpty(result.State, airuntime.TaskStateDone),
		"artifact_count":     len(artifactPaths),
		"observation_count":  len(result.Observations),
	})
	if err != nil {
		return RunResult{}, err
	}
	observationPaths = append(observationPaths, telemetryPath)

	updated, err := r.runtimeStore.MoveTask(task.TaskID, state, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.NextAction = firstNonEmpty(result.NextAction, "external skill completed")
		if state == airuntime.TaskStateFailed {
			t.FailureReason = firstNonEmpty(result.FailureReason, "external skill failed")
		} else {
			t.FailureReason = ""
		}
		for key, value := range result.Metadata {
			t.Metadata[key] = value
		}
		if len(artifactPaths) > 0 {
			t.Metadata["external_artifact"] = artifactPaths[0]
		}
		if len(observationPaths) > 0 {
			t.Metadata["external_observation"] = observationPaths[0]
		}
		t.Metadata["external_manifest_path"] = telemetry.ManifestPath
		t.Metadata["external_command_path"] = telemetry.CommandPath
		t.Metadata["external_duration_ms"] = fmt.Sprintf("%d", telemetry.DurationMS)
		t.Metadata["external_approval_required"] = fmt.Sprintf("%t", telemetry.ApprovalRequired)
		t.Metadata["external_write_root_exposed"] = fmt.Sprintf("%t", telemetry.WriteRootExposed)
		t.Metadata["external_telemetry_observation"] = telemetryPath
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.clearActiveTask(updated.TaskID); err != nil {
		return RunResult{}, err
	}
	if state == airuntime.TaskStateDone {
		if err := r.finalizeSuccessfulTask(updated, nil); err != nil {
			return RunResult{}, err
		}
	}
	return RunResult{
		Task:             updated,
		ArtifactPaths:    artifactPaths,
		ObservationPaths: observationPaths,
	}, nil
}

func (r *Runner) failExternalTask(task airuntime.Task, reason string) (RunResult, error) {
	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateFailed, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.FailureReason = firstNonEmpty(reason, "external skill failed")
		t.NextAction = "inspect external skill output"
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.clearActiveTask(updated.TaskID); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated}, nil
}
