package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerMemoryTools(server *mcp.Server, cfg *MemoryConfig, store *Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_store",
		Description: "Store a knowledge document in long-term memory.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MemoryStoreInput) (*mcp.CallToolResult, any, error) {
		docID, err := store.StoreDocument(ctx, input)
		if err != nil {
			return textResult(fmt.Sprintf("error storing document: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("Stored document %s: %s", docID, input.Title)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_search",
		Description: "Search memory by semantic similarity. Hybrid search (vector + FTS), RRF merge.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MemorySearchInput) (*mcp.CallToolResult, any, error) {
		// Set defaults
		if input.Limit == 0 {
			input.Limit = 5
		}
		if input.Detail == "" {
			input.Detail = "summary"
		}
		// Use config project if not specified
		if input.Project == nil && cfg.Project != "" {
			proj := cfg.Project
			input.Project = &proj
		}

		docs, err := store.HybridSearch(ctx, input.Query, input.Kind, input.Project, input.Limit)
		if err != nil {
			return textResult(fmt.Sprintf("error searching documents: %v", err)), nil, nil
		}
		return jsonResult(docs), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_lookup",
		Description: "Structured lookup by kind, tags, project, or specific doc_id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MemoryLookupInput) (*mcp.CallToolResult, any, error) {
		// Set defaults
		if input.Limit == 0 {
			input.Limit = 10
		}
		if input.Detail == "" {
			input.Detail = "summary"
		}
		// Use config project if not specified
		if input.Project == nil && cfg.Project != "" {
			proj := cfg.Project
			input.Project = &proj
		}

		docs, err := store.Lookup(ctx, input)
		if err != nil {
			return textResult(fmt.Sprintf("error looking up documents: %v", err)), nil, nil
		}
		return jsonResult(docs), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_update",
		Description: "Update an existing document. Content/title changes trigger re-embedding.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MemoryUpdateInput) (*mcp.CallToolResult, any, error) {
		ok, err := store.UpdateDocument(ctx, input)
		if err != nil {
			return textResult(fmt.Sprintf("error updating document: %v", err)), nil, nil
		}
		if !ok {
			return textResult(fmt.Sprintf("document not found: %s", input.DocID)), nil, nil
		}
		return textResult(fmt.Sprintf("Updated document %s", input.DocID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_relate",
		Description: "Create a directed relationship between two documents.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MemoryRelateInput) (*mcp.CallToolResult, any, error) {
		err := store.AddRelation(ctx, input)
		if err != nil {
			return textResult(fmt.Sprintf("error creating relation: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("Related %s -> %s (%s)", input.FromID, input.ToID, input.RelationType)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_ingest",
		Description: "Parse a markdown file into sections and store each as a document.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MemoryIngestInput) (*mcp.CallToolResult, any, error) {
		// Use config project if not specified
		if input.Project == nil && cfg.Project != "" {
			proj := cfg.Project
			input.Project = &proj
		}

		ids, err := store.IngestFile(ctx, input)
		if err != nil {
			return textResult(fmt.Sprintf("error ingesting file: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("Ingested %d sections from %s", len(ids), input.FilePath)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_graph_traverse",
		Description: "Traverse document relationships via multi-hop graph search. Returns connected documents within max_hops of the starting document.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GraphTraverseInput) (*mcp.CallToolResult, any, error) {
		nodes, err := store.GraphTraverse(ctx, input.StartID, input.MaxHops, input.RelTypes)
		if err != nil {
			return textResult(fmt.Sprintf("error traversing graph: %v", err)), nil, nil
		}
		if len(nodes) == 0 {
			return textResult(fmt.Sprintf("No connected documents found from %s", input.StartID)), nil, nil
		}
		return jsonResult(nodes), nil, nil
	})
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return textResult(fmt.Sprintf("error encoding JSON: %v", err))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}
