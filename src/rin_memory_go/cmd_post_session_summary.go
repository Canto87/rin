package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
)

// stopInput is the JSON payload from Claude Code Stop hook.
type stopInput struct {
	// The exact fields depend on Claude Code version.
	// We parse what we can and gracefully ignore the rest.
	StopReason string `json:"stop_reason,omitempty"`
}

var skillOutcomePattern = regexp.MustCompile(
	`(?i)##\s*(?:QA Gate|auto-research|auto-impl|troubleshoot)[:\s]+.*?(PASS|FAILED|FAIL|REJECT|Complete)`)

// runPostSessionSummary fires on Stop hook. Extracts skill outcomes
// from the session and stores them as routing_log entries.
func runPostSessionSummary() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		outputContinue()
		return
	}

	// Try to parse, but don't fail if format is unexpected
	var input stopInput
	json.Unmarshal(raw, &input)

	// The Stop hook payload may not include transcript text directly.
	// Extract what we can from the raw JSON.
	rawStr := string(raw)

	matches := skillOutcomePattern.FindAllStringSubmatch(rawStr, -1)
	if len(matches) == 0 {
		outputContinue()
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("post-session-summary: config: %v", err)
		outputContinue()
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Printf("post-session-summary: store: %v", err)
		outputContinue()
		return
	}
	defer store.Close()

	for _, m := range matches {
		fullMatch := m[0]
		outcome := strings.ToUpper(m[1])
		success := outcome == "PASS" || outcome == "COMPLETE"

		// Extract skill name from the match
		skillName := "unknown"
		lower := strings.ToLower(fullMatch)
		switch {
		case strings.Contains(lower, "qa gate"):
			skillName = "qa-gate"
		case strings.Contains(lower, "auto-research"):
			skillName = "auto-research"
		case strings.Contains(lower, "auto-impl"):
			skillName = "auto-impl"
		case strings.Contains(lower, "troubleshoot"):
			skillName = "troubleshoot"
		}

		source := "auto:session-summary"
		status := "ok"
		if !success {
			status = "fail"
		}

		tags := []string{
			"tool_type:skill",
			fmt.Sprintf("skill:%s", skillName),
			status,
			"auto:session-summary",
		}

		var project *string
		if cfg.Project != "" {
			project = &cfg.Project
		}

		if _, err := store.StoreDocument(ctx, MemoryStoreInput{
			Kind:    "routing_log",
			Title:   fmt.Sprintf("[session] %s: %s", skillName, outcome),
			Content: fmt.Sprintf("skill: %s, outcome: %s, source: session-stop-hook", skillName, outcome),
			Tags:    tags,
			Source:  &source,
			Project: project,
		}); err != nil {
			log.Printf("post-session-summary: store: %v", err)
		}
	}

	outputContinue()
}
