package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	runtimeStore   *airuntime.Store
	orchestrator   *airuntime.Orchestrator
	memoryStore    *memory.Store
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
		runtimeStore:   runtimeStore,
		orchestrator:   orchestrator,
		memoryStore:    memoryStore,
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
		return nil

	default:
		return fmt.Errorf("unsupported step type %q", step.Type)
	}
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
	case AssertSessionStateContain:
		return fmt.Sprintf("session %s field %s contains %q", firstNonEmpty(assertion.SessionID, "default"), assertion.Field, assertion.Contains)
	case AssertMemoryCardCount:
		return fmt.Sprintf("memory card count for type %s", firstNonEmpty(assertion.Field, "all"))
	case AssertMemoryCardContains:
		return fmt.Sprintf("memory card type %s contains %q", firstNonEmpty(assertion.Field, "all"), assertion.Contains)
	case AssertEdgeCount:
		return fmt.Sprintf("edge count for type %s", firstNonEmpty(assertion.Field, "all"))
	case AssertRecallContains:
		return fmt.Sprintf("recall query %q source %s contains %q", assertion.Query, firstNonEmpty(assertion.Source, "all"), assertion.Contains)
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
