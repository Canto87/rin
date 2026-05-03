package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

// runDedupe clusters documents of the requested kinds by embedding cosine
// similarity, keeps the most recent doc in each cluster, archives the rest,
// and records `supersedes` relations from the kept doc to every archived doc.
//
// Per-doc vector = average of all chunk embeddings (pgvector vector_avg).
//
// Defaults are dry-run; pass --apply to commit.
func runDedupe() {
	fs := flag.NewFlagSet("dedupe", flag.ExitOnError)
	kindsFlag := fs.String("kinds", "arch_decision,domain_knowledge,error_pattern,team_pattern", "comma-separated kinds to dedupe")
	threshold := fs.Float64("threshold", 0.95, "cosine similarity threshold (0..1)")
	minAge := fs.Duration("min-age", 6*time.Hour, "skip docs younger than this")
	excludeFlag := fs.String("exclude", "", "comma-separated doc IDs; clusters containing any of these IDs are skipped entirely")
	apply := fs.Bool("apply", false, "commit changes (default dry-run)")
	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	kinds := splitTrim(*kindsFlag)
	if len(kinds) == 0 {
		log.Fatal("--kinds required")
	}
	excludeSet := map[string]bool{}
	for _, id := range splitTrim(*excludeFlag) {
		excludeSet[id] = true
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	ctx := context.Background()
	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}
	defer store.Close()

	cutoff := time.Now().Add(-*minAge)

	// Compute per-doc vector once via CTE (avg of chunk embeddings) and
	// emit candidate pairs whose cosine ≥ threshold.
	rows, err := store.pool.Query(ctx, `
		WITH eligible AS (
			SELECT id, kind FROM documents
			WHERE kind = ANY($1) AND NOT archived AND created_at < $2
		),
		doc_vec AS (
			SELECT dv.doc_id, avg(dv.embedding) AS vec
			FROM document_vectors dv
			JOIN eligible e ON e.id = dv.doc_id
			GROUP BY dv.doc_id
		)
		SELECT a.doc_id, b.doc_id, 1 - (a.vec <=> b.vec) AS sim
		FROM doc_vec a
		JOIN doc_vec b ON a.doc_id < b.doc_id
		JOIN eligible ea ON ea.id = a.doc_id
		JOIN eligible eb ON eb.id = b.doc_id AND eb.kind = ea.kind
		WHERE 1 - (a.vec <=> b.vec) >= $3
		ORDER BY sim DESC
	`, kinds, cutoff, *threshold)
	if err != nil {
		log.Fatalf("query pairs: %v", err)
	}

	type pair struct {
		A, B string
		Sim  float64
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.A, &p.B, &p.Sim); err != nil {
			rows.Close()
			log.Fatalf("scan pair: %v", err)
		}
		pairs = append(pairs, p)
	}
	rows.Close()

	fmt.Printf("kinds=%v threshold=%.3f min-age=%s cutoff=%s\n",
		kinds, *threshold, minAge.String(), cutoff.Format(time.RFC3339))
	fmt.Printf("found %d candidate pairs\n", len(pairs))
	if len(pairs) == 0 {
		return
	}

	// Union-Find over pair endpoints.
	parent := map[string]string{}
	var find func(x string) string
	find = func(x string) string {
		if _, ok := parent[x]; !ok {
			parent[x] = x
			return x
		}
		if parent[x] == x {
			return x
		}
		root := find(parent[x])
		parent[x] = root
		return root
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for _, p := range pairs {
		union(p.A, p.B)
	}

	// Collect cluster members.
	involved := map[string]bool{}
	for _, p := range pairs {
		involved[p.A] = true
		involved[p.B] = true
	}
	clusters := map[string][]string{}
	for id := range involved {
		root := find(id)
		clusters[root] = append(clusters[root], id)
	}

	// Fetch metadata for involved docs.
	idList := make([]string, 0, len(involved))
	for id := range involved {
		idList = append(idList, id)
	}
	type docMeta struct {
		ID, Kind, Title string
		CreatedAt       time.Time
	}
	docMap := map[string]docMeta{}
	mrows, err := store.pool.Query(ctx, `
		SELECT id, kind, title, created_at FROM documents WHERE id = ANY($1)
	`, idList)
	if err != nil {
		log.Fatalf("query docs: %v", err)
	}
	for mrows.Next() {
		var d docMeta
		if err := mrows.Scan(&d.ID, &d.Kind, &d.Title, &d.CreatedAt); err != nil {
			mrows.Close()
			log.Fatalf("scan doc: %v", err)
		}
		docMap[d.ID] = d
	}
	mrows.Close()

	// Build plan: keep most recent, archive rest.
	type plan struct {
		Keep    string
		Archive []string
	}
	plans := make([]plan, 0, len(clusters))
	skippedClusters := 0
	totalArchive := 0
	for _, ids := range clusters {
		sort.Slice(ids, func(i, j int) bool {
			return docMap[ids[i]].CreatedAt.After(docMap[ids[j]].CreatedAt)
		})
		if anyExcluded(ids, excludeSet) {
			skippedClusters++
			continue
		}
		plans = append(plans, plan{Keep: ids[0], Archive: ids[1:]})
		totalArchive += len(ids) - 1
	}
	if skippedClusters > 0 {
		fmt.Printf("skipped %d cluster(s) due to -exclude\n", skippedClusters)
	}
	sort.Slice(plans, func(i, j int) bool {
		return len(plans[i].Archive) > len(plans[j].Archive)
	})

	fmt.Printf("\n=== %d clusters, %d archive candidates ===\n", len(plans), totalArchive)
	for _, p := range plans {
		k := docMap[p.Keep]
		fmt.Printf("[%s] keep %s %q (%s)\n",
			k.Kind, p.Keep, truncTitle(k.Title, 70), k.CreatedAt.Format("2006-01-02"))
		for _, a := range p.Archive {
			d := docMap[a]
			fmt.Printf("    archive %s %q (%s)\n",
				a, truncTitle(d.Title, 70), d.CreatedAt.Format("2006-01-02"))
		}
	}

	if !*apply {
		fmt.Println("\n(dry-run; pass --apply to commit)")
		return
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		log.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	relCount := 0
	for _, p := range plans {
		for _, a := range p.Archive {
			if _, err := tx.Exec(ctx,
				`UPDATE documents SET archived = true, updated_at = NOW() WHERE id = $1`, a); err != nil {
				log.Fatalf("archive %s: %v", a, err)
			}
			// keep --supersedes--> archived  (kept doc supersedes the older one)
			if _, err := tx.Exec(ctx, `
				INSERT INTO relations (from_id, to_id, rel_type, created_at)
				VALUES ($1, $2, 'supersedes', NOW())
				ON CONFLICT (from_id, to_id) DO NOTHING
			`, p.Keep, a); err != nil {
				log.Fatalf("relate %s->%s: %v", p.Keep, a, err)
			}
			relCount++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit: %v", err)
	}
	fmt.Printf("\ncommitted: %d archived, %d supersedes relations\n", totalArchive, relCount)
}

func anyExcluded(ids []string, exclude map[string]bool) bool {
	for _, id := range ids {
		if exclude[id] {
			return true
		}
	}
	return false
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func truncTitle(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
