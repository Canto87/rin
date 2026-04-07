package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

type queryPreferencesResult struct {
	Preferences []prefItem `json:"preferences"`
	Violations  []violItem `json:"violations"`
	Reminder    string     `json:"reminder"`
}

type prefItem struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type violItem struct {
	Title string `json:"title"`
	Count int    `json:"count"`
}

// runQueryPreferences queries PostgreSQL for preferences and optionally
// rule-violation error_patterns. Output is JSON to stdout.
// Errors go to stderr and output an empty result — never a non-zero exit.
func runQueryPreferences() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("query-preferences: config: %v", err)
		outputEmptyResult()
		return
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Printf("query-preferences: store: %v", err)
		outputEmptyResult()
		return
	}
	defer store.Close()

	// Parse flags
	var skillName string
	var includeViolations bool
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--skill":
			if i+1 < len(args) {
				skillName = args[i+1]
				i++
			}
		case "--violations":
			includeViolations = true
		}
	}

	result := queryPreferencesResult{
		Preferences: []prefItem{},
		Violations:  []violItem{},
		Reminder:    "Follow the skill's defined workflow exactly.",
	}

	// Query preferences
	prefs, err := fetchPreferences(ctx, store.pool, skillName)
	if err != nil {
		log.Printf("query-preferences: preference query: %v", err)
	} else {
		result.Preferences = prefs
	}

	// Query violations
	if includeViolations {
		viols, err := fetchViolations(ctx, store.pool)
		if err != nil {
			log.Printf("query-preferences: violation query: %v", err)
		} else {
			result.Violations = viols
		}
	}

	json.NewEncoder(os.Stdout).Encode(result)
}

func outputEmptyResult() {
	json.NewEncoder(os.Stdout).Encode(queryPreferencesResult{
		Preferences: []prefItem{},
		Violations:  []violItem{},
		Reminder:    "Follow the skill's defined workflow exactly.",
	})
}

// fetchPreferences queries preference documents. If skillName is non-empty,
// filters by content containing the skill name.
func fetchPreferences(ctx context.Context, pool *pgxpool.Pool, skillName string) ([]prefItem, error) {
	var query string
	var args []any

	if skillName != "" {
		query = `
			SELECT title, content FROM documents
			WHERE kind = 'preference' AND NOT archived
			  AND content ILIKE $1
			ORDER BY created_at DESC LIMIT 5`
		args = []any{"%" + skillName + "%"}
	} else {
		query = `
			SELECT title, content FROM documents
			WHERE kind = 'preference' AND NOT archived
			ORDER BY created_at DESC LIMIT 10`
	}

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefs := []prefItem{}
	for rows.Next() {
		var p prefItem
		if err := rows.Scan(&p.Title, &p.Content); err != nil {
			continue
		}
		prefs = append(prefs, p)
	}
	return prefs, nil
}

// fetchViolations queries error_pattern documents with 'rule-violation'
// that have 2+ occurrences.
func fetchViolations(ctx context.Context, pool *pgxpool.Pool) ([]violItem, error) {
	rows, err := pool.Query(ctx, `
		SELECT title, COUNT(*) as cnt FROM documents
		WHERE kind = 'error_pattern' AND NOT archived
		  AND content ILIKE '%rule-violation%'
		GROUP BY title HAVING COUNT(*) >= 2
		ORDER BY COUNT(*) DESC LIMIT 3
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	viols := []violItem{}
	for rows.Next() {
		var v violItem
		if err := rows.Scan(&v.Title, &v.Count); err != nil {
			continue
		}
		viols = append(viols, v)
	}
	return viols, nil
}
