---
description: Code Review (review + auto-fix by default)
allowed-tools: Bash, Read, Grep, Glob, AskUserQuestion, Agent
---

# /code-review [target] [options]

Weighted code review using the code-review agent. By default, reviews and auto-fixes issues.

## Usage

- `/code-review` - Review + fix all local uncommitted changes
- `/code-review --staged` - Review + fix staged changes only
- `/code-review pr` - Review + fix current branch's PR
- `/code-review pr 123` - Review + fix specific PR
- `/code-review --dry-run` - Review only, no fixes (report only)

## Arguments

$ARGUMENTS

## Execute

Spawn the code-review agent with `--fix` by default. Use `--dry-run` to skip fixes.

```
Agent(
  subagent_type: "code-review",
  prompt: "Review {target}. $ARGUMENTS --fix"
)
```

If `--dry-run` is in $ARGUMENTS, do NOT append `--fix`.
