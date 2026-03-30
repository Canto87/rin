package main

import (
	"context"
	"fmt"
	"log"
)

// runCount prints the number of non-archived documents to stdout.
// Used by the statusline script to display memory doc count without psql.
func runCount() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		fmt.Println("?")
		return
	}
	defer store.Close()

	var count int
	err = store.pool.QueryRow(ctx, "SELECT COUNT(*) FROM documents WHERE NOT archived").Scan(&count)
	if err != nil {
		fmt.Println("?")
		return
	}

	fmt.Println(count)
}
