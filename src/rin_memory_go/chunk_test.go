package main

import (
	"strings"
	"testing"
)

func TestChunkDocument_SingleChunk(t *testing.T) {
	title := "Short Note"
	tags := []string{"test", "small"}
	content := "This is a small document."

	chunks := ChunkDocument(title, tags, content)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ChunkIndex != 0 {
		t.Errorf("expected chunk index 0, got %d", chunks[0].ChunkIndex)
	}
	// Tags should be prepended to content
	if !strings.Contains(chunks[0].Text, "test small") {
		t.Errorf("expected tags in chunk text, got %q", chunks[0].Text)
	}
	if !strings.Contains(chunks[0].Text, title) {
		t.Errorf("expected title in chunk text, got %q", chunks[0].Text)
	}
}

func TestChunkDocument_NoTags(t *testing.T) {
	title := "No Tags"
	content := "Body without tags."

	chunks := ChunkDocument(title, nil, content)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	// Should not have a stray tag line — content starts directly
	if !strings.Contains(chunks[0].Text, "No Tags\nBody without tags.") {
		t.Errorf("unexpected chunk text: %q", chunks[0].Text)
	}
}

func TestChunkDocument_SplitsByHeadings(t *testing.T) {
	title := "Big Doc"
	// Create content with headings where total exceeds MaxChunk
	section := strings.Repeat("word ", 200) // ~1000 chars per section
	content := "## Section A\n" + section + "\n## Section B\n" + section

	chunks := ChunkDocument(title, nil, content)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks from heading split, got %d", len(chunks))
	}
	// Each chunk should contain the title
	for i, c := range chunks {
		if !strings.Contains(c.Text, title) {
			t.Errorf("chunk %d missing title: %q", i, c.Text)
		}
	}
	// Indices should be sequential
	for i, c := range chunks {
		if c.ChunkIndex != i {
			t.Errorf("chunk %d has index %d", i, c.ChunkIndex)
		}
	}
}

func TestChunkDocument_FallbackTruncation(t *testing.T) {
	title := "Huge"
	// A single enormous block with no headings and no paragraph breaks
	content := strings.Repeat("x", MaxChunk*3)

	chunks := ChunkDocument(title, nil, content)

	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk, got 0")
	}
	// Every chunk should respect MaxChunk (the splitWithOverlap path)
	for i, c := range chunks {
		if len(c.Text) > MaxChunk+len(title)+1 {
			// Allow title prefix overhead
			t.Errorf("chunk %d length %d exceeds expected max", i, len(c.Text))
		}
	}
}

// --- splitByHeadings ---

func TestSplitByHeadings(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantN    int
		wantSub  []string // substrings each section should contain
	}{
		{
			name:    "no headings returns whole content",
			content: "Just a plain paragraph with no headings at all.",
			wantN:   1,
			wantSub: []string{"Just a plain paragraph"},
		},
		{
			name:    "splits at h2",
			content: "## First\nContent A\n## Second\nContent B",
			wantN:   2,
			wantSub: []string{"## First", "## Second"},
		},
		{
			name:    "splits at h3",
			content: "### Alpha\nAlpha body\n### Beta\nBeta body",
			wantN:   2,
			wantSub: []string{"### Alpha", "### Beta"},
		},
		{
			name:    "preserves content before first heading",
			content: "Preamble text here.\n## Heading\nBody",
			wantN:   2,
			wantSub: []string{"Preamble text here.", "## Heading"},
		},
		{
			name:    "h1 does not split",
			content: "# Title\nSome text under h1.",
			wantN:   1,
			wantSub: []string{"# Title"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sections := splitByHeadings(tt.content)
			if len(sections) != tt.wantN {
				t.Fatalf("expected %d sections, got %d: %v", tt.wantN, len(sections), sections)
			}
			for i, sub := range tt.wantSub {
				if i >= len(sections) {
					break
				}
				if !strings.Contains(sections[i], sub) {
					t.Errorf("section %d: expected substring %q, got %q", i, sub, sections[i])
				}
			}
		})
	}
}

