package main

import "testing"

func TestEscapeAGE(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal string unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "single quote escaped",
			input: "it's",
			want:  "it\\'s",
		},
		{
			name:  "backslash escaped",
			input: "a\\b",
			want:  "a\\\\b",
		},
		{
			name:  "both quote and backslash",
			input: "it's a\\b",
			want:  "it\\'s a\\\\b",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "multiple single quotes",
			input: "it's Bob's",
			want:  "it\\'s Bob\\'s",
		},
		{
			name:  "multiple backslashes",
			input: "a\\b\\c",
			want:  "a\\\\b\\\\c",
		},
		{
			name:  "no special characters",
			input: "doc-abc123",
			want:  "doc-abc123",
		},
		{
			name:  "backslash then quote adjacent",
			input: "\\'",
			want:  "\\\\\\'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeAGE(tt.input)
			if got != tt.want {
				t.Errorf("escapeAGE(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseAgtypeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "quoted string",
			input: `"hello"`,
			want:  "hello",
		},
		{
			name:  "unquoted string",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "empty quotes",
			input: `""`,
			want:  "",
		},
		{
			name:  "single char unquoted",
			input: "x",
			want:  "x",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "quoted with spaces",
			input: `"hello world"`,
			want:  "hello world",
		},
		{
			name:  "single quote char not stripped",
			input: `"`,
			want:  `"`,
		},
		{
			name:  "quoted uuid",
			input: `"abc-123-def"`,
			want:  "abc-123-def",
		},
		{
			name:  "numeric unquoted",
			input: "42",
			want:  "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAgtypeString(tt.input)
			if got != tt.want {
				t.Errorf("parseAgtypeString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
