package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/chat"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/recall"
	"mnemosyneos/internal/skills"
)

type runtimeEnv struct {
	runDir         string
	runtimeRoot    string
	scenario       Scenario
	runtimeStore   *airuntime.Store
	orchestrator   *airuntime.Orchestrator
	memoryStore    *memory.Store
	consolidator   *memory.Consolidator
	approvalStore  *approval.Store
	executionStore *execution.Store
	executor       *execution.Executor
	skillRunner    *skills.Runner
	recall         *recall.Service
	chatStore      *chat.Store
	chatService    *chat.Service

	lastTaskID     string
	lastApprovalID string
	stepResults    map[string]StepReport
}

func RunScenario(ctx context.Context, scenario Scenario, outRoot string) (RunReport, error) {
	started := time.Now().UTC()
	report := RunReport{
		ScenarioName:        scenario.Name,
		ScenarioDescription: scenario.Description,
		ScenarioLane:        scenario.Lane,
		ScenarioTags:        append([]string(nil), scenario.Tags...),
		ScenarioPath:        scenario.Dir,
		StartedAt:           started,
		Passed:              false,
	}

	runDir := filepath.Join(outRoot, fmt.Sprintf("%s-%s", started.Format("20060102T150405Z"), slugify(scenario.Name)))
	runtimeRoot := filepath.Join(runDir, "runtime")
	report.RunDir = runDir
	report.RuntimeRoot = runtimeRoot

	env, err := newRuntimeEnv(runDir, runtimeRoot, scenario)
	if err != nil {
		report.Error = err.Error()
		report.FinishedAt = time.Now().UTC()
		_ = writeRunArtifacts(runDir, scenario, report)
		return report, err
	}

	var stepErr error
	for _, step := range scenario.Steps {
		stepReport := StepReport{
			ID:        step.ID,
			Type:      step.Type,
			SessionID: firstNonEmpty(step.SessionID, "default"),
			StartedAt: time.Now().UTC(),
		}
		report.StepReports = append(report.StepReports, stepReport)
		current := &report.StepReports[len(report.StepReports)-1]

		if err := env.executeStep(ctx, step, current); err != nil {
			current.Error = err.Error()
			current.FinishedAt = time.Now().UTC()
			env.stepResults[step.ID] = *current
			stepErr = err
			break
		}
		current.FinishedAt = time.Now().UTC()
		env.stepResults[step.ID] = *current
	}

	if stepErr == nil {
		assertions, passed := env.evaluateAssertions(scenario.Assertions)
		report.AssertionResults = assertions
		report.Passed = passed
	} else {
		report.Error = stepErr.Error()
	}

	report.FinishedAt = time.Now().UTC()
	_ = writeRunArtifacts(runDir, scenario, report)
	if stepErr != nil {
		return report, stepErr
	}
	if !report.Passed {
		return report, fmt.Errorf("scenario %q failed one or more assertions", scenario.Name)
	}
	return report, nil
}

func newRuntimeEnv(runDir, runtimeRoot string, scenario Scenario) (*runtimeEnv, error) {
	if err := bootstrapRuntimeRoot(runtimeRoot); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, err
	}

	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	memoryStore := memory.NewStore()
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	executionStore := execution.NewStore(runtimeRoot)
	workspaceRoot := filepath.Join(runtimeRoot, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		return nil, err
	}
	executor, err := execution.NewExecutorWithApprovals(executionStore, workspaceRoot, "", approvalStore)
	if err != nil {
		return nil, err
	}
	connectorRuntime, err := loadFixtureConnectors(scenario)
	if err != nil {
		return nil, err
	}
	stubModel := stubTextGateway{}
	skillRunner := skills.NewRunner(runtimeStore, memoryStore, executor, connectorRuntime, approvalStore, stubModel)
	recallService := recall.NewService(memoryStore)
	chatStore := chat.NewStore(runtimeRoot)
	chatService := chat.NewService(chatStore, orchestrator, runtimeStore, recallService, skillRunner, stubModel)

	return &runtimeEnv{
		runDir:         runDir,
		runtimeRoot:    runtimeRoot,
		scenario:       scenario,
		runtimeStore:   runtimeStore,
		orchestrator:   orchestrator,
		memoryStore:    memoryStore,
		consolidator:   memory.NewConsolidator(memoryStore),
		approvalStore:  approvalStore,
		executionStore: executionStore,
		executor:       executor,
		skillRunner:    skillRunner,
		recall:         recallService,
		chatStore:      chatStore,
		chatService:    chatService,
		stepResults:    map[string]StepReport{},
	}, nil
}

