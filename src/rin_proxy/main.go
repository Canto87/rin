package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	tm := NewToolMap()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleMessages(w, r, cfg, tm)
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("rin-proxy listening on %s", addr)
	log.Printf("  anthropic passthrough → %s", cfg.AnthropicBaseURL)
	for name, p := range cfg.Providers {
		log.Printf("  %s → %s (%s)", name, p.Model, p.BaseURL)
	}

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func handleMessages(w http.ResponseWriter, r *http.Request, cfg Config, tm *ToolMap) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Peek at the model field to decide routing
	var peek struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &peek); err != nil {
		http.Error(w, "parse request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Check if this is a custom provider alias
	provider, isCustomProvider := cfg.Providers[peek.Model]
	if !isCustomProvider {
		// Passthrough to Anthropic
		r.Body = io.NopCloser(bytes.NewReader(body))
		handlePassthrough(w, r, cfg)
		return
	}

	// Resolve auth (API key or OAuth token)
	var authToken string
	var authIsOAuth bool
	if provider.IsOAuth() {
		authToken = provider.ResolveOAuthToken()
		authIsOAuth = true
		if authToken == "" {
			http.Error(w, fmt.Sprintf("OAuth token not found for %s (file: %s)", peek.Model, provider.TokenFile), http.StatusInternalServerError)
			return
		}
	} else {
		authToken = provider.ResolveAPIKey()
		if authToken == "" {
			http.Error(w, fmt.Sprintf("API key not set for %s (env: %s)", peek.Model, provider.APIKeyEnv), http.StatusInternalServerError)
			return
		}
	}

	// Parse full request
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Route based on provider type
	if provider.IsAnthropicCompat() {
		// Anthropic-compatible passthrough (e.g., GLM via api.z.ai)
		r.Body = io.NopCloser(bytes.NewReader(body))
		handleAnthropicPassthrough(w, r, provider, authToken)
		return
	}

	if provider.IsOpenAI() {
		// OpenAI-compatible path
		openaiReq, err := toOpenAIRequest(req, tm)
		if err != nil {
			http.Error(w, "transform request: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if req.Stream {
			handleOpenAIStream(w, r, openaiReq, provider, authToken, peek.Model, tm)
		} else {
			handleOpenAISync(w, r, openaiReq, provider, authToken, peek.Model, tm)
		}
		return
	}

	// Gemini path
	gemReq, err := toGeminiRequest(req, tm)
	if err != nil {
		http.Error(w, "transform request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auth := geminiAuth{
		token:    authToken,
		isOAuth:  authIsOAuth,
		isVertex: provider.IsVertex(),
		project:  provider.Project,
		location: provider.Location,
	}
	if req.Stream {
		handleGeminiStream(w, r, gemReq, provider, auth, peek.Model, tm)
	} else {
		handleGeminiSync(w, r, gemReq, provider, auth, peek.Model, tm)
	}
}

type geminiAuth struct {
	token    string
	isOAuth  bool
	isVertex bool
	project  string
	location string
}

func (a geminiAuth) buildURL(baseURL, model, action string) string {
	if a.isVertex {
		// Vertex AI: /v1/projects/{project}/locations/{location}/publishers/google/models/{model}:{action}
		return fmt.Sprintf("%s/projects/%s/locations/%s/publishers/google/models/%s:%s",
			baseURL, a.project, a.location, model, action)
	}
	// Google AI Studio
	url := fmt.Sprintf("%s/models/%s:%s", baseURL, model, action)
	if !a.isOAuth {
		if strings.Contains(url, "?") {
			url += "&key=" + a.token
		} else {
			url += "?key=" + a.token
		}
	}
	return url
}

func (a geminiAuth) setHeaders(req *http.Request) {
	if a.isOAuth || a.isVertex {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
}

func handleGeminiSync(w http.ResponseWriter, r *http.Request, req *GeminiRequest, provider ProviderConfig, auth geminiAuth, requestModel string, tm *ToolMap) {
	url := auth.buildURL(provider.BaseURL, provider.Model, "generateContent")

	body, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "marshal gemini request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httpReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "create request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	auth.setHeaders(httpReq)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		http.Error(w, "gemini request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "read gemini response: "+err.Error(), http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("gemini error (%d): %s", resp.StatusCode, string(respBody))
		http.Error(w, fmt.Sprintf("gemini error: %s", string(respBody)), resp.StatusCode)
		return
	}

	var gemResp GeminiResponse
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		http.Error(w, "parse gemini response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	anthResp, err := fromGeminiResponse(gemResp, requestModel, tm)
	if err != nil {
		http.Error(w, "transform response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthResp)
}

func handleGeminiStream(w http.ResponseWriter, r *http.Request, req *GeminiRequest, provider ProviderConfig, auth geminiAuth, requestModel string, tm *ToolMap) {
	action := "streamGenerateContent?alt=sse"
	url := auth.buildURL(provider.BaseURL, provider.Model, action)

	body, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "marshal gemini request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httpReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "create request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	auth.setHeaders(httpReq)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		http.Error(w, "gemini request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("gemini stream error (%d): %s", resp.StatusCode, string(respBody))
		http.Error(w, fmt.Sprintf("gemini error: %s", string(respBody)), resp.StatusCode)
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

	streamGeminiToAnthropic(resp.Body, w, flusher, requestModel, tm)
}

// writeSSE writes a single SSE event.
func writeSSE(w io.Writer, f http.Flusher, event string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
	f.Flush()
}

// debugLog logs to stderr if RIN_PROXY_DEBUG is set.
func debugLog(format string, args ...any) {
	if os.Getenv("RIN_PROXY_DEBUG") != "" {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// parseSSELine extracts data payload from an SSE "data: ..." line.
func parseSSELine(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "data: ") {
		return strings.TrimPrefix(line, "data: "), true
	}
	return "", false
}
