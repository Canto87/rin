package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// --- OpenAI API types ---

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"` // string or nil
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type OpenAIToolCall struct {
	Index    int               `json:"index"`
	ID       string            `json:"id"`
	Type     string            `json:"type"` // "function"
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type OpenAITool struct {
	Type     string         `json:"type"` // "function"
	Function OpenAIFuncDecl `json:"function"`
}

type OpenAIFuncDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   *OpenAIUsage   `json:"usage,omitempty"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIDelta struct {
	Role      string            `json:"role,omitempty"`
	Content   *string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall  `json:"tool_calls,omitempty"`
}

type OpenAIStreamChunk struct {
	ID      string               `json:"id"`
	Choices []OpenAIStreamChoice `json:"choices"`
}

type OpenAIStreamChoice struct {
	Index        int          `json:"index"`
	Delta        OpenAIDelta  `json:"delta"`
	FinishReason *string      `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// --- Request conversion ---

func toOpenAIRequest(req AnthropicRequest, tm *ToolMap) (*OpenAIRequest, error) {
	out := &OpenAIRequest{
		Model:       req.Model, // will be overridden by provider.Model in handler
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	// System prompt → system message
	if req.System != nil {
		sysText, err := extractSystemText(req.System)
		if err == nil && sysText != "" {
			out.Messages = append(out.Messages, OpenAIMessage{
				Role:    "system",
				Content: sysText,
			})
		}
	}

	// Convert messages
	for _, msg := range req.Messages {
		converted, err := convertAnthropicMessage(msg, tm)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, converted...)
	}

	// Tools
	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			out.Tools = append(out.Tools, OpenAITool{
				Type: "function",
				Function: OpenAIFuncDecl{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.InputSchema,
				},
			})
		}
	}

	// Tool choice
	if req.ToolChoice != nil {
		var tc struct{ Type string `json:"type"` }
		if json.Unmarshal(req.ToolChoice, &tc) == nil {
			switch tc.Type {
			case "auto":
				out.ToolChoice = "auto"
			case "any":
				out.ToolChoice = "required"
			case "none":
				out.ToolChoice = "none"
			}
		}
	}

	return out, nil
}

func extractSystemText(raw json.RawMessage) (string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", err
	}
	var text string
	for _, b := range blocks {
		if b.Type == "text" {
			text += b.Text
		}
	}
	return text, nil
}

func convertAnthropicMessage(msg Message, tm *ToolMap) ([]OpenAIMessage, error) {
	// Try string content
	var s string
	if json.Unmarshal(msg.Content, &s) == nil {
		return []OpenAIMessage{{Role: msg.Role, Content: s}}, nil
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil, fmt.Errorf("invalid message content: %w", err)
	}

	if msg.Role == "assistant" {
		return convertAssistantMessage(blocks, tm)
	}
	// user role
	return convertUserMessage(blocks, tm)
}

func convertAssistantMessage(blocks []ContentBlock, tm *ToolMap) ([]OpenAIMessage, error) {
	var textContent string
	var toolCalls []OpenAIToolCall
	for _, b := range blocks {
		switch b.Type {
		case "text":
			textContent += b.Text
		case "tool_use":
			argsStr := "{}"
			if b.Input != nil {
				argsStr = string(b.Input)
			}
			// Register ID mapping
			tm.mu.Lock()
			tm.idToName[b.ID] = b.Name
			tm.mu.Unlock()
			toolCalls = append(toolCalls, OpenAIToolCall{
				ID:   b.ID,
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      b.Name,
					Arguments: argsStr,
				},
			})
		}
	}
	out := OpenAIMessage{Role: "assistant"}
	if textContent != "" {
		out.Content = textContent
	}
	if len(toolCalls) > 0 {
		out.ToolCalls = toolCalls
	}
	return []OpenAIMessage{out}, nil
}

