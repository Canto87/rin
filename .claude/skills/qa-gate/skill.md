---
name: qa-gate
description: >
  Quality gate. Runs code-review + validate in parallel, evaluates combined scores,
  auto-fixes on retry, and returns concise verdict. Use after implementation is complete.
  Use for "qa-gate", "quality gate", "review and validate" requests.
allowed-tools: Read, Bash, Glob, Grep, Agent, AskUserQuestion
user_invocable: true
---

# Quality Gate

## Role
A **read-only orchestrator** that evaluates code quality by running review + validate in parallel,
making gate decisions, and auto-fixing issues on retry. Returns a concise verdict to the caller.

Does not directly modify code. Fix delegation goes through `code-edit` agent via Agent tool.

## Examples
- `/qa-gate user-auth phase3` → Review + validate Phase 3 of user-auth
- `/qa-gate user-auth phase3 --dry-run` → Show gate plan only

## Input Parsing
Extract from user message:
- **feature**: Feature name (e.g., `scratch`)
- **phase**: Phase number (e.g., `1`) or description
- **options**: `--dry-run`

## Execution Flow

### Step 1: Context Gathering
Read in parallel:
- Design docs and acceptance criteria (follow project structure from CLAUDE.md)
- Implementation checklist (if available)
- `git diff --name-only main` → Changed files

With `--dry-run`: Output gate plan only and stop.

### Step 2: Parallel Review + Validate (Teams)

Create a team and spawn review + validate as teammates:

**Setup:**
```
TeamCreate(team_name="qa-{feature}-{phaseN}")
```

**Review Teammate:**
```
Agent(
  subagent_type: "code-review",
  team_name: "qa-{feature}-{phaseN}",
  name: "reviewer",
  prompt: "Review {feature} phase{N}\n\n## Changed Files\n{git diff --name-only}\n\n## AC\n{AC items}"
)
```

**Validate Teammate:**
```
Agent(
  subagent_type: "validate",
  team_name: "qa-{feature}-{phaseN}",
  name: "validator",
  prompt: "Validate {feature} phase{N}\n\n## AC\n{AC items}"
)
```

Wait for results via SendMessage. Parse scores (X/10), Critical Issues, Warnings from both.

### Step 3: Gate Decision

| Condition | Decision | Action |
|-----------|----------|--------|
| Review 7+ AND Validate 7+ (no Critical) | **PASS** | Return verdict |
| Either 5-6 OR has Critical | **RETRY** | Go to Step 4 |
| Either 0-4 | **REJECT** | Return verdict |

### Step 4: Auto-Fix (RETRY only, max 2 retries)

1. Collect all feedback items from both review and validate
2. Delegate fix (reuse existing team):
```
Agent(
  subagent_type: "general-purpose",
  team_name: "qa-{feature}-{phaseN}",
  name: "fixer",
  model: "sonnet",
  prompt: "Follow the code-edit agent instructions in .claude/agents/code-edit.md.
           Apply review + validate feedback\n\n## Review Items\n{Critical + Warnings}\n\n## Validate Items\n{Failed AC + Issues}"
)
```
3. After fix, commit the modified files (from code-edit's Modified Files list):
```bash
git add <modified_files> && git commit -m "fix({feature}): phase{N} - qa-gate feedback #{retry_count}"
```
4. Return to Step 2

### Step 5: Return Verdict

Return **concise summary only** (keep main context lightweight):

```
## QA Gate: {feature} Phase {N} — {PASS/REJECT}

| Check | Score | Status |
|-------|-------|--------|
| Review | {N}/10 | {PASS/FAIL} |
| Validate | {N}/10 | {PASS/FAIL} |

Retries: {count}/2
Critical Issues: {count}
```

If REJECT, append:
```
### Manual Fix Required
1. {issue}: {location} — {description}
```

## Gate Thresholds

| Parameter | Value |
|-----------|-------|
| Review pass | >= 7 (no Critical) |
| Validate pass | >= 7 (no Critical) |
| Retry range | 5-6 or has Critical |
| Reject range | 0-4 |
| Max retries | 2 |

## Constraints

- **Read-only**: Never modify code directly
- **Delegate fixes**: All code changes go through `code-edit` teammate
- **Teams lifecycle**: TeamCreate at Step 2, TeamDelete after Step 5
- **Concise output**: Return verdict summary, not full review/validate reports
- **Bash is read-only**: Only `git` commands for status/diff/commit
