package main

import (
	"context"
	"strings"
)

const RRFK = 60 // RRF smoothing constant

// HybridSearch performs combined vector + text search.
func (s *Store) HybridSearch(ctx context.Context, query string, kind *string, project *string, limit int) ([]Document, error) {
	if limit <= 0 {
		limit = 5
	}

	// Run vector and FTS searches in parallel
	vectorResults := make(map[string]int)
	ftsResults := make(map[string]int)
	var vectorErr, ftsErr error

	// Vector search
	vectorResults, vectorErr = s.vectorSearch(ctx, query, limit*3)
	if vectorErr != nil {
		// Log but continue with FTS-only
		vectorResults = make(map[string]int)
	}

	// FTS search
	ftsResults, ftsErr = s.ftsSearch(ctx, query, kind, project, limit*3)
	if ftsErr != nil {
		ftsResults = make(map[string]int)
	}

	// If both failed, return empty
	if len(vectorResults) == 0 && len(ftsResults) == 0 {
		return []Document{}, nil
	}

	// RRF merge
	merged := rrfMerge(vectorResults, ftsResults, limit*2)

	// Hydrate documents with scores
	docs, err := s.hydrateDocs(ctx, merged, kind, project, limit)
	if err != nil {
		return nil, err
	}

	return docs, nil
}

// vectorSearch performs pgvector similarity search.
func (s *Store) vectorSearch(ctx context.Context, query string, limit int) (map[string]int, error) {
	// Embed the query
	embedding, err := Embed(ctx, s.cfg.OllamaURL, query)
	if err != nil || embedding == nil {
		return nil, err
	}

	// Build vector literal for query
	vecLiteral := vectorToLiteral(embedding)

	// Query: find documents with minimum distance
	querySQL := `
		SELECT doc_id
		FROM document_vectors
		WHERE embedding <=> $1::vector < 1.0
		GROUP BY doc_id
		ORDER BY MIN(embedding <=> $1::vector)
		LIMIT $2
	`

	rows, err := s.pool.Query(ctx, querySQL, vecLiteral, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]int)
	rank := 1
	for rows.Next() {
		var docID string
		if err := rows.Scan(&docID); err != nil {
			continue
		}
		results[docID] = rank
		rank++
	}

	return results, nil
}

// ftsSearch performs PostgreSQL full-text search with trigram fallback.
func (s *Store) ftsSearch(ctx context.Context, query string, kind *string, project *string, limit int) (map[string]int, error) {
	// Tokenize query
	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	// Separate long and short tokens
	var longTokens, shortTokens []string
	for _, t := range tokens {
		if len(t) >= 3 {
			longTokens = append(longTokens, t)
		} else {
			shortTokens = append(shortTokens, t)
		}
	}

	// Build query conditions
	var conditions []string
	var args []any
	argIdx := 1

	// Long tokens: use FTS
	if len(longTokens) > 0 {
		tsQuery := strings.Join(longTokens, " | ")
		conditions = append(conditions, "tsv @@ to_tsquery('simple', $"+itoa(argIdx)+")")
		args = append(args, tsQuery)
		argIdx++
	}

	// Short tokens: use LIKE
	for _, t := range shortTokens {
		likePattern := "%" + t + "%"
		conditions = append(conditions, "(title LIKE $"+itoa(argIdx)+" OR content LIKE $"+itoa(argIdx)+")")
		args = append(args, likePattern)
		argIdx++
	}

	// Combine conditions with OR (any match is good)
	whereClause := "(" + strings.Join(conditions, " OR ") + ")"

	// Add kind and project filters
	if kind != nil {
		whereClause += " AND kind = $" + itoa(argIdx)
		args = append(args, *kind)
		argIdx++
	}

	if project != nil {
		whereClause += " AND (project = $" + itoa(argIdx) + " OR project IS NULL)"
		args = append(args, *project)
		argIdx++
	}

	whereClause += " AND NOT archived"

	// Build final query with ranking
	var querySQL string
	if len(longTokens) > 0 {
		// ts_rank gives a float score; ORDER BY DESC so higher score = better rank
		// $1 is the tsquery parameter (always first arg when longTokens exist)
		querySQL = `
			SELECT id, ts_rank(tsv, to_tsquery('simple', $1)) as ts_score
			FROM documents
			WHERE ` + whereClause + `
			ORDER BY ts_score DESC, created_at DESC
			LIMIT $` + itoa(argIdx)
	} else {
		// Short-token LIKE fallback: no relevance scoring available
		querySQL = `
			SELECT id, 0.0::float4 as ts_score
			FROM documents
			WHERE ` + whereClause + `
			ORDER BY created_at DESC
			LIMIT $` + itoa(argIdx)
	}

	args = append(args, limit)

	rows, err := s.pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]int)
	rank := 1
	for rows.Next() {
		var docID string
		var tsScore float32
		if err := rows.Scan(&docID, &tsScore); err != nil {
			continue
		}
		results[docID] = rank
		rank++
	}

	return results, nil
}

