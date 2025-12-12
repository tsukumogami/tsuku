package install

import (
	"fmt"
	"time"
)

// timeNow is a variable for testing purposes.
// It defaults to time.Now but can be overridden in tests.
var timeNow = time.Now

// RecordGeneration records an LLM generation with its cost.
// It adds the current timestamp to the generation history and updates the daily cost.
// Timestamps older than 1 hour are pruned. Daily cost resets at UTC midnight.
func (sm *StateManager) RecordGeneration(cost float64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	lock := NewFileLock(sm.lockPath())
	if err := lock.LockExclusive(); err != nil {
		return fmt.Errorf("failed to acquire lock for LLM usage update: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	state, err := sm.loadWithoutLock()
	if err != nil {
		return err
	}

	if state.LLMUsage == nil {
		state.LLMUsage = &LLMUsage{}
	}

	now := timeNow().UTC()
	today := now.Format("2006-01-02")

	// Reset daily cost if date changed
	if state.LLMUsage.DailyCostDate != today {
		state.LLMUsage.DailyCost = 0
		state.LLMUsage.DailyCostDate = today
	}

	// Add current timestamp
	state.LLMUsage.GenerationTimestamps = append(state.LLMUsage.GenerationTimestamps, now)

	// Prune timestamps older than 1 hour
	oneHourAgo := now.Add(-time.Hour)
	pruned := make([]time.Time, 0, len(state.LLMUsage.GenerationTimestamps))
	for _, ts := range state.LLMUsage.GenerationTimestamps {
		if ts.After(oneHourAgo) {
			pruned = append(pruned, ts)
		}
	}
	state.LLMUsage.GenerationTimestamps = pruned

	// Update daily cost
	state.LLMUsage.DailyCost += cost

	return sm.saveWithoutLock(state)
}

// CanGenerate checks if a new LLM generation is allowed based on rate limit and daily budget.
// Returns (allowed, reason) where reason explains why generation is denied.
// A zero or negative limit means unlimited.
func (sm *StateManager) CanGenerate(hourlyLimit int, dailyBudget float64) (bool, string) {
	state, err := sm.Load()
	if err != nil {
		// If we can't load state, allow generation but log warning
		return true, ""
	}

	if state.LLMUsage == nil {
		return true, ""
	}

	now := timeNow().UTC()
	today := now.Format("2006-01-02")

	// Check rate limit (if limit > 0)
	if hourlyLimit > 0 {
		oneHourAgo := now.Add(-time.Hour)
		recentCount := 0
		for _, ts := range state.LLMUsage.GenerationTimestamps {
			if ts.After(oneHourAgo) {
				recentCount++
			}
		}
		if recentCount >= hourlyLimit {
			return false, fmt.Sprintf("rate limit exceeded: %d generations in the last hour (limit: %d)", recentCount, hourlyLimit)
		}
	}

	// Check daily budget (if budget > 0)
	if dailyBudget > 0 && state.LLMUsage.DailyCostDate == today {
		if state.LLMUsage.DailyCost >= dailyBudget {
			return false, fmt.Sprintf("daily budget exceeded: $%.2f spent today (budget: $%.2f)", state.LLMUsage.DailyCost, dailyBudget)
		}
	}

	return true, ""
}

// DailySpent returns the total LLM cost spent today in USD.
// Returns 0 if no cost has been recorded today.
func (sm *StateManager) DailySpent() float64 {
	state, err := sm.Load()
	if err != nil {
		return 0
	}

	if state.LLMUsage == nil {
		return 0
	}

	today := timeNow().UTC().Format("2006-01-02")
	if state.LLMUsage.DailyCostDate != today {
		return 0
	}

	return state.LLMUsage.DailyCost
}

// RecentGenerationCount returns the number of LLM generations in the last hour.
func (sm *StateManager) RecentGenerationCount() int {
	state, err := sm.Load()
	if err != nil {
		return 0
	}

	if state.LLMUsage == nil {
		return 0
	}

	now := timeNow().UTC()
	oneHourAgo := now.Add(-time.Hour)
	count := 0
	for _, ts := range state.LLMUsage.GenerationTimestamps {
		if ts.After(oneHourAgo) {
			count++
		}
	}
	return count
}
