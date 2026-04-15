package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// hookInput is the JSON payload from Claude Code PreToolUse/PostToolUse hook.
type hookInput struct {
	ToolName     string         `json:"tool_name"`
	ToolInput    map[string]any `json:"tool_input"`
	ToolResponse any            `json:"tool_response,omitempty"`
}

// hookOutput is the JSON response for Claude Code hooks.
type hookOutput struct {
	Continue bool   `json:"continue"`
	Message  string `json:"message,omitempty"`
}

// runPreSkillHook reads a PreToolUse hook payload from stdin, queries
// preferences for the skill, and writes a hook response to stdout.
// Errors go to stderr; stdout always gets valid hook JSON.
func runPreSkillHook() {
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

	if input.ToolName != "Skill" {
		outputContinue()
		return
	}

	skillName, _ := input.ToolInput["skill"].(string)
	if skillName == "" {
		outputContinue()
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("pre-skill-hook: config: %v", err)
		outputContinue()
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Printf("pre-skill-hook: store: %v", err)
		outputContinue()
		return
	}
	defer store.Close()

	var messages []string

	// Query preferences matching this skill
	prefs, err := fetchPreferences(ctx, store.pool, skillName)
	if err != nil {
		log.Printf("pre-skill-hook: preference query: %v", err)
	} else if len(prefs) > 0 {
		messages = append(messages, fmt.Sprintf("[RIN Memory] Skill '%s' preferences:", skillName))
		for _, p := range prefs {
			content := truncateInline(p.Content, 200)
			messages = append(messages, fmt.Sprintf("  - %s: %s", p.Title, content))
		}
	}

	// Query rule-violation patterns (2+ occurrences)
	violations, err := fetchViolations(ctx, store.pool)
	if err != nil {
		log.Printf("pre-skill-hook: violation query: %v", err)
	} else if len(violations) > 0 {
		messages = append(messages, "[RIN Memory] Repeated violations:")
		for _, v := range violations {
			messages = append(messages, fmt.Sprintf("  - %s (%dx)", v.Title, v.Count))
		}
	}

	// Query past experience (error_pattern) for this skill
	experiences, err := fetchSkillExperience(ctx, store.pool, skillName)
	if err != nil {
		log.Printf("pre-skill-hook: experience query: %v", err)
	} else if len(experiences) > 0 {
		messages = append(messages, fmt.Sprintf("[RIN Memory] Past experience for '%s':", skillName))
		for _, e := range experiences {
			content := truncateInline(e.Content, 300)
			messages = append(messages, fmt.Sprintf("  - %s: %s", e.Title, content))
		}
	}

	// Inject policy rules for this skill
	policyRules := fetchSkillPolicy(skillName)
	if policyRules != "" {
		messages = append(messages, fmt.Sprintf("[RIN Policy] Rules for '%s':", skillName))
		messages = append(messages, policyRules)
	}

	// Inject health warning if skill is underperforming
	if healthWarn := fetchSkillHealthWarning(ctx, store, skillName); healthWarn != "" {
		messages = append(messages, fmt.Sprintf("[RIN Warning] Skill '%s': %s", skillName, healthWarn))
	}

	if len(messages) > 0 {
		messages = append(messages, "")
		messages = append(messages, "[RIN] Follow the skill workflow as defined.")
		json.NewEncoder(os.Stdout).Encode(hookOutput{
			Continue: true,
			Message:  strings.Join(messages, "\n"),
		})
	} else {
		outputContinue()
	}
}

func outputContinue() {
	json.NewEncoder(os.Stdout).Encode(hookOutput{Continue: true})
}
