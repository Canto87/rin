---
name: smart-commit
description: >
  Analyze changes, auto-group by layer/type/feature, and create multiple commits.
  Use for "commit", "smart commit", "group commit" requests.
allowed-tools: Bash, Read, Glob, Grep, Agent
---

# Smart Commit

Analyzes changes, auto-groups by layer/type/feature, and creates multiple commits.

## Workflow

### Step 1: Load Grouping Configuration

Read `config.yaml` in the skill directory if it exists. The config defines:
- Layer paths for auto-grouping
- Project-specific grouping priorities

If no config.yaml exists, use generic grouping (by file type and directory depth).

---

### Step 2: Analyze Changes

```bash
git status --short
git diff --name-only HEAD
```

### Step 3: Auto-Group Files

Classify files by layer/type/feature:

| Priority | Grouping | Description |
|----------|----------|-------------|
| 1 | Layer | Architecture layers defined in config.yaml |
| 2 | Type | Test files, docs, config files |
| 3 | Feature | Domain/feature subdirectories |

**Grouping Logic:**
- Same layer files → group together
- Related layers for same feature → can group
- Test-only changes → separate group
- Doc-only changes → separate group

### Step 4: Commit Each Group

For each group:

1. **Detect Type** (from file paths)
2. **Generate Message** (`<type>: <subject>`)
3. **Execute Commit** (`git add <files> && git commit`)

```bash
git add <file1> <file2> ...
git commit -m "<type>: <subject>"
```

## Commit Message Rules

| Rule | Setting |
|------|---------|
| Format | `<type>: <subject>` (no scope) |
| Language | English |
| Length | ≤72 characters |
| Verb | Imperative mood (add, fix, update) |
| AI Signature | Auto-remove (Co-Authored-By, etc.) |

## Type Guide

| Type | Usage | Example |
|------|-------|---------|
| feat | New feature | feat: add user authentication |
| fix | Bug fix | fix: handle nil pointer error |
| docs | Documentation only | docs: update API documentation |
| test | Test files only | test: add edge case tests |
| chore | Build, config, deps | chore: update dependencies |
| refactor | Restructure (no behavior change) | refactor: extract helper function |

## Grouping Examples

**Example 1: Multi-layer changes**

```
Changed files:
- src/entity/user.py
- src/service/user_service.py
- src/repository/user_repo.py
- docs/README.md

→ 3 commits:
1. feat: add user entity
2. feat: add user service and repository
3. docs: update documentation
```

**Example 2: Feature with tests**

```
Changed files:
- src/entity/payment.py
- src/entity/payment_test.py
- src/service/payment_service.py
- src/service/payment_service_test.py

→ 2 commits:
1. feat: add payment entity
2. feat: add payment service with tests
```

**Example 3: Test-only changes**

```
Changed files:
- src/entity/user_test.py
- src/service/user_service_test.py

→ 1 commit:
1. test: add user entity and service tests
```

## Type Detection Rules

| File Pattern | Detected Type |
|--------------|---------------|
| New files added | feat |
| *_test.* / test_*.* only | test |
| docs/, *.md only | docs |
| Build files, config, lock files | chore |
| Bug fix keywords in diff | fix |
| Ambiguous | Ask user |

## Step 5: Post-Commit GC Scan (Optional)

After all commits complete, run a scoped GC scan if enabled in `config.yaml` (`gc.enabled: true`).

**Scope**: Extract unique parent modules (top-level directories) from committed files.

**Execution**:
```
Agent(
  subagent_type: "general-purpose",
  model: "sonnet",
  run_in_background: true,
  prompt: "Run a lightweight GC scan on these modules: {modules}.
           Check for:
           - GC-01: Unused imports, commented-out code, dead functions (Grep)
           - GC-04: TODO/FIXME/HACK comments (Grep)
           Report findings as: file:line — category — description
           If no findings, say 'Clean.'"
)
```

**Output**: Append findings summary after the commit report.
- 0 findings → `GC: Clean`
- N findings → `GC: {N} findings` + list

**Skip when**:
- `gc.enabled` is false or absent in config.yaml
- Only docs/config files were committed (no source code)

## Error Handling

| Error | Action |
|-------|--------|
| Pre-commit hook failure | Stop all commits. Show hook output. Never use `--no-verify`. |
| Empty commit (no changes in group) | Skip group, continue to next |
| Merge conflict in staging | Abort. Report conflicting files to user |
| Detached HEAD | Warn user, abort all commits |

## Notes

- `refactor` is NOT auto-detected. Use only when explicitly intended.
- If changes span multiple unrelated features, ask user for grouping preference.
- Always verify commit with `git log -1 --oneline` after each commit.
- Layer paths are defined in `config.yaml`; if absent, infer from project structure.
