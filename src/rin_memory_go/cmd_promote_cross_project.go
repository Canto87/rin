package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
)

// promotionCandidate represents an error_pattern that exists in 2+ projects.
type promotionCandidate struct {
	Title    string
	Projects []string
	NewestID string
	OlderIDs []string
}

// runPromoteCrossProject finds error_patterns that appear in 2+ projects
// and promotes the newest to project='*', archiving the duplicates.
// Output: one line per promotion, or "no promotions" if none found.
func runPromoteCrossProject() {
	dryRun := false
	for _, arg := range os.Args[2:] {
		if arg == "--dry-run" {
			dryRun = true
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("promote-cross-project: config: %v", err)
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Fatalf("promote-cross-project: store: %v", err)
	}
	defer store.Close()

	// Find titles that appear in 2+ distinct projects.
	rows, err := store.pool.Query(ctx, `
		SELECT title, array_agg(DISTINCT project) as projects
		FROM documents
		WHERE kind = 'error_pattern' AND NOT archived
		  AND project IS NOT NULL AND project != '*'
		GROUP BY title
		HAVING COUNT(DISTINCT project) >= 2
		LIMIT 20`)
	if err != nil {
		log.Fatalf("promote-cross-project: query: %v", err)
	}
	defer rows.Close()

	var candidates []promotionCandidate
	for rows.Next() {
		var c promotionCandidate
		if err := rows.Scan(&c.Title, &c.Projects); err != nil {
			log.Printf("promote-cross-project: scan: %v", err)
			continue
		}
		candidates = append(candidates, c)
	}

	if len(candidates) == 0 {
		fmt.Println("no promotions")
		return
	}

	promoted := 0
	for _, c := range candidates {
		// Find all docs with this title, ordered newest first.
		docRows, err := store.pool.Query(ctx, `
			SELECT id FROM documents
			WHERE kind = 'error_pattern' AND NOT archived AND title = $1
			  AND project IS NOT NULL AND project != '*'
			ORDER BY created_at DESC`, c.Title)
		if err != nil {
			log.Printf("promote-cross-project: doc query: %v", err)
			continue
		}

		var newestID string
		var olderIDs []string
		first := true
		for docRows.Next() {
			var id string
			if err := docRows.Scan(&id); err != nil {
				continue
			}
			if first {
				newestID = id
				first = false
			} else {
				olderIDs = append(olderIDs, id)
			}
		}
		docRows.Close()

		if newestID == "" {
			continue
		}

		projects := strings.Join(c.Projects, ", ")

		if dryRun {
			fmt.Printf("would promote: %q (projects: %s, keep: %s, archive: %d)\n",
				c.Title, projects, newestID, len(olderIDs))
			continue
		}

		// Promote newest to global scope.
		_, err = store.pool.Exec(ctx,
			`UPDATE documents SET project = '*' WHERE id = $1`, newestID)
		if err != nil {
			log.Printf("promote-cross-project: update %s: %v", newestID, err)
			continue
		}

		// Archive older duplicates.
		for _, oldID := range olderIDs {
			_, err = store.pool.Exec(ctx,
				`UPDATE documents SET archived = true WHERE id = $1`, oldID)
			if err != nil {
				log.Printf("promote-cross-project: archive %s: %v", oldID, err)
			}
		}

		fmt.Printf("promoted: %q (from %s, archived %d duplicates)\n",
			c.Title, projects, len(olderIDs))
		promoted++
	}

	if !dryRun {
		fmt.Printf("total: %d promoted\n", promoted)
	}
}
