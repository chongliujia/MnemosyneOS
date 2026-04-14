package execution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mnemosyneos/internal/approval"
)

func TestExecuteFileWriteAndRead(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutor(store, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}

	writeRecord, err := executor.ExecuteFileWrite(FileWriteActionRequest{
		Path:           "notes/output.txt",
		Content:        "mnemosyne runtime",
		CreateParents:  true,
		Attempt:        2,
		IdempotencyKey: "file-write-notes-output",
	})
	if err != nil {
		t.Fatalf("ExecuteFileWrite returned error: %v", err)
	}
	if writeRecord.Status != ActionStatusCompleted {
		t.Fatalf("expected completed write, got %s", writeRecord.Status)
	}
	if writeRecord.Attempt != 2 {
		t.Fatalf("expected attempt=2, got %d", writeRecord.Attempt)
	}
	if writeRecord.IdempotencyKey != "file-write-notes-output" {
		t.Fatalf("unexpected idempotency key: %q", writeRecord.IdempotencyKey)
	}
	if len(writeRecord.AttemptHistory) != 1 || writeRecord.AttemptHistory[0].Status != ActionStatusCompleted {
		t.Fatalf("expected one completed write attempt, got %#v", writeRecord.AttemptHistory)
	}

	path := filepath.Join(workspaceRoot, "notes", "output.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(data) != "mnemosyne runtime" {
		t.Fatalf("unexpected file content: %q", string(data))
	}

	readRecord, err := executor.ExecuteFileRead(FileReadActionRequest{Path: "notes/output.txt"})
	if err != nil {
		t.Fatalf("ExecuteFileRead returned error: %v", err)
	}
	if readRecord.Stdout != "mnemosyne runtime" {
		t.Fatalf("unexpected read output: %q", readRecord.Stdout)
	}
	if len(readRecord.AttemptHistory) != 1 || readRecord.AttemptHistory[0].Status != ActionStatusCompleted {
		t.Fatalf("expected one completed read attempt, got %#v", readRecord.AttemptHistory)
	}

	if _, err := store.Get(writeRecord.ActionID); err != nil {
		t.Fatalf("expected persisted action record: %v", err)
	}
}

func TestExecuteShellAllowedCommand(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutor(store, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}

	record, err := executor.ExecuteShell(ShellActionRequest{
		Command:        "pwd",
		Workdir:        ".",
		IdempotencyKey: "shell-pwd",
	})
	if err != nil {
		t.Fatalf("ExecuteShell returned error: %v", err)
	}
	if record.Status != ActionStatusCompleted {
		t.Fatalf("expected completed shell action, got %s", record.Status)
	}
	if record.Stdout == "" {
		t.Fatalf("expected stdout from pwd")
	}
	if record.Attempt != 1 {
		t.Fatalf("expected default attempt=1, got %d", record.Attempt)
	}
	if record.IdempotencyKey != "shell-pwd" {
		t.Fatalf("unexpected idempotency key: %q", record.IdempotencyKey)
	}
	if len(record.AttemptHistory) != 1 || record.AttemptHistory[0].Status != ActionStatusCompleted {
		t.Fatalf("expected one completed shell attempt, got %#v", record.AttemptHistory)
	}
}

func TestExecuteShellTimeoutMarkedRetryable(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutor(store, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}

	record, err := executor.ExecuteShell(ShellActionRequest{
		Command:     "python3",
		Args:        []string{"-c", "import time; time.sleep(0.05)"},
		TimeoutMS:   1,
		Attempt:     3,
		MaxAttempts: 4,
	})
	if err != nil {
		t.Fatalf("ExecuteShell returned error: %v", err)
	}
	if record.Status != ActionStatusFailed {
		t.Fatalf("expected failed shell action, got %s", record.Status)
	}
	if !record.Retryable {
		t.Fatalf("expected timed out action to be retryable")
	}
	if record.FailureCategory != ActionFailureTimeout {
		t.Fatalf("expected timeout failure category, got %s", record.FailureCategory)
	}
	if record.Attempt != 4 {
		t.Fatalf("expected final attempt=4, got %d", record.Attempt)
	}
	if len(record.AttemptHistory) != 2 {
		t.Fatalf("expected two timeout attempts, got %#v", record.AttemptHistory)
	}
	for _, attempt := range record.AttemptHistory {
		if attempt.FailureCategory != ActionFailureTimeout || !attempt.Retryable {
			t.Fatalf("expected timeout retryable attempts, got %#v", record.AttemptHistory)
		}
	}
}