func convertUserMessage(blocks []ContentBlock, tm *ToolMap) ([]OpenAIMessage, error) {
	var msgs []OpenAIMessage
	var textContent string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			textContent += b.Text
		case "tool_result":
			// Flush any accumulated text first
			if textContent != "" {
				msgs = append(msgs, OpenAIMessage{Role: "user", Content: textContent})
				textContent = ""
			}
			resultText := extractResultText(b.Content)
			name, _ := tm.ResolveName(b.ToolUseID)
			msgs = append(msgs, OpenAIMessage{
				Role:       "tool",
				Content:    resultText,
				ToolCallID: b.ToolUseID,
				Name:       name,
			})
		}
	}
	if textContent != "" {
		msgs = append(msgs, OpenAIMessage{Role: "user", Content: textContent})
	}
	if len(msgs) == 0 {
		msgs = []OpenAIMessage{{Role: "user", Content: ""}}
	}
	return msgs, nil
}

func extractResultText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []ContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var text string
		for _, b := range blocks {
			if b.Type == "text" {
				text += b.Text
			}
		}
		return text
	}
	return string(raw)
}

// --- Response conversion ---

func fromOpenAIResponse(resp OpenAIResponse, model string, tm *ToolMap) *AnthropicResponse {
	var blocks []ContentBlock
	hasToolUse := false

	if len(resp.Choices) > 0 {
		msg := resp.Choices[0].Message
		if msg.Content != nil {
			if s, ok := msg.Content.(string); ok && s != "" {
				blocks = append(blocks, ContentBlock{Type: "text", Text: s})
			}
		}
		for _, tc := range msg.ToolCalls {
			hasToolUse = true
			id := tm.GenerateID(tc.Function.Name)
			// Override: use the OpenAI tool call ID for reverse lookup
			tm.mu.Lock()
			tm.idToName[tc.ID] = tc.Function.Name
			tm.mu.Unlock()
			var argsRaw json.RawMessage
			if tc.Function.Arguments != "" {
				argsRaw = json.RawMessage(tc.Function.Arguments)
			} else {
				argsRaw = json.RawMessage(`{}`)
			}
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				ID:    id,
				Name:  tc.Function.Name,
				Input: argsRaw,
			})
		}
	}

	if len(blocks) == 0 {
		blocks = []ContentBlock{{Type: "text", Text: ""}}
	}

	stopReason := "end_turn"
	if hasToolUse {
		stopReason = "tool_use"
	}
	if len(resp.Choices) > 0 {
		switch resp.Choices[0].FinishReason {
		case "length":
			stopReason = "max_tokens"
		}
	}

	var usage Usage
	if resp.Usage != nil {
		usage = Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		}
	}

	return &AnthropicResponse{
		ID:         genMsgID(),
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    blocks,
		StopReason: stopReason,
		Usage:      usage,
	}
}

// --- Handlers ---