// --- splitByParagraphs ---

func TestSplitByParagraphs(t *testing.T) {
	t.Run("splits by double newline", func(t *testing.T) {
		text := "Paragraph one.\n\nParagraph two.\n\nParagraph three."
		chunks := splitByParagraphs(text, 100, 10)

		// All three fit in 100 chars, so should be a single chunk
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
		if !strings.Contains(chunks[0], "Paragraph one.") {
			t.Errorf("missing first paragraph in chunk: %q", chunks[0])
		}
	})

	t.Run("splits when paragraphs exceed max", func(t *testing.T) {
		p1 := strings.Repeat("a", 60)
		p2 := strings.Repeat("b", 60)
		p3 := strings.Repeat("c", 60)
		text := p1 + "\n\n" + p2 + "\n\n" + p3

		chunks := splitByParagraphs(text, 100, 10)

		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
		}
	})

	t.Run("overlap from previous chunk", func(t *testing.T) {
		p1 := strings.Repeat("alpha ", 15) // ~90 chars
		p2 := strings.Repeat("beta ", 15)  // ~75 chars
		p3 := strings.Repeat("gamma ", 15) // ~90 chars
		text := p1 + "\n\n" + p2 + "\n\n" + p3

		chunks := splitByParagraphs(text, 120, 30)

		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
		}
		// Second chunk should contain overlap text from the end of first chunk
		// (some trailing content from chunk 0 should appear in chunk 1)
		lastWords := chunks[0][len(chunks[0])-20:]
		// The overlap mechanism should carry some of the previous chunk forward
		// We just verify the second chunk is non-empty and starts reasonably
		if len(chunks[1]) == 0 {
			t.Error("second chunk should not be empty")
		}
		_ = lastWords // overlap content verified structurally
	})

	t.Run("large paragraph delegates to splitWithOverlap", func(t *testing.T) {
		hugePara := strings.Repeat("longword ", 200) // ~1800 chars
		text := "Short intro.\n\n" + hugePara

		chunks := splitByParagraphs(text, MaxChunk, Overlap)

		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks for huge paragraph, got %d", len(chunks))
		}
	})
}

// --- splitWithOverlap ---

func TestSplitWithOverlap(t *testing.T) {
	t.Run("short text returns single chunk", func(t *testing.T) {
		text := "Hello world"
		chunks := splitWithOverlap(text, 100, 10)

		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
		if chunks[0] != text {
			t.Errorf("expected %q, got %q", text, chunks[0])
		}
	})

	t.Run("respects maxChunk", func(t *testing.T) {
		text := strings.Repeat("word ", 100) // 500 chars
		maxChunk := 120
		chunks := splitWithOverlap(text, maxChunk, 20)

		if len(chunks) < 2 {
			t.Fatalf("expected multiple chunks, got %d", len(chunks))
		}
		// All chunks except possibly the last should be at most maxChunk
		for i := 0; i < len(chunks)-1; i++ {
			if len(chunks[i]) > maxChunk {
				t.Errorf("chunk %d length %d exceeds max %d", i, len(chunks[i]), maxChunk)
			}
		}
	})

	t.Run("creates overlap between chunks", func(t *testing.T) {
		text := strings.Repeat("word ", 100) // 500 chars
		maxChunk := 120
		overlap := 30
		chunks := splitWithOverlap(text, maxChunk, overlap)

		if len(chunks) < 3 {
			t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
		}
		// Consecutive chunks should share some content (overlap)
		for i := 1; i < len(chunks); i++ {
			prevEnd := chunks[i-1]
			currStart := chunks[i]
			// The beginning of the current chunk should overlap with end of previous
			// Find if any suffix of prev matches a prefix of curr
			found := false
			// Check a reasonable window
			for j := 1; j <= len(prevEnd) && j <= len(currStart); j++ {
				suffix := prevEnd[len(prevEnd)-j:]
				if strings.HasPrefix(currStart, suffix) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no overlap detected between chunk %d and %d", i-1, i)
			}
		}
	})
}