func (e *runtimeEnv) rebuildServices() error {
	runtimeStore := airuntime.NewStore(e.runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	memoryStore := memory.NewStore()
	approvalStore := approval.NewStore(e.runtimeRoot, 10*time.Minute)
	executionStore := execution.NewStore(e.runtimeRoot)
	workspaceRoot := filepath.Join(e.runtimeRoot, "workspace")
	executor, err := execution.NewExecutorWithApprovals(executionStore, workspaceRoot, "", approvalStore)
	if err != nil {
		return err
	}
	connectorRuntime, err := loadFixtureConnectors(e.scenario)
	if err != nil {
		return err
	}
	stubModel := stubTextGateway{}
	skillRunner := skills.NewRunner(runtimeStore, memoryStore, executor, connectorRuntime, approvalStore, stubModel)
	recallService := recall.NewService(memoryStore)
	chatStore := chat.NewStore(e.runtimeRoot)
	chatService := chat.NewService(chatStore, orchestrator, runtimeStore, recallService, skillRunner, stubModel)

	e.runtimeStore = runtimeStore
	e.orchestrator = orchestrator
	e.memoryStore = memoryStore
	e.consolidator = memory.NewConsolidator(memoryStore)
	e.approvalStore = approvalStore
	e.executionStore = executionStore
	e.executor = executor
	e.skillRunner = skillRunner
	e.recall = recallService
	e.chatStore = chatStore
	e.chatService = chatService
	return nil
}

func bootstrapRuntimeRoot(root string) error {
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
		filepath.Join(root, "actions", execution.ActionStatusPending),
		filepath.Join(root, "actions", execution.ActionStatusRunning),
		filepath.Join(root, "actions", execution.ActionStatusCompleted),
		filepath.Join(root, "actions", execution.ActionStatusFailed),
		filepath.Join(root, "approvals", approval.StatusPending),
		filepath.Join(root, "approvals", approval.StatusApproved),
		filepath.Join(root, "approvals", approval.StatusDenied),
		filepath.Join(root, "approvals", approval.StatusConsumed),
		filepath.Join(root, "artifacts", "reports"),
		filepath.Join(root, "observations", "filesystem"),
		filepath.Join(root, "observations", "os"),
		filepath.Join(root, "sessions", "history"),
		filepath.Join(root, "sessions", "archive"),
		filepath.Join(root, "workspace"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	state := airuntime.RuntimeState{
		RuntimeID:        "harness-runtime",
		ActiveUserID:     "harness-user",
		Status:           "idle",
		ExecutionProfile: "user",
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(root, "state", "runtime.json"), data, 0o644)
}

func (e *runtimeEnv) executeStep(ctx context.Context, step Step, report *StepReport) error {
	switch step.Type {
	case StepTypeSubmitTask:
		req := airuntime.CreateTaskRequest{
			Title:            firstNonEmpty(step.Title, step.Goal),
			Goal:             step.Goal,
			RequestedBy:      firstNonEmpty(step.RequestedBy, "harness"),
			Source:           firstNonEmpty(step.Source, "harness"),
			ExecutionProfile: firstNonEmpty(step.ExecutionProfile, "user"),
			RequiresApproval: step.RequiresApproval,
			SelectedSkill:    step.SelectedSkill,
			Metadata:         cloneMap(step.Metadata),
		}
		task, err := e.orchestrator.SubmitTask(req)
		if err != nil {
			return err
		}
		report.TaskID = task.TaskID
		report.TaskState = task.State
		report.SelectedSkill = task.SelectedSkill
		e.lastTaskID = task.TaskID
		if approvalID := strings.TrimSpace(task.Metadata["root_approval_id"]); approvalID != "" {
			report.ApprovalID = approvalID
			e.lastApprovalID = approvalID
		}
		return nil

	case StepTypeRunTask:
		taskID, err := e.resolveTaskRef(step.TaskRef)
		if err != nil {
			return err
		}
		result, err := e.skillRunner.RunTaskWithProgress(taskID, func(event skills.ProgressEvent) {
			report.Progress = append(report.Progress, StepProgress{
				Stage:   event.Stage,
				Message: event.Message,
			})
		})
		if err != nil {
			return err
		}
		report.TaskID = result.Task.TaskID
		report.TaskState = result.Task.State
		report.SelectedSkill = result.Task.SelectedSkill
		report.ArtifactPaths = append(report.ArtifactPaths, result.ArtifactPaths...)
		report.ObservationPaths = append(report.ObservationPaths, result.ObservationPaths...)
		if result.Action != nil {
			report.ActionID = result.Action.ActionID
			report.ActionStatus = result.Action.Status
			report.ActionFailureCategory = result.Action.FailureCategory
			report.ActionAttempts = len(result.Action.AttemptHistory)
			if report.ActionAttempts == 0 && result.Action.Attempt > 0 {
				report.ActionAttempts = result.Action.Attempt
			}
			if report.ActionAttempts > 1 {
				report.RetryAttempts = report.ActionAttempts - 1
			}
			report.RetrySucceeded = result.Action.Status == execution.ActionStatusCompleted && report.RetryAttempts > 0
		}
		e.lastTaskID = result.Task.TaskID
		if approvalID := strings.TrimSpace(result.Task.Metadata["root_approval_id"]); approvalID != "" {
			report.ApprovalID = approvalID
			e.lastApprovalID = approvalID
		}
		return nil

	case StepTypeApprovePending:
		record, err := e.pickApproval(step.ApprovalRef)
		if err != nil {
			return err
		}
		record, err = e.approvalStore.Approve(record.ApprovalID, firstNonEmpty(step.ApprovedBy, "harness"))
		if err != nil {
			return err
		}
		report.ApprovalID = record.ApprovalID
		report.TaskID = record.TaskID
		e.lastApprovalID = record.ApprovalID
		if record.TaskID != "" {
			updated, moveErr := e.runtimeStore.MoveTask(record.TaskID, airuntime.TaskStatePlanned, func(task *airuntime.Task) {
				if task.Metadata == nil {
					task.Metadata = map[string]string{}
				}
				task.Metadata["root_approval_id"] = record.ApprovalID
				task.NextAction = "root approval granted; rerun task"
				task.FailureReason = ""
			})
			if moveErr != nil {
				return moveErr
			}
			report.TaskState = updated.State
			report.SelectedSkill = updated.SelectedSkill
			e.lastTaskID = updated.TaskID
		}
		return nil

	case StepTypeSendChat:
		beforeCards := latestCardsByID(e.memoryStore)
		resp, err := e.chatService.Send(chat.SendRequest{
			SessionID:        firstNonEmpty(step.SessionID, "default"),
			Message:          step.Message,
			RequestedBy:      firstNonEmpty(step.RequestedBy, "harness"),
			Source:           firstNonEmpty(step.Source, "harness"),
			ExecutionProfile: firstNonEmpty(step.ExecutionProfile, "user"),
		})
		if err != nil {
			return err
		}
		report.SessionID = firstNonEmpty(step.SessionID, "default")
		report.UserContent = resp.UserMessage.Content
		report.AssistantContent = resp.AssistantMessage.Content
		report.TaskID = resp.AssistantMessage.TaskID
		report.TaskState = resp.AssistantMessage.TaskState
		report.SelectedSkill = resp.AssistantMessage.SelectedSkill
		if report.TaskID != "" {
			e.lastTaskID = report.TaskID
			if task, err := e.runtimeStore.GetTask(report.TaskID); err == nil {
				report.TaskState = task.State
				report.SelectedSkill = task.SelectedSkill
				report.ArtifactPaths = append(report.ArtifactPaths, collectTaskArtifacts(task)...)
				if approvalID := strings.TrimSpace(task.Metadata["root_approval_id"]); approvalID != "" {
					report.ApprovalID = approvalID
					e.lastApprovalID = approvalID
				}
			}
		}
		report.MemoryFeedbackUpdates, report.ProcedureFeedbackUpdates = feedbackUpdateCounts(beforeCards, latestCardsByID(e.memoryStore))
		return nil

	case StepTypeRestartRuntime:
		report.Progress = append(report.Progress, StepProgress{
			Stage:   "runtime.restart",
			Message: "Rebuilding harness runtime services from the existing runtime root...",
		})
		if err := e.rebuildServices(); err != nil {
			return err
		}
		return nil

	case StepTypeConsolidate:
		limit := 0
		if raw := strings.TrimSpace(step.Metadata["limit"]); raw != "" {
			fmt.Sscanf(raw, "%d", &limit)
		}
		if parseBool(step.Metadata["extract_procedures"]) {
			report.Progress = append(report.Progress, StepProgress{
				Stage:   "memory.extract_procedures",
				Message: "Extracting procedural candidates from successful task runs...",
			})
			if err := e.extractProcedureCandidates(step.Metadata); err != nil {
				return err
			}
		}
		req := memory.ConsolidateRequest{
			CardType:         strings.TrimSpace(step.Metadata["card_type"]),
			Scope:            strings.TrimSpace(step.Metadata["scope"]),
			Limit:            limit,
			ArchiveRemaining: parseBool(step.Metadata["archive_remaining"]),
		}
		report.Progress = append(report.Progress, StepProgress{
			Stage:   "memory.consolidate",
			Message: fmt.Sprintf("Promoting candidate memory card_type=%s scope=%s", firstNonEmpty(req.CardType, "all"), firstNonEmpty(req.Scope, "all")),
		})
		result, err := e.consolidator.PromoteCandidates(req)
		if err != nil {
			return err
		}
		report.CardType = req.CardType
		report.PromotedCount = result.Promoted
		report.SupersededCount = result.Superseded
		report.ArchivedCount = result.Archived
		report.ObservationPaths = append(report.ObservationPaths, fmt.Sprintf("examined=%d promoted=%d", result.Examined, result.Promoted))
		return nil

	case StepTypeSeedMemoryCard:
		req, err := seedCardRequestFromMetadata(step.Metadata)
		if err != nil {
			return err
		}
		card, err := e.memoryStore.CreateCard(req)
		if err != nil {
			return err
		}
		report.Progress = append(report.Progress, StepProgress{
			Stage:   "memory.seed",
			Message: fmt.Sprintf("Seeded %s %s status=%s", card.CardType, card.CardID, card.Status),
		})
		return nil

	default:
		return fmt.Errorf("unsupported step type %q", step.Type)
	}
}

func seedCardRequestFromMetadata(metadata map[string]string) (memory.CreateCardRequest, error) {
	cardID := strings.TrimSpace(metadata["card_id"])
	cardType := strings.TrimSpace(metadata["card_type"])
	if cardID == "" || cardType == "" {
		return memory.CreateCardRequest{}, fmt.Errorf("seed_memory_card requires metadata.card_id and metadata.card_type")
	}
	req := memory.CreateCardRequest{
		CardID:     cardID,
		CardType:   cardType,
		Scope:      strings.TrimSpace(metadata["scope"]),
		Status:     strings.TrimSpace(metadata["status"]),
		Supersedes: strings.TrimSpace(metadata["supersedes"]),
		Content:    map[string]any{},
		Provenance: memory.Provenance{
			AgentID: "harness",
			Source:  firstNonEmpty(strings.TrimSpace(metadata["source"]), "harness"),
		},
	}
	if raw := strings.TrimSpace(metadata["activation_score"]); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return memory.CreateCardRequest{}, fmt.Errorf("invalid activation_score %q: %w", raw, err)
		}
		req.Activation = &memory.ActivationState{Score: value}
	}
	if raw := strings.TrimSpace(metadata["activation_decay_policy"]); raw != "" {
		if req.Activation == nil {
			req.Activation = &memory.ActivationState{Score: 1.0}
		}
		req.Activation.DecayPolicy = raw
	}
	if raw := strings.TrimSpace(metadata["confidence"]); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return memory.CreateCardRequest{}, fmt.Errorf("invalid confidence %q: %w", raw, err)
		}
		req.Provenance.Confidence = value
	}
	for key, value := range metadata {
		if strings.HasPrefix(key, "content.") {
			req.Content[strings.TrimPrefix(key, "content.")] = value
		}
	}
	return req, nil
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (e *runtimeEnv) extractProcedureCandidates(metadata map[string]string) error {
	tasks, err := e.runtimeStore.ListTasks()
	if err != nil {
		return err
	}
	candidates, _ := memory.BuildProcedureCandidates(memory.ProcedureExtractionRequest{
		Tasks:            tasks,
		TaskClass:        strings.TrimSpace(metadata["task_class"]),
		SelectedSkill:    strings.TrimSpace(metadata["selected_skill"]),
		Scope:            firstNonEmpty(strings.TrimSpace(metadata["scope"]), memory.ScopeProject),
		MinRuns:          parseInt(metadata["min_runs"], 2),
		EvidenceResolver: harnessProcedureEvidenceForTask,
	})
	for _, candidate := range candidates {
		if _, err := e.memoryStore.CreateCard(candidate); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
	}
	return nil
}

