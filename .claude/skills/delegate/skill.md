<!-- Managed by project-rin harness. Source: .claude/skills/delegate/skill.md -->
---
name: delegate
description: >
  Delegate code implementation to external AI CLI (GLM, Codex, Gemini).
  Bypasses MCP A2A overhead by invoking CLI directly via Bash.
  Use for "delegate", "delegate implementation" requests.
allowed-tools: Read, Bash, Glob, Grep, TaskCreate, TaskOutput
user_invocable: true
---

# Delegate Skill

Delegate code implementation tasks to external AI CLIs, bypassing MCP A2A protocol overhead.

## Configuration

Read `config.yaml` in the same folder for defaults:
```yaml
default_model: glm       # glm | codex | gemini
```

If `config.yaml` is absent, default to `glm` model.

## Input Parsing

Extract from user message:
- **model**: `glm` (default), `codex`, or `gemini`
- **prompt**: The task description to delegate

Examples:
```
/delegate Fix the auth middleware to handle expired tokens
/delegate model=codex Refactor the database connection pool
/delegate model=gemini Add unit tests for the parser module
```

## CLI Mapping

| Model | Command | Notes |
|-------|---------|-------|
| `glm` | `glm -p "{prompt}" --yolo --output-format json` | Z.AI GLM-5, Claude Code wrapper. Filters `CLAUDECODE` env to allow nested execution |
| `codex` | `codex exec "{prompt}"` | OpenAI Codex, sandboxed by default |
| `gemini` | `gemini -p "{prompt}" --yolo` | Google Gemini CLI |

## Execution

### Step 1: Prepare Prompt

Construct the delegation prompt by combining:
1. The user's task description
2. Key constraints from the project (read CLAUDE.md for code style/patterns)
3. Current working directory context

Prompt template:
```
## Task
{user's task description}

## Constraints
- Follow existing code patterns and conventions
- Do not modify files outside the task scope
- Run tests if available after making changes
```

### Step 2: Execute CLI

Run via Bash with `run_in_background: true`:

```bash
{cli_command}
```

Where `{cli_command}` is determined by the model mapping above.

**Important**: Use `run_in_background: true` to avoid blocking. The CLI process runs asynchronously and you will be notified when it completes.

### Step 3: Report Result

When the background task completes:
1. Read the output via `TaskOutput`
2. Summarize what was done (files modified, tests run)
3. Report success or failure to the user

## Prompt Escaping

The delegation prompt is passed as a CLI argument. To avoid shell escaping issues,
write the prompt to a temp file and pass it via shell substitution:

```bash
# Write prompt to temp file first (Write tool)
# Then execute:
glm -p "$(cat /tmp/delegate-prompt.txt)" --yolo --output-format json
codex exec "$(cat /tmp/delegate-prompt.txt)"
gemini -p "$(cat /tmp/delegate-prompt.txt)" --yolo
```

**Always use the temp file + `$(cat ...)` approach** — avoids quoting issues with complex prompts.
`glm` does NOT support stdin pipe (`-p -`); it requires the prompt as a string argument.

## Error Handling

| Error | Action |
|-------|--------|
| CLI not found | Report which CLI is missing, suggest install command |
| Non-zero exit | Show stderr output, ask user for guidance |
| Timeout (10min) | Report timeout, suggest breaking task into smaller pieces |

## Important Rules
- Always use `run_in_background: true` for CLI execution
- Never run CLI in interactive mode — always headless/non-interactive
- Escape prompts properly (prefer temp file + pipe for long prompts)
- Report the CLI output faithfully — do not fabricate results
- If the user specifies a model, use it; otherwise read default from config.yaml
