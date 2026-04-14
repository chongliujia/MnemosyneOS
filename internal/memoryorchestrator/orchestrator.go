package memoryorchestrator

import (
	"strings"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/recall"
)

type SessionView struct {
	Topic            string
	FocusTaskID      string
	PendingQuestion  string
	PendingAction    string
	LastAssistantAct string
	WorkingRecallIDs []string
	WorkingSources   []string
}

type TaskRef struct {
	TaskID        string
	Title         string
	State         string
	SelectedSkill string
}

type RecallRef struct {
	Source   string
	CardID   string
	CardType string
	Snippet  string
}

type Packet struct {
	RecentTasks   []TaskRef
	WorkingNotes  []string
	SemanticHits  []RecallRef
	ProcedureHits []RecallRef
	RecallHits    []RecallRef
}

type UsageContext struct {
	TaskClass string
	Outcome   string
}

func (p *Packet) IsEmpty() bool {
	return p == nil || (len(p.RecentTasks) == 0 && len(p.WorkingNotes) == 0 && len(p.SemanticHits) == 0 && len(p.ProcedureHits) == 0 && len(p.RecallHits) == 0)
}

type Orchestrator struct {
	recall       *recall.Service
	runtimeStore *airuntime.Store
	quota        Quota
}

type SourcePolicy struct {
	SemanticSources []string
	ProcedureSource bool
}

type Quota struct {
	MaxRecentTasks        int
	MaxWorkingNotes       int
	MaxProcedureHits      int
	MaxSemanticHits       int
	MaxWorkingRecallHits  int
	MaxFallbackRecallHits int
}

func DefaultQuota() Quota {
	return Quota{
		MaxRecentTasks:        3,
		MaxWorkingNotes:       4,
		MaxProcedureHits:      1,
		MaxSemanticHits:       3,
		MaxWorkingRecallHits:  2,
		MaxFallbackRecallHits: 4,
	}
}

func New(recallService *recall.Service, runtimeStore *airuntime.Store) *Orchestrator {
	return NewWithQuota(recallService, runtimeStore, DefaultQuota())
}

func NewWithQuota(recallService *recall.Service, runtimeStore *airuntime.Store, quota Quota) *Orchestrator {
	quota = normalizeQuota(quota)
	return &Orchestrator{
		recall:       recallService,
		runtimeStore: runtimeStore,
		quota:        quota,
	}
}

func (o *Orchestrator) RecordPacketUse(packet *Packet, usage UsageContext) error {
	if o == nil || o.recall == nil || packet == nil {
		return nil
	}
	usage = normalizeUsageContext(usage)
	feedback := make([]recall.UsageFeedback, 0, len(packet.ProcedureHits)+len(packet.SemanticHits))
	for _, hit := range packet.ProcedureHits {
		if strings.TrimSpace(hit.CardID) == "" {
			continue
		}
		activationDelta, confidenceDelta := procedureFeedbackDelta(usage)
		feedback = append(feedback, recall.UsageFeedback{
			CardID:          hit.CardID,
			ActivationDelta: activationDelta,
			ConfidenceDelta: confidenceDelta,
		})
	}
	for _, hit := range packet.SemanticHits {
		if strings.TrimSpace(hit.CardID) == "" {
			continue
		}
		activationDelta, confidenceDelta := semanticFeedbackDelta(hit, usage)
		feedback = append(feedback, recall.UsageFeedback{
			CardID:          hit.CardID,
			ActivationDelta: activationDelta,
			ConfidenceDelta: confidenceDelta,
		})
	}
	return o.recall.ApplyFeedback(feedback)
}

func (o *Orchestrator) BuildFastPacket(state SessionView) *Packet {
	packet := &Packet{
		WorkingNotes: clipStrings(o.workingNotes(state), o.quota.MaxWorkingNotes),
	}
	if task := o.focusTask(state.FocusTaskID); task != nil && o.quota.MaxRecentTasks > 0 {
		packet.RecentTasks = append(packet.RecentTasks, *task)
	}
	packet.ProcedureHits = o.recallHits(o.cueQuery("", state), []string{"procedure"}, o.quota.MaxProcedureHits)
	packet.SemanticHits = o.recallHits(o.cueQuery("", state), []string{"web", "email", "github"}, min(o.quota.MaxSemanticHits, 2))
	if len(packet.SemanticHits) == 0 && len(packet.ProcedureHits) == 0 {
		packet.RecallHits = o.workingSetHits(state, o.quota.MaxWorkingRecallHits)
	} else {
		packet.RecallHits = aggregateRecallRefs(packet.ProcedureHits, packet.SemanticHits, o.workingSetHits(state, o.quota.MaxWorkingRecallHits))
	}
	if packet.IsEmpty() {
		return nil
	}
	return packet
}

