package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferKind(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Session Journal 2026-03-15", "session_journal"},
		{"Architecture Decision: Use PostgreSQL", "arch_decision"},
		{"ADR: Switch to Go", "arch_decision"},
		{"Domain Knowledge: Ollama API", "domain_knowledge"},
		{"Error: Connection Timeout", "error_pattern"},
		{"Bug Fix: Memory Leak", "error_pattern"},
		{"Active Task: Migrate DB", "active_task"},
		{"Team Pattern: Code Review", "team_pattern"},
		{"Preference: Dark Mode", "preference"},
		{"Config: Editor Settings", "preference"},
		{"Routing Log: Model Stats", "routing_log"},
		{"Change Log: v2.0", "code_change"},
		{"Random Title With No Keywords", "domain_knowledge"},
		{"", "domain_knowledge"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := inferKind(tt.title)
			if got != tt.want {
				t.Errorf("inferKind(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestParseMarkdownSections(t *testing.T) {
	t.Run("single heading with content", func(t *testing.T) {
		content := "## Session Journal 2026-03-15\n\nWorked on database migration.\nCompleted schema changes.\n"
		path := writeTempFile(t, content)

		sections, err := ParseMarkdownSections(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sections) != 1 {
			t.Fatalf("expected 1 section, got %d", len(sections))
		}
		if sections[0].Title != "Session Journal 2026-03-15" {
			t.Errorf("title = %q", sections[0].Title)
		}
		if sections[0].Kind != "session_journal" {
			t.Errorf("kind = %q, want session_journal", sections[0].Kind)
		}
	})

	t.Run("multiple headings", func(t *testing.T) {
		content := "## Session 2026-03-15\n\nDid stuff.\n\n## Architecture Decision\n\nUse PostgreSQL.\n\n## Error: Timeout\n\nFixed it.\n"
		path := writeTempFile(t, content)

		sections, err := ParseMarkdownSections(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sections) != 3 {
			t.Fatalf("expected 3 sections, got %d", len(sections))
		}
		if sections[0].Kind != "session_journal" {
			t.Errorf("section 0 kind = %q", sections[0].Kind)
		}
		if sections[1].Kind != "arch_decision" {
			t.Errorf("section 1 kind = %q", sections[1].Kind)
		}
		if sections[2].Kind != "error_pattern" {
			t.Errorf("section 2 kind = %q", sections[2].Kind)
		}
	})

	t.Run("date extraction", func(t *testing.T) {
		content := "## Session 2026-03-15\n\nContent here.\n"
		path := writeTempFile(t, content)

		sections, err := ParseMarkdownSections(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sections) != 1 {
			t.Fatalf("expected 1 section, got %d", len(sections))
		}
		if len(sections[0].Tags) == 0 || sections[0].Tags[0] != "2026-03-15" {
			t.Errorf("tags = %v, want [2026-03-15]", sections[0].Tags)
		}
		if sections[0].Source == nil || *sections[0].Source != "session:2026-03-15" {
			t.Errorf("source = %v, want session:2026-03-15", sections[0].Source)
		}
	})

	t.Run("preamble before headings", func(t *testing.T) {
		content := "Some intro text.\n\n## First Section\n\nSection content.\n"
		path := writeTempFile(t, content)

		sections, err := ParseMarkdownSections(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sections) != 2 {
			t.Fatalf("expected 2 sections, got %d", len(sections))
		}
		if sections[0].Title != "Introduction" {
			t.Errorf("preamble title = %q, want Introduction", sections[0].Title)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := writeTempFile(t, "")

		sections, err := ParseMarkdownSections(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sections) != 0 {
			t.Errorf("expected 0 sections, got %d", len(sections))
		}
	})

	t.Run("heading with empty content", func(t *testing.T) {
		content := "## Empty Section\n\n## Has Content\n\nSome text.\n"
		path := writeTempFile(t, content)

		sections, err := ParseMarkdownSections(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Empty section should be skipped
		for _, s := range sections {
			if s.Content == "" {
				t.Error("section with empty content should be excluded")
			}
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ParseMarkdownSections("/nonexistent/path.md")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}
