package memory

import (
	"crypto/sha1"
	"fmt"
	"strings"

	"mnemosyneos/internal/airuntime"
)

type ProcedureEvidence struct {
	Steps           string
	Guardrails      string
	Summary         string
	SuccessSignal   string
	ArtifactPath    string
	ObservationPath string
}

type ProcedureEvidenceResolver func(task airuntime.Task) ProcedureEvidence

type ProcedureExtractionRequest struct {
	Tasks            []airuntime.Task
	TaskClass        string
	SelectedSkill    string
	Scope            string
	MinRuns          int
	EvidenceResolver ProcedureEvidenceResolver
}

type ProcedureExtractionResult struct {
	Examined       int      `json:"examined"`
	Matched        int      `json:"matched"`
	Candidates     int      `json:"candidates"`
	CandidateCards []string `json:"candidate_cards,omitempty"`
}

func BuildProcedureCandidates(req ProcedureExtractionRequest) ([]CreateCardRequest, ProcedureExtractionResult) {
	minRuns := req.MinRuns
	if minRuns <= 0 {
		minRuns = 2
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = ScopeProject
	}

	result := ProcedureExtractionResult{}
	groups := map[string][]airuntime.Task{}

	for _, task := range req.Tasks {
		result.Examined++
		if task.State != airuntime.TaskStateDone {
			continue
		}
		taskClass := taskProcedureClass(task)
		if taskClass == "" {
			continue
		}
		if strings.TrimSpace(req.TaskClass) != "" && taskClass != strings.TrimSpace(req.TaskClass) {
			continue
		}
		selectedSkill := strings.TrimSpace(task.SelectedSkill)
		if strings.TrimSpace(req.SelectedSkill) != "" && selectedSkill != strings.TrimSpace(req.SelectedSkill) {
			continue
		}
		evidence := resolveProcedureEvidence(task, req.EvidenceResolver)
		steps := strings.TrimSpace(evidence.Steps)
		if steps == "" {
			continue
		}
		result.Matched++
		signature := proceduralSignature(taskClass, selectedSkill, steps, strings.TrimSpace(evidence.Guardrails))
		groups[signature] = append(groups[signature], task)
	}

	out := make([]CreateCardRequest, 0, len(groups))
	for signature, tasks := range groups {
		if len(tasks) < minRuns {
			continue
		}
		card := buildProcedureCandidate(signature, scope, tasks, req.EvidenceResolver)
		out = append(out, card)
		result.Candidates++
		result.CandidateCards = append(result.CandidateCards, card.CardID)
	}
	return out, result
}

func resolveProcedureEvidence(task airuntime.Task, resolver ProcedureEvidenceResolver) ProcedureEvidence {
	if resolver != nil {
		if evidence := resolver(task); strings.TrimSpace(evidence.Steps) != "" {
			return evidence
		}
	}
	return ProcedureEvidence{
		Steps:         strings.TrimSpace(task.Metadata["procedure_steps"]),
		Guardrails:    strings.TrimSpace(task.Metadata["procedure_guardrails"]),
		Summary:       strings.TrimSpace(task.Metadata["procedure_summary"]),
		SuccessSignal: strings.TrimSpace(task.Metadata["procedure_success_signal"]),
	}
}

func taskProcedureClass(task airuntime.Task) string {
	if task.Metadata == nil {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(task.Metadata["procedure_task_class"]), strings.TrimSpace(task.Metadata["task_class"]))
}

func buildProcedureCandidate(signature, scope string, tasks []airuntime.Task, resolver ProcedureEvidenceResolver) CreateCardRequest {
	first := tasks[0]
	taskClass := taskProcedureClass(first)
	selectedSkill := strings.TrimSpace(first.SelectedSkill)
	evidence := resolveProcedureEvidence(first, resolver)
	steps := strings.TrimSpace(evidence.Steps)
	guardrails := strings.TrimSpace(evidence.Guardrails)
	summary := strings.TrimSpace(evidence.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Recommended procedure for %s using %s.", taskClass, firstNonEmpty(selectedSkill, "runtime"))
	}
	successSignal := strings.TrimSpace(evidence.SuccessSignal)
	if successSignal == "" {
		successSignal = fmt.Sprintf("%d successful runs matched this procedure", len(tasks))
	}

	supportingRuns := make([]string, 0, len(tasks))
	for _, task := range tasks {
		supportingRuns = append(supportingRuns, task.TaskID)
	}

	return CreateCardRequest{
		CardID:   proceduralCardID(taskClass, selectedSkill, signature),
		CardType: "procedure",
		Scope:    scope,
		Status:   CardStatusCandidate,
		Content: map[string]any{
			"name":             procedureName(taskClass, selectedSkill),
			"task_class":       taskClass,
			"selected_skill":   selectedSkill,
			"summary":          summary,
			"steps":            steps,
			"guardrails":       guardrails,
			"success_signal":   successSignal,
			"supporting_runs":  supportingRuns,
			"artifact_path":    strings.TrimSpace(evidence.ArtifactPath),
			"observation_path": strings.TrimSpace(evidence.ObservationPath),
		},
		Provenance: Provenance{
			Source:     "procedure-extractor",
			Confidence: procedureConfidence(len(tasks)),
		},
	}
}

func procedureConfidence(matches int) float64 {
	confidence := 0.65 + float64(matches-2)*0.1
	if confidence > 0.95 {
		confidence = 0.95
	}
	if confidence < 0.65 {
		confidence = 0.65
	}
	return confidence
}

func procedureName(taskClass, selectedSkill string) string {
	switch {
	case taskClass != "" && selectedSkill != "":
		return fmt.Sprintf("%s_%s", strings.ReplaceAll(taskClass, " ", "_"), strings.ReplaceAll(selectedSkill, "-", "_"))
	case taskClass != "":
		return strings.ReplaceAll(taskClass, " ", "_")
	case selectedSkill != "":
		return strings.ReplaceAll(selectedSkill, "-", "_")
	default:
		return "procedure"
	}
}

func proceduralCardID(taskClass, selectedSkill, signature string) string {
	sum := sha1.Sum([]byte(signature))
	prefix := sanitizeToken(firstNonEmpty(taskClass, selectedSkill, "generic"))
	return fmt.Sprintf("procedure:%s:%x", prefix, sum[:6])
}

func proceduralSignature(taskClass, selectedSkill, steps, guardrails string) string {
	return strings.Join([]string{
		strings.TrimSpace(taskClass),
		strings.TrimSpace(selectedSkill),
		strings.TrimSpace(steps),
		strings.TrimSpace(guardrails),
	}, "|")
}

func sanitizeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "generic"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "_", "-", ":", "-", ".", "-")
	value = replacer.Replace(value)
	return strings.Trim(value, "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
