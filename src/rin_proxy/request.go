package main

import (
	"encoding/json"
	"fmt"
)

// toGeminiRequest converts an Anthropic Messages API request to Gemini format.
func toGeminiRequest(req AnthropicRequest, tm *ToolMap) (*GeminiRequest, error) {
	out := &GeminiRequest{}

	// System prompt
	if req.System != nil {
		sys, err := convertSystem(req.System)
		if err != nil {
			return nil, fmt.Errorf("system: %w", err)
		}
		out.SystemInstruction = sys
	}

	// Messages → Contents
	contents, err := convertMessages(req.Messages, tm)
	if err != nil {
		return nil, fmt.Errorf("messages: %w", err)
	}
	out.Contents = contents

	// Tools
	if len(req.Tools) > 0 {
		out.Tools = convertTools(req.Tools)
	}

	// Tool choice
	if req.ToolChoice != nil {
		out.ToolConfig = convertToolChoice(req.ToolChoice)
	}

	// Generation config
	out.GenerationConfig = &GeminiGenConfig{
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		TopK:            req.TopK,
		StopSequences:   req.StopSequences,
	}

	return out, nil
}

func convertSystem(raw json.RawMessage) (*GeminiContent, error) {
	// Try as string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return &GeminiContent{Parts: []GeminiPart{{Text: s}}}, nil
	}

	// Try as array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("invalid system format: %w", err)
	}

	var parts []GeminiPart
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, GeminiPart{Text: b.Text})
		}
	}
	return &GeminiContent{Parts: parts}, nil
}

func convertMessages(msgs []Message, tm *ToolMap) ([]GeminiContent, error) {
	var contents []GeminiContent
	for _, msg := range msgs {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		parts, err := convertContent(msg.Content, tm)
		if err != nil {
			return nil, err
		}
		contents = append(contents, GeminiContent{Role: role, Parts: parts})
	}
	return contents, nil
}

func convertContent(raw json.RawMessage, tm *ToolMap) ([]GeminiPart, error) {
	// Try as string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []GeminiPart{{Text: s}}, nil
	}

	// Array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("invalid content: %w", err)
	}

	var parts []GeminiPart
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, GeminiPart{Text: b.Text})

		case "tool_use":
			parts = append(parts, GeminiPart{
				FunctionCall: &GeminiFunctionCall{Name: b.Name, Args: b.Input},
			})

		case "tool_result":
			name, ok := tm.ResolveName(b.ToolUseID)
			if !ok {
				name = "unknown"
			}
			parts = append(parts, GeminiPart{
				FunctionResponse: &GeminiFuncResponse{
					Name:     name,
					Response: wrapToolResult(b.Content),
				},
			})

		case "image":
			if b.Source != nil {
				parts = append(parts, GeminiPart{
					InlineData: &GeminiInlineData{
						MIMEType: b.Source.MediaType,
						Data:     b.Source.Data,
					},
				})
			}

		case "thinking":
			// No Gemini equivalent — strip
		}
	}
	return parts, nil
}

func wrapToolResult(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return json.RawMessage(`{"result":""}`)
	}
	// Already an object?
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		return raw
	}
	// String?
	var s string
	if json.Unmarshal(raw, &s) == nil {
		b, _ := json.Marshal(map[string]string{"result": s})
		return b
	}
	// Array of content blocks — extract text
	var blocks []ContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var text string
		for _, b := range blocks {
			if b.Type == "text" {
				text += b.Text
			}
		}
		b, _ := json.Marshal(map[string]string{"result": text})
		return b
	}
	return raw
}

func convertTools(tools []Tool) []GeminiToolGroup {
	var decls []GeminiFuncDecl
	for _, t := range tools {
		decls = append(decls, GeminiFuncDecl{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  sanitizeSchema(t.InputSchema),
		})
	}
	return []GeminiToolGroup{{FunctionDeclarations: decls}}
}

// sanitizeSchema removes fields that Vertex AI / Gemini doesn't accept
// in tool parameter schemas. Reference: ccr's cleanupParameters in gemini.util.ts.
func sanitizeSchema(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	var obj any
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	cleaned := cleanSchemaValue(obj)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return raw
	}
	return out
}

// Gemini-accepted schema fields.
var validSchemaFields = map[string]bool{
	"type": true, "format": true, "title": true, "description": true,
	"nullable": true, "enum": true, "maxItems": true, "minItems": true,
	"properties": true, "required": true, "minProperties": true,
	"maxProperties": true, "minLength": true, "maxLength": true,
	"pattern": true, "example": true, "anyOf": true,
	"propertyOrdering": true, "default": true, "items": true,
	"minimum": true, "maximum": true,
}

func cleanSchemaValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cleanSchemaObject(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = cleanSchemaValue(item)
		}
		return out
	default:
		return v
	}
}

func cleanSchemaObject(obj map[string]any) map[string]any {
	out := make(map[string]any)

	for k, v := range obj {
		// "properties" key contains sub-schemas keyed by property name — keep all keys
		if k == "properties" {
			if props, ok := v.(map[string]any); ok {
				cleaned := make(map[string]any)
				for propName, propSchema := range props {
					cleaned[propName] = cleanSchemaValue(propSchema)
				}
				out[k] = cleaned
			}
			continue
		}
		// Strip invalid top-level fields
		if !validSchemaFields[k] {
			continue
		}
		out[k] = cleanSchemaValue(v)
	}

	// Handle nullable anyOf pattern: {anyOf: [{type:"null"}, {type:"object"}]} → nullable + flatten
	if anyOf, ok := out["anyOf"].([]any); ok && len(anyOf) == 2 {
		isNull0 := isNullType(anyOf[0])
		isNull1 := isNullType(anyOf[1])
		if isNull0 || isNull1 {
			out["nullable"] = true
			if isNull0 {
				merged := cleanSchemaValue(anyOf[1])
				if m, ok := merged.(map[string]any); ok {
					for mk, mv := range m {
						out[mk] = mv
					}
				}
			} else {
				merged := cleanSchemaValue(anyOf[0])
				if m, ok := merged.(map[string]any); ok {
					for mk, mv := range m {
						out[mk] = mv
					}
				}
			}
			delete(out, "anyOf")
		}
	}

	return out
}

func isNullType(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	return m["type"] == "null"
}

func convertToolChoice(raw json.RawMessage) *GeminiToolConfig {
	var tc struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &tc) != nil {
		return nil
	}
	modes := map[string]string{
		"auto": "AUTO",
		"any":  "ANY",
		"none": "NONE",
		"tool": "ANY",
	}
	mode, ok := modes[tc.Type]
	if !ok {
		mode = "AUTO"
	}
	return &GeminiToolConfig{
		FunctionCallingConfig: &GeminiFCConfig{Mode: mode},
	}
}
