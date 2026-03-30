---
name: create-pr
description: >
  Create pull request with auto-generated description based on commits and changes.
  Use for "create pr", "pr", "pull request" requests.
allowed-tools: Bash, Read, Grep, AskUserQuestion
---

# Create Pull Request

Intelligent PR creation workflow with automatic description generation based on commits and code changes.

## When to Use

- Ready to create a pull request
- Want consistent, well-structured PR descriptions
- Need to summarize multiple commits into a cohesive PR
- Before requesting code review

## Prerequisites

- [ ] All changes committed (use `smart-commit` skill first)
- [ ] Branch pushed to remote (or will be pushed automatically)
- [ ] `gh` CLI authenticated (`gh auth status`)
- [ ] On a feature branch (not main/master/dev)

## Quick Start

Just ask:
```
use create-pr skill
```

The skill will automatically:
1. Analyze your branch changes
2. Generate PR title and description
3. Ask for confirmation/modifications
4. Create PR via `gh pr create`

---

## Workflow Overview

### Step 0: Load Configuration

**Purpose**: Load PR template configuration

**Process**:
1. Read `config.project.yaml` from the skill directory
2. Load template sections, checklist items, language setting
3. Determine base branch

**Configuration determines**:
- PR description language (default: English)
- PR template structure
- Required checklist items
- Base branch for comparison

---

### Step 1: Pre-Flight Checks

**Purpose**: Ensure environment is ready for PR creation

**Checks**:
```bash
# Verify gh CLI is authenticated
gh auth status

# Check current branch is not base branch
git branch --show-current

# Check for uncommitted changes
git status --short

# Verify remote tracking
git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null || echo "no-upstream"
```

**Failure handling**:
- Not authenticated → Guide user to run `gh auth login`
- On base branch → Ask user to switch to feature branch
- Uncommitted changes → Suggest using `smart-commit` skill first
- No upstream → Will push with `-u` flag

---

### Step 2: Analyze Branch Changes

**Purpose**: Gather information for PR description

**Analysis commands**:
```bash
# Get base branch (from config or detect)
BASE_BRANCH="main"  # overridden by config.project.yaml

# List all commits on this branch
git log ${BASE_BRANCH}..HEAD --oneline

# Get detailed commit messages
git log ${BASE_BRANCH}..HEAD --format="%s%n%b"

# Get changed files summary
git diff ${BASE_BRANCH}...HEAD --stat

# Get file changes by type
git diff ${BASE_BRANCH}...HEAD --name-only
```

**Information extracted**:
- All commit messages and bodies
- Files changed (added, modified, deleted)
- Lines changed statistics
- Affected layers (from config.project.yaml layer definitions)

---

### Step 3: Generate PR Content

**Purpose**: Create structured PR title and description

#### Title Generation

**Format**: Infer from commits
- Multiple `feat` commits → `feat: [summary]`
- Single commit → Use that commit message
- Mixed types → Use primary type

**Rules**:
- Max 72 characters
- Imperative mood
- No period at end

#### Description Generation

**Language**: Determined by `config.project.yaml` → `template.language` (default: `"en"`)

**Template structure** (from config sections):

For English (`language: "en"`):
```markdown
## Background

[Auto-generated from commit messages and context]
- Purpose of changes
- Related issues/tickets (if mentioned in commits)

## Changes

[Auto-generated from git diff analysis]
- Summary of main changes
- New files added
- Modified components

[Screenshots placeholder if UI changes detected]

## Impact & Verification

[Auto-generated based on changed files]
- Affected components/layers
- Test coverage status

## Notes

[Any additional context from commits]

## Checklist

[From config - project-specific items]
- [ ] Item 1
- [ ] Item 2
```

For Japanese (`language: "ja"`), use section names from `config.project.yaml` → `template.sections[].name`.

---

### Step 4: User Review and Modification

**Purpose**: Allow user to review and customize PR content

**Present to user**:
1. Generated PR title
2. Generated PR description (full markdown)
3. Target base branch
4. Files to be included

**Options**:
1. **Approve** → Proceed to create PR
2. **Edit title** → User provides custom title
3. **Edit description** → User modifies sections
4. **Change base branch** → User specifies different base
5. **Cancel** → Exit without creating PR

**Questions to ask** (via AskUserQuestion):
- "Is the PR title appropriate?"
- "Any additions or modifications to the description?"

---

### Step 5: Push and Create PR

**Purpose**: Push branch and create pull request

**Process**:

```bash
# Push to remote (if not already pushed or has new commits)
git push -u origin $(git branch --show-current)

# Create PR
gh pr create \
  --title "PR Title" \
  --body "$(cat <<'EOF'
PR description here...
EOF
)" \
  --base main
```

**Success output**:
```
PR created successfully!

PR: #123 feat: implement user authentication
URL: https://github.com/org/repo/pull/123
Base: main <- feature/auth

Next steps:
- View PR: gh pr view 123
- Add reviewers: gh pr edit 123 --add-reviewer @user
- Check status: gh pr checks 123
```

---

## Configuration & Troubleshooting

See **`references/troubleshooting.md`** for configuration details, layer detection rules, and common issues.

---

## Integration with Other Skills

**Recommended workflow**:
1. `smart-commit` → Validate + Commit changes
2. `create-pr` → Create pull request