func harnessProcedureEvidenceForTask(task airuntime.Task) memory.ProcedureEvidence {
	for _, key := range harnessMetadataKeysBySuffix(task.Metadata, "_observation") {
		if evidence := harnessReadProcedureObservation(strings.TrimSpace(task.Metadata[key])); strings.TrimSpace(evidence.Steps) != "" {
			return evidence
		}
	}
	for _, key := range harnessMetadataKeysBySuffix(task.Metadata, "_artifact") {
		if evidence := harnessReadProcedureArtifact(strings.TrimSpace(task.Metadata[key])); strings.TrimSpace(evidence.Steps) != "" {
			return evidence
		}
	}
	return memory.ProcedureEvidence{
		Steps:           strings.TrimSpace(task.Metadata["procedure_steps"]),
		Guardrails:      strings.TrimSpace(task.Metadata["procedure_guardrails"]),
		Summary:         strings.TrimSpace(task.Metadata["procedure_summary"]),
		SuccessSignal:   strings.TrimSpace(task.Metadata["procedure_success_signal"]),
		ArtifactPath:    harnessFirstMetadataValue(task.Metadata, "_artifact"),
		ObservationPath: harnessFirstMetadataValue(task.Metadata, "_observation"),
	}
}

func harnessMetadataKeysBySuffix(metadata map[string]string, suffix string) []string {
	keys := make([]string, 0)
	for key, value := range metadata {
		if strings.HasSuffix(key, suffix) && strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func harnessFirstMetadataValue(metadata map[string]string, suffix string) string {
	keys := harnessMetadataKeysBySuffix(metadata, suffix)
	if len(keys) == 0 {
		return ""
	}
	return strings.TrimSpace(metadata[keys[0]])
}

func harnessReadProcedureObservation(path string) memory.ProcedureEvidence {
	if strings.TrimSpace(path) == "" {
		return memory.ProcedureEvidence{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return memory.ProcedureEvidence{}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return memory.ProcedureEvidence{}
	}
	return memory.ProcedureEvidence{
		Steps:           strings.TrimSpace(harnessStringValue(payload["procedure_steps"])),
		Guardrails:      strings.TrimSpace(harnessStringValue(payload["procedure_guardrails"])),
		Summary:         strings.TrimSpace(harnessStringValue(payload["procedure_summary"])),
		SuccessSignal:   strings.TrimSpace(harnessStringValue(payload["procedure_success_signal"])),
		ArtifactPath:    strings.TrimSpace(harnessStringValue(payload["artifact_path"])),
		ObservationPath: path,
	}
}

func harnessReadProcedureArtifact(path string) memory.ProcedureEvidence {
	if strings.TrimSpace(path) == "" {
		return memory.ProcedureEvidence{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return memory.ProcedureEvidence{}
	}
	text := string(data)
	steps := harnessMarkdownSectionBullets(text, "Steps:")
	if strings.TrimSpace(steps) == "" {
		return memory.ProcedureEvidence{}
	}
	return memory.ProcedureEvidence{
		Steps:         steps,
		Guardrails:    harnessMarkdownSectionBullets(text, "Guardrails:"),
		Summary:       harnessMarkdownInlinePrefix(text, "Summary:"),
		SuccessSignal: harnessMarkdownInlinePrefix(text, "Success signal:"),
		ArtifactPath:  path,
	}
}

func harnessMarkdownInlinePrefix(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func harnessMarkdownSectionBullets(text, header string) string {
	lines := strings.Split(text, "\n")
	inSection := false
	var collected []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == header:
			inSection = true
			continue
		case !inSection:
			continue
		case trimmed == "":
			if len(collected) > 0 {
				return strings.Join(collected, "\n")
			}
			continue
		case strings.HasPrefix(trimmed, "## "):
			return strings.Join(collected, "\n")
		case strings.HasPrefix(trimmed, "- "):
			collected = append(collected, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}
	return strings.Join(collected, "\n")
}

func harnessStringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func (e *runtimeEnv) evaluateAssertions(assertions []Assertion) ([]AssertionResult, bool) {
	if len(assertions) == 0 {
		return nil, true
	}
	results := make([]AssertionResult, 0, len(assertions))
	passed := true
	for _, assertion := range assertions {
		result := e.evaluateAssertion(assertion)
		results = append(results, result)
		if !result.Passed {
			passed = false
		}
	}
	return results, passed
}

func (e *runtimeEnv) evaluateAssertion(assertion Assertion) AssertionResult {
	switch assertion.Type {
	case AssertTaskState:
		if step := e.stepReport(assertion.Step); step != nil && strings.TrimSpace(step.TaskState) != "" {
			if step.TaskState == assertion.Equals {
				return passAssertion(assertion, fmt.Sprintf("step %s recorded state %s", assertion.Step, step.TaskState))
			}
			return failAssertion(assertion, fmt.Sprintf("step %s task_state=%s expected=%s", assertion.Step, step.TaskState, assertion.Equals))
		}
		taskID, err := e.resolveTaskRef(assertion.Step)
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		task, err := e.runtimeStore.GetTask(taskID)
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		if task.State == assertion.Equals {
			return passAssertion(assertion, fmt.Sprintf("task %s reached %s", task.TaskID, task.State))
		}
		return failAssertion(assertion, fmt.Sprintf("task %s state=%s expected=%s", task.TaskID, task.State, assertion.Equals))

	case AssertSelectedSkill:
		if step := e.stepReport(assertion.Step); step != nil && strings.TrimSpace(step.SelectedSkill) != "" {
			if step.SelectedSkill == assertion.Equals {
				return passAssertion(assertion, fmt.Sprintf("step %s recorded selected skill %s", assertion.Step, step.SelectedSkill))
			}
			return failAssertion(assertion, fmt.Sprintf("step %s selected skill=%s expected=%s", assertion.Step, step.SelectedSkill, assertion.Equals))
		}
		taskID, err := e.resolveTaskRef(assertion.Step)
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		task, err := e.runtimeStore.GetTask(taskID)
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		if task.SelectedSkill == assertion.Equals {
			return passAssertion(assertion, fmt.Sprintf("task %s selected skill %s", task.TaskID, task.SelectedSkill))
		}
		return failAssertion(assertion, fmt.Sprintf("task %s selected skill=%s expected=%s", task.TaskID, task.SelectedSkill, assertion.Equals))

	case AssertApprovalCount:
		records, err := e.approvalStore.List(assertion.Status)
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		return evaluateCountAssertion(assertion, len(records), fmt.Sprintf("approval status=%s", firstNonEmpty(assertion.Status, "all")))

	case AssertArtifactCount:
		step := e.stepReport(assertion.Step)
		if step == nil {
			return failAssertion(assertion, fmt.Sprintf("step %q not found", assertion.Step))
		}
		return evaluateCountAssertion(assertion, len(step.ArtifactPaths), fmt.Sprintf("step %s artifacts", assertion.Step))

	case AssertObservationCount:
		step := e.stepReport(assertion.Step)
		if step == nil {
			return failAssertion(assertion, fmt.Sprintf("step %q not found", assertion.Step))
		}
		return evaluateCountAssertion(assertion, len(step.ObservationPaths), fmt.Sprintf("step %s observations", assertion.Step))

	case AssertArtifactContains:
		step := e.stepReport(assertion.Step)
		if step == nil {
			return failAssertion(assertion, fmt.Sprintf("step %q not found", assertion.Step))
		}
		for _, path := range step.ArtifactPaths {
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if strings.Contains(string(raw), assertion.Contains) {
				return passAssertion(assertion, fmt.Sprintf("artifact %s contained %q", filepath.Base(path), assertion.Contains))
			}
		}
		return failAssertion(assertion, fmt.Sprintf("no artifact from step %s contained %q", assertion.Step, assertion.Contains))

	case AssertAssistantContains:
		step := e.stepReport(assertion.Step)
		if step == nil {
			return failAssertion(assertion, fmt.Sprintf("step %q not found", assertion.Step))
		}
		if strings.Contains(step.AssistantContent, assertion.Contains) {
			return passAssertion(assertion, fmt.Sprintf("assistant reply contained %q", assertion.Contains))
		}
		return failAssertion(assertion, fmt.Sprintf("assistant reply for step %s did not contain %q", assertion.Step, assertion.Contains))

	case AssertFileContains:
		path := assertion.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(e.runtimeRoot, path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		if strings.Contains(string(raw), assertion.Contains) {
			return passAssertion(assertion, fmt.Sprintf("file %s contained %q", path, assertion.Contains))
		}
		return failAssertion(assertion, fmt.Sprintf("file %s did not contain %q", path, assertion.Contains))

	case AssertFileAbsent:
		path := assertion.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(e.runtimeRoot, path)
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return passAssertion(assertion, fmt.Sprintf("file %s is absent", path))
			}
			return failAssertion(assertion, err.Error())
		}
		return failAssertion(assertion, fmt.Sprintf("file %s exists but should be absent", path))

	case AssertSessionStateContain:
		sessionID := firstNonEmpty(assertion.SessionID, "default")
		state, err := e.chatService.SessionState(sessionID)
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		value := sessionStateField(state, assertion.Field)
		if strings.Contains(value, assertion.Contains) {
			return passAssertion(assertion, fmt.Sprintf("session field %s contained %q", assertion.Field, assertion.Contains))
		}
		return failAssertion(assertion, fmt.Sprintf("session field %s=%q did not contain %q", assertion.Field, value, assertion.Contains))

	case AssertWorkingTopicContains:
		state, err := e.chatService.SessionState(firstNonEmpty(assertion.SessionID, "default"))
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		if strings.Contains(state.Topic, assertion.Contains) {
			return passAssertion(assertion, fmt.Sprintf("working topic contained %q", assertion.Contains))
		}
		return failAssertion(assertion, fmt.Sprintf("working topic=%q did not contain %q", state.Topic, assertion.Contains))

	case AssertWorkingFocusTaskEquals:
		state, err := e.chatService.SessionState(firstNonEmpty(assertion.SessionID, "default"))
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		actual := strings.TrimSpace(state.FocusTaskID)
		expected := strings.TrimSpace(assertion.Equals)
		if actual == expected {
			return passAssertion(assertion, fmt.Sprintf("working focus task matched %s", expected))
		}
		return failAssertion(assertion, fmt.Sprintf("working focus_task_id=%q expected=%q", actual, expected))

	case AssertWorkingPendingQuestionContains:
		state, err := e.chatService.SessionState(firstNonEmpty(assertion.SessionID, "default"))
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		if strings.Contains(state.PendingQuestion, assertion.Contains) {
			return passAssertion(assertion, fmt.Sprintf("working pending question contained %q", assertion.Contains))
		}
		return failAssertion(assertion, fmt.Sprintf("working pending_question=%q did not contain %q", state.PendingQuestion, assertion.Contains))

	case AssertWorkingPendingActionContains:
		state, err := e.chatService.SessionState(firstNonEmpty(assertion.SessionID, "default"))
		if err != nil {
			return failAssertion(assertion, err.Error())
		}
		if strings.Contains(state.PendingAction, assertion.Contains) {
			return passAssertion(assertion, fmt.Sprintf("working pending action contained %q", assertion.Contains))
		}
		return failAssertion(assertion, fmt.Sprintf("working pending_action=%q did not contain %q", state.PendingAction, assertion.Contains))

	case AssertMemoryCardCount:
		resp := e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)})
		return evaluateCountAssertion(assertion, len(resp.Cards), fmt.Sprintf("memory cards type=%s", firstNonEmpty(assertion.Field, "all")))

	case AssertMemoryCardContains:
		resp := e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)})
		for _, card := range resp.Cards {
			raw, err := json.Marshal(card.Content)
			if err != nil {
				continue
			}
			if strings.Contains(string(raw), assertion.Contains) {
				return passAssertion(assertion, fmt.Sprintf("memory card %s contained %q", card.CardID, assertion.Contains))
			}
		}
		return failAssertion(assertion, fmt.Sprintf("no memory card type=%s contained %q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))

	case AssertDurableCardCount:
		resp := e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)})
		return evaluateCountAssertion(assertion, len(resp.Cards), fmt.Sprintf("durable cards type=%s", firstNonEmpty(assertion.Field, "all")))

	case AssertDurableCardContains:
		resp := e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)})
		for _, card := range resp.Cards {
			raw, err := json.Marshal(card.Content)
			if err != nil {
				continue
			}
			if strings.Contains(string(raw), assertion.Contains) {
				return passAssertion(assertion, fmt.Sprintf("durable card %s contained %q", card.CardID, assertion.Contains))
			}
		}
		return failAssertion(assertion, fmt.Sprintf("no durable card type=%s contained %q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))

	case AssertDurableCardStatus:
		card, ok := findMatchingCard(
			e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)}).Cards,
			assertion.Contains,
		)
		if !ok {
			return failAssertion(assertion, fmt.Sprintf("no durable card type=%s matched contains=%q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))
		}
		if card.Status == assertion.Equals {
			return passAssertion(assertion, fmt.Sprintf("durable card %s status=%s", card.CardID, card.Status))
		}
		return failAssertion(assertion, fmt.Sprintf("durable card %s status=%s expected=%s", card.CardID, card.Status, assertion.Equals))

	case AssertDurableCardConfidenceRange:
		card, ok := findMatchingCard(
			e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)}).Cards,
			assertion.Contains,
		)
		if !ok {
			return failAssertion(assertion, fmt.Sprintf("no durable card type=%s matched contains=%q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))
		}
		actual := card.Provenance.Confidence
		if confidenceInRange(actual, assertion.MinConfidence, assertion.MaxConfidence) {
			return passAssertion(assertion, fmt.Sprintf("durable card %s confidence=%.3f within [%.3f, %.3f]", card.CardID, actual, assertion.MinConfidence, assertion.MaxConfidence))
		}
		return failAssertion(assertion, fmt.Sprintf("durable card %s confidence=%.3f outside [%.3f, %.3f]", card.CardID, actual, assertion.MinConfidence, assertion.MaxConfidence))

	case AssertDurableCardScope:
		card, ok := findMatchingCard(
			e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)}).Cards,
			assertion.Contains,
		)
		if !ok {
			return failAssertion(assertion, fmt.Sprintf("no durable card type=%s matched contains=%q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))
		}
		actual := cardScope(card)
		if actual == strings.TrimSpace(assertion.Equals) {
			return passAssertion(assertion, fmt.Sprintf("durable card %s scope=%s", card.CardID, actual))
		}
		return failAssertion(assertion, fmt.Sprintf("durable card %s scope=%q expected=%q", card.CardID, actual, assertion.Equals))

	case AssertDurableCardSupersedes:
		card, ok := findMatchingCard(
			e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)}).Cards,
			assertion.Contains,
		)
		if !ok {
			return failAssertion(assertion, fmt.Sprintf("no durable card type=%s matched contains=%q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))
		}
		expected := strings.TrimSpace(assertion.Equals)
		if strings.TrimSpace(card.Supersedes) == expected {
			return passAssertion(assertion, fmt.Sprintf("durable card %s supersedes=%s", card.CardID, card.Supersedes))
		}
		return failAssertion(assertion, fmt.Sprintf("durable card %s supersedes=%q expected=%q", card.CardID, card.Supersedes, expected))

	case AssertDurableCardVersionEquals:
		card, ok := findMatchingCard(
			e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)}).Cards,
			assertion.Contains,
		)
		if !ok {
			return failAssertion(assertion, fmt.Sprintf("no durable card type=%s matched contains=%q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))
		}
		if card.Version == assertion.Expected {
			return passAssertion(assertion, fmt.Sprintf("durable card %s version=%d", card.CardID, card.Version))
		}
		return failAssertion(assertion, fmt.Sprintf("durable card %s version=%d expected=%d", card.CardID, card.Version, assertion.Expected))

	case AssertDurableCardVersionAtLeast:
		card, ok := findMatchingCard(
			e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)}).Cards,
			assertion.Contains,
		)
		if !ok {
			return failAssertion(assertion, fmt.Sprintf("no durable card type=%s matched contains=%q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))
		}
		if card.Version >= assertion.Min {
			return passAssertion(assertion, fmt.Sprintf("durable card %s version=%d min=%d", card.CardID, card.Version, assertion.Min))
		}
		return failAssertion(assertion, fmt.Sprintf("durable card %s version=%d min=%d", card.CardID, card.Version, assertion.Min))

	case AssertDurableCardActivationRange:
		card, ok := findMatchingCard(
			e.memoryStore.Query(memory.QueryRequest{CardType: strings.TrimSpace(assertion.Field)}).Cards,
			assertion.Contains,
		)
		if !ok {
			return failAssertion(assertion, fmt.Sprintf("no durable card type=%s matched contains=%q", firstNonEmpty(assertion.Field, "all"), assertion.Contains))
		}
		actual := card.Activation.Score
		if confidenceInRange(actual, assertion.MinConfidence, assertion.MaxConfidence) {
			return passAssertion(assertion, fmt.Sprintf("durable card %s activation_score=%.3f within [%.3f, %.3f]", card.CardID, actual, assertion.MinConfidence, assertion.MaxConfidence))
		}
		return failAssertion(assertion, fmt.Sprintf("durable card %s activation_score=%.3f outside [%.3f, %.3f]", card.CardID, actual, assertion.MinConfidence, assertion.MaxConfidence))

	case AssertEdgeCount:
		resp := e.memoryStore.Query(memory.QueryRequest{})
		count := 0
		edgeType := strings.TrimSpace(assertion.Field)
		for _, edge := range resp.Edges {
			if edgeType == "" || edge.EdgeType == edgeType {
				count++
			}
		}
		return evaluateCountAssertion(assertion, count, fmt.Sprintf("memory edges type=%s", firstNonEmpty(edgeType, "all")))

	case AssertEdgeExists:
		resp := e.memoryStore.Query(memory.QueryRequest{})
		edgeType := strings.TrimSpace(assertion.Field)
		contains := strings.TrimSpace(assertion.Contains)
		for _, edge := range resp.Edges {
			if edgeType != "" && edge.EdgeType != edgeType {
				continue
			}
			if contains != "" && !strings.Contains(edge.FromCardID, contains) && !strings.Contains(edge.ToCardID, contains) {
				continue
			}
			return passAssertion(assertion, fmt.Sprintf("edge %s exists type=%s from=%s to=%s", edge.EdgeID, edge.EdgeType, edge.FromCardID, edge.ToCardID))
		}
		return failAssertion(assertion, fmt.Sprintf("no edge exists for type=%s contains=%q", firstNonEmpty(edgeType, "all"), contains))

	case AssertRecallContains:
		req := recall.Request{
			Query: strings.TrimSpace(assertion.Query),
			Limit: 10,
		}
		if source := strings.TrimSpace(assertion.Source); source != "" {
			req.Sources = []string{source}
		}
		resp := e.recall.Recall(req)
		for _, hit := range resp.Hits {
			if strings.Contains(hit.Snippet, assertion.Contains) {
				return passAssertion(assertion, fmt.Sprintf("recall hit %s contained %q", hit.CardID, assertion.Contains))
			}
			raw, err := json.Marshal(hit.Card.Content)
			if err == nil && strings.Contains(string(raw), assertion.Contains) {
				return passAssertion(assertion, fmt.Sprintf("recall card %s contained %q", hit.CardID, assertion.Contains))
			}
		}
		return failAssertion(assertion, fmt.Sprintf("recall query=%q source=%q did not contain %q", assertion.Query, assertion.Source, assertion.Contains))

	case AssertRecallNotContains:
		req := recall.Request{
			Query: strings.TrimSpace(assertion.Query),
			Limit: 10,
		}
		if source := strings.TrimSpace(assertion.Source); source != "" {
			req.Sources = []string{source}
		}
		resp := e.recall.Recall(req)
		for _, hit := range resp.Hits {
			if strings.Contains(hit.Snippet, assertion.Contains) {
				return failAssertion(assertion, fmt.Sprintf("recall hit %s unexpectedly contained %q", hit.CardID, assertion.Contains))
			}
			raw, err := json.Marshal(hit.Card.Content)
			if err == nil && strings.Contains(string(raw), assertion.Contains) {
				return failAssertion(assertion, fmt.Sprintf("recall card %s unexpectedly contained %q", hit.CardID, assertion.Contains))
			}
		}
		return passAssertion(assertion, fmt.Sprintf("recall query=%q source=%q did not contain %q", assertion.Query, assertion.Source, assertion.Contains))

	case AssertProcedureCount:
		resp := e.memoryStore.Query(memory.QueryRequest{
			CardType: "procedure",
			Status:   memory.CardStatusActive,
		})
		return evaluateCountAssertion(assertion, len(resp.Cards), "active procedures")

	case AssertProcedureContains:
		resp := e.memoryStore.Query(memory.QueryRequest{
			CardType: "procedure",
			Status:   memory.CardStatusActive,
		})
		for _, card := range resp.Cards {
			raw, err := json.Marshal(card.Content)
			if err != nil {
				continue
			}
			if strings.Contains(string(raw), assertion.Contains) {
				return passAssertion(assertion, fmt.Sprintf("procedure %s contained %q", card.CardID, assertion.Contains))
			}
		}
		return failAssertion(assertion, fmt.Sprintf("no active procedure contained %q", assertion.Contains))

	case AssertProcedureStepContains:
		resp := e.memoryStore.Query(memory.QueryRequest{
			CardType: "procedure",
			Status:   memory.CardStatusActive,
		})
		for _, card := range resp.Cards {
			for _, value := range procedureFields(card) {
				if strings.Contains(value, assertion.Contains) {
					return passAssertion(assertion, fmt.Sprintf("procedure %s step contained %q", card.CardID, assertion.Contains))
				}
			}
		}
		return failAssertion(assertion, fmt.Sprintf("no active procedure step contained %q", assertion.Contains))

	case AssertActionAttemptCount:
		step := e.stepReport(assertion.Step)
		if step == nil {
			return failAssertion(assertion, fmt.Sprintf("step %q not found", assertion.Step))
		}
		return evaluateCountAssertion(assertion, step.ActionAttempts, fmt.Sprintf("step %s action attempts", assertion.Step))

	case AssertRetrySucceeded:
		step := e.stepReport(assertion.Step)
		if step == nil {
			return failAssertion(assertion, fmt.Sprintf("step %q not found", assertion.Step))
		}
		expected := parseBool(assertion.Equals)
		if step.RetrySucceeded == expected {
			return passAssertion(assertion, fmt.Sprintf("step %s retry_succeeded=%t", assertion.Step, step.RetrySucceeded))
		}
		return failAssertion(assertion, fmt.Sprintf("step %s retry_succeeded=%t expected=%t", assertion.Step, step.RetrySucceeded, expected))

	default:
		return failAssertion(assertion, fmt.Sprintf("unsupported assertion type %q", assertion.Type))
	}
}

