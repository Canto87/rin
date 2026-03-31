//go:build integration

package main

import (
	"context"
	"fmt"
	"os"
	"testing"
)

type goldenTest struct {
	query    string
	kind     *string
	project  *string
	expected []string
	note     string
}

func strPtr(s string) *string { return &s }

// Golden test set: Rin's agentic coding workflow patterns.
// Categories: error debugging, architecture decisions, coding preferences,
// system operations, team workflow.
var goldenTests = []goldenTest{
	// ===== Error Pattern Lookup (search error patterns when debugging) =====
	{query: "brain SQLite fallback env PG_URL", expected: []string{"ea674c0a7d23"}, note: "error: Brain SQLite fallback"},
	{query: "memory_store project slug mismatch search miss", expected: []string{"f6c5248410b4"}, note: "error: project slug mismatch"},
	{query: "fetch_url content field missing Anthropic API 400", expected: []string{"d76f9b99413d"}, note: "error: fetch_url empty content"},
	{query: "Go roundCancel context always break loop exit", expected: []string{"62c397202102"}, note: "error: Go context cancel pattern"},
	{query: "code-edit agent Go concurrency goroutine bug", expected: []string{"8b9a63eed7d6"}, note: "error: code-edit concurrency bugs"},
	{query: "RRF score cosine similarity threshold blocking all results", expected: []string{"9286c73a29fe"}, note: "error: RRF score range mismatch"},
	{query: "Haiku Sonnet model upgrade timeout insufficient", expected: []string{"27da4d9a06b9"}, note: "error: model upgrade timeout"},
	{query: "Xcode license build blocked cgo", expected: []string{"07cbe0fa7123"}, note: "error: Xcode license block"},
	{query: "letter_read timezone aware naive datetime comparison", expected: []string{"e3824c348718"}, note: "error: timezone comparison"},

	// ===== Architecture Decisions (check existing decisions when designing) =====
	{query: "thinking loop timeout cycle 300s round counter", expected: []string{"1506976bfa45"}, note: "arch: thinking loop timeout"},
	{query: "auto-research autonomous experiment loop design", expected: []string{"c9643bb140a3"}, note: "arch: auto-research skill design"},
	{query: "token history truncation trimHistoryToTokenBudget", expected: []string{"17cfdcc56c4e"}, note: "arch: token-based history truncation"},
	{query: "webhook notification implementation", expected: []string{"6a3d51433834"}, note: "arch: webhook notification"},
	{query: "BehaviorRule duplicate detection RuneJaccard similarity", expected: []string{"a84f518c011c"}, note: "arch: behavior rule dedup"},
	{query: "project structure refactoring design", expected: []string{"163630d18086"}, note: "arch: project restructure"},

	// ===== Coding Preferences (operator preference lookup) =====
	{query: "delegate code edits Opus avoid direct modification", expected: []string{"82e9580ca488"}, note: "pref: delegate code edits"},
	{query: "Gemini CLI flat-rate token saving strategy", expected: []string{"72fe4543ab92"}, note: "pref: Gemini CLI cost rules"},
	{query: "subagent prompts write in English token efficiency", expected: []string{"4b9082a704ae"}, note: "pref: English subagent prompts"},

	// ===== Domain Knowledge (operations/system search) =====
	{query: "backend API 8080 8081 service configuration", expected: []string{"2ed5f93ef323"}, note: "domain: service port mapping"},
	{query: "conversation log auto-collection session harvest launchd", expected: []string{"18a8897170d4"}, note: "domain: session harvest pipeline"},
	{query: "ProcessEventWithPrefetch contextBuf double-add duplicate", expected: []string{"7e043c15ffea"}, note: "domain: contextBuf double-add bug"},
	{query: "reverse proxy SSH tunnel access configuration", expected: []string{"d26cbcea59d1"}, note: "domain: SSH tunnel setup"},

	// ===== Team Workflow (team pattern search) =====
	{query: "multi-model routing criteria comparison", expected: []string{"cad17d603993"}, note: "team: multi-model routing"},
	{query: "service restart start-serving.sh script", expected: []string{"f1f2278910e3"}, note: "team: service restart workflow"},
	{query: "harness sync deploy workflow sync-harness", expected: []string{"fc0c42c48bb9"}, note: "team: harness sync workflow"},
}

const recallK = 5

func TestRecall(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	hits, total := 0, 0
	for _, tt := range goldenTests {
		docs, err := store.HybridSearch(ctx, tt.query, tt.kind, tt.project, recallK)
		if err != nil {
			t.Logf("FAIL q=%q err=%v", tt.query, err)
			total += len(tt.expected)
			continue
		}

		resultIDs := make(map[string]bool, len(docs))
		for _, d := range docs {
			resultIDs[d.ID] = true
		}

		for _, expectedID := range tt.expected {
			total++
			if resultIDs[expectedID] {
				hits++
				if testing.Verbose() {
					t.Logf("HIT  q=%-50s expected=%s", tt.query, expectedID[:12])
				}
			} else {
				if testing.Verbose() {
					t.Logf("MISS q=%-50s expected=%s", tt.query, expectedID[:12])
				}
			}
		}
	}

	recall := 0
	if total > 0 {
		recall = 100 * hits / total
	}

	t.Logf("Recall@%d: %d/%d = %d%%", recallK, hits, total, recall)

	// Print score to stdout for auto-research metric parsing
	fmt.Println(recall)

	// Write to file for easy extraction
	os.WriteFile("/tmp/rin-memory-recall-score.txt", []byte(fmt.Sprintf("%d\n", recall)), 0644)
}
