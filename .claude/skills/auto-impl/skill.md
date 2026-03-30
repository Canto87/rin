---
name: auto-impl
description: >
  Phase automation orchestrator. Reads design docs from the plans path defined in config.yaml,
  delegates tasks to code-edit agent, verifies results, and maintains checkpoints.
  Use for "auto implement", "auto-impl", "run phase" requests.
allowed-tools: Read, Write, Edit, Bash, Glob, Grep, Agent
user_invocable: true
---

# Phase Automation Orchestrator

## Role
An orchestrator that autonomously implements tasks from design documents.
Delegates each task to code-edit agent, verifies results, and maintains checkpoints.
Does not directly modify code—focuses on orchestration (delegation/verification/checkpoint/commit).

## Configuration

Read `config.yaml` in the same folder for project-specific paths:
```yaml
paths:
  plans: "docs/plans"   # Design docs location (default)
```

If `config.yaml` is absent, default to `docs/plans/`.

## Examples
- `/auto-impl user-auth 3` → Phase 3 of user-auth feature, sequential mode
- `/auto-impl user-auth all --parallel` → All phases, parallel mode
- `/auto-impl user-auth 2 --continue` → Resume Phase 2 from last checkpoint

## Input Parsing
Extract from user message:
- **feature**: Feature name (e.g., `user-auth`)
- **phase**: Phase number (e.g., `3`) or `all`
- **options**: `--dry-run`, `--no-commit`, `--continue`, `--parallel`

## Execution Loop (Gather → Delegate → Verify → Checkpoint → Report)

### Step 1: Context Gathering (Parallel Read)
Read the following files to collect context:
- `{config.paths.plans}/{feature}/` → Design docs (phases, tasks, acceptance criteria)
- `{config.paths.plans}/checklists/{feature}.md` → Existing checklist (if any, for `--continue`)
- `CLAUDE.md` → Project code style, patterns

> **LSP MCP Note:** If a language-specific MCP server is available (Serena, gopls, etc.), prefer it for token-efficient code exploration; otherwise use Read/Grep automatically.

### Step 1.5: Checklist Generation (Auto)

If `{config.paths.plans}/checklists/{feature}.md` does not exist, auto-generate it from design docs:

1. Parse all phase documents in `{config.paths.plans}/{feature}/`
2. Extract phase headers and numbered tasks
3. Generate checklist in the following format:

```markdown
# {Feature Name} Implementation Checklist

## Phase 1: {Phase Title}
- [ ] Task 1 description
- [ ] Task 2 description

## Phase 2: {Phase Title}
- [ ] Task 1 description
- [ ] Task 2 description
```

4. Write to `{config.paths.plans}/checklists/{feature}.md`
5. `git add` + `git commit "chore({feature}): generate implementation checklist"`

If checklist already exists (`--continue` mode), read it to determine completed tasks.

### Step 2: Task Extraction
- Read checklist to identify completed (`[x]`) and pending (`[ ]`) tasks
- Match checklist items to design doc sections for detailed context
- Skip tasks where items are already `[x]`
- With `--dry-run`: Output task list only and stop
- With `--dry-run --parallel`: Output task list + dependency graph and stop

### Step 2.5: Dependency Analysis (--parallel mode only)

See **`references/parallel-mode.md`** for full parallel execution details (dependency analysis, safety rules, checkpoint flow, error handling).

### Steps 3-7: Per-Task Loop
For each incomplete task:

**3. DELEGATE (code-edit delegation)**:

**Sequential mode (default):**
Create a team, then spawn code-edit as teammate:

```
TeamCreate(team_name="impl-{feature}-phase{N}")
```

For each task:
```
Agent(
  subagent_type: "general-purpose",
  team_name: "impl-{feature}-phase{N}",
  name: "editor-{taskN}",
  model: "sonnet",
  prompt: "Follow the code-edit agent instructions in .claude/agents/code-edit.md.
           task: {task title}\n\n## Target\n{files}\n\n## Constraints\n- Do not modify {plans_path}/\n\n## Options\n--no-commit --scope {file|module|cross-module}"
)
```

Scope auto-detect:
- Task mentions 1-3 files → `file`
- Task mentions 4-10 files or one package → `module`
- Task spans multiple packages → `cross-module`