func (e *runtimeEnv) resolveTaskRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	switch ref {
	case "", "last":
		if strings.TrimSpace(e.lastTaskID) == "" {
			return "", fmt.Errorf("no last task is available")
		}
		return e.lastTaskID, nil
	}
	if report := e.stepReport(ref); report != nil && strings.TrimSpace(report.TaskID) != "" {
		return report.TaskID, nil
	}
	return ref, nil
}

func (e *runtimeEnv) pickApproval(ref string) (approval.Request, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "pending" {
		records, err := e.approvalStore.List(approval.StatusPending)
		if err != nil {
			return approval.Request{}, err
		}
		if len(records) == 0 {
			return approval.Request{}, fmt.Errorf("no pending approvals available")
		}
		sort.Slice(records, func(i, j int) bool {
			return records[i].CreatedAt.After(records[j].CreatedAt)
		})
		return records[0], nil
	}
	if ref == "last" {
		if strings.TrimSpace(e.lastApprovalID) == "" {
			return approval.Request{}, fmt.Errorf("no last approval is available")
		}
		return e.approvalStore.Get(e.lastApprovalID)
	}
	if report := e.stepReport(ref); report != nil && strings.TrimSpace(report.ApprovalID) != "" {
		return e.approvalStore.Get(report.ApprovalID)
	}
	return e.approvalStore.Get(ref)
}

