<!-- Managed by project-rin harness. Source: .claude/skills/dream/skill.md -->
---
name: dream
description: >
  Memory consolidation skill. Reflective pass over memory files — orient on existing
  memories, detect drift, merge duplicates, prune stale entries, and update the index.
  Use for "dream", "memory consolidation", "memory cleanup", "consolidate memories" requests.
  Can also be registered as a Desktop scheduled task for daily automatic runs.
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
user_invocable: true
---

# Dream (Memory Consolidation)

## Role
Performs a reflective consolidation pass over memory files. Synthesizes recent signal
into durable, well-organized memories so that future sessions can orient quickly.

This skill operates on **Claude Code file memory** (`memory/` directory in the project's
Claude config). If rin-memory MCP tools are available, it also consolidates rin-memory.

Unlike GC (code entropy), Dream targets **memory entropy** — stale facts, duplicates,
contradictions, under-indexed entries, and drifted references.

## Input Parsing
Extract from user message:
- **scope**: `all` (default) or `file-only` or `mcp-only`
- **options**:
  - `--dry-run` — Report what would change without modifying anything

Examples:
- `/dream` → full consolidation (file memory + rin-memory if available)
- `/dream --dry-run` → report only
- `/dream file-only` → only file memory
- `/dream mcp-only` → only rin-memory MCP

## Configuration
Read `config.yaml` in the same folder for project-specific settings:
```yaml
prune_after_days: 90
min_tags: 3
drift_check: true
index_max_lines: 200
index_max_kb: 25
```

If `config.yaml` is absent, use defaults shown above.

---

## Execution Flow

### Phase 1 — Orient

Get a full picture of current memory state.

#### File Memory
1. `ls` the memory directory to see what exists
2. Read `MEMORY.md` (the index) to understand current structure
3. Skim existing topic files — read first ~20 lines of each to understand scope
4. Note: total file count, index line count, any obvious issues

#### rin-memory MCP (if available)
Run in parallel:
- `memory_lookup(kind='active_task')` — unarchived tasks
- `memory_lookup(kind='arch_decision', limit=20)`
- `memory_lookup(kind='error_pattern', limit=20)`
- `memory_lookup(kind='preference', limit=20)`
- `memory_lookup(kind='domain_knowledge', limit=20)`

### Phase 2 — Drift Detection

Verify that memories still reference things that exist.

#### File Memory
For each memory file that mentions file paths or function names:
1. Extract path references (patterns: `src/`, `scripts/`, `.claude/`, config files)
2. Glob to verify paths exist
3. Grep to verify function/symbol names exist
4. Note drifted references for Phase 3

#### rin-memory MCP (if available)
Same logic using memory content. Add "STALE" tag via `memory_update` for broken references.

Limit: 15 memories per run to keep execution time reasonable.

### Phase 3 — Consolidate

#### File Memory
1. **Merge near-duplicates**: If two files cover the same topic, merge content into one
   file and delete the other. Update `MEMORY.md` index accordingly.
2. **Fix contradictions**: If two files disagree on a fact, keep the newer/correct version
   and remove the wrong one.
3. **Convert relative dates**: "yesterday", "last week" → absolute dates (e.g., 2026-03-25)
   so memories remain interpretable after time passes.
4. **Update stale content**: If drift detection found broken references, either:
   - Update the reference to the current path/name if it was renamed
   - Add a note that the referenced item was removed
   - Remove the memory if it's no longer relevant

#### rin-memory MCP (if available)
1. Duplicate memories → `memory_update` to merge, archive the older one
2. Contradicting decisions → `memory_relate(contradicts)`, archive outdated one
3. Unarchived superseded docs → archive via `memory_update(archive=True)`

### Phase 4 — Prune and Index

#### File Memory
1. **Prune**: Delete memory files that are completely stale, wrong, or superseded
2. **Index cleanup** (`MEMORY.md`):
   - Remove pointers to deleted/stale memories
   - Shorten verbose entries (>200 chars → move detail to topic file)
   - Add pointers to newly important memories
   - Keep under `index_max_lines` lines and `index_max_kb` KB
3. **Frontmatter check**: Ensure each memory file has valid frontmatter
   (`name`, `description`, `type`). Fix if missing.

#### rin-memory MCP (if available)
1. Archive completed `active_task` entries
2. Archive `session_summary` entries older than `prune_after_days`
3. Enrich under-tagged memories (fewer than `min_tags` tags)

---

## Output

Report what changed:

```
## Dream Report

### File Memory
- Oriented: {N} files, {N} index lines
- Drift: {N} stale references found
- Consolidated: {N} files merged, {N} contradictions resolved
- Pruned: {N} files removed, {N} index entries updated

### rin-memory (if applicable)
- Oriented: {N} memories across {N} kinds
- Drift: {N} stale references tagged
- Consolidated: {N} merged, {N} contradictions resolved
- Pruned: {N} archived, {N} tags enriched

### Changes
- [list each specific change made]
```

If nothing changed: "Memory state is healthy — no changes needed."

With `--dry-run`: prefix each change with `[WOULD]` and do not modify any files.

---

## Desktop Scheduled Task Setup

To register as a daily automatic task in Claude Code Desktop:

1. Open Desktop → Schedule → New task → New local task
2. Name: `dream`
3. Prompt: `/dream`
4. Frequency: Daily, 9:00 AM (or preferred time)
5. Permission mode: Auto accept edits
6. Working folder: your project directory

Or ask Claude in any Desktop session:
"Set up a daily scheduled task that runs /dream every morning at 9am"

The task file is stored at `~/.claude/scheduled-tasks/dream/SKILL.md`.

---

## Important Rules
- **Read before write**: Always read a memory file before modifying it
- **No new memories**: Dream consolidates existing memories, it does not create new ones.
  If you discover something worth remembering, note it in the report — the user or a
  future session will decide whether to save it.
- **Preserve intent**: When merging or editing memories, preserve the original meaning.
  If unsure whether a fact is still correct, keep it and add a [NEEDS VERIFICATION] note.
- **Index is an index**: `MEMORY.md` contains only pointers. Never write memory content
  directly into it.
- **Dry-run is read-only**: With `--dry-run`, absolutely no modifications.
- **MCP is optional**: If rin-memory MCP tools are not available, skip all MCP phases
  silently. Do not error or warn about missing MCP.
