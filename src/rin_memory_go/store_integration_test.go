//go:build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// These tests require a running PostgreSQL instance with pgvector and AGE.
// Run with: go test -tags=integration -run TestIntegration
//
// Set RIN_MEMORY_DSN or have ~/.rin/memory-config.json configured.

func setupTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()

	cfg, err := LoadConfig()
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}

	// Override DSN from env if set
	if dsn := os.Getenv("RIN_MEMORY_DSN"); dsn != "" {
		cfg.DSN = dsn
	}

	// Disable Ollama for CRUD tests (no embedding needed)
	cfg.OllamaURL = ""

	store, err := NewStore(ctx, cfg)
	if err != nil {
		t.Skipf("skipping integration test (cannot connect): %v", err)
	}

	t.Cleanup(func() { store.Close() })
	return store, ctx
}

func TestIntegrationStoreDocument(t *testing.T) {
	store, ctx := setupTestStore(t)

	input := MemoryStoreInput{
		Kind:    "domain_knowledge",
		Title:   "Integration Test Doc " + time.Now().Format("15:04:05"),
		Content: "This is a test document for integration testing.",
		Tags:    []string{"test", "integration"},
		Project: stringPtr("test-project"),
	}

	docID, err := store.StoreDocument(ctx, input)
	if err != nil {
		t.Fatalf("StoreDocument failed: %v", err)
	}
	if docID == "" {
		t.Fatal("StoreDocument returned empty ID")
	}

	// Cleanup
	t.Cleanup(func() {
		store.pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", docID)
	})

	// Verify via GetByID
	doc, err := store.GetByID(ctx, docID, "full")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if doc == nil {
		t.Fatal("GetByID returned nil")
	}
	if doc.Title != input.Title {
		t.Errorf("title = %q, want %q", doc.Title, input.Title)
	}
	if doc.Kind != input.Kind {
		t.Errorf("kind = %q, want %q", doc.Kind, input.Kind)
	}
	if doc.Content != input.Content {
		t.Errorf("content = %q, want %q", doc.Content, input.Content)
	}
}

func TestIntegrationLookup(t *testing.T) {
	store, ctx := setupTestStore(t)

	// Store test documents
	tag := "lookup-test-" + time.Now().Format("150405")
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		input := MemoryStoreInput{
			Kind:    "error_pattern",
			Title:   "Lookup Test " + string(rune('A'+i)),
			Content: "Content " + string(rune('A'+i)),
			Tags:    []string{tag},
			Project: stringPtr("test-project"),
		}
		id, err := store.StoreDocument(ctx, input)
		if err != nil {
			t.Fatalf("StoreDocument failed: %v", err)
		}
		ids[i] = id
	}

	t.Cleanup(func() {
		for _, id := range ids {
			store.pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", id)
		}
	})

	// Lookup by kind
	lookupInput := MemoryLookupInput{
		Kind:    stringPtr("error_pattern"),
		Tags:    []string{tag},
		Project: stringPtr("test-project"),
		Limit:   intPtr(10),
	}

	docs, err := store.Lookup(ctx, lookupInput)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if len(docs) < 3 {
		t.Errorf("expected at least 3 docs, got %d", len(docs))
	}
}

func TestIntegrationUpdateDocument(t *testing.T) {
	store, ctx := setupTestStore(t)

	// Create
	input := MemoryStoreInput{
		Kind:    "active_task",
		Title:   "Update Test",
		Content: "Original content",
		Tags:    []string{"test"},
	}
	docID, err := store.StoreDocument(ctx, input)
	if err != nil {
		t.Fatalf("StoreDocument failed: %v", err)
	}
	t.Cleanup(func() {
		store.pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", docID)
	})

	// Update
	newContent := "Updated content"
	newTitle := "Updated Title"
	updated, err := store.UpdateDocument(ctx, MemoryUpdateInput{
		DocID:   docID,
		Content: &newContent,
		Title:   &newTitle,
	})
	if err != nil {
		t.Fatalf("UpdateDocument failed: %v", err)
	}
	if !updated {
		t.Error("UpdateDocument returned false")
	}

	// Verify
	doc, err := store.GetByID(ctx, docID, "full")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if doc.Title != newTitle {
		t.Errorf("title = %q, want %q", doc.Title, newTitle)
	}
	if doc.Content != newContent {
		t.Errorf("content = %q, want %q", doc.Content, newContent)
	}
}

func TestIntegrationArchiveDocument(t *testing.T) {
	store, ctx := setupTestStore(t)

	input := MemoryStoreInput{
		Kind:    "preference",
		Title:   "Archive Test",
		Content: "Will be archived",
	}
	docID, err := store.StoreDocument(ctx, input)
	if err != nil {
		t.Fatalf("StoreDocument failed: %v", err)
	}
	t.Cleanup(func() {
		store.pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", docID)
	})

	// Archive
	archived := true
	_, err = store.UpdateDocument(ctx, MemoryUpdateInput{
		DocID:   docID,
		Archive: &archived,
	})
	if err != nil {
		t.Fatalf("UpdateDocument (archive) failed: %v", err)
	}

	// GetByID should still return it
	doc, err := store.GetByID(ctx, docID, "summary")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if doc == nil {
		t.Fatal("archived doc should still be retrievable by ID")
	}
	if !doc.Archived {
		t.Error("doc should be archived")
	}
}

func TestIntegrationRelations(t *testing.T) {
	store, ctx := setupTestStore(t)

	// Create two documents
	id1, err := store.StoreDocument(ctx, MemoryStoreInput{
		Kind: "arch_decision", Title: "Decision A", Content: "Old decision",
	})
	if err != nil {
		t.Fatalf("StoreDocument 1 failed: %v", err)
	}
	id2, err := store.StoreDocument(ctx, MemoryStoreInput{
		Kind: "arch_decision", Title: "Decision B", Content: "New decision supersedes A",
	})
	if err != nil {
		t.Fatalf("StoreDocument 2 failed: %v", err)
	}
	t.Cleanup(func() {
		store.pool.Exec(ctx, "DELETE FROM relations WHERE from_id = $1 OR to_id = $1", id1)
		store.pool.Exec(ctx, "DELETE FROM relations WHERE from_id = $1 OR to_id = $1", id2)
		store.pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", id1)
		store.pool.Exec(ctx, "DELETE FROM documents WHERE id = $1", id2)
	})

	// Add relation
	err = store.AddRelation(ctx, MemoryRelateInput{
		FromID:       id2,
		ToID:         id1,
		RelationType: "supersedes",
	})
	if err != nil {
		t.Fatalf("AddRelation failed: %v", err)
	}

	// Verify relation exists
	rels, err := store.GetRelations(ctx, id2)
	if err != nil {
		t.Fatalf("GetRelations failed: %v", err)
	}
	if len(rels) == 0 {
		t.Error("expected at least 1 relation")
	}

	found := false
	for _, r := range rels {
		if r.ToID == id1 && r.RelType == "supersedes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("supersedes relation not found in %+v", rels)
	}
}

func TestIntegrationGetByIDNotFound(t *testing.T) {
	store, ctx := setupTestStore(t)

	doc, err := store.GetByID(ctx, "nonexistent-id-12345", "full")
	if err != nil {
		t.Fatalf("GetByID should not error for missing doc: %v", err)
	}
	if doc != nil {
		t.Error("expected nil for nonexistent doc")
	}
}

// Helpers

func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }
