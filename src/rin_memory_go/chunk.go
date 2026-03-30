package main

import (
	"regexp"
	"strings"
)

const (
	MaxChunk = 1200
	Overlap  = 100
)

// Chunk represents a document chunk for embedding.
type Chunk struct {
	ChunkIndex int    `json:"chunk_index"`
	Text       string `json:"text"`
}

// headingRegex matches markdown headings (## or ###).
var headingRegex = regexp.MustCompile(`(?m)^#{2,3}\s`)

// ChunkDocument splits a document into chunks for embedding.
// Tags are prepended to content so that tag keywords are included in embedding vectors.
func ChunkDocument(title string, tags []string, content string) []Chunk {
	// Prepend tags to content so tag keywords are searchable via embedding
	if len(tags) > 0 {
		content = strings.Join(tags, " ") + "\n" + content
	}

	// Combine title and content
	full := title + "\n" + content

	// If small enough, return single chunk
	if len(full) <= MaxChunk {
		return []Chunk{{ChunkIndex: 0, Text: full}}
	}

	var chunks []Chunk
	chunkIndex := 0

	// Split by headings (## or ###)
	sections := splitByHeadings(content)

	for _, section := range sections {
		// Prefix each section with title
		sectionWithTitle := title + "\n" + section

		if len(sectionWithTitle) <= MaxChunk {
			// Section fits in one chunk
			chunks = append(chunks, Chunk{
				ChunkIndex: chunkIndex,
				Text:       sectionWithTitle,
			})
			chunkIndex++
		} else {
			// Section too large, split by paragraphs
			paraChunks := splitByParagraphs(sectionWithTitle, MaxChunk, Overlap)
			for _, pc := range paraChunks {
				chunks = append(chunks, Chunk{
					ChunkIndex: chunkIndex,
					Text:       pc,
				})
				chunkIndex++
			}
		}
	}

	// Fallback: if no chunks were created, return truncated full text
	if len(chunks) == 0 {
		chunks = append(chunks, Chunk{
			ChunkIndex: 0,
			Text:       truncateText(full, MaxChunk),
		})
	}

	return chunks
}

// splitByHeadings splits content by ## or ### headings.
func splitByHeadings(content string) []string {
	indices := headingRegex.FindAllStringIndex(content, -1)

	if len(indices) == 0 {
		return []string{content}
	}

	var sections []string

	// Add content before first heading if any
	if indices[0][0] > 0 {
		sections = append(sections, strings.TrimSpace(content[:indices[0][0]]))
	}

	// Extract each section
	for i, idx := range indices {
		start := idx[0]
		var end int
		if i+1 < len(indices) {
			end = indices[i+1][0]
		} else {
			end = len(content)
		}
		section := strings.TrimSpace(content[start:end])
		if section != "" {
			sections = append(sections, section)
		}
	}

	return sections
}

// splitByParagraphs splits text by paragraphs with overlap.
func splitByParagraphs(text string, maxChunk, overlap int) []string {
	paragraphs := regexp.MustCompile(`\n\n+`).Split(text, -1)

	var chunks []string
	var currentChunk strings.Builder
	currentLen := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		paraLen := len(para)

		// If single paragraph exceeds max, split it with overlap
		if paraLen > maxChunk {
			// First, flush current chunk if any
			if currentLen > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
				currentLen = 0
			}
			// Split large paragraph
			paraChunks := splitWithOverlap(para, maxChunk, overlap)
			chunks = append(chunks, paraChunks...)
			continue
		}

		// Check if adding this paragraph would exceed limit
		if currentLen > 0 && currentLen+1+paraLen > maxChunk {
			// Flush current chunk
			chunks = append(chunks, currentChunk.String())

			// Start new chunk with overlap from previous
			prevChunk := currentChunk.String()
			overlapText := getOverlapText(prevChunk, overlap)
			currentChunk.Reset()
			if overlapText != "" {
				currentChunk.WriteString(overlapText)
				currentChunk.WriteString("\n")
				currentLen = len(overlapText) + 1
			} else {
				currentLen = 0
			}
		}

		if currentLen > 0 {
			currentChunk.WriteString("\n")
			currentLen++
		}
		currentChunk.WriteString(para)
		currentLen += paraLen
	}

	// Don't forget the last chunk
	if currentLen > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}

// splitWithOverlap splits a long string into chunks with overlap.
func splitWithOverlap(text string, maxChunk, overlap int) []string {
	if len(text) <= maxChunk {
		return []string{text}
	}

	var chunks []string
	start := 0

	for start < len(text) {
		end := start + maxChunk
		if end >= len(text) {
			chunks = append(chunks, text[start:])
			break
		}

		// Try to find a good break point (space or newline)
		breakPoint := findBreakPoint(text, end)
		chunks = append(chunks, text[start:breakPoint])

		// Move start back by overlap, but not before previous start
		newStart := breakPoint - overlap
		if newStart <= start {
			newStart = breakPoint
		}
		start = newStart
	}

	return chunks
}

// findBreakPoint finds a good point to break text near the target position.
func findBreakPoint(text string, target int) int {
	// Look for newline or space before target
	for i := target; i > target-50 && i > 0; i-- {
		if text[i] == '\n' || text[i] == ' ' {
			return i
		}
	}
	// No good break point found, just break at target
	return target
}

// getOverlapText gets the last N characters from a chunk for overlap.
func getOverlapText(chunk string, overlap int) string {
	if len(chunk) <= overlap {
		return chunk
	}

	// Find a good starting point (after a newline or space if possible)
	start := len(chunk) - overlap
	for i := start; i < len(chunk) && i < start+20; i++ {
		if chunk[i] == '\n' || chunk[i] == ' ' {
			start = i + 1
			break
		}
	}

	return chunk[start:]
}
