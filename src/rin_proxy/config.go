package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ProviderConfig struct {
	APIKeyEnv string `json:"api_key_env"`
	AuthType  string `json:"auth_type"` // "api_key" (default), "oauth", or "vertex"
	Model     string `json:"model"`
	BaseURL   string `json:"base_url"`
	// For OAuth/Vertex: path to token file (e.g., ~/.gemini/oauth_creds.json)
	TokenFile string `json:"token_file,omitempty"`
	// For Vertex AI: project and location
	Project  string `json:"project,omitempty"`
	Location string `json:"location,omitempty"`
}

type Config struct {
	Port             int                       `json:"port"`
	AnthropicBaseURL string                    `json:"anthropic_base_url"`
	Providers        map[string]ProviderConfig `json:"providers"`
}

func DefaultConfig() Config {
	// Vertex AI defaults from environment (same vars as Gemini CLI)
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	location := os.Getenv("GOOGLE_CLOUD_LOCATION")
	if location == "" {
		location = "global"
	}

	// Detect auth type: if GOOGLE_GENAI_USE_VERTEXAI=true → vertex, else check for API key
	authType := "api_key"
	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if os.Getenv("GOOGLE_GENAI_USE_VERTEXAI") == "true" && project != "" {
		authType = "vertex"
		baseURL = "https://aiplatform.googleapis.com/v1"
	}

	return Config{
		Port:             3456,
		AnthropicBaseURL: "https://api.anthropic.com",
		Providers: map[string]ProviderConfig{
			"gemini-pro": {
				AuthType:  authType,
				APIKeyEnv: "GEMINI_API_KEY",
				Model:     "gemini-2.5-pro",
				BaseURL:   baseURL,
				Project:   project,
				Location:  location,
			},
			"gemini-flash": {
				AuthType:  authType,
				APIKeyEnv: "GEMINI_API_KEY",
				Model:     "gemini-2.5-flash",
				BaseURL:   baseURL,
				Project:   project,
				Location:  location,
			},
			"glm-5": {
				AuthType:  "anthropic",
				APIKeyEnv: "GLM_API_KEY",
				Model:     "glm-5",
				BaseURL:   "https://api.z.ai/api/anthropic",
			},
		},
	}
}

func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultConfig(), nil
	}

	path := filepath.Join(home, ".rin", "proxy-config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func (p ProviderConfig) ResolveAPIKey() string {
	return os.Getenv(p.APIKeyEnv)
}

func (p ProviderConfig) IsOAuth() bool {
	return p.AuthType == "oauth" || p.AuthType == "vertex"
}

func (p ProviderConfig) IsVertex() bool {
	return p.AuthType == "vertex"
}

func (p ProviderConfig) IsOpenAI() bool {
	return p.AuthType == "openai"
}

func (p ProviderConfig) IsAnthropicCompat() bool {
	return p.AuthType == "anthropic"
}

// ResolveOAuthToken gets an access token for Gemini API access.
// For Vertex AI: uses gcloud auth print-access-token (cached for 50 min).
// For OAuth: reads from Gemini CLI's oauth_creds.json.
func (p ProviderConfig) ResolveOAuthToken() string {
	if p.IsVertex() {
		return gcloudTokenCache.get()
	}
	// Fallback: Gemini CLI OAuth creds
	path := p.TokenFile
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".gemini", "oauth_creds.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var creds struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(data, &creds) != nil {
		return ""
	}
	return creds.AccessToken
}

// gcloudTokenCache caches gcloud access tokens (they expire after ~60 min).
var gcloudTokenCache = &tokenCache{}

type tokenCache struct {
	mu      sync.Mutex
	token   string
	expires time.Time
}

func (c *tokenCache) get() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expires) {
		return c.token
	}

	out, err := exec.Command("gcloud", "auth", "print-access-token").Output()
	if err != nil {
		return ""
	}
	c.token = strings.TrimSpace(string(out))
	c.expires = time.Now().Add(50 * time.Minute)
	return c.token
}
