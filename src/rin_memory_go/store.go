package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Store provides PostgreSQL operations for memory documents.
type Store struct {
	pool *pgxpool.Pool
	cfg  *MemoryConfig
}

// NewStore creates a new Store with connection pool and runs schema migration.
func NewStore(ctx context.Context, cfg *MemoryConfig) (*Store, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, err
	}

	// Load AGE extension and set search_path on each new connection.
	// public MUST be first so DDL/DML targets public schema, not ag_catalog.
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, _ = conn.Exec(ctx, "LOAD 'age'")
		_, _ = conn.Exec(ctx, `SET search_path = public, ag_catalog`)
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	// Run schema migration
	if err := runMigration(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	return &Store{pool: pool, cfg: cfg}, nil
}

// runMigration executes schema.sql statements on a single connection.
// Using a single connection ensures LOAD 'age' persists for subsequent AGE statements.
func runMigration(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	statements := strings.Split(schemaSQL, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		if strings.Contains(stmt, "create_graph") {
			_, err := conn.Exec(ctx, stmt)
			if err != nil && !isAlreadyExistsError(err) {
				log.Printf("warning: AGE graph creation skipped: %v", err)
				continue
			}
			continue
		}

		_, err := conn.Exec(ctx, stmt)
		if err != nil && !isAlreadyExistsError(err) {
			return err
		}
	}

	// Ensure tsv trigger exists (tags included in FTS)
	_, _ = conn.Exec(ctx, `
		CREATE OR REPLACE FUNCTION update_tsv() RETURNS trigger AS $fn$
		BEGIN
			NEW.tsv := to_tsvector('simple',
				coalesce(NEW.title, '') || ' ' ||
				coalesce(NEW.content, '') || ' ' ||
				coalesce(array_to_string(NEW.tags, ' '), ''));
			RETURN NEW;
		END;
		$fn$ LANGUAGE plpgsql`)
	_, _ = conn.Exec(ctx, `
		DROP TRIGGER IF EXISTS trg_update_tsv ON documents`)
	_, _ = conn.Exec(ctx, `
		CREATE TRIGGER trg_update_tsv
			BEFORE INSERT OR UPDATE ON documents
			FOR EACH ROW EXECUTE FUNCTION update_tsv()`)

	return nil
}

// isAlreadyExistsError checks if the error is "already exists" type.
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate")
}

// Close closes the connection pool.
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// generateID creates a random 12-character hex string.
func generateID() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// StoreDocument inserts a new document and returns its ID.
func (s *Store) StoreDocument(ctx context.Context, input MemoryStoreInput) (string, error) {
	docID, err := generateID()
	if err != nil {
		return "", err
	}

	project := input.Project
	if project == nil && s.cfg.Project != "" {
		proj := s.cfg.Project
		project = &proj
	}

	now := time.Now()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO documents (id, kind, title, content, summary, tags, source, created_at, archived, project)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, $9)
	`, docID, input.Kind, input.Title, input.Content, input.Summary, input.Tags, input.Source, now, project)

	if err != nil {
		return "", err
	}

	// Chunk and embed the document
	go s.embedDocumentAsync(docID, input.Title, input.Tags, input.Content)

	return docID, nil
}

// embedDocumentAsync handles embedding in the background.
func (s *Store) embedDocumentAsync(docID, title string, tags []string, content string) {
	ctx := context.Background()

	chunks := ChunkDocument(title, tags, content)
	if len(chunks) == 0 {
		return
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	vectors, err := EmbedBatch(ctx, s.cfg.OllamaURL, texts)
	if err != nil {
		logEmbedWarning(docID, err)
		return
	}

	if len(vectors) > 0 {
		if err := s.upsertVectors(ctx, docID, chunks, vectors); err != nil {
			log.Printf("warning: failed to store vectors for %s: %v", docID, err)
		}
	}
}

// GetByID retrieves a document by ID.
func (s *Store) GetByID(ctx context.Context, docID, detail string) (*Document, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, kind, title, content, summary, tags, source, created_at, updated_at, archived, project
		FROM documents
		WHERE id = $1 AND NOT archived
	`, docID)

	doc, err := scanDocument(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	formatted := formatDocument(doc, detail)
	return &formatted, nil
}

