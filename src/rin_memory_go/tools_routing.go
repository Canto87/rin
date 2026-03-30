package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// levelDefaults maps complexity level to default routing config.
var levelDefaults = map[string]struct {
	model      string
	mode       string
	agentCount int
}{
	"L1": {"glm-5", "solo", 1},
	"L2": {"glm-5", "solo", 1},
	"L3": {"glm-5", "team", 2},
}

// classifyLevel returns "L1", "L2", or "L3" based on inputs.
func classifyLevel(fileCount int, hasDependencies, needsDesign bool) string {
	if hasDependencies || needsDesign || fileCount > 3 {
		return "L3"
	}
	if fileCount >= 2 {
		return "L2"
	}
	return "L1"
}

// routingLogEntry is the JSON structure stored in routing_log content.
type routingLogEntry struct {
	Task         string   `json:"task"`
	Model        string   `json:"model"`
	DurationS    int      `json:"duration_s"`
	Success      bool     `json:"success"`
	Level        string   `json:"level,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	AgentCount   int      `json:"agent_count,omitempty"`
	FilesChanged *int     `json:"files_changed,omitempty"`
	FilesList    []string `json:"files_list,omitempty"`
	FallbackUsed bool     `json:"fallback_used,omitempty"`
	FallbackFrom *string  `json:"fallback_from,omitempty"`
	ErrorType    *string  `json:"error_type,omitempty"`
	Project      string   `json:"project,omitempty"`
	LoggedAt     string   `json:"logged_at"`
}

// modelStats accumulates stats per model from routing logs.
type modelStats struct {
	total    int
	success  int
	durations []int
}

// routingHistoryEntry is one entry in the suggest response history.
type routingHistoryEntry struct {
	Model         string  `json:"model"`
	Total         int     `json:"total"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationS  float64 `json:"avg_duration_s"`
}

func registerRoutingTools(server *mcp.Server, cfg *MemoryConfig, store *Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "routing_suggest",
		Description: "Suggest model routing based on past experience. Returns level, model, mode, confidence.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RoutingSuggestInput) (*mcp.CallToolResult, any, error) {
		// 1. Classify level.
		fileCount := 0
		if input.FileCount != nil {
			fileCount = *input.FileCount
		}
		level := classifyLevel(fileCount, input.HasDependencies, input.NeedsDesign)
		def := levelDefaults[level]

		// 2. Resolve project.
		project := cfg.Project
		if input.Project != nil {
			project = *input.Project
		}

		// 3. Query recent routing logs.
		query := `
			SELECT content FROM documents
			WHERE kind = 'routing_log' AND NOT archived
			ORDER BY created_at DESC LIMIT 20
		`
		var args []any
		if project != "" {
			query = `
				SELECT content FROM documents
				WHERE kind = 'routing_log' AND NOT archived
				  AND (project = $1 OR project IS NULL)
				ORDER BY created_at DESC LIMIT 20
			`
			args = []any{project}
		}

		rows, err := store.pool.Query(ctx, query, args...)
		if err != nil {
			return textResult(fmt.Sprintf("error querying routing logs: %v", err)), nil, nil
		}
		defer rows.Close()

		// 4. Parse and aggregate per model.
		statsMap := map[string]*modelStats{}
		for rows.Next() {
			var contentStr string
			if err := rows.Scan(&contentStr); err != nil {
				continue
			}
			var entry routingLogEntry
			if err := json.Unmarshal([]byte(contentStr), &entry); err != nil {
				continue
			}
			m := entry.Model
			if _, ok := statsMap[m]; !ok {
				statsMap[m] = &modelStats{}
			}
			s := statsMap[m]
			s.total++
			if entry.Success {
				s.success++
			}
			s.durations = append(s.durations, entry.DurationS)
		}

		// 5. Score models and pick best.
		type scoredModel struct {
			model string
			score float64
			stats *modelStats
		}
		var scored []scoredModel
		for m, s := range statsMap {
			successRate := float64(s.success) / float64(s.total)
			avgDur := 0.0
			for _, d := range s.durations {
				avgDur += float64(d)
			}
			if len(s.durations) > 0 {
				avgDur /= float64(len(s.durations))
			}
			speedScore := 1.0 - avgDur/300.0
			if speedScore < 0 {
				speedScore = 0
			}
			score := successRate*0.7 + speedScore*0.3
			scored = append(scored, scoredModel{m, score, s})
		}
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].score > scored[j].score
		})

		// 6. Compute confidence and pick model.
		totalLogs := 0
		for _, s := range statsMap {
			totalLogs += s.total
		}
		confidence := float64(totalLogs) / 10.0
		if confidence > 1.0 {
			confidence = 1.0
		}

		chosenModel := def.model
		reason := fmt.Sprintf("default: no routing history")
		if len(scored) > 0 {
			best := scored[0]
			chosenModel = best.model
			successRate := float64(best.stats.success) / float64(best.stats.total)
			reason = fmt.Sprintf("%s: %.0f%% success (%d logs)", best.model, successRate*100, best.stats.total)
		}

		// 7. Build history slice.
		history := make([]routingHistoryEntry, 0, len(scored))
		for _, sc := range scored {
			avgDur := 0.0
			for _, d := range sc.stats.durations {
				avgDur += float64(d)
			}
			if len(sc.stats.durations) > 0 {
				avgDur /= float64(len(sc.stats.durations))
			}
			history = append(history, routingHistoryEntry{
				Model:        sc.model,
				Total:        sc.stats.total,
				SuccessRate:  float64(sc.stats.success) / float64(sc.stats.total),
				AvgDurationS: avgDur,
			})
		}

		result := map[string]any{
			"level":       level,
			"model":       chosenModel,
			"mode":        def.mode,
			"agent_count": def.agentCount,
			"confidence":  confidence,
			"reason":      reason,
			"history":     history,
		}
		return jsonResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "routing_log",
		Description: "Log a routing decision result. Detects failure patterns.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RoutingLogInput) (*mcp.CallToolResult, any, error) {
		// 1. Resolve level and project.
		level := "L1"
		if input.Level != nil {
			level = *input.Level
		}
		project := cfg.Project
		if input.Project != nil {
			project = *input.Project
		}

		// 2. Build status string.
		status := "ok"
		if !input.Success {
			status = "fail"
		}

		// 3. Build tags.
		tags := []string{
			fmt.Sprintf("model:%s", input.Model),
			fmt.Sprintf("level:%s", level),
			status,
			fmt.Sprintf("mode:%s", input.Mode),
		}
		if input.FallbackUsed {
			tags = append(tags, "fallback")
		}
		if input.ErrorType != nil {
			tags = append(tags, fmt.Sprintf("error:%s", *input.ErrorType))
		}

		// 4. Build content JSON.
		entry := routingLogEntry{
			Task:         input.Task,
			Model:        input.Model,
			DurationS:    input.DurationS,
			Success:      input.Success,
			Level:        level,
			Mode:         input.Mode,
			AgentCount:   input.AgentCount,
			FilesChanged: input.FilesChanged,
			FilesList:    input.FilesList,
			FallbackUsed: input.FallbackUsed,
			FallbackFrom: input.FallbackFrom,
			ErrorType:    input.ErrorType,
			Project:      project,
			LoggedAt:     time.Now().Format(time.RFC3339),
		}
		contentBytes, err := json.Marshal(entry)
		if err != nil {
			return textResult(fmt.Sprintf("error encoding log entry: %v", err)), nil, nil
		}

		// 5. Build source and title.
		title := fmt.Sprintf("routing:%s:%s:%s", input.Model, level, status)
		source := "routing:" + time.Now().Format("2006-01-02")
		var projPtr *string
		if project != "" {
			projPtr = &project
		}
		var sourcePtr *string
		sourcePtr = &source

		storeInput := MemoryStoreInput{
			Kind:    "routing_log",
			Title:   title,
			Content: string(contentBytes),
			Tags:    tags,
			Source:  sourcePtr,
			Project: projPtr,
		}

		_, err = store.StoreDocument(ctx, storeInput)
		if err != nil {
			return textResult(fmt.Sprintf("error storing routing log: %v", err)), nil, nil
		}

		// 6. Consecutive failure detection on failed logs.
		if !input.Success {
			modelTag := fmt.Sprintf("model:%s", input.Model)
			failRows, err := store.pool.Query(ctx, `
				SELECT tags FROM documents
				WHERE kind = 'routing_log'
				  AND tags @> ARRAY[$1]::text[]
				  AND NOT archived
				ORDER BY created_at DESC LIMIT 3
			`, modelTag)
			if err == nil {
				allFail := true
				count := 0
				for failRows.Next() {
					count++
					var rowTags []string
					if scanErr := failRows.Scan(&rowTags); scanErr != nil {
						allFail = false
						break
					}
					hasFail := false
					for _, t := range rowTags {
						if t == "fail" {
							hasFail = true
							break
						}
					}
					if !hasFail {
						allFail = false
						break
					}
				}
				failRows.Close()

				if allFail && count == 3 {
					warnContent := fmt.Sprintf("%s has failed 3 consecutive times. Consider using a fallback model.", input.Model)
					warnTitle := fmt.Sprintf("[Warning] %s consecutive failure detected", input.Model)
					warnTags := []string{"routing_alert", fmt.Sprintf("model:%s", input.Model)}
					warnSource := "routing:" + time.Now().Format("2006-01-02")
					warnSourcePtr := &warnSource
					_, _ = store.StoreDocument(ctx, MemoryStoreInput{
						Kind:    "team_pattern",
						Title:   warnTitle,
						Content: warnContent,
						Tags:    warnTags,
						Source:  warnSourcePtr,
						Project: projPtr,
					})
				}
			}
		}

		return textResult(fmt.Sprintf("Logged routing: %s %s %s (%ds)", input.Model, level, status, input.DurationS)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "routing_stats",
		Description: "Routing performance statistics. Success rate, avg/p90 duration by model and level.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RoutingStatsInput) (*mcp.CallToolResult, any, error) {
		// 1. Resolve defaults.
		days := input.Days
		if days <= 0 {
			days = 7
		}
		project := cfg.Project
		if input.Project != nil {
			project = *input.Project
		}

		// 2. Build query.
		var queryBuilder strings.Builder
		var args []any
		argIdx := 1

		queryBuilder.WriteString(`
			SELECT content FROM documents
			WHERE kind = 'routing_log' AND NOT archived
			  AND created_at >= NOW() - INTERVAL '`)
		queryBuilder.WriteString(fmt.Sprintf("%d", days))
		queryBuilder.WriteString(` days'`)

		if project != "" {
			queryBuilder.WriteString(fmt.Sprintf(" AND (project = $%d OR project IS NULL)", argIdx))
			args = append(args, project)
			argIdx++
		}
		queryBuilder.WriteString(" ORDER BY created_at DESC")

		rows, err := store.pool.Query(ctx, queryBuilder.String(), args...)
		if err != nil {
			return textResult(fmt.Sprintf("error querying routing stats: %v", err)), nil, nil
		}
		defer rows.Close()

		// 3. Parse and group by model -> level.
		type levelKey struct{ model, level string }
		type levelBucket struct {
			total     int
			success   int
			durations []int
		}
		buckets := map[levelKey]*levelBucket{}

		for rows.Next() {
			var contentStr string
			if err := rows.Scan(&contentStr); err != nil {
				continue
			}
			var entry routingLogEntry
			if err := json.Unmarshal([]byte(contentStr), &entry); err != nil {
				continue
			}

			// Apply model/level filters if set.
			if input.Model != nil && entry.Model != *input.Model {
				continue
			}
			if input.Level != nil && entry.Level != *input.Level {
				continue
			}

			key := levelKey{entry.Model, entry.Level}
			if _, ok := buckets[key]; !ok {
				buckets[key] = &levelBucket{}
			}
			b := buckets[key]
			b.total++
			if entry.Success {
				b.success++
			}
			b.durations = append(b.durations, entry.DurationS)
		}

		// 4. Build result map: model -> level -> stats.
		type levelStat struct {
			Total         int     `json:"total"`
			Success       int     `json:"success"`
			SuccessRate   float64 `json:"success_rate"`
			AvgDurationS  float64 `json:"avg_duration_s"`
			P90DurationS  float64 `json:"p90_duration_s"`
		}
		result := map[string]map[string]levelStat{}

		for key, b := range buckets {
			if _, ok := result[key.model]; !ok {
				result[key.model] = map[string]levelStat{}
			}

			// Compute avg.
			avgDur := 0.0
			for _, d := range b.durations {
				avgDur += float64(d)
			}
			if len(b.durations) > 0 {
				avgDur /= float64(len(b.durations))
			}

			// Compute p90.
			sorted := make([]int, len(b.durations))
			copy(sorted, b.durations)
			sort.Ints(sorted)
			p90Dur := 0.0
			if n := len(sorted); n > 0 {
				idx := int(float64(n) * 0.9)
				if idx >= n {
					idx = n - 1
				}
				p90Dur = float64(sorted[idx])
			}

			successRate := 0.0
			if b.total > 0 {
				successRate = float64(b.success) / float64(b.total)
			}

			result[key.model][key.level] = levelStat{
				Total:        b.total,
				Success:      b.success,
				SuccessRate:  successRate,
				AvgDurationS: avgDur,
				P90DurationS: p90Dur,
			}
		}

		return jsonResult(result), nil, nil
	})
}
