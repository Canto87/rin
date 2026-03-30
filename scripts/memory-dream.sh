#!/bin/bash
# memory-dream: Memory cleanup/consolidation
# If session-review is extraction, this is pruning.
# Dream's 4 phases: Orient → Drift Detection → Consolidate → Prune

RIN_HOME="${RIN_HOME:-$(cd "$(dirname "$0")/.." && pwd)}"
RIN_CTX="$RIN_HOME/context/rin-context.md"

if [ ! -f "$RIN_CTX" ]; then
  echo "[ERROR] rin-context.md not found: $RIN_CTX"
  exit 1
fi

CLAUDE_BIN="${CLAUDE_BIN:-$(command -v claude 2>/dev/null || echo "$HOME/.local/bin/claude")}"

# Assemble prompt (temp file — avoid ARG_MAX)
TMPFILE=$(mktemp /tmp/rin-dream-XXXXXX)
trap 'rm -f "$TMPFILE"' EXIT

cat > "$TMPFILE" << 'INSTRUCTIONS'
You are performing a memory dream — a reflective consolidation pass over rin-memory.
Your job is to IMPROVE existing memories, not create new ones.
memory_store is intentionally not available to you.

Work through all 4 phases sequentially. Use tools in parallel within each phase where possible.

## Phase 1 — Orient

Get a full picture of current memory state. Run these lookups in parallel:
1. memory_lookup(kind='active_task') — find tasks that may be completed but not archived
2. memory_lookup(kind='arch_decision', limit=20) — recent architectural decisions
3. memory_lookup(kind='error_pattern', limit=20) — error patterns
4. memory_lookup(kind='preference', limit=20) — Operator preferences
5. memory_lookup(kind='domain_knowledge', limit=20) — domain knowledge
6. memory_lookup(kind='session_summary', limit=10) — oldest session summaries

Note the total count per kind and any obvious issues (duplicates in titles, stale active_tasks).

## Phase 2 — Drift Detection

For memories that reference specific file paths, function names, or config values,
verify they still exist in the codebase:

1. Extract file paths from memory content (patterns like `src/`, `scripts/`, `.claude/`, `~/.rin/`)
2. Use Glob to check if those paths exist
3. For function/symbol names, use Grep to verify they exist in the codebase
4. If a referenced path or symbol no longer exists, add tag "STALE" to that memory via memory_update

Focus on arch_decision, domain_knowledge, and error_pattern kinds — these are most likely to drift.
Skip session_summary (they are historical records, not references).

Limit: check at most 15 memories for drift per run to keep execution time reasonable.

## Phase 3 — Consolidate

Look for memories that should be merged or related:

1. **Duplicate detection**: Within each kind, look for memories with very similar titles or content.
   If found, memory_update the newer one to include all information, then archive the older one.
2. **Contradiction resolution**: If two arch_decisions or preferences contradict each other,
   use memory_relate(relation_type='contradicts') and archive the outdated one.
3. **Supersedes chains**: Check if any memory has a supersedes relation where the superseded
   doc is not yet archived. If so, archive it.

Use memory_search with targeted queries to find potential duplicates/contradictions.

## Phase 4 — Prune

1. **Completed active_tasks**: If an active_task's content indicates it's done
   (e.g., "completed", "merged", "deployed", status mentions completion),
   archive it via memory_update(doc_id=..., archive=True)
2. **Old session_summaries**: Archive session_summaries older than 90 days
   (check created_at field; today's date is provided below)
3. **Tag enrichment**: For memories with fewer than 3 tags, add relevant tags
   based on their content via memory_update(doc_id=..., tags=[...])

## Output

At the end, output a summary in this exact format:

DREAM_RESULT|oriented: {N} memories|drift: {N} stale|consolidated: {N} merged|pruned: {N} archived|tags: {N} enriched

If nothing changed: DREAM_RESULT|no changes needed — memory state is healthy
INSTRUCTIONS

# Append today's date
echo "" >> "$TMPFILE"
echo "Today's date: $(date +%Y-%m-%d)" >> "$TMPFILE"

SYSTEM_PROMPT="$(cat "$RIN_CTX")

You are RIN. Follow the identity and decision boundaries above.
This is an automated memory consolidation session. Be thorough but efficient."

echo "[$(date -u +%Y-%m-%dT%H:%M:%S)] Starting memory dream"

dream_output=$("$CLAUDE_BIN" -p "$(cat "$TMPFILE")" \
  --append-system-prompt "$SYSTEM_PROMPT" \
  --allowedTools "mcp__rin-memory__memory_search,mcp__rin-memory__memory_lookup,mcp__rin-memory__memory_update,mcp__rin-memory__memory_relate,Glob,Grep" \
  --permission-mode bypassPermissions \
  --no-session-persistence)

exit_code=$?

if [ $exit_code -eq 0 ]; then
  # Extract DREAM_RESULT line
  result_line=$(echo "$dream_output" | grep '^DREAM_RESULT|' | tail -1)
  if [ -n "$result_line" ]; then
    echo "[$(date -u +%Y-%m-%dT%H:%M:%S)] $result_line"
  else
    echo "[$(date -u +%Y-%m-%dT%H:%M:%S)] Dream complete (no result marker found)"
  fi
else
  echo "[$(date -u +%Y-%m-%dT%H:%M:%S)] Dream failed (claude exit $exit_code)"
fi
