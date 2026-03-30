package main

import "encoding/json"

// fromGeminiResponse converts a non-streaming Gemini response to Anthropic format.
func fromGeminiResponse(resp GeminiResponse, model string, tm *ToolMap) (*AnthropicResponse, error) {
	if len(resp.Candidates) == 0 {
		return &AnthropicResponse{
			ID:         genMsgID(),
			Type:       "message",
			Role:       "assistant",
			Model:      model,
			Content:    []ContentBlock{{Type: "text", Text: ""}},
			StopReason: "end_turn",
		}, nil
	}

	candidate := resp.Candidates[0]
	blocks, hasToolUse := partsToBlocks(candidate.Content, tm)

	var usage Usage
	if resp.UsageMetadata != nil {
		usage = Usage{
			InputTokens:  resp.UsageMetadata.PromptTokenCount,
			OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
		}
	}

	return &AnthropicResponse{
		ID:         genMsgID(),
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    blocks,
		StopReason: mapFinishReason(candidate.FinishReason, hasToolUse),
		Usage:      usage,
	}, nil
}

func partsToBlocks(content *GeminiContent, tm *ToolMap) ([]ContentBlock, bool) {
	if content == nil {
		return []ContentBlock{}, false
	}

	var blocks []ContentBlock
	hasToolUse := false

	for _, p := range content.Parts {
		if p.Text != "" {
			blocks = append(blocks, ContentBlock{Type: "text", Text: p.Text})
		}
		if p.FunctionCall != nil {
			hasToolUse = true
			id := tm.GenerateID(p.FunctionCall.Name)
			input := p.FunctionCall.Args
			if input == nil {
				input = json.RawMessage(`{}`)
			}
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				ID:    id,
				Name:  p.FunctionCall.Name,
				Input: input,
			})
		}
	}

	if len(blocks) == 0 {
		blocks = []ContentBlock{{Type: "text", Text: ""}}
	}
	return blocks, hasToolUse
}

func mapFinishReason(reason string, hasToolUse bool) string {
	if hasToolUse {
		return "tool_use"
	}
	switch reason {
	case "MAX_TOKENS":
		return "max_tokens"
	case "STOP", "SAFETY", "":
		return "end_turn"
	default:
		return "end_turn"
	}
}