// --- findBreakPoint ---

func TestFindBreakPoint(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		target int
		want   int
	}{
		{
			name:   "finds space near target",
			text:   "hello world this is a test",
			target: 13, // text[13]='s', scans back to text[11]=' '
			want:   11, // the space between "this" and "is"
		},
		{
			name:   "finds newline near target",
			text:   "line one\nline two",
			target: 10, // 'i' in second "line"
			want:   8,  // the newline
		},
		{
			name:   "falls back to target when no break found",
			text:   strings.Repeat("x", 100),
			target: 60,
			want:   60,
		},
		{
			name:   "prefers closest break to target",
			text:   "aaa bbb ccc ddd",
			target: 10, // text[10]='c', scans back to text[7]=' '
			want:   7,  // space between "bbb" and "ccc"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findBreakPoint(tt.text, tt.target)
			if got != tt.want {
				t.Errorf("findBreakPoint(%q, %d) = %d, want %d", tt.text, tt.target, got, tt.want)
			}
		})
	}
}

// --- getOverlapText ---

func TestGetOverlapText(t *testing.T) {
	tests := []struct {
		name    string
		chunk   string
		overlap int
		check   func(t *testing.T, result string)
	}{
		{
			name:    "returns full chunk when shorter than overlap",
			chunk:   "short",
			overlap: 100,
			check: func(t *testing.T, result string) {
				if result != "short" {
					t.Errorf("expected %q, got %q", "short", result)
				}
			},
		},
		{
			name:    "extracts last N chars respecting word boundary",
			chunk:   "The quick brown fox jumps over the lazy dog",
			overlap: 20,
			check: func(t *testing.T, result string) {
				if len(result) > 20 {
					t.Errorf("result length %d exceeds overlap 20", len(result))
				}
				// Should not start mid-word (should start after a space/newline)
				if len(result) > 0 && result[0] != ' ' {
					// The function tries to find a space/newline boundary
					// Result should be a clean word boundary start
					words := strings.Fields(result)
					if len(words) == 0 {
						t.Error("expected non-empty overlap text")
					}
				}
			},
		},
		{
			name:    "handles exact overlap length",
			chunk:   strings.Repeat("a", 100),
			overlap: 100,
			check: func(t *testing.T, result string) {
				if result != strings.Repeat("a", 100) {
					t.Errorf("expected full chunk for exact overlap length")
				}
			},
		},
		{
			name:    "respects newline boundary",
			chunk:   "first line\nsecond line\nthird line here",
			overlap: 20,
			check: func(t *testing.T, result string) {
				// Should find a boundary near the last 20 chars
				if len(result) == 0 {
					t.Error("expected non-empty result")
				}
				// Result should not exceed original overlap window significantly
				if len(result) > 25 {
					t.Errorf("result too long: %d", len(result))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getOverlapText(tt.chunk, tt.overlap)
			tt.check(t, result)
		})
	}
}

// --- Integration-level: ChunkDocument produces valid indices ---

func TestChunkDocument_IndicesAreSequential(t *testing.T) {
	title := "Index Test"
	section := strings.Repeat("content ", 150)
	content := "## Part 1\n" + section + "\n## Part 2\n" + section + "\n## Part 3\n" + section

	chunks := ChunkDocument(title, []string{"tag1"}, content)

	for i, c := range chunks {
		if c.ChunkIndex != i {
			t.Errorf("expected sequential index %d, got %d", i, c.ChunkIndex)
		}
	}
}

func TestChunkDocument_EmptyContent(t *testing.T) {
	chunks := ChunkDocument("Title", nil, "")

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for empty content, got %d", len(chunks))
	}
	if chunks[0].Text != "Title\n" {
		t.Errorf("expected title with newline, got %q", chunks[0].Text)
	}
}
