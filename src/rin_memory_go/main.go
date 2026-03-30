package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// Subcommand routing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "reindex":
			runReindex()
			return
		case "reembed":
			runReembed()
			return
		case "insert-log":
			runInsertLog()
			return
		case "recall":
			runRecall()
			return
		case "count":
			runCount()
			return
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := NewStore(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}
	defer store.Close()

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "rin-memory",
			Version: "2.0.0",
		},
		nil,
	)

	registerMemoryTools(server, cfg, store)
	registerRoutingTools(server, cfg, store)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// runReembed generates embeddings for all documents without vectors.
func runReembed() {
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

	// Find documents without embeddings
	rows, err := store.pool.Query(ctx, `
		SELECT d.id, d.title, d.tags, d.content
		FROM documents d
		LEFT JOIN document_vectors v ON d.id = v.doc_id
		WHERE v.doc_id IS NULL AND NOT d.archived
		ORDER BY d.created_at
	`)
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}

	type docInfo struct {
		id, title, content string
		tags               []string
	}
	var docs []docInfo
	for rows.Next() {
		var d docInfo
		if err := rows.Scan(&d.id, &d.title, &d.tags, &d.content); err != nil {
			continue
		}
		docs = append(docs, d)
	}
	rows.Close()

	total := len(docs)
	if total == 0 {
		log.Println("All documents already have embeddings.")
		return
	}

	log.Printf("Re-embedding %d documents...", total)

	ok, fail := 0, 0
	for i, d := range docs {
		chunks := ChunkDocument(d.title, d.tags, d.content)
		if len(chunks) == 0 {
			continue
		}

		texts := make([]string, len(chunks))
		for j, c := range chunks {
			texts[j] = c.Text
		}

		vectors, err := EmbedBatch(ctx, cfg.OllamaURL, texts)
		if err != nil {
			log.Printf("  [%d/%d] %s: embed failed: %v", i+1, total, d.id, err)
			fail++
			continue
		}

		if len(vectors) > 0 {
			if err := store.upsertVectors(ctx, d.id, chunks, vectors); err != nil {
				log.Printf("  [%d/%d] %s: upsert failed: %v", i+1, total, d.id, err)
				fail++
				continue
			}
		}

		ok++
		if ok%50 == 0 {
			log.Printf("  Progress: %d/%d", ok, total)
		}
	}

	log.Printf("Done: %d embedded, %d failed, %d total", ok, fail, total)
}

// runReindex rebuilds tsv (with tags) and re-embeds all documents.
// Use after schema changes that affect FTS indexing or embedding content.
func runReindex() {
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

	// Step 1: Refresh tsv for all documents (trigger includes tags now)
	log.Println("Step 1/2: Refreshing tsv for all documents...")
	res, err := store.pool.Exec(ctx, `
		UPDATE documents SET updated_at = NOW() WHERE NOT archived
	`)
	if err != nil {
		log.Fatalf("tsv refresh failed: %v", err)
	}
	log.Printf("  Updated %d documents' tsv", res.RowsAffected())

	// Step 2: Re-embed all documents
	log.Println("Step 2/2: Re-embedding all documents...")
	rows, err := store.pool.Query(ctx, `
		SELECT id, title, tags, content
		FROM documents
		WHERE NOT archived
		ORDER BY created_at
	`)
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}

	type docInfo struct {
		id, title, content string
		tags               []string
	}
	var docs []docInfo
	for rows.Next() {
		var d docInfo
		if err := rows.Scan(&d.id, &d.title, &d.tags, &d.content); err != nil {
			continue
		}
		docs = append(docs, d)
	}
	rows.Close()

	total := len(docs)
	if total == 0 {
		log.Println("No documents to re-embed.")
		return
	}

	log.Printf("  Re-embedding %d documents...", total)

	ok, fail := 0, 0
	for i, d := range docs {
		chunks := ChunkDocument(d.title, d.tags, d.content)
		if len(chunks) == 0 {
			continue
		}

		texts := make([]string, len(chunks))
		for j, c := range chunks {
			texts[j] = c.Text
		}

		vectors, err := EmbedBatch(ctx, cfg.OllamaURL, texts)
		if err != nil {
			log.Printf("  [%d/%d] %s: embed failed: %v", i+1, total, d.id, err)
			fail++
			continue
		}

		if len(vectors) > 0 {
			if err := store.upsertVectors(ctx, d.id, chunks, vectors); err != nil {
				log.Printf("  [%d/%d] %s: upsert failed: %v", i+1, total, d.id, err)
				fail++
				continue
			}
		}

		ok++
		if ok%50 == 0 {
			log.Printf("  Progress: %d/%d", ok, total)
		}
	}

	log.Printf("Done: %d re-embedded, %d failed, %d total", ok, fail, total)
}
