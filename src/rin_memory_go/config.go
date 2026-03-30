package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type MemoryConfig struct {
	DSN       string `json:"dsn"`
	OllamaURL string `json:"ollama_url"`
	Project   string `json:"project"`
}

func DefaultConfig() *MemoryConfig {
	return &MemoryConfig{
		DSN:       "postgres:///rin_memory",
		OllamaURL: "http://localhost:11434",
		Project:   os.Getenv("RIN_PROJECT"),
	}
}

func LoadConfig() (*MemoryConfig, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}

	path := filepath.Join(home, ".rin", "memory-config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// env override
	if dsn := os.Getenv("RIN_MEMORY_DSN"); dsn != "" {
		cfg.DSN = dsn
	}
	if proj := os.Getenv("RIN_PROJECT"); proj != "" {
		cfg.Project = proj
	}

	return cfg, nil
}
