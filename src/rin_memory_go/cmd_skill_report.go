package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

const (
	degradationThreshold = -20.0 // percentage points
	improvementThreshold = 10.0
	minEventsForTrend    = 3
)

type periodStats struct {
	total   int
	success int
	fails   []string // 실패 레코드의 skill_args (에러 힌트)
}

// runSkillReport is a user-facing CLI command (not a hook path).
// It analyzes accumulated skill usage data, detects degradation,
// and generates improvement suggestions.
func runSkillReport() {
	fs := flag.NewFlagSet("skill-report", flag.ExitOnError)
	days := fs.Int("days", 30, "Current period lookback in days")
	storeResult := fs.Bool("store", false, "Store report to rin-memory")
	fs.Parse(os.Args[2:])

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("skill-report: config: %v", err)
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Fatalf("skill-report: store: %v", err)
	}
	defer store.Close()

	// 1. Current period stats
	current := querySkillPeriod(ctx, store, *days, 0)
	if current == nil {
		log.Fatalf("skill-report: query current period failed")
	}

	// 2. Previous period stats (for comparison)
	previous := querySkillPeriod(ctx, store, *days, *days)
	if previous == nil {
		log.Printf("skill-report: query previous period failed, proceeding without comparison")
		previous = map[string]*periodStats{}
	}

	// 3. Output report
	var report strings.Builder

	fmt.Fprintf(&report, "=== Skill Report (%dd) ===\n\n", *days)

	// Stats section
	report.WriteString("[Stats]\n")
	fmt.Fprintf(&report, "%-16s | %4s | %6s | %s\n", "Skill", "Uses", "Rate", "Trend")
	report.WriteString(strings.Repeat("-", 50) + "\n")

	// Sort by usage
	type entry struct {
		name string
		cur  *periodStats
		prev *periodStats
	}
	var entries []entry
	for name, cur := range current {
		entries = append(entries, entry{name, cur, previous[name]})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].cur.total > entries[j].cur.total
	})

	if len(entries) == 0 {
		report.WriteString("(no skill events found)\n")
	}

	var degradations []entry
	for _, e := range entries {
		// Guard against division by zero
		rate := 0.0
		if e.cur.total > 0 {
			rate = float64(e.cur.success) / float64(e.cur.total) * 100
		}
		trend := "stable"
		if e.prev != nil && e.prev.total >= minEventsForTrend && e.cur.total >= minEventsForTrend {
			prevRate := float64(e.prev.success) / float64(e.prev.total) * 100
			delta := rate - prevRate
			if delta <= degradationThreshold {
				trend = "▼ degrading"
				degradations = append(degradations, e)
			} else if delta >= improvementThreshold {
				trend = "▲ improving"
			}
		} else if e.prev == nil || e.prev.total == 0 {
			trend = "new"
		}
		fmt.Fprintf(&report, "%-16s | %4d | %5.1f%% | %s\n", e.name, e.cur.total, rate, trend)
	}

	// Degradation alerts
	if len(degradations) > 0 {
		report.WriteString("\n[!] Degradation Detected:\n")
		for _, e := range degradations {
			curRate := 0.0
			if e.cur.total > 0 {
				curRate = float64(e.cur.success) / float64(e.cur.total) * 100
			}
			prevRate := 0.0
			if e.prev != nil && e.prev.total > 0 {
				prevRate = float64(e.prev.success) / float64(e.prev.total) * 100
			}
			delta := curRate - prevRate
			fmt.Fprintf(&report, "  %s: %.1f%% (%dd avg) vs %.1f%% (prior %dd). Δ=%.1f%%p\n",
				e.name, curRate, *days, prevRate, *days, delta)

			// Show recent failures
			if len(e.cur.fails) > 0 {
				report.WriteString("  Recent failures:\n")
				limit := min(3, len(e.cur.fails))
				for _, f := range e.cur.fails[:limit] {
					fmt.Fprintf(&report, "    - %s\n", f)
				}
			}
		}
	}

	// Suggestions
	if len(degradations) > 0 {
		report.WriteString("\n[Suggestions]\n")
		for _, e := range degradations {
			suggestion := generateSuggestion(e.name, e.cur.fails)
			fmt.Fprintf(&report, "  %s: %s\n", e.name, suggestion)
		}
	}

	fmt.Print(report.String())

	// Store to rin-memory if --store flag
	if *storeResult {
		source := "skill-report"
		var project *string
		if cfg.Project != "" {
			project = &cfg.Project
		}
		_, err = store.StoreDocument(ctx, MemoryStoreInput{
			Kind:    "skill_report",
			Title:   fmt.Sprintf("Skill Report %dd", *days),
			Content: report.String(),
			Tags:    []string{"skill_report", "auto:cron"},
			Source:  &source,
			Project: project,
		})
		if err != nil {
			log.Printf("skill-report: store: %v", err)
		}
	}
}

// querySkillPeriod queries skill events for a specific time window.
// offset=0 means current period (last N days), offset=days means previous period.
// Returns nil on query error.
func querySkillPeriod(ctx context.Context, store *Store, days, offset int) map[string]*periodStats {
	query := `
		SELECT content, tags FROM documents
		WHERE kind = 'routing_log'
		  AND NOT archived
		  AND tags @> ARRAY['tool_type:skill']::text[]
		  AND created_at >= NOW() - ($1 * INTERVAL '1 day')
		  AND created_at < NOW() - ($2 * INTERVAL '1 day')
		ORDER BY created_at DESC
	`

	rows, err := store.pool.Query(ctx, query, days+offset, offset)
	if err != nil {
		log.Printf("skill-report: query (offset=%d): %v", offset, err)
		return nil
	}
	defer rows.Close()

	result := map[string]*periodStats{}
	for rows.Next() {
		var contentStr string
		var tags []string
		if err := rows.Scan(&contentStr, &tags); err != nil {
			continue
		}

		name := ""
		for _, t := range tags {
			if after, ok := strings.CutPrefix(t, "skill:"); ok {
				name = after
				break
			}
		}
		if name == "" {
			continue
		}

		var payload skillLogPayload
		if err := json.Unmarshal([]byte(contentStr), &payload); err != nil {
			continue
		}

		if _, ok := result[name]; !ok {
			result[name] = &periodStats{}
		}
		s := result[name]
		s.total++
		if payload.Success {
			s.success++
		} else {
			hint := payload.SkillArgs
			if hint == "" {
				hint = "(no args)"
			}
			s.fails = append(s.fails, hint)
		}
	}

	if err := rows.Err(); err != nil {
		log.Printf("skill-report: row iteration: %v", err)
	}

	return result
}

// generateSuggestion creates a simple keyword-based suggestion for a degrading skill.
func generateSuggestion(_ string, fails []string) string {
	if len(fails) == 0 {
		return "실패 데이터 부족. 추가 모니터링 필요."
	}

	failText := strings.Join(fails, " ")
	switch {
	case strings.Contains(failText, "timeout"):
		return "timeout 관련 실패 다수. skill.md에 timeout 대응 로직 추가 검토."
	case strings.Contains(failText, "error"):
		return "에러 발생 빈도 높음. 에러 처리 로직 보강 검토."
	default:
		return fmt.Sprintf("실패 %d건. skill.md 워크플로우 재검토 권장.", len(fails))
	}
}