func (e *runtimeEnv) stepReport(id string) *StepReport {
	report, ok := e.stepResults[id]
	if !ok {
		return nil
	}
	copy := report
	return &copy
}

func writeRunArtifacts(runDir string, scenario Scenario, report RunReport) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	scenarioCopy := scenario
	scenarioCopy.Dir = ""
	if err := writeJSON(filepath.Join(runDir, "scenario.json"), scenarioCopy); err != nil {
		return err
	}
	return writeJSON(filepath.Join(runDir, "report.json"), report)
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func passAssertion(assertion Assertion, details string) AssertionResult {
	return AssertionResult{
		Type:        assertion.Type,
		Step:        assertion.Step,
		Passed:      true,
		Description: assertionDescription(assertion),
		Details:     details,
	}
}

func failAssertion(assertion Assertion, details string) AssertionResult {
	return AssertionResult{
		Type:        assertion.Type,
		Step:        assertion.Step,
		Passed:      false,
		Description: assertionDescription(assertion),
		Details:     details,
	}
}

func evaluateCountAssertion(assertion Assertion, actual int, subject string) AssertionResult {
	if assertion.Min > 0 {
		if actual >= assertion.Min {
			return passAssertion(assertion, fmt.Sprintf("%s count=%d min=%d", subject, actual, assertion.Min))
		}
		return failAssertion(assertion, fmt.Sprintf("%s count=%d min=%d", subject, actual, assertion.Min))
	}
	if actual == assertion.Expected {
		return passAssertion(assertion, fmt.Sprintf("%s count=%d", subject, actual))
	}
	return failAssertion(assertion, fmt.Sprintf("%s count=%d expected=%d", subject, actual, assertion.Expected))
}

