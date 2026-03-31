package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxChars int
		want     string
	}{
		{
			name:     "short text returns unchanged",
			text:     "hello",
			maxChars: 10,
			want:     "hello",
		},
		{
			name:     "text at exact byte boundary",
			text:     "hello",
			maxChars: 5,
			want:     "hello",
		},
		{
			name:     "truncates long ASCII text",
			text:     "hello world",
			maxChars: 5,
			want:     "hello",
		},
		{
			name:     "empty text",
			text:     "",
			maxChars: 10,
			want:     "",
		},
		{
			name:     "utf8 backs up from mid-sequence to leading byte",
			text:     "Hello 世界",
			maxChars: 8,
			// Bytes: H(0) e(1) l(2) l(3) o(4) ' '(5) E4(6) B8(7) 96(8) E4(9) B8(10) 96(11)
			// maxChars=8 → text[:8] ends at B8 (continuation byte), backs up to i=7
			// where byte E4 is a leading byte (not continuation), so returns text[:7].
			want: "Hello \xe4",
		},
		{
			name:     "utf8 backs up through multiple continuation bytes",
			text:     "abc世界def",
			maxChars: 5,
			// Bytes: a(0) b(1) c(2) E4(3) B8(4) 96(5) ...
			// text[:5] ends at B8 (continuation), i=4 → B8 (continuation),
			// i=3 → E4 (leading byte). Returns text[:4].
			want: "abc\xe4",
		},
		{
			name:     "utf8 at complete multibyte char end still backs up",
			text:     "abc世界",
			maxChars: 6,
			// text[:6] = a b c E4 B8 96 — last byte 0x96 is continuation.
			// Backs up through 96, B8, lands on E4 (leading). Returns text[:4].
			want: "abc\xe4",
		},
		{
			name:     "ascii truncation mid-word",
			text:     "abcdef",
			maxChars: 3,
			want:     "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateText(tt.text, tt.maxChars)
			if got != tt.want {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tt.text, tt.maxChars, got, tt.want)
			}
		})
	}
}

func TestUtf8ValidLast(t *testing.T) {
	// utf8ValidLast returns true when the last byte is NOT a continuation byte
	// (i.e., not in the 10xxxxxx / 0x80-0xBF range).
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "empty string",
			s:    "",
			want: true,
		},
		{
			name: "ASCII only",
			s:    "hello",
			want: true, // 'o' = 0x6F → not continuation
		},
		{
			name: "ends with continuation byte from complete multibyte char",
			s:    "hello世", // 世 = E4 B8 96; last byte 0x96 is continuation
			want: false,
		},
		{
			name: "ends with leading byte (orphan)",
			s:    "abc" + string([]byte{0xC3}), // 0xC3 is a leading byte, not continuation
			want: true,
		},
		{
			name: "single ASCII char",
			s:    "A",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utf8ValidLast(tt.s)
			if got != tt.want {
				lastByte := tt.s[len(tt.s)-1]
				t.Errorf("utf8ValidLast(%q) = %v, want %v (last byte: 0x%02X)", tt.s, got, tt.want, lastByte)
			}
		})
	}
}

func TestIsContextLengthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "context length exceeded",
			err:  errors.New("context length exceeded"),
			want: true,
		},
		{
			name: "input length too long",
			err:  errors.New("input length too long"),
			want: true,
		},
		{
			name: "token limit reached",
			err:  errors.New("token limit reached"),
			want: true,
		},
		{
			name: "text too long",
			err:  errors.New("text too long for model"),
			want: true,
		},
		{
			name: "case insensitive upper",
			err:  errors.New("CONTEXT LENGTH exceeded"),
			want: true,
		},
		{
			name: "case insensitive mixed",
			err:  errors.New("Token Limit Reached"),
			want: true,
		},
		{
			name: "wrapped error preserves message",
			err:  fmt.Errorf("ollama: %w", errors.New("context length exceeded")),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isContextLengthError(tt.err)
			if got != tt.want {
				t.Errorf("isContextLengthError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestVectorToLiteral(t *testing.T) {
	tests := []struct {
		name string
		v    []float32
		want string
	}{
		{
			name: "empty slice",
			v:    []float32{},
			want: "[]",
		},
		{
			name: "nil slice",
			v:    nil,
			want: "[]",
		},
		{
			name: "single element",
			v:    []float32{1.5},
			want: "[1.5]",
		},
		{
			name: "multiple elements",
			v:    []float32{1.5, 2.3, 0.1},
			want: "[1.5,2.3,0.1]",
		},
		{
			name: "negative numbers",
			v:    []float32{-1.5, -0.3},
			want: "[-1.5,-0.3]",
		},
		{
			name: "zero values",
			v:    []float32{0, 0, 0},
			want: "[0,0,0]",
		},
		{
			name: "mixed positive negative zero",
			v:    []float32{1.0, -2.0, 0, 0.5},
			want: "[1,-2,0,0.5]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vectorToLiteral(tt.v)
			if got != tt.want {
				t.Errorf("vectorToLiteral(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}
