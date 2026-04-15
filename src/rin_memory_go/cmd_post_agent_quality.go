package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// scorePattern matches patterns like "Score: 8/10", "점수: 7/10", "Rating: 9/10"
var scorePattern = regexp.MustCompile(`(?i)(?:score|점수|rating)[:\s]*(\d+)\s*/\s*(\d+)`)

// verdictPattern matches PASS/FAIL/RETRY keywords
var verdictPattern = regexp.MustCompile(`(?i)\b(PASS|FAIL|REJECT|RETRY)\b`)

// qualityAgentTypes are agent types whose output quality we want to track.
var qualityAgentTypes = map[string]bool{
	"code-review": true,
	"validate":    true,
}

// runPostAgentQuality reads a PostToolUse hook payload for Agent,
// extracts quality signals (scores, verdicts) from code-review/validate agents,
// and stores them as routing_log entries.
func runPostAgentQuality() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		outputContinue()
		return
	}

	var input hookInput
	if err := json.Unmarshal(raw, &input); err != nil {
		outputContinue()
		return
	}

	if input.ToolName != "Agent" {
		outputContinue()
		return
	}

	agentType, _ := input.ToolInput["subagent_type"].(string)
	if !qualityAgentTypes[agentType] {
		outputContinue()
		return
	}

	responseText := extractResponseText(input.ToolResponse)
	if responseText == "" {
		outputContinue()
		return
	}

	// Extract score.
	score, maxScore := extractScore(responseText)
	verdict := extractVerdict(responseText)

	if score == 0 && verdict == "" {
		// No quality signal found.
		outputContinue()
		return
	}

	storeQualitySignal(agentType, score, maxScore, verdict, input.ToolInput)
	outputContinue()
}

func extractScore(text string) (int, int) {
	matches := scorePattern.FindStringSubmatch(text)
	if len(matches) < 3 {
		return 0, 0
	}
	score, _ := strconv.Atoi(matches[1])
	max, _ := strconv.Atoi(matches[2])
	return score, max
}

func extractVerdict(text string) string {
	matches := verdictPattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	return strings.ToUpper(matches[1])
}

func storeQualitySignal(agentType string, score, maxScore int, verdict string, toolInput map[string]any) {
	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("post-agent-quality: config: %v", err)
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Printf("post-agent-quality: store: %v", err)
		return
	}
	defer store.Close()

	// Build content.
	var parts []string
	if score > 0 {
		parts = append(parts, fmt.Sprintf("score: %d/%d", score, maxScore))
	}
	if verdict != "" {
		parts = append(parts, fmt.Sprintf("verdict: %s", verdict))
	}

	// Extract prompt snippet for context.
	prompt, _ := toolInput["prompt"].(string)
	if len(prompt) > 200 {
		prompt = prompt[:200] + "..."
	}
	if prompt != "" {
		parts = append(parts, fmt.Sprintf("context: %s", prompt))
	}

	content := strings.Join(parts, "\n")
	title := fmt.Sprintf("[quality] %s", agentType)
	if score > 0 {
		title = fmt.Sprintf("[quality] %s: %d/%d", agentType, score, maxScore)
	}
	if verdict != "" {
		title += " " + verdict
	}

	source := "auto:quality"
	tags := []string{
		"auto:quality",
		fmt.Sprintf("agent:%s", agentType),
	}
	if score > 0 {
		tags = append(tags, fmt.Sprintf("score:%d", score))
	}
	if verdict != "" {
		tags = append(tags, fmt.Sprintf("verdict:%s", strings.ToLower(verdict)))
	}

	var project *string
	if p := os.Getenv("RIN_PROJECT"); p != "" {
		project = &p
	} else if cfg.Project != "" {
		project = &cfg.Project
	}

	_, err = store.StoreDocument(ctx, MemoryStoreInput{
		Kind:    "routing_log",
		Title:   title,
		Content: content,
		Tags:    tags,
		Source:  &source,
		Project: project,
	})
	if err != nil {
		log.Printf("post-agent-quality: store: %v", err)
	}
}