func findMatchingCard(cards []memory.Card, contains string) (memory.Card, bool) {
	if len(cards) == 0 {
		return memory.Card{}, false
	}
	contains = strings.TrimSpace(contains)
	if contains == "" {
		return cards[0], true
	}
	for _, card := range cards {
		raw, err := json.Marshal(card.Content)
		if err != nil {
			continue
		}
		if strings.Contains(string(raw), contains) {
			return card, true
		}
	}
	return memory.Card{}, false
}

func latestCardsByID(store *memory.Store) map[string]memory.Card {
	if store == nil {
		return nil
	}
	cards := store.LatestCards()
	out := make(map[string]memory.Card, len(cards))
	for _, card := range cards {
		out[card.CardID] = card
	}
	return out
}

func feedbackUpdateCounts(before, after map[string]memory.Card) (int, int) {
	if len(after) == 0 {
		return 0, 0
	}
	memoryUpdates := 0
	procedureUpdates := 0
	for cardID, next := range after {
		prev, ok := before[cardID]
		if !ok || next.Version <= prev.Version {
			continue
		}
		memoryUpdates++
		if next.CardType == "procedure" {
			procedureUpdates++
		}
	}
	return memoryUpdates, procedureUpdates
}

func confidenceInRange(actual, minValue, maxValue float64) bool {
	if minValue != 0 && actual < minValue {
		return false
	}
	if maxValue != 0 && actual > maxValue {
		return false
	}
	return true
}

