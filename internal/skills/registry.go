package skills

import (
	"fmt"
	"sort"
	"strings"

	"mnemosyneos/internal/airuntime"
)

type Handler func(*Runner, airuntime.Task, func(ProgressEvent)) (RunResult, error)

type MaintenancePolicy struct {
	Enabled          bool     `json:"enabled"`
	Scope            string   `json:"scope,omitempty"`
	AllowedCardTypes []string `json:"allowed_card_types,omitempty"`
	MinCandidates    int      `json:"min_candidates,omitempty"`
}

type ExternalConfig struct {
	Kind            string            `json:"kind"`
	Command         string            `json:"command,omitempty"`
	Args            []string          `json:"args,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	WorkDir         string            `json:"workdir,omitempty"`
	TimeoutMS       int               `json:"timeout_ms,omitempty"`
	AllowWriteRoot  bool              `json:"allow_write_root,omitempty"`
	RequireApproval bool              `json:"require_approval,omitempty"`
}

type Definition struct {
	Name              string             `json:"name"`
	Description       string             `json:"description,omitempty"`
	Source            string             `json:"source,omitempty"`
	Uses              string             `json:"uses,omitempty"`
	Enabled           bool               `json:"enabled"`
	DefaultMetadata   map[string]string  `json:"default_metadata,omitempty"`
	ExecutionProfile  string             `json:"execution_profile,omitempty"`
	Handler           Handler            `json:"-"`
	MaintenancePolicy *MaintenancePolicy `json:"maintenance_policy,omitempty"`
	External          *ExternalConfig    `json:"external,omitempty"`
}

type Registry struct {
	defs map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{defs: map[string]Definition{}}
}

func (r *Registry) Register(def Definition) error {
	if r == nil {
		return fmt.Errorf("skill registry is nil")
	}
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	if def.Handler == nil {
		return fmt.Errorf("skill %s handler is required", name)
	}
	if _, exists := r.defs[name]; exists {
		return fmt.Errorf("skill %s already registered", name)
	}
	def.Name = name
	r.defs[name] = def
	return nil
}

func (r *Registry) Resolve(name string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	def, ok := r.defs[strings.TrimSpace(name)]
	return def, ok
}

func (r *Registry) SetEnabled(name string, enabled bool) error {
	if r == nil {
		return fmt.Errorf("skill registry is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	def, ok := r.defs[name]
	if !ok {
		return fmt.Errorf("skill %s not registered", name)
	}
	def.Enabled = enabled
	r.defs[name] = def
	return nil
}

func (r *Registry) List() []Definition {
	if r == nil {
		return nil
	}
	out := make([]Definition, 0, len(r.defs))
	for _, def := range r.defs {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