// Lookup retrieves documents based on filter criteria.
func (s *Store) Lookup(ctx context.Context, input MemoryLookupInput) ([]Document, error) {
	// If DocID is set, delegate to GetByID
	if input.DocID != nil {
		doc, err := s.GetByID(ctx, *input.DocID, input.Detail)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			return []Document{}, nil
		}
		return []Document{*doc}, nil
	}

	// Build dynamic query
	var conditions []string
	var args []any
	argIdx := 1

	// Kind filter
	if input.Kind != nil {
		conditions = append(conditions, "kind = $"+itoa(argIdx))
		args = append(args, *input.Kind)
		argIdx++
	}

	// Project scoping
	if input.Project != nil {
		conditions = append(conditions, "(project = $"+itoa(argIdx)+" OR project IS NULL)")
		args = append(args, *input.Project)
		argIdx++
	}

	// Tags filter (array overlap)
	if len(input.Tags) > 0 {
		conditions = append(conditions, "tags && $"+itoa(argIdx))
		args = append(args, input.Tags)
		argIdx++
	}

	// Exclude archived
	conditions = append(conditions, "NOT archived")

	// Default limit
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	query := "SELECT id, kind, title, content, summary, tags, source, created_at, updated_at, archived, project FROM documents"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT $" + itoa(argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		doc, err := scanDocumentFromRow(rows)
		if err != nil {
			return nil, err
		}
		formatted := formatDocument(&doc, input.Detail)
		docs = append(docs, formatted)
	}

	if docs == nil {
		docs = []Document{}
	}
	return docs, nil
}

// UpdateDocument updates an existing document.
func (s *Store) UpdateDocument(ctx context.Context, input MemoryUpdateInput) (bool, error) {
	// Check existence first and get current title/content/tags for re-embedding
	var currentTitle, currentContent string
	var currentTags []string
	err := s.pool.QueryRow(ctx, "SELECT title, content, tags FROM documents WHERE id = $1", input.DocID).Scan(&currentTitle, &currentContent, &currentTags)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	// Build dynamic SET clause
	var setClauses []string
	var args []any
	argIdx := 1

	needsReembed := false

	if input.Content != nil {
		setClauses = append(setClauses, "content = $"+itoa(argIdx))
		args = append(args, *input.Content)
		argIdx++
		currentContent = *input.Content
		needsReembed = true
	}
	if input.Title != nil {
		setClauses = append(setClauses, "title = $"+itoa(argIdx))
		args = append(args, *input.Title)
		argIdx++
		currentTitle = *input.Title
		needsReembed = true
	}
	if input.Tags != nil {
		setClauses = append(setClauses, "tags = $"+itoa(argIdx))
		args = append(args, input.Tags)
		argIdx++
		currentTags = input.Tags
		needsReembed = true
	}
	if input.Archive != nil {
		setClauses = append(setClauses, "archived = $"+itoa(argIdx))
		args = append(args, *input.Archive)
		argIdx++
	}

	if len(setClauses) == 0 {
		return true, nil // Nothing to update
	}

	// Always set updated_at
	setClauses = append(setClauses, "updated_at = $"+itoa(argIdx))
	args = append(args, time.Now())
	argIdx++

	// Add WHERE clause argument
	args = append(args, input.DocID)

	query := "UPDATE documents SET " + strings.Join(setClauses, ", ") + " WHERE id = $" + itoa(argIdx)
	_, err = s.pool.Exec(ctx, query, args...)
	if err != nil {
		return false, err
	}

	// Re-embed if content, title, or tags changed
	if needsReembed {
		go s.embedDocumentAsync(input.DocID, currentTitle, currentTags, currentContent)
	}

	return true, nil
}

