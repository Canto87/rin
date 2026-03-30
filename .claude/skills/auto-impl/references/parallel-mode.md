# Auto-Impl Parallel Mode

Activated with `--parallel` flag. Analyzes file dependencies between tasks and executes independent tasks concurrently.

## Dependency Analysis (Step 2.5)

When `--parallel` is specified, analyze file dependencies between tasks:

1. **Extract file targets** from each task
2. **Build dependency graph**: Task B depends on Task A if B reads/imports files that A creates/modifies
3. **Assign levels**: Tasks with no dependencies = Level 0, tasks depending on Level 0 = Level 1, etc.
4. **Output dependency graph** (for `--dry-run --parallel`):
```
Level 0 (parallel): Task 1, Task 3, Task 5
Level 1 (parallel): Task 2, Task 4
Level 2 (sequential): Task 6
```

## Parallel Execution

For each level in the dependency graph:
- Spawn all tasks in the level as teammates simultaneously
- Wait for all tasks in current level to complete (via SendMessage) before next level

```
# Level 0: Spawn all independent tasks as teammates
Agent(
  subagent_type: "general-purpose",
  team_name: "impl-{feature}-phase{N}",
  name: "editor-task1",
  model: "sonnet",
  prompt: "..."
)
Agent(
  subagent_type: "general-purpose",
  team_name: "impl-{feature}-phase{N}",
  name: "editor-task3",
  model: "sonnet",
  prompt: "..."
)

# Wait for results via SendMessage, then proceed to Level 1
...
```

## Safety Rules

| Rule | Description |
|------|-------------|
| **No shared file mutation** | Tasks in the same level MUST NOT modify the same file |
| **Level boundary = sync point** | All tasks in a level must complete before next level starts |
| **Failure isolation** | If one task in a level fails, wait for others to complete, then stop |
| **Checkpoint after level** | Commit once per level, not per task |
| **Max concurrent** | Max 3 background agents per level |

## Checkpoint Flow

```
Level 0: Task 1, Task 3, Task 5 (parallel, run_in_background: true)
  → Wait all complete
  → Verify all results
  → Update checklist [ ] → [x]
  → git add (all modified files from all tasks + checklist)
  → git commit "feat({feature}): phase{N} - level 0 tasks"

Level 1: Task 2, Task 4 (parallel, run_in_background: true)
  → Wait all complete
  → Verify all results
  → Update checklist [ ] → [x]
  → git add (all modified files + checklist)
  → git commit "feat({feature}): phase{N} - level 1 tasks"
```

## Error Handling

| Scenario | Action |
|----------|--------|
| **One task fails, others succeed** | Keep successful task changes, update checklist for successful tasks only |
| **Partial commit** | Commit successful tasks' files + checklist. Failed task remains `[ ]` |
| **`--continue` resume** | Re-run only failed tasks from the level (successful tasks already committed) |
| **All tasks fail** | No commit for the level, `--continue` re-runs entire level |
| **Post-level file conflict** | Run `git diff --name-only` after level; if same file modified by 2+ tasks, abort level and fall back to sequential |

**Generated file handling**: Run the project's generate command (if any) once after all tasks in a level complete, not within individual tasks.

## Git Commit Format (Parallel)

```
feat({feature}): phase{N} - level {L} tasks
```
