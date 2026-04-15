package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// runSkillHealth outputs a compact skill health summary for the statusline.
// Default: single-line compact (e.g., "Skills 87% (3 active)")
// --full: executes skill-report as a separate invocation
func runSkillHealth() {
	fs := flag.NewFlagSet("skill-health", flag.ExitOnError)
	days := fs.Int("days", 7, "Lookback period in days")
	full := fs.Bool("full", false, "Full report mode (delegates to skill-report)")
	fs.Parse(os.Args[2:])

	if *full {
		// Delegate to skill-report via exec (avoids os.Args mutation)
		cmd := exec.Command(os.Args[0], "skill-report", fmt.Sprintf("-days=%d", *days))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Print("Skills ?")
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		fmt.Print("Skills ?")
		return
	}
	defer store.Close()

	// Reuse querySkillPeriod from cmd_skill_report.go
	current := querySkillPeriod(ctx, store, *days, 0)
	if len(current) == 0 {
		fmt.Print("Skills --")
		return
	}

	// Aggregate
	totalAll, successAll := 0, 0
	var issues []string

	type entry struct {
		name string
		*periodStats
	}
	var entries []entry
	for name, s := range current {
		entries = append(entries, entry{name, s})
		totalAll += s.total
		successAll += s.success
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].total > entries[j].total
	})

	// Identify issues: skills with < 70% success rate and 3+ invocations
	for _, e := range entries {
		if e.total >= 3 {
			rate := float64(e.success) / float64(e.total) * 100
			if rate < 70 {
				issues = append(issues, e.name)
			}
		}
	}

	overallRate := float64(successAll) / float64(totalAll) * 100

	// Build compact output (always include active count for consistency)
	if len(issues) == 0 {
		fmt.Printf("Skills %.0f%% (%d active)", overallRate, len(current))
	} else if len(issues) <= 2 {
		fmt.Printf("Skills %.0f%% (%d active) ⚠%s", overallRate, len(current), strings.Join(issues, ","))
	} else {
		fmt.Printf("Skills %.0f%% (%d active) ⚠%d", overallRate, len(current), len(issues))
	}
}
