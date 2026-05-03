package main

import (
	"context"
	"fmt"
	"log"
)

// runPruneRoutingLog archives routing_log entries older than 30 days.
// routing_suggest uses a short rolling window (≤20 latest, days-bounded), so
// older entries are dead weight. Deterministic SQL — no LLM judgment required.
func runPruneRoutingLog() {
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
		WHERE kind = 'routing_log'
		  AND NOT archived
		  AND created_at < NOW() - INTERVAL '30 days'
	`)
	if err != nil {
		log.Fatalf("prune-routing-log failed: %v", err)
	}

	fmt.Printf("archived %d routing_log entries older than 30 days\n", tag.RowsAffected())
}
