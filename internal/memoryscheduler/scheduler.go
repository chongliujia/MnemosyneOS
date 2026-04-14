package memoryscheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/memory"
)

type Config struct {
	Scope             string
	Cooldown          time.Duration
	MinCandidates     int
	AllowedCardTypes  []string
	ExtractProcedures bool
	ProcedureMinRuns  int
}

type State struct {
	LastTriggeredAt    *time.Time `json:"last_triggered_at,omitempty"`
	LastReason         string     `json:"last_reason,omitempty"`
	LastTaskID         string     `json:"last_task_id,omitempty"`
	LastCandidateCount int        `json:"last_candidate_count,omitempty"`
	LastSkipReason     string     `json:"last_skip_reason,omitempty"`
}

type Decision struct {
	Triggered      bool   `json:"triggered"`
	TaskID         string `json:"task_id,omitempty"`
	TaskState      string `json:"task_state,omitempty"`
	SelectedSkill  string `json:"selected_skill,omitempty"`
	CandidateCount int    `json:"candidate_count,omitempty"`
	SkipReason     string `json:"skip_reason,omitempty"`
}

type Scheduler struct {
	runtimeStore *airuntime.Store
	orchestrator *airuntime.Orchestrator
	memoryStore  *memory.Store
	config       Config
	statePath    string
}

func New(runtimeStore *airuntime.Store, memoryStore *memory.Store, config Config) *Scheduler {
	if config.Cooldown <= 0 {
		config.Cooldown = 5 * time.Minute
	}
	if config.MinCandidates <= 0 {
		config.MinCandidates = 1
	}
	if strings.TrimSpace(config.Scope) == "" {
		config.Scope = memory.ScopeProject
	}
	if config.ProcedureMinRuns <= 0 {
		config.ProcedureMinRuns = 2
	}
	root := ""
	if runtimeStore != nil {
		root = runtimeStore.RootDir()
	}
	return &Scheduler{
		runtimeStore: runtimeStore,
		orchestrator: airuntime.NewOrchestrator(runtimeStore),
		memoryStore:  memoryStore,
		config:       config,
		statePath:    filepath.Join(root, "state", "memory_scheduler.json"),
	}
}

