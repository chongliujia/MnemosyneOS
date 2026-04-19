package memory

import (
	"log"
	"sync"
	"time"
)

// GovernanceScheduler runs periodic DecayAndCompact cycles.
type GovernanceScheduler struct {
	store    *Store
	interval time.Duration
	opts     GovernanceOptions

	stopCh chan struct{}
	once   sync.Once
}

// GovernanceSchedulerConfig holds tunables for the background scheduler.
type GovernanceSchedulerConfig struct {
	Interval         time.Duration
	BaseDecayRate    float64
	StaleThreshold   float64
	ArchiveThreshold float64
}

func defaultSchedulerConfig() GovernanceSchedulerConfig {
	return GovernanceSchedulerConfig{
		Interval:         1 * time.Hour,
		BaseDecayRate:    0.05,
		StaleThreshold:   0.5,
		ArchiveThreshold: 0.1,
	}
}

// NewGovernanceScheduler creates (but does not start) a scheduler.
// Pass nil config to use defaults.
func NewGovernanceScheduler(store *Store, cfg *GovernanceSchedulerConfig) *GovernanceScheduler {
	c := defaultSchedulerConfig()
	if cfg != nil {
		if cfg.Interval > 0 {
			c.Interval = cfg.Interval
		}
		if cfg.BaseDecayRate > 0 {
			c.BaseDecayRate = cfg.BaseDecayRate
		}
		if cfg.StaleThreshold > 0 {
			c.StaleThreshold = cfg.StaleThreshold
		}
		if cfg.ArchiveThreshold > 0 {
			c.ArchiveThreshold = cfg.ArchiveThreshold
		}
	}
	return &GovernanceScheduler{
		store:    store,
		interval: c.Interval,
		opts: GovernanceOptions{
			BaseDecayRate:    c.BaseDecayRate,
			StaleThreshold:   c.StaleThreshold,
			ArchiveThreshold: c.ArchiveThreshold,
		},
		stopCh: make(chan struct{}),
	}
}

// Start launches the background goroutine. Safe to call once.
func (gs *GovernanceScheduler) Start() {
	go gs.loop()
}

// Stop signals the scheduler to exit.
func (gs *GovernanceScheduler) Stop() {
	gs.once.Do(func() { close(gs.stopCh) })
}

func (gs *GovernanceScheduler) loop() {
	ticker := time.NewTicker(gs.interval)
	defer ticker.Stop()
	for {
		select {
		case <-gs.stopCh:
			return
		case <-ticker.C:
			gs.tick()
		}
	}
}

func (gs *GovernanceScheduler) tick() {
	// 1. Decay & compact
	opts := gs.opts
	opts.Now = time.Now().UTC()
	result, err := gs.store.DecayAndCompact(opts)
	if err != nil {
		log.Printf("[memory/governance] decay error: %v", err)
	} else if result.Decayed > 0 || result.Staled > 0 || result.Archived > 0 {
		log.Printf("[memory/governance] decay: examined=%d decayed=%d staled=%d archived=%d",
			result.Examined, result.Decayed, result.Staled, result.Archived)
	}

	// 2. Event → Fact upgrade (consolidation)
	upgradeResult, err := gs.store.UpgradeEventsToFacts(UpgradeRequest{
		Scope:          ScopeSession,
		MinOccurrences: 2,
		MinConfidence:  0.5,
	})
	if err != nil {
		log.Printf("[memory/governance] upgrade error: %v", err)
	} else if upgradeResult.FactsCreated > 0 {
		log.Printf("[memory/governance] upgrade: examined=%d clusters=%d facts_created=%d",
			upgradeResult.EventsExamined, upgradeResult.ClustersFound, upgradeResult.FactsCreated)
	}

	// Also upgrade project-scoped events
	upgradeProject, err := gs.store.UpgradeEventsToFacts(UpgradeRequest{
		Scope:          ScopeProject,
		MinOccurrences: 2,
		MinConfidence:  0.5,
	})
	if err != nil {
		log.Printf("[memory/governance] upgrade(project) error: %v", err)
	} else if upgradeProject.FactsCreated > 0 {
		log.Printf("[memory/governance] upgrade(project): facts_created=%d", upgradeProject.FactsCreated)
	}

	// 3. Conflict detection (log only)
	for _, scope := range []string{ScopeSession, ScopeProject, ScopeUser} {
		conflicts := gs.store.DetectConflicts(ConflictDetectionRequest{Scope: scope})
		if len(conflicts.Conflicts) > 0 {
			log.Printf("[memory/governance] conflicts(%s): %d found", scope, len(conflicts.Conflicts))
			for _, c := range conflicts.Conflicts {
				log.Printf("[memory/governance]   %s vs %s: %s", c.CardA, c.CardB, c.Reason)
			}
		}
	}
}
