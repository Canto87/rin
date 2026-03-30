---
name: gc
description: >
  Garbage collection skill. Scans codebase for entropy (dead code, pattern drift,
  stale artifacts) and auto-fixes via code-edit delegation. Also manages memory
  entropy (stale/duplicate/drifted memories in rin-memory).
  Use for "gc", "garbage collection", "cleanup", "entropy scan", "memory cleanup" requests.
allowed-tools: Read, Bash, Glob, Grep, Agent
user_invocable: true
---

# Garbage Collection (Entropy Manager)

## Role
Scans existing codebase for accumulated entropy — dead code, pattern drift, duplication,
stale artifacts, dependency violations — and optionally auto-fixes via code-edit delegation.

Unlike code-review (diff-scoped, read-only) or validate (artifact-scoped, read-only),
GC targets the **entire codebase or module** and can **auto-fix**.

## Model Strategy
- **Scanning/collection**: Delegate to Agent(model="sonnet") per category — parallel
- **Triage/scoring/reporting**: This skill performs directly — Deep analysis
- **Fixes**: Delegate to Agent(subagent_type="general-purpose", model="sonnet") with code-edit.md instructions

## Input Parsing
Extract from user message:
- **mode**: `scan` (default), `fix`, or `memory`
- **options**:
  - `--module <path>` — Scope to specific directory (default: config scan.include paths)
  - `--category <cat>` — Run only specified category (default: all enabled)
  - `--dry-run` — In fix mode, output plan only without applying changes
  - `--kind <kind>` — Memory mode only: filter by memory kind (active_task, arch_decision, etc.)

Examples:
- `/gc` → scan mode, all categories, full scope
- `/gc fix` → fix mode, all categories
- `/gc fix --module src/rin_memory --category dead-code` → fix dead code in rin_memory only
- `/gc fix --dry-run` → show fix plan without applying
- `/gc memory` → memory consolidation (rin-memory dream)
- `/gc memory --kind active_task` → only clean up active_tasks

## Configuration
Read `config.yaml` in the same folder for project-specific settings:
```yaml
scan:
  include: ["src/"]      # Directories to scan
  exclude: ["vendor/"]   # Patterns to exclude
categories:
  enabled: [dead-code, pattern-drift, duplication, stale-artifacts, dependency-violation]
```

If `config.yaml` is absent, scan the project root excluding common non-source directories.

---

## Detection Categories

| ID | Category | What | Method | Severity |
|----|----------|------|--------|----------|
| GC-01 | Dead Code | Unused exports/functions, commented-out code, unused imports | LSP MCP (references) + Grep | Warning |
| GC-02 | Pattern Drift | Inconsistent patterns for same concern (error handling, logging, naming) | CLAUDE.md rules + sampling | Warning |
| GC-03 | Duplication | Similar logic repeated in 2+ locations | Structural comparison | Info |
| GC-04 | Stale Artifacts | TODO/FIXME/HACK comments, outdated comments, unused config files | Grep patterns | Info |
| GC-05 | Dependency Violation | Architecture layer violations, circular dependencies | LSP MCP + import analysis | Critical |
| GC-06 | Stale Memory | Drifted references, duplicates, unarchived completed tasks | rin-memory MCP + Glob/Grep | Warning |

### Category Details

#### GC-01: Dead Code
- Unused functions/methods: Use LSP MCP `find_references` or `go_symbol_references`; if no LSP, use Grep for call sites
- Commented-out code blocks: Grep for `^\s*(//|#)\s*(func|def|class|const|var|let|import)` patterns
- Unused imports: Language-specific LSP diagnostics or Grep

#### GC-02: Pattern Drift
- Read CLAUDE.md for declared patterns/conventions
- Sample 3-5 files per pattern category (error handling, logging, naming)
- Compare against declared golden patterns
- Flag deviations with before/after examples

#### GC-03: Duplication
- Identify structurally similar code blocks (5+ lines)
- Compare function signatures and bodies across files
- Report groups of duplicates with file locations

#### GC-04: Stale Artifacts
- Grep for `TODO|FIXME|HACK|XXX|DEPRECATED` with surrounding context
- Identify comments that reference removed code or outdated behavior
- Find config/data files not referenced by any source file

#### GC-05: Dependency Violation
- Parse import/require statements to build dependency graph
- Check against architecture rules in CLAUDE.md (if layer rules exist)
- Detect circular imports between packages/modules
- Flag cross-layer access violations

#### GC-06: Stale Memory
- **Drift**: memory content references file paths/functions that no longer exist in codebase
- **Duplicates**: multiple memories covering the same topic within the same kind
- **Unarchived completed tasks**: active_task with completion indicators still not archived
- **Contradictions**: arch_decisions or preferences that conflict with each other
- **Under-tagged**: memories with fewer than `memory.min_tags` tags (config.yaml)

---

## Execution Flow

### Memory Mode (`/gc memory`)

When mode is `memory`, skip all code scanning and operate on rin-memory instead.
Read `config.yaml` `memory` section for settings (prune_after_days, min_tags, drift_check).

If `--kind <kind>` is specified, limit all phases to that kind only.

#### Phase 1 — Orient
Run in parallel:
- `memory_lookup(kind='active_task')` — find unarchived tasks
- `memory_lookup(kind='arch_decision', limit=20)`
- `memory_lookup(kind='error_pattern', limit=20)`
- `memory_lookup(kind='preference', limit=20)`
- `memory_lookup(kind='domain_knowledge', limit=20)`

Note total counts and obvious issues.

