package main

import "time"

// Document represents a memory document stored in PostgreSQL.
type Document struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Content   string     `json:"content,omitempty"`
	Summary   *string    `json:"summary,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Source    *string     `json:"source,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Archived  bool       `json:"archived"`
	Project   *string    `json:"project,omitempty"`
	Relevance float64    `json:"relevance,omitempty"`
}

// Relation represents a directed relationship between two documents.
type Relation struct {
	FromID    string    `json:"from_id"`
	ToID      string    `json:"to_id"`
	RelType   string    `json:"rel_type"`
	CreatedAt time.Time `json:"created_at"`
}

// SearchResult wraps a document with relevance score.
type SearchResult struct {
	Document
	Relevance float64 `json:"relevance"`
}

// -- Tool input types --

type MemoryStoreInput struct {
	Kind    string   `json:"kind"    jsonschema:"Document type: session_journal, arch_decision, domain_knowledge, code_change, team_pattern, routing_log, active_task, error_pattern, preference"`
	Title   string   `json:"title"   jsonschema:"Brief descriptive title"`
	Content string   `json:"content" jsonschema:"Full content of the knowledge"`
	Tags    []string `json:"tags,omitempty"    jsonschema:"Optional tags for structured lookup"`
	Source  *string  `json:"source,omitempty"  jsonschema:"Origin reference (e.g. session:2026-02-21)"`
	Summary *string  `json:"summary,omitempty" jsonschema:"Optional short summary for progressive disclosure"`
	Project *string  `json:"project,omitempty" jsonschema:"Project scope. Use '*' for all projects"`
}

type MemorySearchInput struct {
	Query   string  `json:"query"              jsonschema:"Natural language search query"`
	Kind    *string `json:"kind,omitempty"      jsonschema:"Filter by document type"`
	Detail  string  `json:"detail,omitempty"    jsonschema:"Detail level: summary, detail, full"`
	Project *string `json:"project,omitempty"   jsonschema:"Project scope"`
	Limit   int     `json:"limit,omitempty"     jsonschema:"Maximum number of results"`
}

type MemoryLookupInput struct {
	DocID   *string  `json:"doc_id,omitempty"   jsonschema:"Specific document ID to retrieve"`
	Kind    *string  `json:"kind,omitempty"      jsonschema:"Filter by document type"`
	Tags    []string `json:"tags,omitempty"      jsonschema:"Filter by tags (any match)"`
	Project *string  `json:"project,omitempty"   jsonschema:"Project scope"`
	Limit   int      `json:"limit,omitempty"     jsonschema:"Maximum number of results"`
	Detail  string   `json:"detail,omitempty"    jsonschema:"Detail level: summary, detail, full"`
}

type MemoryUpdateInput struct {
	DocID   string   `json:"doc_id"             jsonschema:"Document ID to update"`
	Content *string  `json:"content,omitempty"  jsonschema:"New content (triggers re-embedding)"`
	Title   *string  `json:"title,omitempty"    jsonschema:"New title (triggers re-embedding)"`
	Tags    []string `json:"tags,omitempty"     jsonschema:"New tags (replaces existing)"`
	Archive *bool    `json:"archive,omitempty"  jsonschema:"Set to true for soft delete"`
}

type MemoryRelateInput struct {
	FromID       string `json:"from_id"        jsonschema:"Source document ID"`
	ToID         string `json:"to_id"          jsonschema:"Target document ID"`
	RelationType string `json:"relation_type"  jsonschema:"Relation type: supersedes, related, implements, contradicts"`
}

type MemoryIngestInput struct {
	FilePath string  `json:"file_path"          jsonschema:"Path to markdown file"`
	Kind     string  `json:"kind,omitempty"     jsonschema:"Default document type for sections"`
	Source   *string `json:"source,omitempty"   jsonschema:"Origin reference"`
	Project  *string `json:"project,omitempty"  jsonschema:"Project scope"`
}

type GraphTraverseInput struct {
	StartID  string   `json:"start_id"            jsonschema:"Starting document ID for traversal"`
	MaxHops  int      `json:"max_hops,omitempty"   jsonschema:"Maximum traversal depth (1-5, default 3)"`
	RelTypes []string `json:"rel_types,omitempty"  jsonschema:"Filter by relation types (e.g. supersedes, related)"`
}

// GraphNode represents a connected document found by graph traversal.
type GraphNode struct {
	DocID string `json:"doc_id"`
	Hops  int    `json:"hops"`
	Title string `json:"title,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

type RoutingSuggestInput struct {
	Task            string  `json:"task"                       jsonschema:"Natural language task description"`
	FileCount       *int    `json:"file_count,omitempty"       jsonschema:"Expected number of files to modify"`
	HasDependencies bool    `json:"has_dependencies,omitempty" jsonschema:"Whether files have inter-dependencies"`
	NeedsDesign     bool    `json:"needs_design,omitempty"     jsonschema:"Whether upfront design is needed"`
	Project         *string `json:"project,omitempty"          jsonschema:"Project scope"`
}

type RoutingLogInput struct {
	Task         string   `json:"task"                       jsonschema:"Brief task description"`
	Model        string   `json:"model"                      jsonschema:"Model used (gemini-pro, gemini-flash, claude-opus-4-6, claude-sonnet-4-6, etc.)"`
	DurationS    int      `json:"duration_s"                 jsonschema:"Duration in seconds"`
	Success      bool     `json:"success"                    jsonschema:"Whether the task succeeded"`
	Level        *string  `json:"level,omitempty"            jsonschema:"Task complexity (L1/L2/L3)"`
	Mode         string   `json:"mode,omitempty"             jsonschema:"Execution mode: solo or team"`
	AgentCount   int      `json:"agent_count,omitempty"      jsonschema:"Number of parallel agents"`
	FilesChanged *int     `json:"files_changed,omitempty"    jsonschema:"Number of files modified"`
	FilesList    []string `json:"files_list,omitempty"       jsonschema:"List of modified file paths"`
	FallbackUsed bool    `json:"fallback_used,omitempty"    jsonschema:"Whether fallback model was used"`
	FallbackFrom *string  `json:"fallback_from,omitempty"   jsonschema:"Original model if fallback was used"`
	ErrorType    *string  `json:"error_type,omitempty"       jsonschema:"Error category: timeout, quality, crash"`
	Project      *string  `json:"project,omitempty"          jsonschema:"Project scope"`
}

type RoutingStatsInput struct {
	Model   *string `json:"model,omitempty"   jsonschema:"Filter by specific model"`
	Level   *string `json:"level,omitempty"   jsonschema:"Filter by task level (L1/L2/L3)"`
	Days    int     `json:"days,omitempty"    jsonschema:"Lookback period in days"`
	Project *string `json:"project,omitempty" jsonschema:"Project scope"`
}
