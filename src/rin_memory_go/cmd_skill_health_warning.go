package main

import (
	"context"
	"fmt"
)

// fetchSkillHealthWarning checks if the skill's recent success rate is below 70%.
// Returns a warning string if so, empty string otherwise.
func fetchSkillHealthWarning(ctx context.Context, store *Store, skillName string) string {
	current := querySkillPeriod(ctx, store, 7, 0)
	if current == nil {
		return ""
	}
	stats, ok := current[skillName]
	if !ok || stats.total < 3 {
		return ""
	}
	rate := float64(stats.success) / float64(stats.total) * 100
	if rate >= 70 {
		return ""
	}
	return fmt.Sprintf("Recent success rate: %.0f%% (%d/%d in last 7d). Exercise caution.",
		rate, stats.success, stats.total)
}