// AddRelation creates or updates a relationship between two documents.
// Dual-writes to both the relations table and the AGE graph.
func (s *Store) AddRelation(ctx context.Context, input MemoryRelateInput) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relations (from_id, to_id, rel_type, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (from_id, to_id) DO UPDATE SET rel_type = EXCLUDED.rel_type
	`, input.FromID, input.ToID, input.RelationType)
	if err != nil {
		return err
	}

	// AGE graph dual-write (non-fatal)
	if graphErr := s.AddGraphEdge(ctx, input.FromID, input.ToID, input.RelationType); graphErr != nil {
		log.Printf("warning: AGE graph edge failed: %v", graphErr)
	}

	return nil
}

// RelationWithDoc includes related document info.
type RelationWithDoc struct {
	FromID    string    `json:"from_id"`
	ToID      string    `json:"to_id"`
	RelType   string    `json:"rel_type"`
	CreatedAt time.Time `json:"created_at"`
	// Related doc info
	RelatedID    *string `json:"related_id,omitempty"`
	RelatedTitle *string `json:"related_title,omitempty"`
	RelatedKind  *string `json:"related_kind,omitempty"`
}

// GetRelations retrieves all relations for a document.
func (s *Store) GetRelations(ctx context.Context, docID string) ([]RelationWithDoc, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.from_id, r.to_id, r.rel_type, r.created_at,
			CASE WHEN r.from_id = $1 THEN d_to.id ELSE d_from.id END as related_id,
			CASE WHEN r.from_id = $1 THEN d_to.title ELSE d_from.title END as related_title,
			CASE WHEN r.from_id = $1 THEN d_to.kind ELSE d_from.kind END as related_kind
		FROM relations r
		LEFT JOIN documents d_from ON r.from_id = d_from.id
		LEFT JOIN documents d_to ON r.to_id = d_to.id
		WHERE r.from_id = $1 OR r.to_id = $1
	`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []RelationWithDoc
	for rows.Next() {
		var r RelationWithDoc
		err := rows.Scan(
			&r.FromID, &r.ToID, &r.RelType, &r.CreatedAt,
			&r.RelatedID, &r.RelatedTitle, &r.RelatedKind,
		)
		if err != nil {
			return nil, err
		}
		rels = append(rels, r)
	}

	if rels == nil {
		rels = []RelationWithDoc{}
	}
	return rels, nil
}

// scanDocument scans a single row into Document.
func scanDocument(row pgx.Row) (*Document, error) {
	var doc Document
	err := row.Scan(
		&doc.ID, &doc.Kind, &doc.Title, &doc.Content,
		&doc.Summary, &doc.Tags, &doc.Source,
		&doc.CreatedAt, &doc.UpdatedAt, &doc.Archived, &doc.Project,
	)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// scanDocumentFromRow scans from pgx.Rows.
func scanDocumentFromRow(rows pgx.Rows) (Document, error) {
	var doc Document
	err := rows.Scan(
		&doc.ID, &doc.Kind, &doc.Title, &doc.Content,
		&doc.Summary, &doc.Tags, &doc.Source,
		&doc.CreatedAt, &doc.UpdatedAt, &doc.Archived, &doc.Project,
	)
	return doc, err
}

// formatDocument applies progressive disclosure based on detail level.
func formatDocument(doc *Document, detail string) Document {
	result := Document{
		ID:        doc.ID,
		Kind:      doc.Kind,
		Title:     doc.Title,
		Tags:      doc.Tags,
		CreatedAt: doc.CreatedAt,
		Project:   doc.Project,
	}

	switch detail {
	case "full":
		result.Content = doc.Content
		result.Summary = doc.Summary
		result.Source = doc.Source
		result.UpdatedAt = doc.UpdatedAt
	case "detail":
		result.Summary = doc.Summary
		result.Source = doc.Source
		result.UpdatedAt = doc.UpdatedAt
	// "summary" or default: only basic fields
	default:
		// Keep only basic fields already set
	}

	return result
}

// itoa converts int to string (simple helper to avoid strconv import).
func itoa(i int) string {
	if i < 0 {
		return "-" + itoa(-i)
	}
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// upsertVectors inserts or updates embeddings for a document.
func (s *Store) upsertVectors(ctx context.Context, docID string, chunks []Chunk, vectors [][]float32) error {
	// First, delete existing vectors for this document
	if err := s.deleteVectors(ctx, docID); err != nil {
		return err
	}

	// Insert new vectors
	for i, chunk := range chunks {
		if i >= len(vectors) {
			break
		}
		vec := vectors[i]
		if len(vec) == 0 {
			continue
		}

		vecLiteral := vectorToLiteral(vec)
		_, err := s.pool.Exec(ctx, `
			INSERT INTO document_vectors (doc_id, chunk_index, embedding)
			VALUES ($1, $2, $3::vector)
		`, docID, chunk.ChunkIndex, vecLiteral)
		if err != nil {
			return err
		}
	}

	return nil
}

// deleteVectors removes all embeddings for a document.
func (s *Store) deleteVectors(ctx context.Context, docID string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM document_vectors WHERE doc_id = $1", docID)
	return err
}
