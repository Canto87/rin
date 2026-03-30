package main

import (
	"io"
	"log"
	"net/http"
	"strings"
)

// handleAnthropicPassthrough forwards the request to an Anthropic-compatible provider (e.g., GLM via api.z.ai).
// It replaces the API key and routes to the provider's base URL while keeping the Anthropic request format.
func handleAnthropicPassthrough(w http.ResponseWriter, r *http.Request, provider ProviderConfig, apiKey string) {
	target := provider.BaseURL + r.URL.Path

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, "failed to create proxy request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy all headers, replace auth (strip both x-api-key and Authorization)
	for k, vv := range r.Header {
		if strings.EqualFold(k, "x-api-key") || strings.EqualFold(k, "authorization") {
			continue
		}
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}
	proxyReq.Header.Set("x-api-key", apiKey)

	log.Printf("anthropic-compat → %s%s (model: %s)", provider.BaseURL, r.URL.Path, provider.Model)

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		log.Printf("anthropic-compat error: %v", err)
		http.Error(w, "upstream request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("anthropic-compat ← %d %s (content-type: %s)",
		resp.StatusCode, resp.Status, resp.Header.Get("Content-Type"))

	if resp.StatusCode != http.StatusOK {
		// Non-200: log response body for debugging
		body, _ := io.ReadAll(resp.Body)
		log.Printf("anthropic-compat error body: %s", string(body))
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if f, ok := w.(http.Flusher); ok {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				f.Flush()
			}
			if err != nil {
				break
			}
		}
	} else {
		io.Copy(w, resp.Body)
	}
}

// handlePassthrough forwards the request to Anthropic API as-is.
func handlePassthrough(w http.ResponseWriter, r *http.Request, cfg Config) {
	target := cfg.AnthropicBaseURL + r.URL.Path

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, "failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy all headers
	for k, vv := range r.Header {
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		log.Printf("passthrough error: %v", err)
		http.Error(w, "upstream request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream the response body through
	if f, ok := w.(http.Flusher); ok {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				f.Flush()
			}
			if err != nil {
				break
			}
		}
	} else {
		io.Copy(w, resp.Body)
	}
}
