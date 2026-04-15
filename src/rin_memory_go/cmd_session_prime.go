package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// runSessionPrime injects context at session start: active tasks,
// last session summary, and skill health issues.
func runSessionPrime() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("session-prime: config: %v", err)
		outputContinue()
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Printf("session-prime: store: %v", err)
		outputContinue()
		return
	}
	defer store.Close()

	var messages []string

	// 1. Active tasks
	tasks, err := store.pool.Query(ctx, `
		SELECT title FROM documents
		WHERE kind = 'active_task' AND NOT archived
		ORDER BY created_at DESC LIMIT 5`)
	if err == nil {
		defer tasks.Close()
		var taskLines []string
		for tasks.Next() {
			var title string
			if err := tasks.Scan(&title); err != nil {
				continue
			}
			taskLines = append(taskLines, fmt.Sprintf("  - %s", title))
		}
		if len(taskLines) > 0 {
			messages = append(messages, "[RIN Context] Active tasks:")
			messages = append(messages, taskLines...)
		}
	}

	// 2. Last session summary (compact)
	var lastTitle string
	err = store.pool.QueryRow(ctx, `
		SELECT title FROM documents
		WHERE kind = 'session_summary' AND NOT archived
		ORDER BY created_at DESC LIMIT 1`).Scan(&lastTitle)
	if err == nil && lastTitle != "" {
		messages = append(messages, fmt.Sprintf("[RIN Context] Last session: %s", lastTitle))
	}

	// 3. Skill health issues
	current := querySkillPeriod(ctx, store, 7, 0)
	if len(current) > 0 {
		var issues []string
		for name, s := range current {
			if s.total >= 3 {
				rate := float64(s.success) / float64(s.total) * 100
				if rate < 70 {
					issues = append(issues, fmt.Sprintf("%s(%.0f%%)", name, rate))
				}
			}
		}
		if len(issues) > 0 {
			messages = append(messages, fmt.Sprintf("[RIN Context] Skill issues: %s", strings.Join(issues, ", ")))
		}
	}

	if len(messages) > 0 {
		json.NewEncoder(os.Stdout).Encode(hookOutput{
			Continue: true,
			Message:  strings.Join(messages, "\n"),
		})
	} else {
		outputContinue()
	}
}
