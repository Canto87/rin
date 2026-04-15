package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
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
		Reminder:    "Follow the skill workflow as defined.",
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
		Reminder:    "Follow the skill workflow as defined.",
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

// skillIntentConfig controls what kinds of memory to query per skill.
type skillIntentConfig struct {
	Kinds []string // document kinds to search
	Limit int      // max results
}

// skillIntents maps skill names to their intent-based query configuration.
// Skills not listed use the default (error_pattern + domain_knowledge).
var skillIntents = map[string]skillIntentConfig{
	"troubleshoot":  {Kinds: []string{"error_pattern"}, Limit: 5},
	"qa-gate":       {Kinds: []string{"error_pattern", "preference"}, Limit: 3},
	"auto-impl":     {Kinds: []string{"arch_decision", "domain_knowledge"}, Limit: 3},
	"plan-feature":  {Kinds: []string{"arch_decision", "domain_knowledge"}, Limit: 3},
	"ideate":        {Kinds: []string{"arch_decision"}, Limit: 2},
	"gc":            {Kinds: []string{"error_pattern"}, Limit: 3},
	"smart-commit":  {Kinds: []string{"preference"}, Limit: 2},
	"create-pr":     {Kinds: []string{"preference"}, Limit: 2},
}

var defaultIntent = skillIntentConfig{
	Kinds: []string{"error_pattern", "domain_knowledge"},
	Limit: 3,
}

// fetchSkillExperience queries memory documents relevant to a specific skill,
// adapting the query based on the skill's intent profile.
//
// Filtering rules:
//   - Exclude auto-captured errors that haven't been analyzed yet (title starts with "[auto]"
//     and content contains "미분석")
//   - Exclude documents tagged with confidence:low
//   - Prefer confidence:high documents first, then medium, then untagged
func fetchSkillExperience(ctx context.Context, pool *pgxpool.Pool, skillName string) ([]prefItem, error) {
	if skillName == "" {
		return nil, nil
	}

	intent, ok := skillIntents[skillName]
	if !ok {
		intent = defaultIntent
	}

	rows, err := pool.Query(ctx, `
		SELECT title, content, tags FROM documents
		WHERE kind = ANY($3::text[]) AND NOT archived
		  AND (content ILIKE $1 OR tags @> ARRAY[$2]::text[])
		  AND NOT (title LIKE '[auto]%' AND content LIKE '%미분석%')
		  AND NOT tags @> ARRAY['confidence:low']::text[]
		ORDER BY
		  CASE WHEN tags @> ARRAY['confidence:high']::text[]
		         AND created_at >= NOW() - INTERVAL '180 days' THEN 0
		       WHEN tags @> ARRAY['confidence:high']::text[] THEN 1
		       WHEN tags @> ARRAY['confidence:medium']::text[]
		         AND created_at >= NOW() - INTERVAL '180 days' THEN 1
		       WHEN tags @> ARRAY['confidence:medium']::text[] THEN 2
		       ELSE 3 END,
		  created_at DESC
		LIMIT `+fmt.Sprintf("%d", intent.Limit),
		"%"+skillName+"%", "skill:"+skillName, intent.Kinds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []prefItem
	for rows.Next() {
		var p prefItem
		var tags []string
		if err := rows.Scan(&p.Title, &p.Content, &tags); err != nil {
			continue
		}
		results = append(results, p)
	}
	return results, nil
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

// fetchSkillPolicy reads policy.yaml and returns the section for the given
// skill as a formatted string for context injection. Returns "" if policy.yaml
// is not found or the skill has no policy section.
func fetchSkillPolicy(skillName string) string {
	path := findPolicyYAML()
	if path == "" {
		return ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	// Parse as generic map to extract skill-specific section
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ""
	}

	section, ok := raw[skillName]
	if !ok {
		// Try case-insensitive with hyphens stripped (e.g., "qagate" matches "qa-gate")
		for key, val := range raw {
			if strings.EqualFold(key, skillName) ||
				strings.EqualFold(strings.ReplaceAll(key, "-", ""), strings.ReplaceAll(skillName, "-", "")) {
				section = val
				ok = true
				break
			}
		}
	}
	if !ok {
		return ""
	}

	// Marshal back to readable YAML
	out, err := yaml.Marshal(section)
	if err != nil {
		return ""
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		lines = append(lines, fmt.Sprintf("  %s", line))
	}
	return strings.Join(lines, "\n")
}
