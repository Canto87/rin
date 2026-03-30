---
name: auto-research
description: >
  Autonomous experiment loop. Repeatedly hypothesizes, modifies code, measures metrics,
  and keeps or discards changes. Inspired by karpathy/autoresearch, generalized beyond ML.
  Use for "auto-research", "autonomous experiment", "overnight optimize" requests.
allowed-tools: Read, Write, Edit, Bash, Glob, Grep, Agent
user_invocable: true
---

# Autonomous Research Loop

## Role
An orchestrator that runs an autonomous experiment loop on a given codebase.
Repeatedly: hypothesize a code change, delegate implementation to code-edit,
measure against a fixed metric, and keep or discard the result.
Does not directly modify target code — delegates all edits to code-edit agent.

## Configuration
Read `config.yaml` in the same folder for project-specific defaults.

## Input Parsing
Extract from user message:
- **name**: Experiment name (e.g., `proxy-perf`) — used for branch and results file
- **metric**: Shell command that outputs a measurable number (required)
- **scope**: File or directory to optimize (required)
- **options**: `--direction lower|higher`, `--budget N` (min/experiment), `--max N`, `--unattended`, `--continue`, `--program PATH`, `--dry-run`

## Examples
```
/auto-research proxy-perf --metric "go test -bench=BenchmarkStream -benchtime=3s ./src/rin_proxy/ 2>&1 | grep 'ns/op' | awk '{print \$3}'" --scope src/rin_proxy/streaming.go --direction lower --max 50
```

---

## Execution

### Phase 0: Setup

1. **Validate inputs**:
   - Metric command runs without error and outputs a parseable number (integer or float as the last numeric token in stdout)
   - Scope files/directory exist
   - If `--continue`: skip to Phase 0.5

2. **Create experiment branch**:
   ```bash
   git checkout -b autoresearch/{name}
   ```

3. **Run baseline**:
   ```bash
   {metric_command} 2>&1
   ```
   Parse the last number from stdout as the baseline metric value.

4. **Initialize results file** (`{name}-results.tsv` in repo root) with columns `experiment commit metric lines_delta status description`, baseline row `0 {hash7} {baseline} 0 keep baseline`; append filename to `.gitignore` if absent.

5. **Confirm**: report branch, scope, metric command, direction, baseline value, budget, and max count as a brief summary. With `--dry-run`: stop here.

### Phase 0.5: Resume (--continue)

Checkout `autoresearch/{name}`, read `{name}-results.tsv` to recover best metric/commit, total count, and tried descriptions. Report resume state and enter loop.

### Phase 1: Experiment Loop

```
experiment_num = last_experiment + 1
best_metric = last keep's metric value
best_commit = last keep's commit hash

LOOP while experiment_num <= max:
```

#### Step 1: HYPOTHESIZE

Read to generate the next experiment idea: scope files (use Agent(Explore) if large), `{name}-results.tsv` history, `--program` directives if provided, and the metric output.

Generate a hypothesis: what to change and why, expected metric impact, and a 1-line description for results.tsv.

**Hypothesis quality rules**: After 3+ consecutive discards, go more radical. After a keep, explore variations. Never repeat a discarded idea (check results.tsv). Prefer simplifying changes; consider combining previous keeps.

**Next-hypothesis priority**: (a) fixes for crashed experiments, (b) variations of the most recent keep, (c) novel approaches not yet tried.

#### Step 2: IMPLEMENT

Delegate to code-edit agent:

```
Agent(
  subagent_type: "code-edit",
  model: "sonnet",
  prompt: "Follow the code-edit agent instructions in .claude/agents/code-edit.md.

task: {hypothesis description}

## Target
{scope files}

## Constraints
- No test/benchmark/metric files
- Focused, minimal changes only

## Context
Experiment #{experiment_num} for auto-research '{name}'.
{last 5 entries from results.tsv}

## Options
--no-commit --scope file"
)
```