**Parallel mode (`--parallel`):**
See **`references/parallel-mode.md`** for level-based parallel execution.

**4. VERIFY (Parse code-edit result)**:
Parse code-edit report to determine result:

| code-edit Result | Action |
|------------------|--------|
| `code-edit Complete` | SUCCESS → Step 5 |
| `code-edit FAILED` (build) | Re-delegate with additional context (1 retry) → STOP on re-failure |
| `code-edit FAILED` (scope) | Re-delegate with higher scope (1 retry) → STOP on re-failure |
| `code-edit WARN` (test) | Log warning, continue to Step 5 |
| Report parse failure | Skip task + recommend manual verification |

Phase-level verification: Run test commands defined in project CLAUDE.md (warn only on failure)

**5. CHECKPOINT**:
- Update checklist `[ ]` → `[x]` for completed task
- `git add`: code-edit's "Modified Files" list + checklist file
- `git commit` (unless `--no-commit`)

**6. REPORT**: Output progress

**7. STOP CHECK**:
- All tasks complete → SUCCESS report
- code-edit re-delegation failed → CRITICAL ERROR report
- 20 iteration limit exceeded → MAX TURNS report
- Otherwise → Continue to next task

## code-edit Delegation Protocol

### Message Template
```
task: {task title}

## Detailed Instructions
{Task details extracted from design document}

## Target
{Primary file/directory list}

## Constraints
- Do not modify {plans_path}/ files (including checklists)
- {additional constraints}

## Options
--no-commit
--scope {file|module|cross-module}
```

### Report Parsing Method
Extract from code-edit result:
- **Status**: `code-edit Complete` / `code-edit FAILED` / `code-edit WARN` header
- **Modified Files**: File path list under `### Modified Files` → Use for `git add`
- **Verification**: Build/Test results
- **Failure**: Failure cause (pass as context on re-delegation)

### Re-delegation Additional Context Format
```
context: |
  Previous attempt failure info:
  - Cause: {Failure section from code-edit report}
  - Error message: {build/test error excerpt}
  Fix direction: {hint based on error analysis}
```

## Stop Conditions (Priority)
1. **SUCCESS**: All tasks complete
2. **CRITICAL**: code-edit re-delegation failed
3. **MAX_TURNS**: 20 iteration limit exceeded
4. **DEPENDENCY**: Previous Phase incomplete dependency

## Error Handling
| Error | Severity | Action |
|-------|----------|--------|
| code-edit FAIL (build) | Critical | Re-delegate with context (1 retry) → STOP |
| code-edit FAIL (scope) | Warning | Re-delegate with higher scope (1 retry) → STOP |
| code-edit WARN (test) | Warning | Log, continue |
| Phase Test failure | Warning | Log, continue |
| Report parse failure | Critical | Skip task + recommend manual verification |

## Progress Report Format
Output after each task completion:
```
## Progress: {feature} Phase {N} -- Task {K}/{total}
- Task: {task title}
- Delegation: code-edit {SUCCESS/FAIL/WARN}
- Phase Test: PASS/FAIL/SKIP
- Commit: {hash}
- Checklist: {checked}/{total_items} items complete
```

## Completion Report Format
```
## auto-impl Complete: {feature} Phase {N}
- Tasks: {done}/{total} complete
- Delegations: {success}/{total} successful
- Mode: {sequential|parallel}
- Commits: {commit hash list}
- Recommended: Run validate {feature} phase{N}
```

## Git Commit Format
```
feat({feature}): phase{N} - {task summary}
```
For parallel mode commit format, see **`references/parallel-mode.md`**.

## Important Rules
- **Teams lifecycle**: TeamCreate at Step 3 start, TeamDelete after completion/failure
- Always pass `--no-commit` to code-edit
- Pass constraints preventing design doc and checklist modification to code-edit
- Parse code-edit's Modified Files for git add after each task
- Update checklist only after code-edit success
- With `--dry-run`: Output task list only, no actual delegation
- With `--no-commit`: Skip git commit
- With `--continue`: Resume from last stopped point
- With `--parallel`: Use dependency-based level parallelism
