package execution

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mnemosyneos/internal/approval"
)

var (
	ErrExecutionInvalidInput = errors.New("invalid execution input")
	ErrExecutionDenied       = errors.New("execution denied")
)

type Executor struct {
	store         *Store
	workspaceRoot string
	rootAuthToken string
	approvalStore *approval.Store
}

type shellAttemptResult struct {
	stdout          string
	stderr          string
	exitCode        int
	err             error
	failureCategory string
	retryable       bool
	startedAt       time.Time
	finishedAt      time.Time
}

func NewExecutor(store *Store, workspaceRoot string) (*Executor, error) {
	return NewExecutorWithRootToken(store, workspaceRoot, "")
}

func NewExecutorWithRootToken(store *Store, workspaceRoot, rootAuthToken string) (*Executor, error) {
	return NewExecutorWithApprovals(store, workspaceRoot, rootAuthToken, nil)
}

func NewExecutorWithApprovals(store *Store, workspaceRoot, rootAuthToken string, approvalStore *approval.Store) (*Executor, error) {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &Executor{
		store:         store,
		workspaceRoot: absRoot,
		rootAuthToken: strings.TrimSpace(rootAuthToken),
		approvalStore: approvalStore,
	}, nil
}

func (e *Executor) ExecuteShell(req ShellActionRequest) (ActionRecord, error) {
	if strings.TrimSpace(req.Command) == "" {
		return ActionRecord{}, fmt.Errorf("%w: command is required", ErrExecutionInvalidInput)
	}
	profile := firstNonEmpty(req.ExecutionProfile, "user")
	if err := e.authorizeProfile(profile, req.ApprovalToken); err != nil {
		return ActionRecord{}, err
	}

	commandPath, err := e.resolveCommand(req.Command)
	if err != nil {
		return ActionRecord{}, err
	}
	workdir, err := e.resolveWorkdir(req.Workdir)
	if err != nil {
		return ActionRecord{}, err
	}

	timeout := 30 * time.Second
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	metadata := cloneMetadata(req.Metadata)
	fingerprint := executionFingerprint(ActionKindShell, profile, commandPath, workdir, fmt.Sprintf("%d", req.TimeoutMS), strings.Join(req.Args, "\x00"))
	metadata["request_fingerprint"] = fingerprint

	record := ActionRecord{
		ActionID:         fmt.Sprintf("action-%d", time.Now().UTC().UnixNano()),
		TaskID:           strings.TrimSpace(req.TaskID),
		Kind:             ActionKindShell,
		Status:           ActionStatusRunning,
		Attempt:          normalizeAttempt(req.Attempt, req.Metadata),
		IdempotencyKey:   firstNonEmpty(req.IdempotencyKey, req.Metadata["idempotency_key"]),
		ExecutionProfile: profile,
		Command:          commandPath,
		Args:             req.Args,
		Workdir:          workdir,
		Metadata:         metadata,
		StartedAt:        time.Now().UTC(),
	}
	if replayed, ok, err := e.tryReplay(record, fingerprint); err != nil {
		return ActionRecord{}, err
	} else if ok {
		return replayed, nil
	}
	maxAttempts := normalizeMaxAttempts(req.MaxAttempts, record.Attempt)
	if err := e.store.Save(record); err != nil {
		return ActionRecord{}, err
	}

	for attempt := record.Attempt; attempt <= maxAttempts; attempt++ {
		record.Attempt = attempt
		result := e.runShellAttempt(commandPath, req.Args, workdir, timeout)
		record.AttemptHistory = append(record.AttemptHistory, ActionAttempt{
			Attempt:         attempt,
			Status:          attemptStatus(result.err),
			FailureCategory: result.failureCategory,
			Retryable:       result.retryable,
			ExitCode:        result.exitCode,
			Error:           errorString(result.err),
			StartedAt:       result.startedAt,
			FinishedAt:      timestampPtr(result.finishedAt),
		})
		record.Stdout = result.stdout
		record.Stderr = result.stderr
		record.ExitCode = result.exitCode
		record.Error = errorString(result.err)
		record.FailureCategory = result.failureCategory
		record.Retryable = result.retryable
		record.FinishedAt = timestampPtr(result.finishedAt)

		if result.err == nil {
			record.Status = ActionStatusCompleted
			record.Retryable = false
			record.Error = ""
			record.FailureCategory = ""
			if err := e.store.Move(record, ActionStatusCompleted); err != nil {
				return ActionRecord{}, err
			}
			return record, nil
		}
		if !shouldRetry(ActionKindShell, result.failureCategory, attempt, maxAttempts) {
			record.Status = ActionStatusFailed
			if err := e.store.Move(record, ActionStatusFailed); err != nil {
				return ActionRecord{}, err
			}
			return record, nil
		}
	}

	record.Status = ActionStatusFailed
	if err := e.store.Move(record, ActionStatusFailed); err != nil {
		return ActionRecord{}, err
	}
	return record, nil
}

