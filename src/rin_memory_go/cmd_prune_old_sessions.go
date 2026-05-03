package main

import (
	"context"
	"fmt"
	"log"
)

// runPruneOldSessions archives session_summary and session_journal entries
// older than 30 days. These kinds are short-term recall indexes — older
// entries lose value while persistent knowledge lives in arch_decision,
// domain_knowledge, error_pattern, preference, team_pattern.
//
// Deterministic SQL — no LLM judgment required.
func runPruneOldSessions() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}
	defer store.Close()

	tag, err := store.pool.Exec(ctx, `
		UPDATE documents
		SET archived = true, updated_at = NOW()
		WHERE kind IN ('session_summary', 'session_journal')
		  AND NOT archived
		  AND created_at < NOW() - INTERVAL '30 days'
	`)
	if err != nil {
		log.Fatalf("prune-old-sessions failed: %v", err)
	}

	fmt.Printf("archived %d session_summary/session_journal entries older than 30 days\n", tag.RowsAffected())
}
