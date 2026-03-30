---
description: Create pull request with auto-generated description
allowed-tools: Bash, Read, Grep, AskUserQuestion, Skill
---

# /pr [options]

Create a pull request with auto-generated description based on commits and changes.

## Usage

- `/pr` - Create PR with auto-generated content
- `/pr draft` - Create as draft PR
- `/pr --base main` - Specify base branch

## What It Does

1. Analyzes all commits on current branch (vs base branch)
2. Generates structured PR description (language and sections from config)
3. Asks for confirmation/modifications
4. Creates PR via `gh pr create`

## Arguments

$ARGUMENTS

## Execute

Use the create-pr skill.

If `draft` argument is provided, create the PR as a draft.

If `--base <branch>` is provided, use that as the base branch instead of the default.

Otherwise, use default settings from `.claude/skills/create-pr/config.project.yaml`.