func TestExecuteShellProcessExitNotRetryable(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutor(store, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}

	record, err := executor.ExecuteShell(ShellActionRequest{
		Command: "python3",
		Args:    []string{"-c", "import sys; sys.exit(7)"},
	})
	if err != nil {
		t.Fatalf("ExecuteShell returned error: %v", err)
	}
	if record.Status != ActionStatusFailed {
		t.Fatalf("expected failed shell action, got %s", record.Status)
	}
	if record.Retryable {
		t.Fatalf("expected process exit to stay non-retryable")
	}
	if record.FailureCategory != ActionFailureProcessExit {
		t.Fatalf("expected process_exit failure category, got %s", record.FailureCategory)
	}
	if record.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", record.ExitCode)
	}
	if len(record.AttemptHistory) != 1 || record.AttemptHistory[0].FailureCategory != ActionFailureProcessExit {
		t.Fatalf("expected one process-exit attempt, got %#v", record.AttemptHistory)
	}
}

func TestExecuteShellReplaysCompletedActionForMatchingIdempotencyKey(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutor(store, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}

	args := []string{"-c", "import pathlib; p=pathlib.Path('counter.txt'); n=int(p.read_text()) + 1 if p.exists() else 1; p.write_text(str(n)); print(n)"}
	first, err := executor.ExecuteShell(ShellActionRequest{
		Command:        "python3",
		Args:           args,
		Workdir:        ".",
		IdempotencyKey: "counter-once",
	})
	if err != nil {
		t.Fatalf("ExecuteShell first returned error: %v", err)
	}
	second, err := executor.ExecuteShell(ShellActionRequest{
		Command:        "python3",
		Args:           args,
		Workdir:        ".",
		IdempotencyKey: "counter-once",
	})
	if err != nil {
		t.Fatalf("ExecuteShell second returned error: %v", err)
	}
	if first.Replayed {
		t.Fatalf("expected first action to be fresh, got %+v", first)
	}
	if !second.Replayed {
		t.Fatalf("expected second action to be replayed, got %+v", second)
	}
	if second.ReplayOfActionID != first.ActionID {
		t.Fatalf("expected replay_of_action_id=%q, got %q", first.ActionID, second.ReplayOfActionID)
	}
	if strings.TrimSpace(second.Stdout) != "1" {
		t.Fatalf("expected replayed stdout to stay 1, got %q", second.Stdout)
	}
	data, err := os.ReadFile(filepath.Join(workspaceRoot, "counter.txt"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if strings.TrimSpace(string(data)) != "1" {
		t.Fatalf("expected counter file to remain 1, got %q", string(data))
	}
}

func TestExecuteShellDifferentFingerprintDoesNotReplay(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutor(store, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}

	first, err := executor.ExecuteShell(ShellActionRequest{
		Command:        "python3",
		Args:           []string{"-c", "print('alpha')"},
		IdempotencyKey: "shared-key",
	})
	if err != nil {
		t.Fatalf("ExecuteShell first returned error: %v", err)
	}
	second, err := executor.ExecuteShell(ShellActionRequest{
		Command:        "python3",
		Args:           []string{"-c", "print('beta')"},
		IdempotencyKey: "shared-key",
	})
	if err != nil {
		t.Fatalf("ExecuteShell second returned error: %v", err)
	}
	if second.Replayed {
		t.Fatalf("expected different fingerprint not to replay, got %+v", second)
	}
	if first.ActionID == second.ActionID {
		t.Fatalf("expected distinct action ids, got %q", first.ActionID)
	}
	if strings.TrimSpace(second.Stdout) != "beta" {
		t.Fatalf("expected second stdout beta, got %q", second.Stdout)
	}
}

func TestExecuteShellRejectsRootProfile(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutor(store, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}

	if _, err := executor.ExecuteShell(ShellActionRequest{
		Command:          "pwd",
		ExecutionProfile: "root",
	}); err == nil {
		t.Fatalf("expected root profile to be rejected")
	}
}

func TestExecuteShellAllowsRootProfileWithApprovalToken(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutorWithRootToken(store, workspaceRoot, "root-secret")
	if err != nil {
		t.Fatalf("NewExecutorWithRootToken returned error: %v", err)
	}

	record, err := executor.ExecuteShell(ShellActionRequest{
		Command:          "pwd",
		Workdir:          ".",
		ExecutionProfile: "root",
		ApprovalToken:    "root-secret",
	})
	if err != nil {
		t.Fatalf("ExecuteShell returned error: %v", err)
	}
	if record.Status != ActionStatusCompleted {
		t.Fatalf("expected completed shell action, got %s", record.Status)
	}
	if record.ExecutionProfile != "root" {
		t.Fatalf("expected root execution profile, got %s", record.ExecutionProfile)
	}
}

func TestExecuteShellAllowsRootProfileWithApprovedRequest(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	record, err := approvalStore.Create(approval.CreateRequest{
		ExecutionProfile: "root",
		ActionKind:       ActionKindShell,
		Summary:          "root shell test",
		RequestedBy:      "test",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	record, err = approvalStore.Approve(record.ApprovalID, "tester")
	if err != nil {
		t.Fatalf("Approve returned error: %v", err)
	}

	executor, err := NewExecutorWithApprovals(store, workspaceRoot, "", approvalStore)
	if err != nil {
		t.Fatalf("NewExecutorWithApprovals returned error: %v", err)
	}

	action, err := executor.ExecuteShell(ShellActionRequest{
		Command:          "pwd",
		Workdir:          ".",
		ExecutionProfile: "root",
		ApprovalToken:    record.ApprovalToken,
	})
	if err != nil {
		t.Fatalf("ExecuteShell returned error: %v", err)
	}
	if action.Status != ActionStatusCompleted {
		t.Fatalf("expected completed action, got %s", action.Status)
	}

	consumed, err := approvalStore.Get(record.ApprovalID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if consumed.Status != approval.StatusConsumed {
		t.Fatalf("expected consumed approval, got %s", consumed.Status)
	}
}

func TestExecuteFileWriteRejectsRootProfileWithoutApprovalToken(t *testing.T) {
	runtimeRoot := tempExecutionRoot(t)
	workspaceRoot := t.TempDir()

	store := NewStore(runtimeRoot)
	executor, err := NewExecutorWithRootToken(store, workspaceRoot, "root-secret")
	if err != nil {
		t.Fatalf("NewExecutorWithRootToken returned error: %v", err)
	}

	if _, err := executor.ExecuteFileWrite(FileWriteActionRequest{
		Path:             "notes/output.txt",
		Content:          "denied",
		ExecutionProfile: "root",
	}); err == nil {
		t.Fatalf("expected root file write without token to be rejected")
	}
}

func tempExecutionRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	dirs := []string{
		filepath.Join(root, "approvals", approval.StatusPending),
		filepath.Join(root, "approvals", approval.StatusApproved),
		filepath.Join(root, "approvals", approval.StatusDenied),
		filepath.Join(root, "approvals", approval.StatusConsumed),
		filepath.Join(root, "actions", ActionStatusPending),
		filepath.Join(root, "actions", ActionStatusRunning),
		filepath.Join(root, "actions", ActionStatusCompleted),
		filepath.Join(root, "actions", ActionStatusFailed),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	return root
}

func TestStoreGetMissingAction(t *testing.T) {
	root := tempExecutionRoot(t)
	store := NewStore(root)
	if _, err := store.Get("missing"); err == nil {
		t.Fatalf("expected missing action error")
	}
}

func TestStoreMovePersistsUpdatedRecord(t *testing.T) {
	root := tempExecutionRoot(t)
	store := NewStore(root)
	record := ActionRecord{
		ActionID: "action-1",
		Kind:     ActionKindShell,
		Status:   ActionStatusRunning,
	}
	if err := store.Save(record); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	record.Stdout = "done"
	if err := store.Move(record, ActionStatusCompleted); err != nil {
		t.Fatalf("Move returned error: %v", err)
	}

	path := filepath.Join(root, "actions", ActionStatusCompleted, "action-1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var saved ActionRecord
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if saved.Status != ActionStatusCompleted || saved.Stdout != "done" {
		t.Fatalf("unexpected saved record: %+v", saved)
	}
}