func cardScope(card memory.Card) string {
	if scope := strings.TrimSpace(card.Scope); scope != "" {
		return scope
	}
	if card.Content != nil {
		if scope, ok := card.Content["scope"].(string); ok {
			return strings.TrimSpace(scope)
		}
	}
	switch card.CardType {
	case "email_inbox", "email_summary", "email_thread", "email_message":
		return "user"
	case "web_search", "search_summary", "web_result":
		return "project"
	case "github_issue_search", "github_issue_summary", "github_issue":
		return "project"
	default:
		return ""
	}
}

func assertionDescription(assertion Assertion) string {
	switch assertion.Type {
	case AssertTaskState:
		return fmt.Sprintf("task from %s reaches %s", assertion.Step, assertion.Equals)
	case AssertSelectedSkill:
		return fmt.Sprintf("task from %s selects %s", assertion.Step, assertion.Equals)
	case AssertApprovalCount:
		return fmt.Sprintf("approval count for status %s", firstNonEmpty(assertion.Status, "all"))
	case AssertArtifactCount:
		return fmt.Sprintf("artifact count for %s", assertion.Step)
	case AssertObservationCount:
		return fmt.Sprintf("observation count for %s", assertion.Step)
	case AssertArtifactContains:
		return fmt.Sprintf("artifact from %s contains %q", assertion.Step, assertion.Contains)
	case AssertAssistantContains:
		return fmt.Sprintf("assistant reply from %s contains %q", assertion.Step, assertion.Contains)
	case AssertFileContains:
		return fmt.Sprintf("file %s contains %q", assertion.Path, assertion.Contains)
	case AssertFileAbsent:
		return fmt.Sprintf("file %s is absent", assertion.Path)
	case AssertSessionStateContain:
		return fmt.Sprintf("session %s field %s contains %q", firstNonEmpty(assertion.SessionID, "default"), assertion.Field, assertion.Contains)
	case AssertWorkingTopicContains:
		return fmt.Sprintf("working topic for session %s contains %q", firstNonEmpty(assertion.SessionID, "default"), assertion.Contains)
	case AssertWorkingFocusTaskEquals:
		return fmt.Sprintf("working focus task for session %s equals %q", firstNonEmpty(assertion.SessionID, "default"), assertion.Equals)
	case AssertWorkingPendingQuestionContains:
		return fmt.Sprintf("working pending question for session %s contains %q", firstNonEmpty(assertion.SessionID, "default"), assertion.Contains)
	case AssertWorkingPendingActionContains:
		return fmt.Sprintf("working pending action for session %s contains %q", firstNonEmpty(assertion.SessionID, "default"), assertion.Contains)
	case AssertMemoryCardCount:
		return fmt.Sprintf("memory card count for type %s", firstNonEmpty(assertion.Field, "all"))
	case AssertMemoryCardContains:
		return fmt.Sprintf("memory card type %s contains %q", firstNonEmpty(assertion.Field, "all"), assertion.Contains)
	case AssertDurableCardCount:
		return fmt.Sprintf("durable card count for type %s", firstNonEmpty(assertion.Field, "all"))
	case AssertDurableCardContains:
		return fmt.Sprintf("durable card type %s contains %q", firstNonEmpty(assertion.Field, "all"), assertion.Contains)
	case AssertDurableCardStatus:
		return fmt.Sprintf("durable card type %s has status %q", firstNonEmpty(assertion.Field, "all"), assertion.Equals)
	case AssertDurableCardConfidenceRange:
		return fmt.Sprintf("durable card type %s confidence is within [%.3f, %.3f]", firstNonEmpty(assertion.Field, "all"), assertion.MinConfidence, assertion.MaxConfidence)
	case AssertDurableCardScope:
		return fmt.Sprintf("durable card type %s has scope %q", firstNonEmpty(assertion.Field, "all"), assertion.Equals)
	case AssertDurableCardSupersedes:
		return fmt.Sprintf("durable card type %s supersedes %q", firstNonEmpty(assertion.Field, "all"), assertion.Equals)
	case AssertDurableCardVersionEquals:
		return fmt.Sprintf("durable card type %s version equals %d", firstNonEmpty(assertion.Field, "all"), assertion.Expected)
	case AssertDurableCardVersionAtLeast:
		return fmt.Sprintf("durable card type %s version is at least %d", firstNonEmpty(assertion.Field, "all"), assertion.Min)
	case AssertDurableCardActivationRange:
		return fmt.Sprintf("durable card type %s activation score is within [%.3f, %.3f]", firstNonEmpty(assertion.Field, "all"), assertion.MinConfidence, assertion.MaxConfidence)
	case AssertEdgeCount:
		return fmt.Sprintf("edge count for type %s", firstNonEmpty(assertion.Field, "all"))
	case AssertEdgeExists:
		return fmt.Sprintf("edge type %s exists with card match %q", firstNonEmpty(assertion.Field, "all"), assertion.Contains)
	case AssertRecallContains:
		return fmt.Sprintf("recall query %q source %s contains %q", assertion.Query, firstNonEmpty(assertion.Source, "all"), assertion.Contains)
	case AssertRecallNotContains:
		return fmt.Sprintf("recall query %q source %s does not contain %q", assertion.Query, firstNonEmpty(assertion.Source, "all"), assertion.Contains)
	case AssertActionAttemptCount:
		return fmt.Sprintf("action attempt count for %s", assertion.Step)
	case AssertRetrySucceeded:
		return fmt.Sprintf("retry succeeded for %s equals %q", assertion.Step, assertion.Equals)
	default:
		return assertion.Type
	}
}

