package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// IngestSection represents a parsed markdown section.
type IngestSection struct {
	Title   string
	Content string
	Kind    string   // inferred from heading keywords
	Tags    []string // date tags if found
	Source  *string  // "session:YYYY-MM-DD" if date found
}

// kindKeywords maps heading keywords to document kinds.
var kindKeywords = map[string]string{
	"session journal":        "session_journal",
	"session":                "session_journal",
	"architectural decision": "arch_decision",
	"architecture decision":  "arch_decision",
	"adr":                    "arch_decision",
	"domain knowledge":       "domain_knowledge",
	"knowledge":              "domain_knowledge",
	"code change":            "code_change",
	"change log":             "code_change",
	"changelog":              "code_change",
	"team pattern":           "team_pattern",
	"pattern":                "team_pattern",
	"routing log":            "routing_log",
	"routing":                "routing_log",
	"active task":            "active_task",
	"task":                   "active_task",
	"error pattern":          "error_pattern",
	"error":                  "error_pattern",
	"bug":                    "error_pattern",
	"preference":             "preference",
	"config":                 "preference",
}

// dateRegex matches YYYY-MM-DD pattern.
var dateRegex = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)

// headingRegexIngest matches markdown headings.
var headingRegexIngest = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// ParseMarkdownSections parses a markdown file into sections.
func ParseMarkdownSections(filePath string) ([]IngestSection, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	var sections []IngestSection
	var currentSection *IngestSection
	var contentLines []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check for heading
		matches := headingRegexIngest.FindStringSubmatch(line)
		if matches != nil {
			// Save previous section if any
			if currentSection != nil && len(contentLines) > 0 {
				currentSection.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
				if currentSection.Content != "" {
					sections = append(sections, *currentSection)
				}
			}

			// Start new section
			title := strings.TrimSpace(matches[2])
			currentSection = &IngestSection{
				Title: title,
				Kind:  inferKind(title),
			}

			// Extract date from title
			if dateMatch := dateRegex.FindStringSubmatch(title); dateMatch != nil {
				dateStr := dateMatch[1]
				currentSection.Tags = []string{dateStr}
				source := "session:" + dateStr
				currentSection.Source = &source
			}

			contentLines = nil
		} else if currentSection != nil {
			// Add to current section content
			contentLines = append(contentLines, line)
		} else {
			// Content before first heading - create a section with filename as title
			if len(contentLines) == 0 {
				// Use filename as title for preamble
				currentSection = &IngestSection{
					Title: "Introduction",
					Kind:  "domain_knowledge",
				}
			}
			contentLines = append(contentLines, line)
		}
	}

	// Don't forget the last section
	if currentSection != nil && len(contentLines) > 0 {
		currentSection.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
		if currentSection.Content != "" {
			sections = append(sections, *currentSection)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return sections, nil
}

// inferKind infers document kind from title keywords.
func inferKind(title string) string {
	titleLower := strings.ToLower(title)

	for keyword, kind := range kindKeywords {
		if strings.Contains(titleLower, keyword) {
			return kind
		}
	}

	// Default kind
	return "domain_knowledge"
}

// IngestFile ingests a markdown file into the memory store.
func (s *Store) IngestFile(ctx context.Context, input MemoryIngestInput) ([]string, error) {
	sections, err := ParseMarkdownSections(input.FilePath)
	if err != nil {
		return nil, err
	}

	var docIDs []string
	for _, section := range sections {
		// Determine kind: explicit > inferred
		kind := section.Kind
		if input.Kind != "" {
			kind = input.Kind
		}

		// Determine source: explicit > extracted
		source := section.Source
		if input.Source != nil {
			source = input.Source
		}

		// Merge tags
		tags := section.Tags

		storeInput := MemoryStoreInput{
			Kind:    kind,
			Title:   section.Title,
			Content: section.Content,
			Tags:    tags,
			Source:  source,
			Project: input.Project,
		}

		docID, err := s.StoreDocument(ctx, storeInput)
		if err != nil {
			// Log but continue with other sections
			continue
		}
		docIDs = append(docIDs, docID)
	}

	return docIDs, nil
}