func handleOpenAISync(w http.ResponseWriter, r *http.Request, req *OpenAIRequest, provider ProviderConfig, authToken string, requestModel string, tm *ToolMap) {
	req.Model = provider.Model
	req.Stream = false

	url := provider.BaseURL + "/chat/completions"

	body, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "marshal openai request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httpReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "create request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		http.Error(w, "openai request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "read openai response: "+err.Error(), http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("openai error (%d): %s", resp.StatusCode, string(respBody))
		http.Error(w, fmt.Sprintf("openai error: %s", string(respBody)), resp.StatusCode)
		return
	}

	var openaiResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		http.Error(w, "parse openai response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	anthResp := fromOpenAIResponse(openaiResp, requestModel, tm)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthResp)
}

func handleOpenAIStream(w http.ResponseWriter, r *http.Request, req *OpenAIRequest, provider ProviderConfig, authToken string, requestModel string, tm *ToolMap) {
	req.Model = provider.Model
	req.Stream = true

	url := provider.BaseURL + "/chat/completions"

	body, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "marshal openai request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httpReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "create request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		http.Error(w, "openai request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("openai stream error (%d): %s", resp.StatusCode, string(respBody))
		http.Error(w, fmt.Sprintf("openai error: %s", string(respBody)), resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	streamOpenAIToAnthropic(resp.Body, w, flusher, requestModel, tm)
}

func streamOpenAIToAnthropic(body io.Reader, w io.Writer, f http.Flusher, requestModel string, tm *ToolMap) {
	msgID := genMsgID()

	// Local type for accumulating streaming tool calls
	type openAIStreamToolCall struct {
		id        string
		name      string
		arguments string
		blockIdx  int
	}

	var (
		started         bool
		currentBlockIdx = -1
		nextBlockIdx    int
		hasToolUse      bool
		textBlockOpen   bool
		// For streaming tool calls, accumulate per index
		toolCallAccum = map[int]*openAIStreamToolCall{}
	)

	assignBlockIndex := func() int {
		idx := nextBlockIdx
		nextBlockIdx++
		return idx
	}

	closeCurrentBlock := func() {
		if currentBlockIdx >= 0 {
			writeSSE(w, f, "content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": currentBlockIdx,
			})
			currentBlockIdx = -1
			textBlockOpen = false
		}
	}

	emitMessageStart := func() {
		if started {
			return
		}
		started = true
		writeSSE(w, f, "message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":    msgID, "type": "message", "role": "assistant",
				"content": []any{}, "model": requestModel,
				"stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
			},
		})
	}

	emitTextDelta := func(text string) {
		if !textBlockOpen {
			closeCurrentBlock()
			idx := assignBlockIndex()
			writeSSE(w, f, "content_block_start", map[string]any{
				"type": "content_block_start", "index": idx,
				"content_block": map[string]any{"type": "text", "text": ""},
			})
			currentBlockIdx = idx
			textBlockOpen = true
		}
		writeSSE(w, f, "content_block_delta", map[string]any{
			"type": "content_block_delta", "index": currentBlockIdx,
			"delta": map[string]any{"type": "text_delta", "text": text},
		})
	}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 512*1024), 512*1024)

	for scanner.Scan() {
		line := scanner.Text()
		data, ok := parseSSELine(line)
		if !ok {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk OpenAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			debugLog("parse openai chunk: %v, data: %s", err, data)
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		emitMessageStart()

		delta := chunk.Choices[0].Delta

		if delta.Content != nil && *delta.Content != "" {
			emitTextDelta(*delta.Content)
		}

		// Handle streaming tool calls
		for _, tc := range delta.ToolCalls {
			acc, exists := toolCallAccum[tc.Index]
			if !exists {
				// New tool call — open a block
				closeCurrentBlock()
				hasToolUse = true
				idx := assignBlockIndex()
				acc = &openAIStreamToolCall{blockIdx: idx}
				toolCallAccum[tc.Index] = acc
				acc.id = tc.ID
				acc.name = tc.Function.Name
				writeSSE(w, f, "content_block_start", map[string]any{
					"type": "content_block_start", "index": idx,
					"content_block": map[string]any{
						"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": map[string]any{},
					},
				})
				currentBlockIdx = idx
			}
			if tc.Function.Arguments != "" {
				acc.arguments += tc.Function.Arguments
				writeSSE(w, f, "content_block_delta", map[string]any{
					"type": "content_block_delta", "index": acc.blockIdx,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": tc.Function.Arguments},
				})
			}
		}

		// Check finish
		if chunk.Choices[0].FinishReason != nil {
			break
		}
	}

	// Register tool IDs in tm
	for _, acc := range toolCallAccum {
		if acc.name != "" {
			tm.mu.Lock()
			tm.idToName[acc.id] = acc.name
			tm.mu.Unlock()
		}
	}

	emitMessageStart()
	if nextBlockIdx == 0 {
		emitTextDelta("")
	}
	closeCurrentBlock()

	stopReason := "end_turn"
	if hasToolUse {
		stopReason = "tool_use"
	}

	writeSSE(w, f, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
	})
	writeSSE(w, f, "message_stop", map[string]any{"type": "message_stop"})
}
