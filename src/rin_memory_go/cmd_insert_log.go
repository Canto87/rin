package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

// insertLogPayload is the JSON structure expected from stdin.
type insertLogPayload struct {
	TaskDescription string  `json:"task_description"`
	Model           string  `json:"model"`
	Level           string  `json:"level"`
	Mode            string  `json:"mode"`
	AgentCount      int     `json:"agent_count"`
	Success         bool    `json:"success"`
	SubagentType    string  `json:"subagent_type"`
	DurationS       *int    `json:"duration_s"`
	Background      bool    `json:"background"`
	Source          string  `json:"source"`
}

// runInsertLog reads a routing_log JSON from stdin and inserts it into PostgreSQL.
// Errors are logged to stderr but never cause a non-zero exit so that hook
// failure cannot block Claude Code.
func runInsertLog() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Printf("insert-log: failed to read stdin: %v", err)
		return
	}

	var payload insertLogPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		log.Printf("insert-log: invalid JSON: %v", err)
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("insert-log: failed to load config: %v", err)
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Printf("insert-log: failed to connect to store: %v", err)
		return
	}
	defer store.Close()

	// Build tags
	successTag := "success"
	if !payload.Success {
		successTag = "fail"
	}
	tags := []string{
		fmt.Sprintf("model:%s", payload.Model),
		fmt.Sprintf("type:%s", payload.SubagentType),
		successTag,
		"auto:hook",
	}

	// Build title (task truncated to 50 chars)
	task := payload.TaskDescription
	if len(task) > 50 {
		task = task[:50]
	}
	title := fmt.Sprintf("[auto] %s: %s", payload.SubagentType, task)

	// Content is the raw JSON as received
	content := string(raw)

	// Source literal
	source := "auto:hook"

	// Project from env (config already picks up RIN_PROJECT, but env wins)
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
		log.Printf("insert-log: failed to store document: %v", err)
	}
}