type scoredDoc struct {
	id    string
	score float64
}

// rrfMerge combines vector and FTS results using Reciprocal Rank Fusion.
func rrfMerge(vectorResults, ftsResults map[string]int, limit int) []scoredDoc {
	// Collect all unique doc IDs
	allIDs := make(map[string]bool)
	for id := range vectorResults {
		allIDs[id] = true
	}
	for id := range ftsResults {
		allIDs[id] = true
	}

	scored := make([]scoredDoc, 0, len(allIDs))
	for id := range allIDs {
		vecRank := vectorResults[id]
		ftsRank := ftsResults[id]

		score := 0.0
		if vecRank > 0 {
			score += 1.0 / float64(RRFK+vecRank)
		}
		if ftsRank > 0 {
			score += 1.0 / float64(RRFK+ftsRank)
		}

		scored = append(scored, scoredDoc{id: id, score: score})
	}

	// Sort by score descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	if len(scored) > limit {
		scored = scored[:limit]
	}

	return scored
}

// hydrateDocs fetches full documents by IDs and applies formatting with scores.
func (s *Store) hydrateDocs(ctx context.Context, scored []scoredDoc, kind *string, project *string, limit int) ([]Document, error) {
	if len(scored) == 0 {
		return []Document{}, nil
	}

	// Build score map
	scoreMap := make(map[string]float64, len(scored))
	docIDs := make([]string, len(scored))
	for i, s := range scored {
		docIDs[i] = s.id
		scoreMap[s.id] = s.score
	}

	// Build IN clause
	placeholders := make([]string, len(docIDs))
	args := make([]any, len(docIDs))
	for i, id := range docIDs {
		placeholders[i] = "$" + itoa(i+1)
		args[i] = id
	}

	query := `
		SELECT id, kind, title, content, summary, tags, source, created_at, updated_at, archived, project
		FROM documents
		WHERE id IN (` + strings.Join(placeholders, ",") + `) AND NOT archived
	`

	// Add optional filters
	argIdx := len(docIDs) + 1
	if kind != nil {
		query += " AND kind = $" + itoa(argIdx)
		args = append(args, *kind)
		argIdx++
	}
	if project != nil {
		query += " AND (project = $" + itoa(argIdx) + " OR project IS NULL)"
		args = append(args, *project)
		argIdx++
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Preserve order using a map
	docMap := make(map[string]Document)
	for rows.Next() {
		doc, err := scanDocumentFromRow(rows)
		if err != nil {
			continue
		}
		formatted := formatDocument(&doc, "summary")
		formatted.Relevance = scoreMap[doc.ID]
		docMap[doc.ID] = formatted
	}

	// Build result in original order
	result := make([]Document, 0, limit)
	for _, id := range docIDs {
		if doc, ok := docMap[id]; ok {
			result = append(result, doc)
			if len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// tokenizeQuery splits a query into tokens for FTS.
func tokenizeQuery(query string) []string {
	// Split by whitespace
	words := strings.Fields(query)

	// Filter tokens >= 2 chars
	var tokens []string
	for _, w := range words {
		// Remove punctuation
		w = strings.Trim(w, ".,!?;:\"'()[]{}")
		w = strings.ToLower(w)
		if len(w) >= 2 {
			tokens = append(tokens, w)
		}
	}

	return tokens
}