func (o *Orchestrator) BuildTaskPacket(query, selectedSkill string, state SessionView, recentTasks []TaskRef) *Packet {
	packet := &Packet{
		WorkingNotes: clipStrings(dedupeStrings(append([]string{}, o.workingNotes(state)...)), o.quota.MaxWorkingNotes),
		RecentTasks:  clipTasks(append([]TaskRef{}, recentTasks...), o.quota.MaxRecentTasks),
	}
	if task := o.focusTask(state.FocusTaskID); task != nil && o.quota.MaxRecentTasks > 0 && !containsTaskRef(packet.RecentTasks, task.TaskID) {
		packet.RecentTasks = append([]TaskRef{*task}, packet.RecentTasks...)
		packet.RecentTasks = clipTasks(packet.RecentTasks, o.quota.MaxRecentTasks)
	}
	cue := o.cueQuery(query, state)
	policy := sourcePolicyForSkill(selectedSkill)
	if policy.ProcedureSource {
		packet.ProcedureHits = o.recallHits(cue, []string{"procedure"}, o.quota.MaxProcedureHits)
	}
	packet.SemanticHits = o.recallHits(cue, policy.SemanticSources, o.quota.MaxSemanticHits)
	packet.RecallHits = aggregateRecallRefs(packet.ProcedureHits, packet.SemanticHits, o.workingSetHits(state, o.quota.MaxWorkingRecallHits))
	if len(packet.RecallHits) == 0 {
		packet.RecallHits = o.recallHits(cue, nil, o.quota.MaxFallbackRecallHits)
	}
	if packet.IsEmpty() {
		return nil
	}
	return packet
}

func (o *Orchestrator) workingNotes(state SessionView) []string {
	notes := make([]string, 0, 4)
	if topic := strings.TrimSpace(state.Topic); topic != "" {
		notes = append(notes, "topic: "+topic)
	}
	if question := strings.TrimSpace(state.PendingQuestion); question != "" {
		notes = append(notes, "pending question: "+question)
	}
	if action := strings.TrimSpace(state.PendingAction); action != "" {
		notes = append(notes, "pending action: "+action)
	}
	if act := strings.TrimSpace(state.LastAssistantAct); act != "" {
		notes = append(notes, "last assistant act: "+act)
	}
	return notes
}

func (o *Orchestrator) cueQuery(query string, state SessionView) string {
	parts := make([]string, 0, 3)
	if query = strings.TrimSpace(query); query != "" {
		parts = append(parts, query)
	}
	if topic := strings.TrimSpace(state.Topic); topic != "" && !containsString(parts, topic) {
		parts = append(parts, topic)
	}
	if question := strings.TrimSpace(state.PendingQuestion); question != "" && !containsString(parts, question) {
		parts = append(parts, question)
	}
	return strings.Join(parts, "\n")
}

func (o *Orchestrator) focusTask(taskID string) *TaskRef {
	if o == nil || o.runtimeStore == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	task, err := o.runtimeStore.GetTask(taskID)
	if err != nil {
		return nil
	}
	return &TaskRef{
		TaskID:        task.TaskID,
		Title:         task.Title,
		State:         task.State,
		SelectedSkill: task.SelectedSkill,
	}
}

func (o *Orchestrator) recallHits(query string, sources []string, limit int) []RecallRef {
	if o == nil || o.recall == nil || strings.TrimSpace(query) == "" || limit <= 0 {
		return nil
	}
	resp := o.recall.Recall(recall.Request{
		Query:   query,
		Sources: sources,
		Limit:   limit,
	})
	if len(resp.Hits) == 0 {
		return nil
	}
	refs := make([]RecallRef, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		refs = append(refs, RecallRef{
			Source:   hit.Source,
			CardID:   hit.CardID,
			CardType: hit.CardType,
			Snippet:  hit.Snippet,
		})
	}
	return refs
}

func (o *Orchestrator) workingSetHits(state SessionView, limit int) []RecallRef {
	if limit <= 0 {
		return nil
	}
	refs := make([]RecallRef, 0, min(limit, len(state.WorkingRecallIDs)))
	for i, cardID := range state.WorkingRecallIDs {
		if len(refs) >= limit {
			break
		}
		source := ""
		if i < len(state.WorkingSources) {
			source = state.WorkingSources[i]
		}
		refs = append(refs, RecallRef{
			Source: source,
			CardID: cardID,
		})
	}
	return refs
}