func (e *Executor) ExecuteFileRead(req FileReadActionRequest) (ActionRecord, error) {
	if strings.TrimSpace(req.Path) == "" {
		return ActionRecord{}, fmt.Errorf("%w: path is required", ErrExecutionInvalidInput)
	}
	profile := firstNonEmpty(req.ExecutionProfile, "user")
	if err := e.authorizeProfile(profile, req.ApprovalToken); err != nil {
		return ActionRecord{}, err
	}

	path, err := e.resolveAllowedPath(req.Path)
	if err != nil {
		return ActionRecord{}, err
	}
	metadata := cloneMetadata(req.Metadata)
	fingerprint := executionFingerprint(ActionKindFileRead, profile, path)
	metadata["request_fingerprint"] = fingerprint

	record := ActionRecord{
		ActionID:         fmt.Sprintf("action-%d", time.Now().UTC().UnixNano()),
		TaskID:           strings.TrimSpace(req.TaskID),
		Kind:             ActionKindFileRead,
		Status:           ActionStatusRunning,
		Attempt:          normalizeAttempt(req.Attempt, req.Metadata),
		IdempotencyKey:   firstNonEmpty(req.IdempotencyKey, req.Metadata["idempotency_key"]),
		ExecutionProfile: profile,
		Path:             path,
		Metadata:         metadata,
		StartedAt:        time.Now().UTC(),
	}
	if replayed, ok, err := e.tryReplay(record, fingerprint); err != nil {
		return ActionRecord{}, err
	} else if ok {
		return replayed, nil
	}
	if err := e.store.Save(record); err != nil {
		return ActionRecord{}, err
	}

	data, readErr := os.ReadFile(path)
	finishedAt := time.Now().UTC()
	record.FinishedAt = &finishedAt
	if readErr != nil {
		record.Status = ActionStatusFailed
		record.Error = readErr.Error()
		record.FailureCategory = ActionFailureIO
		record.AttemptHistory = []ActionAttempt{{
			Attempt:         record.Attempt,
			Status:          ActionStatusFailed,
			FailureCategory: ActionFailureIO,
			Retryable:       false,
			Error:           readErr.Error(),
			StartedAt:       record.StartedAt,
			FinishedAt:      &finishedAt,
		}}
		if err := e.store.Move(record, ActionStatusFailed); err != nil {
			return ActionRecord{}, err
		}
		return record, nil
	}

	record.Status = ActionStatusCompleted
	record.Stdout = string(data)
	record.AttemptHistory = []ActionAttempt{{
		Attempt:    record.Attempt,
		Status:     ActionStatusCompleted,
		StartedAt:  record.StartedAt,
		FinishedAt: &finishedAt,
	}}
	if err := e.store.Move(record, ActionStatusCompleted); err != nil {
		return ActionRecord{}, err
	}
	return record, nil
}

