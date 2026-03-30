package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// runRecall queries PostgreSQL and outputs the "Recent Memory" markdown section
// to stdout. It is a direct port of scripts/rin-memory-recall.py.
func runRecall() {
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

	project := os.Getenv("RIN_PROJECT")
	loadSession := os.Getenv("RIN_LOAD_SESSION")

	// Build project filter fragment for use in queries.
	// When project is set, restrict to matching project or NULL.
	hasProject := project != ""

	lines := []string{"## Recent Memory (auto-loaded)", ""}

	// ── 1. Loaded Session Context ────────────────────────────────────────
	if loadSession != "" {
		type sessionDoc struct {
			kind    string
			title   string
			content string
			summary string
		}

		rows, err := store.pool.Query(ctx, `
			SELECT kind, title, content, COALESCE(summary, '') FROM documents
			WHERE source = $1 AND NOT archived
			ORDER BY kind, created_at
		`, loadSession)
		if err != nil {
			log.Printf("warning: loaded session query failed: %v", err)
		} else {
			var docs []sessionDoc
			for rows.Next() {
				var d sessionDoc
				if err := rows.Scan(&d.kind, &d.title, &d.content, &d.summary); err != nil {
					continue
				}
				docs = append(docs, d)
			}
			rows.Close()

			if len(docs) > 0 {
				// Extract date from source: "session:2026-02-23" -> "2026-02-23"
				sourceDate := loadSession
				if strings.HasPrefix(loadSession, "session:") {
					sourceDate = loadSession[8:]
				}

				lines = append(lines, fmt.Sprintf("### Loaded Session Context (%s)", sourceDate))
				lines = append(lines, "")

				for _, d := range docs {
					switch d.kind {
					case "session_journal":
						lines = append(lines, fmt.Sprintf("**%s**", d.title))
						text := d.summary
						if text == "" {
							text = truncate(d.content, 500)
						}
						if text != "" {
							lines = append(lines, text)
						}
						lines = append(lines, "")
					case "arch_decision":
						lines = append(lines, fmt.Sprintf("**[Decision] %s**", d.title))
						lines = append(lines, truncate(d.content, 300))
						lines = append(lines, "")
					default:
						lines = append(lines, fmt.Sprintf("**[%s] %s**", d.kind, d.title))
						lines = append(lines, truncate(d.content, 200))
						lines = append(lines, "")
					}
				}

				lines = append(lines, "---")
				lines = append(lines, "")
			}
		}
	}

	// ── 2. Active Tasks ───────────────────────────────────────────────────
	{
		type taskRow struct {
			id      string
			title   string
			content string
		}

		var rows interface{ Next() bool; Scan(...any) error; Close() }
		var queryErr error

		if hasProject {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT id, title, content FROM documents
				WHERE kind = 'active_task' AND NOT archived
				  AND (project = $1 OR project IS NULL)
				ORDER BY created_at DESC LIMIT 3
			`, project)
		} else {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT id, title, content FROM documents
				WHERE kind = 'active_task' AND NOT archived
				ORDER BY created_at DESC LIMIT 3
			`)
		}

		if queryErr != nil {
			log.Printf("warning: active_task query failed: %v", queryErr)
		} else {
			var tasks []taskRow
			for rows.Next() {
				var t taskRow
				if err := rows.Scan(&t.id, &t.title, &t.content); err != nil {
					continue
				}
				tasks = append(tasks, t)
			}
			rows.Close()

			if len(tasks) > 0 {
				lines = append(lines, "### Active Tasks (unfinished)")
				for _, t := range tasks {
					content := truncateInline(t.content, 200)
					lines = append(lines, fmt.Sprintf("- [%s] **%s**: %s", t.id, t.title, content))
				}
				lines = append(lines, "")
			}
		}
	}

	// ── 3. Recent Sessions ────────────────────────────────────────────────
	{
		type journalRow struct {
			title   string
			summary string
		}

		var rows interface{ Next() bool; Scan(...any) error; Close() }
		var queryErr error

		if hasProject {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT title, COALESCE(summary, '') FROM documents
				WHERE kind = 'session_journal' AND NOT archived
				  AND (project = $1 OR project IS NULL)
				ORDER BY created_at DESC LIMIT 3
			`, project)
		} else {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT title, COALESCE(summary, '') FROM documents
				WHERE kind = 'session_journal' AND NOT archived
				ORDER BY created_at DESC LIMIT 3
			`)
		}

		if queryErr != nil {
			log.Printf("warning: session_journal query failed: %v", queryErr)
		} else {
			var journals []journalRow
			for rows.Next() {
				var j journalRow
				if err := rows.Scan(&j.title, &j.summary); err != nil {
					continue
				}
				journals = append(journals, j)
			}
			rows.Close()

			if len(journals) > 0 {
				lines = append(lines, "### Recent Sessions")
				for _, j := range journals {
					if j.summary != "" {
						lines = append(lines, fmt.Sprintf("- %s — %s", j.title, j.summary))
					} else {
						lines = append(lines, fmt.Sprintf("- %s", j.title))
					}
				}
				lines = append(lines, "")
			}
		}
	}

	// ── 4. Recent Decisions ───────────────────────────────────────────────
	{
		type decisionRow struct {
			title   string
			content string
		}

		var rows interface{ Next() bool; Scan(...any) error; Close() }
		var queryErr error

		if hasProject {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT title, content FROM documents
				WHERE kind = 'arch_decision' AND NOT archived
				  AND (project = $1 OR project IS NULL)
				ORDER BY created_at DESC LIMIT 3
			`, project)
		} else {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT title, content FROM documents
				WHERE kind = 'arch_decision' AND NOT archived
				ORDER BY created_at DESC LIMIT 3
			`)
		}

		if queryErr != nil {
			log.Printf("warning: arch_decision query failed: %v", queryErr)
		} else {
			var decisions []decisionRow
			for rows.Next() {
				var d decisionRow
				if err := rows.Scan(&d.title, &d.content); err != nil {
					continue
				}
				decisions = append(decisions, d)
			}
			rows.Close()

			if len(decisions) > 0 {
				lines = append(lines, "### Recent Decisions")
				for _, d := range decisions {
					content := truncateInline(d.content, 120)
					lines = append(lines, fmt.Sprintf("- **%s**: %s...", d.title, content))
				}
				lines = append(lines, "")
			}
		}
	}

	// ── 5. Team Patterns ──────────────────────────────────────────────────
	{
		type patternRow struct {
			title   string
			content string
		}

		var rows interface{ Next() bool; Scan(...any) error; Close() }
		var queryErr error

		if hasProject {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT title, content FROM documents
				WHERE kind = 'team_pattern' AND NOT archived
				  AND (project = $1 OR project IS NULL)
				ORDER BY created_at DESC LIMIT 5
			`, project)
		} else {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT title, content FROM documents
				WHERE kind = 'team_pattern' AND NOT archived
				ORDER BY created_at DESC LIMIT 5
			`)
		}

		if queryErr != nil {
			log.Printf("warning: team_pattern query failed: %v", queryErr)
		} else {
			var patterns []patternRow
			for rows.Next() {
				var p patternRow
				if err := rows.Scan(&p.title, &p.content); err != nil {
					continue
				}
				patterns = append(patterns, p)
			}
			rows.Close()

			if len(patterns) > 0 {
				lines = append(lines, "### Team Patterns")
				for _, p := range patterns {
					content := truncateInline(p.content, 200)
					lines = append(lines, fmt.Sprintf("- %s", content))
				}
				lines = append(lines, "")
			}
		}
	}

	// ── 6. Operator Preferences ─────────────────────────────────────────
	{
		type prefRow struct {
			title   string
			content string
		}

		var rows interface{ Next() bool; Scan(...any) error; Close() }
		var queryErr error

		if hasProject {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT title, content FROM documents
				WHERE kind = 'preference' AND NOT archived
				  AND (project = $1 OR project IS NULL)
				ORDER BY created_at DESC LIMIT 5
			`, project)
		} else {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT title, content FROM documents
				WHERE kind = 'preference' AND NOT archived
				ORDER BY created_at DESC LIMIT 5
			`)
		}

		if queryErr != nil {
			log.Printf("warning: preference query failed: %v", queryErr)
		} else {
			var prefs []prefRow
			for rows.Next() {
				var p prefRow
				if err := rows.Scan(&p.title, &p.content); err != nil {
					continue
				}
				prefs = append(prefs, p)
			}
			rows.Close()

			if len(prefs) > 0 {
				lines = append(lines, "### Operator Preferences")
				for _, p := range prefs {
					content := truncateInline(p.content, 150)
					lines = append(lines, fmt.Sprintf("- **%s**: %s", p.title, content))
				}
				lines = append(lines, "")
			}
		}
	}

	// ── 7. Routing Stats (7d) ─────────────────────────────────────────────
	{
		var rows interface{ Next() bool; Scan(...any) error; Close() }
		var queryErr error

		if hasProject {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT content FROM documents
				WHERE kind = 'routing_log' AND NOT archived
				  AND (project = $1 OR project IS NULL)
				  AND created_at >= NOW() - INTERVAL '7 days'
				ORDER BY created_at DESC LIMIT 50
			`, project)
		} else {
			rows, queryErr = store.pool.Query(ctx, `
				SELECT content FROM documents
				WHERE kind = 'routing_log' AND NOT archived
				  AND created_at >= NOW() - INTERVAL '7 days'
				ORDER BY created_at DESC LIMIT 50
			`)
		}

		if queryErr != nil {
			log.Printf("warning: routing_log query failed: %v", queryErr)
		} else {
			type modelStat struct {
				ok   int
				fail int
			}
			modelStats := map[string]*modelStat{}

			for rows.Next() {
				var content string
				if err := rows.Scan(&content); err != nil {
					continue
				}

				var data map[string]any
				if err := json.Unmarshal([]byte(content), &data); err != nil {
					continue
				}

				model, _ := data["model"].(string)
				if model == "" {
					model = "?"
				}
				success, _ := data["success"].(bool)

				if _, ok := modelStats[model]; !ok {
					modelStats[model] = &modelStat{}
				}
				if success {
					modelStats[model].ok++
				} else {
					modelStats[model].fail++
				}
			}
			rows.Close()

			if len(modelStats) > 0 {
				// Sort model names for deterministic output.
				models := make([]string, 0, len(modelStats))
				for m := range modelStats {
					models = append(models, m)
				}
				sort.Strings(models)

				lines = append(lines, "### Routing Stats (7d)")
				for _, m := range models {
					s := modelStats[m]
					total := s.ok + s.fail
					rate := 0
					if total > 0 {
						rate = int(float64(s.ok) / float64(total) * 100)
					}
					lines = append(lines, fmt.Sprintf("- %s: %d%% (%d/%d)", m, rate, s.ok, total))
				}
				lines = append(lines, "")
			}
		}
	}

	// Only output if there's actual content beyond the header (header = 2 lines).
	if len(lines) <= 2 {
		return
	}

	lines = append(lines, "Use `memory_search` for detailed context on any topic.")
	fmt.Println(strings.Join(lines, "\n"))
}

// truncate returns s truncated to at most n runes, without newline replacement.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// truncateInline truncates s to n runes and replaces newlines with spaces,
// matching the Python [:N].replace("\n", " ") pattern.
func truncateInline(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		runes = runes[:n]
	}
	result := string(runes)
	return strings.ReplaceAll(result, "\n", " ")
}
