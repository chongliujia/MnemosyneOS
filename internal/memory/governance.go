package memory

import (
	"fmt"
	"time"
)

type GovernanceOptions struct {
	BaseDecayRate    float64
	StaleThreshold   float64
	ArchiveThreshold float64
	Now              time.Time
}

type GarbageCollectionResult struct {
	Examined int `json:"examined"`
	Decayed  int `json:"decayed"`
	Staled   int `json:"staled"`
	Archived int `json:"archived"`
}

// DecayAndCompact applies time-based decay to Active and Stale cards.
// If a card's score drops below StaleThreshold or ArchiveThreshold, its status is updated.
func (s *Store) DecayAndCompact(opts GovernanceOptions) (GarbageCollectionResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.StaleThreshold <= 0 {
		opts.StaleThreshold = 0.5
	}
	if opts.ArchiveThreshold <= 0 {
		opts.ArchiveThreshold = 0.1
	}
	if opts.BaseDecayRate <= 0 {
		// Default to decaying 0.05 score per hour
		opts.BaseDecayRate = 0.05
	}

	result := GarbageCollectionResult{}

	s.mu.Lock()
	defer s.mu.Unlock()

	for cardID, versions := range s.cards {
		if len(versions) == 0 {
			continue
		}
		latest := versions[len(versions)-1]
		if latest.Status == CardStatusArchived || latest.Status == CardStatusSuperseded || latest.Status == CardStatusCandidate {
			continue
		}

		result.Examined++

		lastEval := latest.CreatedAt
		if latest.Activation.LastEvaluatedAt != nil {
			lastEval = *latest.Activation.LastEvaluatedAt
		}

		hoursSinceEval := opts.Now.Sub(lastEval).Hours()
		if hoursSinceEval <= 0 {
			continue
		}

		// Apply decay based on policy
		rate := opts.BaseDecayRate
		switch latest.Activation.DecayPolicy {
		case "session_use":
			rate = opts.BaseDecayRate * 5.0
		case "durable":
			rate = opts.BaseDecayRate * 0.2
		case "never":
			rate = 0
		}

		decayAmount := hoursSinceEval * rate
		if decayAmount <= 0.001 && rate > 0 {
			// Skip negligible decay
			continue
		}

		newScore := clampUnit(latest.Activation.Score - decayAmount)
		newStatus := latest.Status

		if newScore < opts.ArchiveThreshold && latest.Status == CardStatusStale {
			newStatus = CardStatusArchived
			result.Archived++
		} else if newScore < opts.StaleThreshold && latest.Status == CardStatusActive {
			newStatus = CardStatusStale
			result.Staled++
		}

		// We only create a new version if there was meaningful decay.
		// Since rate=0 means decayAmount=0, it won't decay.
		if newScore < latest.Activation.Score || newStatus != latest.Status {
			now := opts.Now

			updated := latest
			updated.Version = latest.Version + 1
			updated.PrevVersion = latest.CardID + "#v" + fmt.Sprintf("%d", latest.Version)
			updated.Status = newStatus

			// Update Activation
			updated.Activation = latest.Activation
			updated.Activation.Score = newScore
			updated.Activation.LastEvaluatedAt = &now

			// Append new version
			s.cards[cardID] = append(versions, updated)
			result.Decayed++
		}
	}

	return result, nil
}