func (e *Executor) ExecuteFileWrite(req FileWriteActionRequest) (ActionRecord, error) {
	if strings.TrimSpace(req.Path) == "" {
		return ActionRecord{}, fmt.Errorf("%w: path is required", ErrExecutionInvalidInput)
	}
	profile := firstNonEmpty(req.ExecutionProfile, "user")
	if err := e.authorizeProfile(profile, req.ApprovalToken); err != nil {
		return ActionRecord{}, err
	}

	path, err := e.resolveAllowedPath(req.Path)
	if err != nil {
		return ActionRecord{}, err
	}
	metadata := cloneMetadata(req.Metadata)
	fingerprint := executionFingerprint(ActionKindFileWrite, profile, path, req.Content, fmt.Sprintf("%t", req.CreateParents))
	metadata["request_fingerprint"] = fingerprint

	record := ActionRecord{
		ActionID:         fmt.Sprintf("action-%d", time.Now().UTC().UnixNano()),
		TaskID:           strings.TrimSpace(req.TaskID),
		Kind:             ActionKindFileWrite,
		Status:           ActionStatusRunning,
		Attempt:          normalizeAttempt(req.Attempt, req.Metadata),
		IdempotencyKey:   firstNonEmpty(req.IdempotencyKey, req.Metadata["idempotency_key"]),
		ExecutionProfile: profile,
		Path:             path,
		ChangedFiles:     []string{path},
		Metadata:         metadata,
		StartedAt:        time.Now().UTC(),
	}
	if replayed, ok, err := e.tryReplay(record, fingerprint); err != nil {
		return ActionRecord{}, err
	} else if ok {
		return replayed, nil
	}
	if err := e.store.Save(record); err != nil {
		return ActionRecord{}, err
	}

	if req.CreateParents {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			record.Status = ActionStatusFailed
			record.Error = err.Error()
			record.FailureCategory = ActionFailureIO
			finishedAt := time.Now().UTC()
			record.FinishedAt = &finishedAt
			record.AttemptHistory = []ActionAttempt{{
				Attempt:         record.Attempt,
				Status:          ActionStatusFailed,
				FailureCategory: ActionFailureIO,
				Retryable:       false,
				Error:           err.Error(),
				StartedAt:       record.StartedAt,
				FinishedAt:      &finishedAt,
			}}
			if moveErr := e.store.Move(record, ActionStatusFailed); moveErr != nil {
				return ActionRecord{}, moveErr
			}
			return record, nil
		}
	}
	writeErr := os.WriteFile(path, []byte(req.Content), 0o644)
	finishedAt := time.Now().UTC()
	record.FinishedAt = &finishedAt
	if writeErr != nil {
		record.Status = ActionStatusFailed
		record.Error = writeErr.Error()
		record.FailureCategory = ActionFailureIO
		record.AttemptHistory = []ActionAttempt{{
			Attempt:         record.Attempt,
			Status:          ActionStatusFailed,
			FailureCategory: ActionFailureIO,
			Retryable:       false,
			Error:           writeErr.Error(),
			StartedAt:       record.StartedAt,
			FinishedAt:      &finishedAt,
		}}
		if err := e.store.Move(record, ActionStatusFailed); err != nil {
			return ActionRecord{}, err
		}
		return record, nil
	}

	record.Status = ActionStatusCompleted
	record.Stdout = fmt.Sprintf("wrote %d bytes", len(req.Content))
	record.AttemptHistory = []ActionAttempt{{
		Attempt:    record.Attempt,
		Status:     ActionStatusCompleted,
		StartedAt:  record.StartedAt,
		FinishedAt: &finishedAt,
	}}
	if err := e.store.Move(record, ActionStatusCompleted); err != nil {
		return ActionRecord{}, err
	}
	return record, nil
}

func (e *Executor) GetAction(actionID string) (ActionRecord, error) {
	return e.store.Get(actionID)
}

func (e *Executor) ListActions(limit int) ([]ActionRecord, error) {
	return e.store.List(limit)
}

func (e *Executor) resolveCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if strings.Contains(command, string(filepath.Separator)) {
		path, err := e.resolveAllowedPath(command)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%w: command path is not executable", ErrExecutionDenied)
		}
		return path, nil
	}

	if _, ok := allowedCommands[command]; !ok {
		return "", fmt.Errorf("%w: command %q is not allowed", ErrExecutionDenied, command)
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return "", err
	}
	return path, nil
}

func (e *Executor) resolveWorkdir(workdir string) (string, error) {
	if strings.TrimSpace(workdir) == "" {
		return e.workspaceRoot, nil
	}
	return e.resolveAllowedPath(workdir)
}

func (e *Executor) resolveAllowedPath(path string) (string, error) {
	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Join(e.workspaceRoot, path)
	}
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", err
	}
	if isWithin(absPath, e.workspaceRoot) || isWithin(absPath, os.TempDir()) {
		return absPath, nil
	}
	return "", fmt.Errorf("%w: path %q is outside allowed roots", ErrExecutionDenied, absPath)
}

func isWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func normalizeAttempt(explicit int, metadata map[string]string) int {
	if explicit > 0 {
		return explicit
	}
	if metadata != nil {
		if value := strings.TrimSpace(metadata["attempt"]); value != "" {
			var parsed int
			if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 1
}

func normalizeMaxAttempts(explicit, attempt int) int {
	if explicit > 0 {
		if explicit < attempt {
			return attempt
		}
		return explicit
	}
	if attempt <= 0 {
		return 1
	}
	return attempt
}

func performBackoff(attempt int) {
	if attempt <= 1 {
		return
	}
	// Cap the backoff to avoid excessively long sleeps during retries
	ms := 100 * (1 << (attempt - 1))
	if ms > 5000 {
		ms = 5000
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func shouldRetry(actionKind, failureCategory string, attempt, maxAttempts int) bool {
	if attempt >= maxAttempts {
		return false
	}
	switch actionKind {
	case ActionKindShell:
		return failureCategory == ActionFailureTimeout
	case ActionKindFileRead, ActionKindFileWrite:
		return failureCategory == ActionFailureIO
	default:
		return false
	}
}

func (e *Executor) tryReplay(record ActionRecord, fingerprint string) (ActionRecord, bool, error) {
	if strings.TrimSpace(record.IdempotencyKey) == "" {
		return ActionRecord{}, false, nil
	}
	existing, err := e.store.FindCompletedByIdempotency(record.Kind, record.ExecutionProfile, record.IdempotencyKey, fingerprint)
	if err != nil {
		if errors.Is(err, ErrActionNotFound) {
			return ActionRecord{}, false, nil
		}
		return ActionRecord{}, false, err
	}
	finishedAt := time.Now().UTC()
	replayed := record
	replayed.Status = ActionStatusCompleted
	replayed.Replayed = true
	replayed.ReplayOfActionID = existing.ActionID
	replayed.Command = firstNonEmpty(replayed.Command, existing.Command)
	replayed.Args = firstNonEmptySlice(replayed.Args, existing.Args)
	replayed.Path = firstNonEmpty(replayed.Path, existing.Path)
	replayed.Workdir = firstNonEmpty(replayed.Workdir, existing.Workdir)
	replayed.ChangedFiles = firstNonEmptySlice(replayed.ChangedFiles, existing.ChangedFiles)
	replayed.Stdout = existing.Stdout
	replayed.Stderr = existing.Stderr
	replayed.ExitCode = existing.ExitCode
	replayed.StartedAt = time.Now().UTC()
	replayed.FinishedAt = &finishedAt
	replayed.AttemptHistory = []ActionAttempt{{
		Attempt:    replayed.Attempt,
		Status:     ActionStatusCompleted,
		StartedAt:  replayed.StartedAt,
		FinishedAt: &finishedAt,
	}}
	if err := e.store.Save(replayed); err != nil {
		return ActionRecord{}, false, err
	}
	return replayed, true, nil
}

func executionFingerprint(parts ...string) string {
	joined := strings.Join(parts, "\x1f")
	sum := sha1.Sum([]byte(joined))
	return fmt.Sprintf("%x", sum)
}

func (e *Executor) runShellAttempt(commandPath string, args []string, workdir string, timeout time.Duration) shellAttemptResult {
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, commandPath, args...)
	cmd.Dir = workdir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	result := shellAttemptResult{
		stdout:     stdout.String(),
		stderr:     stderr.String(),
		err:        runErr,
		startedAt:  startedAt,
		finishedAt: time.Now().UTC(),
	}
	if runErr == nil {
		return result
	}
	var exitErr *exec.ExitError
	if errors.Is(runErr, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
		result.failureCategory = ActionFailureTimeout
		result.retryable = true
		return result
	}
	if errors.As(runErr, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		result.failureCategory = ActionFailureProcessExit
		return result
	}
	result.failureCategory = ActionFailureExecution
	return result
}

func attemptStatus(err error) string {
	if err == nil {
		return ActionStatusCompleted
	}
	return ActionStatusFailed
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmptySlice[T any](primary, fallback []T) []T {
	if len(primary) > 0 {
		return primary
	}
	if len(fallback) == 0 {
		return nil
	}
	out := make([]T, len(fallback))
	copy(out, fallback)
	return out
}

func timestampPtr(ts time.Time) *time.Time {
	out := ts
	return &out
}

func (e *Executor) authorizeProfile(profile, approvalToken string) error {
	switch profile {
	case "user":
		return nil
	case "root":
		if e.rootAuthToken == "" && e.approvalStore == nil {
			return fmt.Errorf("%w: root profile is not configured", ErrExecutionDenied)
		}
		if strings.TrimSpace(approvalToken) == "" {
			return fmt.Errorf("%w: root profile requires approval token", ErrExecutionDenied)
		}
		if e.approvalStore != nil {
			if _, err := e.approvalStore.Consume(profile, approvalToken); err == nil {
				return nil
			}
		}
		if subtleConstantCompare(strings.TrimSpace(approvalToken), e.rootAuthToken) {
			return nil
		}
		return fmt.Errorf("%w: root approval token is invalid", ErrExecutionDenied)
	default:
		return fmt.Errorf("%w: profile %q is not available in MVP executor", ErrExecutionDenied, profile)
	}
}

func subtleConstantCompare(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	result := byte(0)
	for i := 0; i < len(left); i++ {
		result |= left[i] ^ right[i]
	}
	return result == 0
}

var allowedCommands = map[string]struct{}{
	"cat":     {},
	"echo":    {},
	"git":     {},
	"ls":      {},
	"pwd":     {},
	"python3": {},
	"rg":      {},
	"sed":     {},
}