func (s *Scheduler) EvaluateAndSchedule(reason string) (Decision, error) {
	decision := Decision{}
	if s == nil || s.runtimeStore == nil || s.memoryStore == nil || s.orchestrator == nil {
		decision.SkipReason = "not_configured"
		return decision, nil
	}

	// Apply decay and compaction before evaluating consolidation tasks
	_, _ = s.memoryStore.DecayAndCompact(memory.GovernanceOptions{})

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual"
	}

	candidates := s.eligibleCandidates()
	candidateCount := len(candidates)
	decision.CandidateCount = candidateCount
	if len(candidates) == 0 {
		decision.SkipReason = "no_eligible_candidates"
		_ = s.saveState(State{
			LastReason:         reason,
			LastCandidateCount: 0,
			LastSkipReason:     decision.SkipReason,
		})
		return decision, nil
	}
	if candidateCount < s.config.MinCandidates {
		decision.SkipReason = "candidate_threshold"
		_ = s.saveState(State{
			LastReason:         reason,
			LastCandidateCount: candidateCount,
			LastSkipReason:     decision.SkipReason,
		})
		return decision, nil
	}

	if len(s.memoryStore.Query(memory.QueryRequest{
		Scope:  strings.TrimSpace(s.config.Scope),
		Status: memory.CardStatusCandidate,
	}).Cards) == 0 {
		decision.SkipReason = "no_candidates"
		_ = s.saveState(State{
			LastReason:         reason,
			LastCandidateCount: 0,
			LastSkipReason:     decision.SkipReason,
		})
		return decision, nil
	}

	state, _ := s.loadState()
	if state.LastTriggeredAt != nil && time.Since(*state.LastTriggeredAt) < s.config.Cooldown {
		decision.SkipReason = "cooldown"
		s.hydrateDecisionFromTaskID(&decision, state.LastTaskID)
		_ = s.saveState(State{
			LastTriggeredAt:    state.LastTriggeredAt,
			LastReason:         reason,
			LastTaskID:         state.LastTaskID,
			LastCandidateCount: candidateCount,
			LastSkipReason:     decision.SkipReason,
		})
		return decision, nil
	}

	runtimeState, err := s.runtimeStore.LoadState()
	if err != nil {
		return decision, err
	}
	if runtimeState.ActiveTaskID != nil {
		decision.SkipReason = "runtime_busy"
		_ = s.saveState(State{
			LastReason:         reason,
			LastCandidateCount: candidateCount,
			LastSkipReason:     decision.SkipReason,
		})
		return decision, nil
	}

	tasks, err := s.runtimeStore.ListTasks()
	if err != nil {
		return decision, err
	}
	for _, task := range tasks {
		if task.SelectedSkill != "memory-consolidate" {
			continue
		}
		switch task.State {
		case airuntime.TaskStateInbox, airuntime.TaskStatePlanned, airuntime.TaskStateActive, airuntime.TaskStateAwaitingApproval, airuntime.TaskStateBlocked:
			decision.SkipReason = "existing_consolidation_task"
			decision.TaskID = task.TaskID
			decision.TaskState = task.State
			decision.SelectedSkill = task.SelectedSkill
			_ = s.saveState(State{
				LastReason:         reason,
				LastTaskID:         task.TaskID,
				LastCandidateCount: candidateCount,
				LastSkipReason:     decision.SkipReason,
			})
			return decision, nil
		}
	}

	task, err := s.orchestrator.SubmitTask(airuntime.CreateTaskRequest{
		Title:         "Consolidate memory maintenance",
		Goal:          "Consolidate candidate memory into durable runtime state",
		RequestedBy:   "memory-scheduler",
		Source:        "memory-scheduler",
		SelectedSkill: "memory-consolidate",
		Metadata: map[string]string{
			"scope":              s.config.Scope,
			"extract_procedures": boolString(s.config.ExtractProcedures),
			"min_runs":           intString(s.config.ProcedureMinRuns),
			"scheduled_reason":   reason,
		},
	})
	if err != nil {
		return decision, err
	}

	now := time.Now().UTC()
	decision.Triggered = true
	decision.TaskID = task.TaskID
	decision.TaskState = task.State
	decision.SelectedSkill = task.SelectedSkill
	if err := s.saveState(State{
		LastTriggeredAt:    &now,
		LastReason:         reason,
		LastTaskID:         task.TaskID,
		LastCandidateCount: candidateCount,
	}); err != nil {
		return decision, err
	}
	return decision, nil
}

func (s *Scheduler) eligibleCandidates() []memory.Card {
	if s == nil || s.memoryStore == nil {
		return nil
	}
	cards := s.memoryStore.Query(memory.QueryRequest{
		Scope:  strings.TrimSpace(s.config.Scope),
		Status: memory.CardStatusCandidate,
	}).Cards
	if len(s.config.AllowedCardTypes) == 0 {
		return cards
	}
	allowed := make(map[string]struct{}, len(s.config.AllowedCardTypes))
	for _, cardType := range s.config.AllowedCardTypes {
		cardType = strings.TrimSpace(cardType)
		if cardType != "" {
			allowed[cardType] = struct{}{}
		}
	}
	filtered := make([]memory.Card, 0, len(cards))
	for _, card := range cards {
		if _, ok := allowed[strings.TrimSpace(card.CardType)]; ok {
			filtered = append(filtered, card)
		}
	}
	return filtered
}

func (s *Scheduler) hydrateDecisionFromTaskID(decision *Decision, taskID string) {
	if decision == nil || s == nil || s.runtimeStore == nil || strings.TrimSpace(taskID) == "" {
		return
	}
	task, err := s.runtimeStore.GetTask(strings.TrimSpace(taskID))
	if err != nil {
		return
	}
	decision.TaskID = task.TaskID
	decision.TaskState = task.State
	decision.SelectedSkill = task.SelectedSkill
}

func (s *Scheduler) loadState() (State, error) {
	var state State
	raw, err := os.ReadFile(s.statePath)
	if err != nil {
		return State{}, err
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Scheduler) saveState(state State) error {
	if strings.TrimSpace(s.statePath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(s.statePath, raw, 0o644)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func intString(value int) string {
	if value <= 0 {
		return "0"
	}
	return strconv.Itoa(value)
}
