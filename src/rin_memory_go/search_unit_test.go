package main

import (
	"math"
	"testing"
)

func TestTokenizeQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "simple words",
			query: "hello world",
			want:  []string{"hello", "world"},
		},
		{
			name:  "punctuation trimmed",
			query: "hello, world!",
			want:  []string{"hello", "world"},
		},
		{
			name:  "single char tokens filtered",
			query: "a is ok",
			want:  []string{"is", "ok"},
		},
		{
			name:  "lowercased",
			query: "Hello World",
			want:  []string{"hello", "world"},
		},
		{
			name:  "empty string",
			query: "",
			want:  nil,
		},
		{
			name:  "only short tokens",
			query: "a b c",
			want:  nil,
		},
		{
			name:  "mixed punctuation and short",
			query: "(x) [hi] {ok}",
			want:  []string{"hi", "ok"},
		},
		{
			name:  "whitespace only",
			query: "   \t  ",
			want:  nil,
		},
		{
			name:  "surrounding quotes stripped",
			query: `"hello" 'world'`,
			want:  []string{"hello", "world"},
		},
		{
			name:  "multiple spaces between words",
			query: "foo   bar   baz",
			want:  []string{"foo", "bar", "baz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizeQuery(tt.query)
			if !sliceEqual(got, tt.want) {
				t.Errorf("tokenizeQuery(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestRRFMerge(t *testing.T) {
	const K = RRFK // 60

	tests := []struct {
		name          string
		vectorResults map[string]int
		ftsResults    map[string]int
		limit         int
		wantLen       int
		// checkFn runs additional assertions on the result.
		checkFn func(t *testing.T, result []scoredDoc)
	}{
		{
			name:          "both sources same doc",
			vectorResults: map[string]int{"doc1": 1},
			ftsResults:    map[string]int{"doc1": 1},
			limit:         10,
			wantLen:       1,
			checkFn: func(t *testing.T, result []scoredDoc) {
				expected := 1.0/float64(K+1) + 1.0/float64(K+1)
				if !closeEnough(result[0].score, expected) {
					t.Errorf("score = %f, want %f", result[0].score, expected)
				}
			},
		},
		{
			name:          "only in vector",
			vectorResults: map[string]int{"doc1": 2},
			ftsResults:    map[string]int{},
			limit:         10,
			wantLen:       1,
			checkFn: func(t *testing.T, result []scoredDoc) {
				expected := 1.0 / float64(K+2)
				if !closeEnough(result[0].score, expected) {
					t.Errorf("score = %f, want %f", result[0].score, expected)
				}
			},
		},
		{
			name:          "only in FTS",
			vectorResults: map[string]int{},
			ftsResults:    map[string]int{"doc1": 3},
			limit:         10,
			wantLen:       1,
			checkFn: func(t *testing.T, result []scoredDoc) {
				expected := 1.0 / float64(K+3)
				if !closeEnough(result[0].score, expected) {
					t.Errorf("score = %f, want %f", result[0].score, expected)
				}
			},
		},
		{
			name:          "limit respected",
			vectorResults: map[string]int{"a": 1, "b": 2, "c": 3},
			ftsResults:    map[string]int{"d": 1, "e": 2},
			limit:         2,
			wantLen:       2,
			checkFn:       nil,
		},
		{
			name:          "empty inputs",
			vectorResults: map[string]int{},
			ftsResults:    map[string]int{},
			limit:         10,
			wantLen:       0,
			checkFn:       nil,
		},
		{
			name:          "score formula verification K=60",
			vectorResults: map[string]int{"doc1": 1},
			ftsResults:    map[string]int{},
			limit:         10,
			wantLen:       1,
			checkFn: func(t *testing.T, result []scoredDoc) {
				// rank=1, score = 1/(60+1) = 1/61
				expected := 1.0 / 61.0
				if !closeEnough(result[0].score, expected) {
					t.Errorf("score = %f, want 1/61 = %f", result[0].score, expected)
				}
			},
		},
		{
			name:          "higher rank means higher score",
			vectorResults: map[string]int{"top": 1, "bottom": 5},
			ftsResults:    map[string]int{},
			limit:         10,
			wantLen:       2,
			checkFn: func(t *testing.T, result []scoredDoc) {
				// rank 1 should score higher than rank 5
				if result[0].score <= result[1].score {
					t.Errorf("rank 1 score (%f) should be > rank 5 score (%f)", result[0].score, result[1].score)
				}
				if result[0].id != "top" {
					t.Errorf("first result should be 'top', got %q", result[0].id)
				}
			},
		},
		{
			name:          "doc in both sources ranks higher than single source",
			vectorResults: map[string]int{"both": 1, "vec_only": 2},
			ftsResults:    map[string]int{"both": 1, "fts_only": 2},
			limit:         10,
			wantLen:       3,
			checkFn: func(t *testing.T, result []scoredDoc) {
				// "both" should be first since it gets contributions from both sources
				if result[0].id != "both" {
					t.Errorf("first result should be 'both', got %q", result[0].id)
				}
				bothScore := 2.0 / float64(K+1) // 1/(K+1) + 1/(K+1)
				if !closeEnough(result[0].score, bothScore) {
					t.Errorf("both score = %f, want %f", result[0].score, bothScore)
				}
			},
		},
		{
			name:          "nil maps treated as empty",
			vectorResults: nil,
			ftsResults:    nil,
			limit:         5,
			wantLen:       0,
			checkFn:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rrfMerge(tt.vectorResults, tt.ftsResults, tt.limit)
			if len(result) != tt.wantLen {
				t.Errorf("len(result) = %d, want %d", len(result), tt.wantLen)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, result)
			}
		})
	}
}

// sliceEqual checks if two string slices are equal (nil and empty are considered equal).
func sliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// closeEnough checks if two floats are within a small epsilon.
func closeEnough(a, b float64) bool {
	return math.Abs(a-b) < 1e-12
}
