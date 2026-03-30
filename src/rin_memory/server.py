"""RIN Memory MCP Server."""

import os

from fastmcp import FastMCP

from .store import MemoryStore

mcp = FastMCP(
    "rin-memory",
    instructions=(
        "Long-term memory for RIN development agent. "
        "Use memory_search for semantic queries, memory_lookup for structured browsing."
    ),
)

_store: MemoryStore | None = None


async def get_store() -> MemoryStore:
    global _store
    if _store is None:
        db_dir = os.environ.get("RIN_MEMORY_DIR", os.path.expanduser("~/.rin"))
        os.makedirs(db_dir, exist_ok=True)
        _store = MemoryStore(
            db_path=os.path.join(db_dir, "memory.db"),
            lance_path=os.path.join(db_dir, "vectors"),
        )
        await _store.initialize()
    return _store


def _resolve_project(explicit: str | None = None) -> str | None:
    """Resolve project scope.

    Priority: explicit param > RIN_PROJECT env > None (no filter).
    Pass '*' to explicitly search all projects.
    """
    if explicit is not None:
        return None if explicit == "*" else explicit
    return os.environ.get("RIN_PROJECT")


@mcp.tool()
async def memory_store(
    kind: str,
    title: str,
    content: str,
    tags: list[str] | None = None,
    source: str | None = None,
    summary: str | None = None,
    project: str | None = None,
) -> str:
    """Store a knowledge document in long-term memory.

    Args:
        kind: Document type — session_journal, arch_decision, domain_knowledge,
              code_change, team_pattern, routing_log, active_task, error_pattern, preference
        title: Brief descriptive title
        content: Full content of the knowledge
        tags: Optional tags for structured lookup
        source: Origin reference (e.g. 'session:2026-02-21', 'commit:abc123')
        summary: Optional short summary for progressive disclosure
        project: Project scope. Defaults to RIN_PROJECT env. Use '*' for all projects.
    """
    store = await get_store()
    doc_id = await store.store_document(
        kind, title, content, tags, source, summary, project=_resolve_project(project)
    )
    return f"Stored document {doc_id}: {title}"


@mcp.tool()
async def memory_search(
    query: str,
    kind: str | None = None,
    detail: str = "summary",
    project: str | None = None,
    limit: int = 5,
) -> list[dict]:
    """Search memory by semantic similarity.

    Args:
        query: Natural language search query
        kind: Filter by document kind
        detail: Detail level — 'summary' (minimal tokens), 'detail' (+ source/relations), 'full' (+ content)
        project: Project scope. Defaults to RIN_PROJECT env. Use '*' for all projects.
        limit: Maximum results (default 5)
    """
    store = await get_store()
    return await store.search(query, kind, _resolve_project(project), limit, detail)


@mcp.tool()
async def memory_lookup(
    doc_id: str | None = None,
    kind: str | None = None,
    tags: list[str] | None = None,
    project: str | None = None,
    limit: int = 10,
    detail: str = "summary",
) -> dict | list[dict]:
    """Browse memory by structured filters, or fetch a single document by ID.

    Args:
        doc_id: Fetch a specific document by ID (ignores other filters)
        kind: Filter by document kind
        tags: Filter by tags (any match)
        project: Project scope. Defaults to RIN_PROJECT env. Use '*' for all projects.
        limit: Maximum results (default 10)
        detail: Detail level — 'summary', 'detail', 'full'
    """
    store = await get_store()
    if doc_id:
        doc = await store.get_by_id(doc_id, detail)
        return doc if doc else {"error": f"Document {doc_id} not found"}
    return await store.lookup(kind, tags, _resolve_project(project), limit, detail)


@mcp.tool()
async def memory_update(
    doc_id: str,
    content: str | None = None,
    title: str | None = None,
    tags: list[str] | None = None,
    archive: bool | None = None,
) -> str:
    """Update or archive an existing document.

    Args:
        doc_id: Document ID to update
        content: New content (triggers re-embedding)
        title: New title (triggers re-embedding)
        tags: New tags (replaces existing)
        archive: Set True to archive (soft delete)
    """
    store = await get_store()
    ok = await store.update_document(doc_id, content, title, tags, archive)
    return f"Updated document {doc_id}" if ok else f"Document {doc_id} not found"