func sessionStateField(state chat.SessionState, field string) string {
	switch strings.TrimSpace(field) {
	case "topic":
		return state.Topic
	case "focus_task_id":
		return state.FocusTaskID
	case "pending_action":
		return state.PendingAction
	case "pending_question":
		return state.PendingQuestion
	case "last_user_act":
		return state.LastUserAct
	case "last_assistant_act":
		return state.LastAssistantAct
	default:
		return ""
	}
}

func collectTaskArtifacts(task airuntime.Task) []string {
	if len(task.Metadata) == 0 {
		return nil
	}
	paths := make([]string, 0)
	for key, value := range task.Metadata {
		if strings.HasSuffix(key, "_artifact") && strings.TrimSpace(value) != "" {
			paths = append(paths, value)
		}
	}
	sort.Strings(paths)
	return dedupeStrings(paths)
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

func cloneMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-", ":", "-", ",", "-", "(", "", ")", "")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "scenario"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func procedureFields(card memory.Card) []string {
	values := make([]string, 0, 3)
	for _, key := range []string{"steps", "guardrails", "summary"} {
		switch typed := card.Content[key].(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				values = append(values, typed)
			}
		case []string:
			if len(typed) > 0 {
				values = append(values, strings.Join(typed, "\n"))
			}
		case []any:
			parts := make([]string, 0, len(typed))
			for _, item := range typed {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
			if len(parts) > 0 {
				values = append(values, strings.Join(parts, "\n"))
			}
		}
	}
	return values
}
