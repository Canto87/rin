package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
)

// escapeAGE sanitizes a string for safe inclusion in Cypher queries.
func escapeAGE(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// parseAgtypeString strips double quotes from an agtype text representation.
func parseAgtypeString(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// EnsureGraphVertex creates a vertex in the AGE graph if it doesn't exist.
func (s *Store) EnsureGraphVertex(ctx context.Context, docID string) error {
	query := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('rin_memory', $$
			MERGE (n:doc {id: '%s'})
			RETURN n
		$$) AS (v ag_catalog.agtype)
	`, escapeAGE(docID))
	_, err := s.pool.Exec(ctx, query)
	return err
}

// AddGraphEdge creates a directed edge in the AGE graph.
// Uses rel_type as edge label (Cypher-native approach) for efficient label-based filtering.
func (s *Store) AddGraphEdge(ctx context.Context, fromID, toID, relType string) error {
	if err := s.EnsureGraphVertex(ctx, fromID); err != nil {
		return fmt.Errorf("ensuring from vertex: %w", err)
	}
	if err := s.EnsureGraphVertex(ctx, toID); err != nil {
		return fmt.Errorf("ensuring to vertex: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('rin_memory', $$
			MATCH (a:doc {id: '%s'}), (b:doc {id: '%s'})
			MERGE (a)-[r:%s]->(b)
			RETURN r
		$$) AS (r ag_catalog.agtype)
	`, escapeAGE(fromID), escapeAGE(toID), escapeAGE(relType))
	_, err := s.pool.Exec(ctx, query)
	return err
}

// GraphTraverse performs multi-hop traversal from a starting document.
func (s *Store) GraphTraverse(ctx context.Context, startID string, maxHops int, relTypes []string) ([]GraphNode, error) {
	if maxHops <= 0 || maxHops > 5 {
		maxHops = 3
	}

	// Build relationship pattern: [:type1|type2*1..N] or [*1..N]
	var relPattern string
	if len(relTypes) > 0 {
		escaped := make([]string, len(relTypes))
		for i, rt := range relTypes {
			escaped[i] = escapeAGE(rt)
		}
		relPattern = fmt.Sprintf("[:%s*1..%d]", strings.Join(escaped, "|"), maxHops)
	} else {
		relPattern = fmt.Sprintf("[*1..%d]", maxHops)
	}

	// Wrap cypher output in subquery to cast agtype → text for pgx compatibility
	query := fmt.Sprintf(`
		SELECT d::text, h::text FROM (
			SELECT * FROM ag_catalog.cypher('rin_memory', $$
				MATCH path = (start:doc {id: '%s'})-%s-(target:doc)
				RETURN DISTINCT target.id, length(path) as hops
				ORDER BY hops
			$$) AS (d ag_catalog.agtype, h ag_catalog.agtype)
		) sub
	`, escapeAGE(startID), relPattern)

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []GraphNode
	seen := make(map[string]bool)
	for rows.Next() {
		var docIDRaw, hopsRaw string
		if err := rows.Scan(&docIDRaw, &hopsRaw); err != nil {
			continue
		}
		docID := parseAgtypeString(docIDRaw)
		hops, _ := strconv.Atoi(hopsRaw)
		if docID == startID || seen[docID] {
			continue
		}
		seen[docID] = true
		nodes = append(nodes, GraphNode{DocID: docID, Hops: hops})
	}

	// Hydrate with document title/kind
	if len(nodes) > 0 {
		s.hydrateGraphNodes(ctx, nodes)
	}

	if nodes == nil {
		nodes = []GraphNode{}
	}
	return nodes, nil
}

// hydrateGraphNodes fills Title and Kind from the documents table.
func (s *Store) hydrateGraphNodes(ctx context.Context, nodes []GraphNode) {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.DocID
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "$" + itoa(i+1)
		args[i] = id
	}

	query := "SELECT id, title, kind FROM documents WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		log.Printf("warning: failed to hydrate graph nodes: %v", err)
		return
	}
	defer rows.Close()

	info := make(map[string][2]string)
	for rows.Next() {
		var id, title, kind string
		if err := rows.Scan(&id, &title, &kind); err != nil {
			continue
		}
		info[id] = [2]string{title, kind}
	}

	for i := range nodes {
		if v, ok := info[nodes[i].DocID]; ok {
			nodes[i].Title = v[0]
			nodes[i].Kind = v[1]
		}
	}
}
