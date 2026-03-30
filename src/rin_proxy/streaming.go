package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
)

// streamGeminiToAnthropic reads Gemini SSE chunks and synthesizes
// Anthropic's SSE lifecycle events.
//
// Gemini streams complete GenerateContentResponse objects per chunk.
// Anthropic expects: message_start → (content_block_start → content_block_delta* →
// content_block_stop)* → message_delta → message_stop
//
// Reference: ccr's anthropic.transformer.ts convertOpenAIStreamToAnthropic
func streamGeminiToAnthropic(geminiBody io.Reader, w io.Writer, f http.Flusher, requestModel string, tm *ToolMap) {
	msgID := genMsgID()

	var (
		started              bool
		currentBlockIdx      = -1 // currently open block (-1 = none)
		nextBlockIdx         int  // monotonic counter
		hasToolUse           bool
		lastUsage            *GeminiUsage
		textBlockOpen        bool
	)

	assignBlockIndex := func() int {
		idx := nextBlockIdx
		nextBlockIdx++
		return idx
	}

	// closeCurrentBlock closes the currently open content block if any.
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

	// emitMessageStart sends the initial message_start event.
	emitMessageStart := func(model string) {
		if started {
			return
		}
		started = true
		writeSSE(w, f, "message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            msgID,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		})
	}

	// emitTextDelta opens a text block if needed and sends text_delta.
	emitTextDelta := func(text string) {
		if !textBlockOpen {
			closeCurrentBlock()
			idx := assignBlockIndex()
			writeSSE(w, f, "content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			})
			currentBlockIdx = idx
			textBlockOpen = true
		}
		writeSSE(w, f, "content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": currentBlockIdx,
			"delta": map[string]any{
				"type": "text_delta",
				"text": text,
			},
		})
	}

	// emitToolUse opens a tool_use block and sends input_json_delta.
	emitToolUse := func(fc *GeminiFunctionCall) {
		closeCurrentBlock()
		hasToolUse = true

		id := tm.GenerateID(fc.Name)
		idx := assignBlockIndex()

		writeSSE(w, f, "content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": idx,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    id,
				"name":  fc.Name,
				"input": map[string]any{},
			},
		})
		currentBlockIdx = idx

		// Emit the entire args as a single input_json_delta.
		// Gemini sends function calls as complete objects, not streamed.
		argsJSON := "{}"
		if fc.Args != nil {
			argsJSON = string(fc.Args)
		}
		writeSSE(w, f, "content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": idx,
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": argsJSON,
			},
		})
	}

	scanner := bufio.NewScanner(geminiBody)
	// Increase scanner buffer for large chunks
	scanner.Buffer(make([]byte, 0, 512*1024), 512*1024)

	for scanner.Scan() {
		line := scanner.Text()

		data, ok := parseSSELine(line)
		if !ok {
			continue
		}

		var chunk GeminiResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			debugLog("parse gemini chunk error: %v, data: %s", err, data)
			continue
		}

		if len(chunk.Candidates) == 0 {
			continue
		}

		candidate := chunk.Candidates[0]

		// Emit message_start on first valid chunk
		emitMessageStart(requestModel)

		// Track usage from each chunk (last one wins)
		if chunk.UsageMetadata != nil {
			lastUsage = chunk.UsageMetadata
		}

		// Process parts
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.FunctionCall != nil {
					emitToolUse(part.FunctionCall)
					closeCurrentBlock()
				} else if part.Text != "" {
					emitTextDelta(part.Text)
				}
			}
		}

		// If this is the final chunk (has finishReason), we're done
		if candidate.FinishReason != "" {
			break
		}
	}

	// Ensure message_start was sent (edge case: empty response)
	emitMessageStart(requestModel)

	// If no content was emitted at all, send empty text block
	if nextBlockIdx == 0 {
		emitTextDelta("")
	}

	// Close any remaining open block
	closeCurrentBlock()

	// Determine stop reason
	stopReason := "end_turn"
	if hasToolUse {
		stopReason = "tool_use"
	}

	// Build usage
	usageMap := map[string]any{
		"input_tokens":  0,
		"output_tokens": 0,
	}
	if lastUsage != nil {
		usageMap["input_tokens"] = lastUsage.PromptTokenCount
		usageMap["output_tokens"] = lastUsage.CandidatesTokenCount
	}

	// message_delta
	writeSSE(w, f, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": usageMap,
	})

	// message_stop
	writeSSE(w, f, "message_stop", map[string]any{
		"type": "message_stop",
	})
}