func aggregateRecallRefs(groups ...[]RecallRef) []RecallRef {
	out := make([]RecallRef, 0)
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, ref := range group {
			key := strings.TrimSpace(ref.CardID)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ref)
		}
	}
	return out
}

func containsTaskRef(tasks []TaskRef, taskID string) bool {
	for _, task := range tasks {
		if task.TaskID == taskID {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(needle) {
			return true
		}
	}
	return false
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizeQuota(quota Quota) Quota {
	defaults := DefaultQuota()
	if quota.MaxRecentTasks <= 0 {
		quota.MaxRecentTasks = defaults.MaxRecentTasks
	}
	if quota.MaxWorkingNotes <= 0 {
		quota.MaxWorkingNotes = defaults.MaxWorkingNotes
	}
	if quota.MaxProcedureHits <= 0 {
		quota.MaxProcedureHits = defaults.MaxProcedureHits
	}
	if quota.MaxSemanticHits <= 0 {
		quota.MaxSemanticHits = defaults.MaxSemanticHits
	}
	if quota.MaxWorkingRecallHits <= 0 {
		quota.MaxWorkingRecallHits = defaults.MaxWorkingRecallHits
	}
	if quota.MaxFallbackRecallHits <= 0 {
		quota.MaxFallbackRecallHits = defaults.MaxFallbackRecallHits
	}
	return quota
}

func clipStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string{}, values[:limit]...)
}

func clipTasks(values []TaskRef, limit int) []TaskRef {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]TaskRef{}, values[:limit]...)
}

func semanticConfidenceDelta(hit RecallRef) float64 {
	switch hit.Source {
	case "web", "email", "github":
		return 0.01
	default:
		return 0
	}
}

func normalizeUsageContext(usage UsageContext) UsageContext {
	usage.TaskClass = strings.TrimSpace(strings.ToLower(usage.TaskClass))
	usage.Outcome = strings.TrimSpace(strings.ToLower(usage.Outcome))
	return usage
}

func procedureFeedbackDelta(usage UsageContext) (float64, float64) {
	var activationBase float64
	var confidenceBase float64
	switch usage.TaskClass {
	case "task-plan", "memory-consolidate":
		activationBase = 0.10
		confidenceBase = 0.03
	case "direct_reply", "followup_reply":
		activationBase = 0.06
		confidenceBase = 0.01
	default:
		activationBase = 0.08
		confidenceBase = 0.02
	}
	activationWeight, confidenceWeight := outcomeFeedbackWeights(usage.Outcome)
	return activationBase * activationWeight, confidenceBase * confidenceWeight
}

func semanticFeedbackDelta(hit RecallRef, usage UsageContext) (float64, float64) {
	activationBase := 0.04
	switch usage.TaskClass {
	case "task-plan", "memory-consolidate":
		activationBase = 0.05
	case "direct_reply", "followup_reply":
		activationBase = 0.03
	}
	confidenceBase := semanticConfidenceDelta(hit)
	if usage.TaskClass == "direct_reply" || usage.TaskClass == "followup_reply" {
		confidenceBase *= 0.5
	}
	activationWeight, confidenceWeight := outcomeFeedbackWeights(usage.Outcome)
	return activationBase * activationWeight, confidenceBase * confidenceWeight
}

func outcomeFeedbackWeights(outcome string) (float64, float64) {
	switch strings.TrimSpace(strings.ToLower(outcome)) {
	case "", airuntime.TaskStateDone, "responded":
		return 1.0, 1.0
	case airuntime.TaskStateAwaitingApproval, airuntime.TaskStateBlocked:
		return 0.6, 0.25
	case airuntime.TaskStateFailed:
		return 0.2, 0.0
	default:
		return 1.0, 1.0
	}
}

func sourcePolicyForSkill(skill string) SourcePolicy {
	switch strings.TrimSpace(skill) {
	case "web-search":
		return SourcePolicy{SemanticSources: []string{"web"}, ProcedureSource: true}
	case "email-inbox":
		return SourcePolicy{SemanticSources: []string{"email"}, ProcedureSource: true}
	case "github-issue-search":
		return SourcePolicy{SemanticSources: []string{"github"}, ProcedureSource: true}
	case "memory-consolidate":
		return SourcePolicy{SemanticSources: []string{"web", "email", "github"}, ProcedureSource: true}
	case "task-plan":
		return SourcePolicy{SemanticSources: []string{"web", "email", "github"}, ProcedureSource: true}
	default:
		return SourcePolicy{SemanticSources: []string{"web", "email", "github"}, ProcedureSource: true}
	}
}
