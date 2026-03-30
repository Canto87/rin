package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	EmbedModel    = "mxbai-embed-large"
	MaxChars      = 1500
	FallbackChars = 800
)

// ollamaEmbedRequest is the request body for Ollama embed API.
type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input any      `json:"input"`
}

// ollamaEmbedResponse is the response from Ollama embed API.
type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// Embed generates embedding for a single text using Ollama API.
func Embed(ctx context.Context, ollamaURL, text string) ([]float32, error) {
	if ollamaURL == "" {
		return nil, nil
	}

	// Truncate if needed
	text = truncateText(text, MaxChars)

	embeddings, err := embedBatch(ctx, ollamaURL, []string{text})
	if err != nil {
		// Check for context length error and retry with fallback
		if isContextLengthError(err) {
			text = truncateText(text, FallbackChars)
			embeddings, err = embedBatch(ctx, ollamaURL, []string{text})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func EmbedBatch(ctx context.Context, ollamaURL string, texts []string) ([][]float32, error) {
	if ollamaURL == "" {
		return nil, nil
	}

	// Truncate all texts
	truncated := make([]string, len(texts))
	for i, t := range texts {
		truncated[i] = truncateText(t, MaxChars)
	}

	embeddings, err := embedBatch(ctx, ollamaURL, truncated)
	if err != nil {
		// Check for context length error and retry with fallback
		if isContextLengthError(err) {
			for i, t := range texts {
				truncated[i] = truncateText(t, FallbackChars)
			}
			embeddings, err = embedBatch(ctx, ollamaURL, truncated)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return embeddings, nil
}

// embedBatch makes the actual HTTP request to Ollama.
func embedBatch(ctx context.Context, ollamaURL string, texts []string) ([][]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model: EmbedModel,
		Input: texts,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := ollamaURL + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// Convert float64 to float32
	result := make([][]float32, len(ollamaResp.Embeddings))
	for i, emb := range ollamaResp.Embeddings {
		result[i] = make([]float32, len(emb))
		for j, v := range emb {
			result[i][j] = float32(v)
		}
	}

	return result, nil
}

// truncateText truncates text to maxChars, handling UTF-8 properly.
func truncateText(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}

	// Find the last valid UTF-8 boundary before maxChars
	for i := maxChars; i > 0; i-- {
		if utf8ValidLast(text[:i]) {
			return text[:i]
		}
	}
	return text[:maxChars]
}

// utf8ValidLast checks if the last byte of s is a valid UTF-8 boundary.
func utf8ValidLast(s string) bool {
	if len(s) == 0 {
		return true
	}
	// Check if the last byte is a continuation byte (0x80-0xBF)
	last := s[len(s)-1]
	return (last & 0xC0) != 0x80
}

// isContextLengthError checks if the error is related to context/input length.
func isContextLengthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context length") ||
		strings.Contains(msg, "input length") ||
		strings.Contains(msg, "token limit") ||
		strings.Contains(msg, "too long")
}

// vectorToLiteral converts a float32 slice to PostgreSQL vector literal string.
func vectorToLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i, f := range v {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%g", f))
	}
	sb.WriteString("]")
	return sb.String()
}

// logEmbedWarning logs embedding failures without failing the operation.
func logEmbedWarning(docID string, err error) {
	log.Printf("warning: embedding failed for %s: %v", docID, err)
}
