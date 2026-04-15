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

type skillStat struct {
	total      int
	success    int
	hookCount  int
	selfReport int
}

// runSkillStats is a user-facing CLI command (not a hook path).
// It may call log.Fatalf to surface errors to the user.
func runSkillStats() {
	fs := flag.NewFlagSet("skill-stats", flag.ExitOnError)
	days := fs.Int("days", 7, "Number of days to look back")
	skillFilter := fs.String("skill", "", "Filter by skill name")
	fs.Parse(os.Args[2:])

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("skill-stats: config: %v", err)
	}

	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Fatalf("skill-stats: store: %v", err)
	}
	defer store.Close()

	query := fmt.Sprintf(`
		SELECT content, tags
		FROM documents
		WHERE kind = 'routing_log'
		  AND NOT archived
		  AND tags @> ARRAY['tool_type:skill']::text[]
		  AND created_at >= NOW() - INTERVAL '%d days'
		ORDER BY created_at DESC
	`, *days)

	rows, err := store.pool.Query(ctx, query)
	if err != nil {
		log.Fatalf("skill-stats: query: %v", err)
	}
	defer rows.Close()

	stats := map[string]*skillStat{}

	for rows.Next() {
		var content string
		var tags []string
		if err := rows.Scan(&content, &tags); err != nil {
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

		if *skillFilter != "" && name != *skillFilter {
			continue
		}

		var p skillLogPayload
		if err := json.Unmarshal([]byte(content), &p); err != nil {
			continue
		}

		isHook := false
		isSelfReport := false
		for _, t := range tags {
			switch t {
			case "auto:hook":
				isHook = true
			case "self_report":
				isSelfReport = true
			}
		}

		if _, ok := stats[name]; !ok {
			stats[name] = &skillStat{}
		}
		stats[name].total++
		if p.Success {
			stats[name].success++
		}
		if isHook {
			stats[name].hookCount++
		}
		if isSelfReport {
			stats[name].selfReport++
		}
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("skill-stats: row iteration: %v", err)
	}

	if len(stats) == 0 {
		fmt.Println("(no skill events found)")
		return
	}

	type row struct {
		name string
		*skillStat
	}
	var rows2 []row
	for name, s := range stats {
		rows2 = append(rows2, row{name, s})
	}
	sort.Slice(rows2, func(i, j int) bool {
		return rows2[i].total > rows2[j].total
	})

	fmt.Printf("=== Skill Usage Stats (%dd) ===\n", *days)
	fmt.Printf("%-16s | %4s | %4s | %4s | %6s | %7s\n", "Skill", "Uses", "OK", "Fail", "Rate", "Abandon")
	fmt.Println(strings.Repeat("-", 56))

	totalAll, successAll := 0, 0
	for _, r := range rows2 {
		fail := r.total - r.success
		rate := 0.0
		if r.total > 0 {
			rate = float64(r.success) / float64(r.total) * 100
		}
		abandon := "   n/a"
		if r.hookCount > 0 {
			missed := max(0, r.hookCount-r.selfReport)
			abandon = fmt.Sprintf("%5.1f%%", float64(missed)/float64(r.hookCount)*100)
		}
		fmt.Printf("%-16s | %4d | %4d | %4d | %5.1f%% | %7s\n", r.name, r.total, r.success, fail, rate, abandon)
		totalAll += r.total
		successAll += r.success
	}

	fmt.Println(strings.Repeat("-", 56))
	failAll := totalAll - successAll
	rateAll := 0.0
	if totalAll > 0 {
		rateAll = float64(successAll) / float64(totalAll) * 100
	}
	fmt.Printf("%-16s | %4d | %4d | %4d | %5.1f%% | %7s\n", "Total", totalAll, successAll, failAll, rateAll, "")
}