#### Phase 2 — Drift Detection (if `memory.drift_check` is true)
For arch_decision, domain_knowledge, error_pattern memories:
1. Extract file paths from content
2. Glob to verify paths exist
3. Grep to verify function/symbol names exist
4. Add "STALE" tag to memories with broken references

Limit: 15 memories per run.

#### Phase 3 — Consolidate
1. Search for duplicate memories (similar titles within same kind)
2. Merge duplicates: update newer, archive older
3. Set `memory_relate(contradicts)` for conflicting decisions
4. Archive superseded memories that are still active

#### Phase 4 — Prune
1. Archive completed active_tasks
2. Archive session_summaries older than `memory.prune_after_days`
3. Archive STALE memories that are not critical
4. Enrich under-tagged memories (< `memory.min_tags` tags)

#### Output
Report in GC Report format (see below) with GC-06 findings, then summary:
```
Memory dream: oriented N | drift N stale | consolidated N merged | pruned N archived | tags N enriched
```

---

## Code Execution Flow

### Step 1: Context
Read in parallel:
- `config.yaml` → scan scope, enabled categories, exclude patterns
- Project `CLAUDE.md` → golden patterns, architecture rules, conventions

### Step 2: Scope
Apply scope resolution:
1. `--module <path>` overrides config `scan.include`
2. Expand `scan.include` paths via Glob
3. Filter out `scan.exclude` patterns
4. `--category <cat>` filters enabled categories

### Step 3: Scan (Parallel Agents)
For each enabled category, spawn a scanning agent:

```
Agent(
  subagent_type: "general-purpose",
  model: "sonnet",
  run_in_background: true,
  prompt: "Scan for {category} in {scope}. Rules from CLAUDE.md: {rules}.
           Use LSP MCP tools if available, otherwise Grep/Glob.
           Output findings as structured list:
           - file:line — description — severity — fixable(yes/no)"
)
```

Each category runs independently — all spawn in parallel.

> **LSP MCP preference**: If a language-specific MCP is available (gopls, Serena, etc.),
> instruct agents to use it for reference lookups and diagnostics. Otherwise fall back to Grep/Glob.

### Step 4: Triage (Direct)
Collect all agent results. This skill directly:
1. Deduplicate findings across categories
2. Validate findings (reject false positives where obvious)
3. Classify by severity: Critical > Warning > Info
4. Mark fixable items (can be auto-fixed without human judgment)
5. Sort by severity, then by file path

### Step 5a: Report (scan mode)
Output the GC Report (see format below).

### Step 5b: Fix (fix mode)
1. With `--dry-run`: Output fix plan only and stop
2. Group fixable items by file proximity
3. Create a team and delegate fixes:

```
TeamCreate(team_name="gc-fix")

Agent(
  subagent_type: "general-purpose",
  team_name: "gc-fix",
  name: "fixer-{N}",
  model: "sonnet",
  prompt: "Follow the code-edit agent instructions in .claude/agents/code-edit.md.
           Fix the following GC findings:\n{findings list}\n\n## Constraints\n- Only fix listed items\n- Do not refactor surrounding code\n--no-commit --scope {file|module}"
)
```

4. After fixes complete, commit:
```bash
git add <modified_files> && git commit -m "chore: gc - {summary of fixes}"
```

5. TeamDelete after completion
6. Output GC Report with Auto-Fix Summary appended

---

## Report Format

### Scan Report
```
## GC Report: {project/module}

| Category | Found | Fixable | Severity |
|----------|-------|---------|----------|
| Dead Code | N | N | Warning |
| Pattern Drift | N | N | Warning |
| Duplication | N | N | Info |
| Stale Artifacts | N | N | Info |
| Dependency Violation | N | N | Critical |
| **Total** | **N** | **N** | |

### Findings

#### GC-05: Dependency Violation (Critical)
- `file:line` — {description}

#### GC-01: Dead Code (Warning)
- `file:line` — {description}

#### GC-02: Pattern Drift (Warning)
- `file:line` — {description} — expected: {pattern} / actual: {pattern}

#### GC-03: Duplication (Info)
- `file_a:line` ↔ `file_b:line` — {description}

#### GC-04: Stale Artifacts (Info)
- `file:line` — {description}
```

### Fix Report (appended in fix mode)
```
### Auto-Fix Summary
| # | Category | File | Fix | Status |
|---|----------|------|-----|--------|
| 1 | GC-01 | path:line | remove unused func | Done |
| 2 | GC-04 | path:line | remove stale TODO | Done |
| 3 | GC-02 | path:line | align error pattern | Skipped (needs review) |

Commit: {hash}
```

---

## Automatic Triggers

GC scan is triggered automatically at these points:

| Trigger | Scope | Categories | Mode |
|---------|-------|------------|------|
| `/commit` (smart-commit) | Changed modules only | GC-01, GC-04 (lightweight) | scan |

Full scan (`/gc`) and fix mode (`/gc fix`) remain manual invocations.

## Important Rules
- **scan mode is read-only**: Never modify files in scan mode
- **fix mode delegates**: All code changes go through code-edit agent, never direct edits
- **LSP MCP first**: Prefer language-specific MCP for reference lookups; Grep/Glob as fallback
- **No over-cleanup**: Only fix clear entropy. Do not refactor working code
- **Respect exclude patterns**: Never scan or modify files matching exclude patterns
- **Teams lifecycle**: TeamCreate at fix Step 5b, TeamDelete after completion
- **Category independence**: Each category scans independently — no cross-category dependencies during scan
- **False positive caution**: In triage, reject findings that require human judgment to confirm