If code-edit returns FAILED: log as `crash` in results.tsv, run `git reset --hard {best_commit}`, continue to next iteration.

#### Step 3: COMMIT

```bash
git add {modified files from code-edit report}
git commit -m "experiment({name}): {description}"
git rev-parse --short HEAD
```

Record the output as `commit7` for results.tsv.

#### Step 4: MEASURE

Run with timeout:
```bash
timeout {budget * 60}s bash -c '{metric_command}' 2>&1
```

Extract the last number (integer or float) from stdout as the metric value.

Error handling:
- Non-zero exit: read last 30 lines, attempt 1 fix via code-edit, re-measure; if fix also fails, log as `crash` and revert to best_commit.
- Timeout exceeded or no parseable number: log as `crash`, revert.

#### Step 5: DECIDE

Compare `current_metric` to `best_metric` (respecting `--direction`):

```
lines_delta = (lines in modified files after) - (lines before)
improved = (direction == "lower" AND current < best) OR
           (direction == "higher" AND current > best)
```

| Condition | Decision | Action |
|-----------|----------|--------|
| Improved | **KEEP** | `best_metric = current`, `best_commit = current_hash` |
| Equal AND `lines_delta < 0` | **KEEP** (simplicity win) | Same as above |
| < 0.5% gain AND `lines_delta > 20` | **DISCARD** (complexity) | `git reset --hard {best_commit}` |
| Equal or worse | **DISCARD** | `git reset --hard {best_commit}` |

#### Step 6: LOG

Append to `{name}-results.tsv`:
```
{experiment_num}	{commit7}	{metric_value}	{lines_delta}	{keep|discard|crash}	{description}
```

#### Step 7: REPORT

Output one-line progress:
```
#{N}: {description} -> {metric} [{status}] (best: {best_metric}, kept: {kept_count}/{N})
```

#### Step 8: CONTEXT CHECK

**Every 10 experiments**: summarize history to trim context, then output a one-paragraph milestone report (best metric vs baseline, kept ratio, last 5 summary).

**If not `--unattended`**: ask `{N} experiments complete. Best: {best_metric} ({delta_pct}% improvement). Continue?`

**Context survival**: If approaching context limit, save state to memory (`memory_store`, kind='active_task') — name, branch, scope, metric, direction, best metric/commit, total count, recent hypothesis directions. Recover via `--continue` + memory recall.

```
END LOOP
```

### Phase 2: Completion

When loop ends (max reached, manual stop, or context limit):

1. **Final report**:
   ```
   ## auto-research Complete: {name}
   Experiments: {total} | Kept: {kept} ({pct}%) | Crashed: {crashed}
   Baseline: {baseline_metric} → Best: {best_metric} ({delta_pct}% improvement)
   Branch: autoresearch/{name}

   ### Top Improvements
   (top 5 keeps: commit, metric, delta, description)

   ### Recommendation
   git diff main...autoresearch/{name}  →  git merge autoresearch/{name}
   ```

2. **Memory log**: Store completion summary (`memory_store`, kind='session_journal')

---

## Important Rules

- **Metric is sacred**: Never modify metric evaluation, benchmarks, or test files
- **Git discipline**: Always commit before measuring (enables `git reset --hard`); all experiments on dedicated branch, never touch main
- **results.tsv is the source of truth**: Everything can be reconstructed from it
- **Simplicity criterion applies**: Small gains with added complexity are not worth it; prefer simplifying changes
- **Delegate all edits**: Use code-edit agent — never modify target files directly; no dependency changes without explicit `--program` permission
- **Crash tolerance + context conservation**: A crash is not a stop — log, revert, continue; keep main context lean by routing heavy reads to subagents
- **Hard stop**: On git conflicts or unrecoverable errors (e.g., corrupt working tree), stop the loop immediately, save state to memory (`memory_store`, kind='active_task'), and report the stopping reason
