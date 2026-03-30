---
name: plan-feature
description: Generate phase-based design documents for new features. Use for feature planning, roadmap creation, design documents, or "design a feature" requests.
allowed-tools: Read, Write, Glob, Grep, AskUserQuestion, Task, Bash
user_invocable: true
---

# Phase Document Generator

Collects requirements through interactive Q&A and generates phase-based design documents.

## When to Use

- "Design a XX feature"
- "Plan a new feature: YY"
- "Create a roadmap for ZZ"
- When design is needed before implementing a large feature

## Configuration File

Skill settings are managed in `config.yaml` in the same folder.

```yaml
# config.yaml example
project:
  name: "my-project"
  language: go  # go | python | typescript | java | rust | other

paths:
  source: "src"         # Source code path
  plans: "docs/plans"   # Design docs output path

# Optional: integration layers for Q&A
integrations:
  - label: "Module A"
    description: "Description"
    path: "src/module_a"

# Optional: storage options for Q&A
storage:
  - label: "PostgreSQL"
    description: "Primary database"
    recommended: true
```

## Execution Flow

```
0. Session Check          → Check existing state, resume or fresh start
       ↓
1. Check Config           → Read config.yaml (use defaults if missing)
       ↓
2. Basic Info (Required)  → Feature name, Core goal
       ↓
3. Codebase Analysis      → Explore related modules, detect architecture
       ↓
   Interim Summary 1
       ↓
3.5 Brainstorming Session
   3.5a. Prior Art Scan       → Codebase internal similar impl auto-scan
   3.5b. Approach Comparison  → 2-3 technical approaches, user discussion
   3.5c. What-if Exploration  → Scalability/edge cases/failure scenarios
   3.5d. Open Discussion      → Free-form technical discussion (until user ends)
   Brainstorming Summary
       ↓
4. Architecture Q&A       → Integration, Storage, API (reflects brainstorming)
       ↓
   Interim Summary 2
       ↓
5. Functional Design      → Use cases, Interface spec, Error handling
       ↓
   Interim Summary 3
       ↓
5.5a Feature Size Check   → Assess size, suggest split if large
       ↓
5.5 Implementation Pattern → Select implementation approach
       ↓
6. Auto Phase Proposal    → Analyze & suggest phase structure
       ↓
6.5 Risk Analysis         → Identify risks & rollback strategies
       ↓
7. Details (Optional)     → Priority, Scheduling
       ↓
7.5 Validation            → Verify completeness & consistency
       ↓
8. Preview & Confirm      → Show each file preview, allow edits
       ↓
9. Generate Documents     → Write confirmed docs
```

**Key Rules:**
- All questions include "Generate design docs" option (can exit anytime)
- Interim summaries to check progress
- Question format: See **`questions.md`**

## Question Categories

| Step | Question | Required |
|------|----------|----------|
| 0 | Session resume (if exists) | O |
| 2 | Feature name confirmation | O |
| 2 | Core goal | O |
| 3.5b | Approach selection | O |
| 3.5c | What-if scenario selection (multiSelect) | O |
| 3.5d | Continue/end brainstorming | O |
| 4 | System integration (multiSelect) | O |
| 4 | Data storage | O |
| 4 | API requirement | O |
| 5 | Core use cases (multiSelect) | O |
| 5 | Interface specification | O |
| 5 | Error handling strategy | O |
| 5 | Security/Validation | - |
| 5.5a | Feature size decision (if large) | O |
| 5.5 | Implementation pattern selection | O |
| 6 | Phase proposal confirmation | O |
| 7 | Priority | - |
| 7 | Scheduling | - |
| 7.5 | Validation proceed | O |

## Output

Generated in `{config.paths.plans}/{feature_name}/` folder:

```
{plans_path}/{feature_name}/
├── 00_OVERVIEW.md     ← Overall overview
├── 01_{PHASE1}.md     ← Phase 1 details
├── 02_{PHASE2}.md     ← Phase 2 details
└── ...
```

## Completion Output

```
## Design Documents Generated

{plans_path}/{feature_name}/
├── 00_OVERVIEW.md     (confirmed)
├── 01_{name}.md       (confirmed)
├── 02_{name}.md       (modified)
└── 03_{name}.md       (skipped)

### Collected Information Summary
- Feature name: {feature_name}
- Core goal: {goal}
- Integration: {systems}
- Storage: {storage}
- Use cases: {use_cases}
- Interface: {interface_spec}
- Error handling: {error_strategy}
- Phase count: {count}

### Next Steps
"{feature_name} prepare for implementation" → /auto-impl skill
```

**Status indicators:**
- (confirmed) - Approved without changes
- (modified) - Approved after user modifications
- (skipped) - User chose to skip this file

## Phase Division Criteria

1. **Dependencies**: Does another Phase need to complete first?
2. **Difficulty**: Low/Medium/High
3. **Impact**: Low/Medium/High
4. **Implementation Order**: Logical sequence

Recommended Phase count: 3-7

## Limitations

- **AskUserQuestion: Max 4 options**
- To modify previous answer: Select "Other" then type "redo previous question"

---

## Codebase Analysis

During Step 3, if a language-specific MCP server is available (Serena, gopls, etc.), prefer it for code exploration (symbol overview, find symbol, find references).

Otherwise, use Glob and Grep to explore the project structure.

## Related Skills

| Skill | Purpose | When to Use |
|-------|---------|-------------|
| `/auto-impl` | Implement from design docs | After design docs are generated |
| `/qa-gate` | Quality review | After completing each phase |

## Templates

- **`templates/overview.md`** - OVERVIEW template
- **`templates/phase.md`** - Phase template
- **`templates/phase-analysis.md`** - Phase analysis guide
- **`templates/codebase-analysis.md`** - Codebase analysis guide