@mcp.tool()
async def memory_relate(
    from_id: str,
    to_id: str,
    relation_type: str,
) -> str:
    """Create a relationship between two documents.

    Args:
        from_id: Source document ID
        to_id: Target document ID
        relation_type: Relationship type — supersedes, related, implements, contradicts
    """
    store = await get_store()
    ok = await store.add_relation(from_id, to_id, relation_type)
    return (
        f"Related {from_id} -> {to_id} ({relation_type})"
        if ok
        else "Failed to create relation"
    )


@mcp.tool()
async def memory_ingest(
    file_path: str,
    kind: str = "session_journal",
    source: str | None = None,
    project: str | None = None,
) -> str:
    """Ingest a markdown file into memory, splitting into sections.

    Args:
        file_path: Path to markdown file
        kind: Default document kind for sections
        source: Origin reference
        project: Project scope. Defaults to RIN_PROJECT env. Use '*' for all projects.
    """
    from .ingest import parse_markdown_sections

    store = await get_store()
    sections = parse_markdown_sections(file_path)
    resolved_project = _resolve_project(project)

    ids = []
    for section in sections:
        doc_id = await store.store_document(
            kind=section.get("kind", kind),
            title=section["title"],
            content=section["content"],
            tags=section.get("tags"),
            source=source or section.get("source"),
            project=resolved_project,
        )
        ids.append(doc_id)

    return f"Ingested {len(ids)} sections from {file_path}"


# ---------------------------------------------------------------------------
# Routing tools — experience-based model routing
# ---------------------------------------------------------------------------


@mcp.tool()
async def routing_suggest(
    task: str,
    file_count: int | None = None,
    has_dependencies: bool = False,
    needs_design: bool = False,
    project: str | None = None,
) -> dict:
    """Suggest routing for a task based on past experience.

    Analyzes the task description against past routing logs to recommend
    the best model, mode, and agent count.

    Args:
        task: Natural language description of the task
        file_count: Estimated number of files to modify
        has_dependencies: Whether the task has cross-file dependencies
        needs_design: Whether upfront design is needed
        project: Project scope. Defaults to RIN_PROJECT env. Use '*' for all projects.
    """
    from .routing import suggest

    store = await get_store()
    return await suggest(
        store, task, file_count, has_dependencies, needs_design,
        _resolve_project(project),
    )


@mcp.tool()
async def routing_log(
    task: str,
    model: str,
    duration_s: int,
    success: bool,
    level: str | None = None,
    mode: str = "solo",
    agent_count: int = 1,
    files_changed: int | None = None,
    files_list: list[str] | None = None,
    fallback_used: bool = False,
    fallback_from: str | None = None,
    error_type: str | None = None,
    project: str | None = None,
) -> str:
    """Log the result of a routing decision for future reference.

    Args:
        task: Brief description of the task performed
        model: Model used (glm-5, gpt-5.3-codex, sonnet-code-edit, etc.)
        duration_s: Time taken in seconds
        success: Whether the task completed successfully
        level: Task complexity level (L1/L2/L3). Auto-classified if omitted.
        mode: Execution mode — solo or team
        agent_count: Number of parallel agents used
        files_changed: Number of files modified
        files_list: List of file paths modified
        fallback_used: Whether a fallback model was needed
        fallback_from: Original model if fallback was used
        error_type: Error category if failed (timeout, quality, crash)
        project: Project scope. Defaults to RIN_PROJECT env. Use '*' for all projects.
    """
    from .routing import log_routing

    store = await get_store()
    doc_id = await log_routing(
        store, task, model, duration_s, success,
        level, mode, agent_count, files_changed, files_list,
        fallback_used, fallback_from, error_type,
        _resolve_project(project),
    )
    return f"Logged routing: {model} {'OK' if success else 'FAIL'} ({duration_s}s) — {doc_id}"


@mcp.tool()
async def routing_stats(
    model: str | None = None,
    level: str | None = None,
    days: int = 30,
    project: str | None = None,
) -> dict:
    """Get routing performance statistics.

    Args:
        model: Filter by specific model
        level: Filter by task level (L1/L2/L3)
        days: Look back period in days (default 30)
        project: Project scope. Defaults to RIN_PROJECT env. Use '*' for all projects.
    """
    from .routing import stats

    store = await get_store()
    return await stats(store, model, level, days, _resolve_project(project))


def main():
    """Start the MCP server via stdio transport."""
    mcp.run(transport="stdio")
