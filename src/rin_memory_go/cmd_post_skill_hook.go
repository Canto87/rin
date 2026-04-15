package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

type skillLogPayload struct {
	Task      string `json:"task"`
	SkillName string `json:"skill_name"`
	SkillArgs string `json:"skill_args,omitempty"`
	Success   bool   `json:"success"`
	Source    string `json:"source"`
}

// runPostSkillHook reads a PostToolUse hook payload from stdin and logs
// Skill tool invocations to PostgreSQL.
// Errors go to stderr; stdout always gets valid hook JSON.
func runPostSkillHook() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Printf("post-skill-hook: stdin: %v", err)
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

	skillArgs, _ := input.ToolInput["args"].(string)

	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("post-skill-hook: config: %v", err)
		outputContinue()
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Printf("post-skill-hook: store: %v", err)
		outputContinue()
		return
	}
	defer store.Close()

	// Skill tool always succeeds (loads skill.md), but check ToolResponse
	// for is_error in case the load itself failed.
	success := true
	if resp, ok := input.ToolResponse.(map[string]any); ok {
		if isErr, _ := resp["is_error"].(bool); isErr {
			success = false
		}
	}

	status := "ok"
	if !success {
		status = "fail"
	}

	payload := skillLogPayload{
		Task:      fmt.Sprintf("skill:%s", skillName),
		SkillName: skillName,
		SkillArgs: skillArgs,
		Success:   success,
		Source:    "auto:hook",
	}

	contentBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("post-skill-hook: marshal: %v", err)
		outputContinue()
		return
	}

	title := fmt.Sprintf("[skill] %s: %s", skillName, status)
	source := "auto:hook"
	tags := []string{
		"tool_type:skill",
		fmt.Sprintf("skill:%s", skillName),
		status,
		"auto:hook",
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
		Content: string(contentBytes),
		Tags:    tags,
		Source:  &source,
		Project: project,
	})
	if err != nil {
		log.Printf("post-skill-hook: store document: %v", err)
	}

	outputContinue()
}
