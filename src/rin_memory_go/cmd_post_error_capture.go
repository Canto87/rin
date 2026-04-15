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

// Build/test/lint command prefixes worth capturing errors from.
var captureCommands = []string{
	"go build", "go test", "go vet", "go run",
	"make",
	"npm run", "npm test", "npx ",
	"pytest", "python -m pytest",
	"ruff ", "eslint", "golangci-lint",
	"cargo build", "cargo test", "cargo check",
	"docker build", "docker compose",
}

// Noise patterns to skip even if they match captureCommands.
var noisePatterns = []string{
	"no test files",
	"no Go files",
	"already up to date",
}

// runPostErrorCapture reads a PostToolUse hook payload for Bash,
// detects build/test/lint failures, and stores them as error_pattern.
func runPostErrorCapture() {
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

	if input.ToolName != "Bash" {
		outputContinue()
		return
	}

	command, _ := input.ToolInput["command"].(string)
	if command == "" {
		outputContinue()
		return
	}

	if !isCaptureWorthy(command) {
		outputContinue()
		return
	}

	responseText := extractResponseText(input.ToolResponse)
	if responseText == "" {
		outputContinue()
		return
	}

	if !looksLikeError(responseText) {
		outputContinue()
		return
	}

	if isNoise(responseText) {
		outputContinue()
		return
	}

	storeErrorCapture(command, responseText)
	outputContinue()
}

func isCaptureWorthy(command string) bool {
	cmd := strings.TrimSpace(command)
	for _, prefix := range captureCommands {
		if strings.HasPrefix(cmd, prefix) {
			return true
		}
	}
	return false
}

func looksLikeError(text string) bool {
	lower := strings.ToLower(text)
	indicators := []string{
		"error", "fail", "panic:", "fatal:",
		"undefined:", "cannot find", "not found",
		"exit code", "exit status",
		"compilation", "unresolved",
		"permission denied", "connection refused",
		"segmentation fault", "signal:",
	}
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	return false
}

func isNoise(text string) bool {
	lower := strings.ToLower(text)
	for _, pat := range noisePatterns {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

func extractResponseText(resp any) string {
	switch v := resp.(type) {
	case string:
		return v
	case map[string]any:
		// Try common fields.
		for _, key := range []string{"content", "stdout", "stderr", "output", "text"} {
			if s, ok := v[key].(string); ok && s != "" {
				return s
			}
		}
		// Try content array (MCP format).
		if arr, ok := v["content"].([]any); ok {
			var parts []string
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					if t, ok := m["text"].(string); ok {
						parts = append(parts, t)
					}
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n")
			}
		}
	}
	return ""
}

func storeErrorCapture(command, responseText string) {
	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("post-error-capture: config: %v", err)
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Printf("post-error-capture: store init: %v", err)
		return
	}
	defer store.Close()

	// Truncate response for storage.
	truncated := responseText
	if len(truncated) > 1500 {
		truncated = truncated[:1500] + "\n... (truncated)"
	}

	// Classify error type from command.
	errorType := classifyErrorType(command)

	title := fmt.Sprintf("[auto] %s failure: %s", errorType, truncateInline(command, 80))

	content := fmt.Sprintf("## 증상\n`%s` 실행 시 에러 발생 (자동 캡처)\n\n## 출력\n```\n%s\n```\n\n## 원인\n(미분석 — session-review 또는 수동 분석 대기)\n\n## 해결\n(미해결)", command, truncated)

	source := "auto:error-capture"
	tags := []string{
		"auto:error-capture",
		fmt.Sprintf("error_type:%s", errorType),
		extractMainTool(command),
	}

	var project *string
	if p := os.Getenv("RIN_PROJECT"); p != "" {
		project = &p
	} else if cfg.Project != "" {
		project = &cfg.Project
	}

	_, err = store.StoreDocument(ctx, MemoryStoreInput{
		Kind:    "error_pattern",
		Title:   title,
		Content: content,
		Tags:    tags,
		Source:  &source,
		Project: project,
	})
	if err != nil {
		log.Printf("post-error-capture: store: %v", err)
	}
}

func classifyErrorType(command string) string {
	cmd := strings.TrimSpace(command)
	switch {
	case strings.Contains(cmd, "test"):
		return "test"
	case strings.Contains(cmd, "vet") || strings.Contains(cmd, "lint") ||
		strings.Contains(cmd, "ruff") || strings.Contains(cmd, "eslint"):
		return "lint"
	case strings.Contains(cmd, "docker"):
		return "docker"
	case strings.Contains(cmd, "build") || strings.HasPrefix(cmd, "make"):
		return "build"
	default:
		return "runtime"
	}
}

func extractMainTool(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "unknown"
	}
	tool := parts[0]
	// Strip path prefix.
	if idx := strings.LastIndex(tool, "/"); idx >= 0 {
		tool = tool[idx+1:]
	}
	return fmt.Sprintf("tool:%s", tool)
}
