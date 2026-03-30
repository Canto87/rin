# Q&A Template

Question format for interactive requirements gathering.

**Rules:**
- All questions include "Generate design docs" option
- AskUserQuestion: Max 4 options
- To modify previous answer: Select "Other" then input

---

## Step 0: Session Check

Before starting, check for existing session state.

### Check Logic

```
1. Check if .plan-feature/{feature_name}/state.md exists
2. If exists → Show resume prompt
3. If not exists → Proceed to Step 1
```

### Resume Prompt Display

```
Found Existing Session

Feature: {feature_name}
Progress: Step {N} of 9 ({step_name})
Last Updated: {timestamp}

Previous session context:
- {decision_1}
- {decision_2}
- {preference_1}
```

### Question: Session Resume

```json
{
  "questions": [{
    "header": "Session",
    "question": "Found existing session for '{feature_name}'. How would you like to proceed?",
    "multiSelect": false,
    "options": [
      {"label": "Resume", "description": "Continue from Step {N} ({step_name})"},
      {"label": "Start fresh", "description": "Delete existing state and start over"},
      {"label": "View state", "description": "Show full state before deciding"}
    ]
  }]
}
```

### Behavior by Selection

| Selection | Action |
|-----------|--------|
| Resume | Load state.md, jump to current step |
| Start fresh | Delete .plan-feature/{feature_name}/, start Step 1 |
| View state | Display full state.md content, ask again |

---

## Step 1: Check Configuration

Read `config.yaml` from the skill folder:

```yaml
# Defaults (when no config file exists)
project:
  name: "my-project"
  language: other

paths:
  source: "src"
  plans: "docs/plans"

integrations: []  # Define per project

storage:
  - label: "SQLite"
    description: "Lightweight embedded DB"
    recommended: true
  - label: "PostgreSQL"
    description: "Production DB"
  - label: "Memory only"
    description: "Resets on restart"
  - label: "File-based"
    description: "JSON/YAML files"
```

---

## Step 2: Basic Information (Required)

### Question 1: Feature Name Confirmation

```json
{
  "questions": [{
    "header": "Feature Name",
    "question": "Converting '{user mentioned feature}' to snake_case gives '{converted_name}'. Is this correct?",
    "options": [
      {"label": "Yes, correct", "description": "Proceed with this name"},
      {"label": "Use different name", "description": "Enter manually"},
      {"label": "Generate design docs", "description": "Start design with current info"}
    ],
    "multiSelect": false
  }]
}
```

### Question 2: Core Goal

```json
{
  "questions": [{
    "header": "Core Goal",
    "question": "What is the most important goal for this feature?",
    "options": [
      {"label": "Real-time processing", "description": "Needs immediate response"},
      {"label": "Batch processing", "description": "Process periodically in bulk"},
      {"label": "Data collection", "description": "Gather and store information"},
      {"label": "Generate design docs", "description": "Start design with current info"}
    ],
    "multiSelect": false
  }]
}
```

---

## Step 3: Intelligent Codebase Analysis

No questions - automatic deep analysis using Task tool with Explore agent.

### Analysis Steps

```
1. Structure Scan
   - Glob: {config.paths.source}/**/*
   - Identify directory patterns
   - Detect architecture style

2. Component Discovery
   - Search for types, interfaces, classes by language
   - Find shared utilities

3. Dependency Analysis
   - Import/require statements
   - Map module relationships
   - Detect potential reuse

4. Pattern Recognition
   - Check for common patterns (Repository, Service, Handler)
   - Identify naming conventions
   - Find configuration patterns
```

### Architecture Detection Rules

| Directory Pattern | Detected Architecture | Confidence |
|-------------------|----------------------|------------|
| `domain/`, `usecase/`, `adapter/` | Clean Architecture | High |
| `models/`, `views/`, `controllers/` | MVC | High |
| `api/`, `service/`, `repository/` | Layered | High |
| `ports/`, `adapters/` | Hexagonal | High |
| `modules/{name}/` | Modular Monolith | Medium |
| `{feature}/handler.*`, `{feature}/service.*` | Feature-based | Medium |
| Single `src/` or `internal/` | Undetermined | Low |

---

## Step 3.5: Brainstorming Session

### 3.5a. Prior Art Scan (Automatic)

Search for similar technical patterns in the codebase.

### 3.5b. Approach Comparison (Interactive)

Compare 2-3 technical implementation strategies.

```json
{
  "questions": [{
    "header": "Approach",
    "question": "Which approach best fits this feature?",
    "multiSelect": false,
    "options": [
      {"label": "Approach A (Recommended)", "description": "{name} - {key benefit}"},
      {"label": "Approach B", "description": "{name} - {key benefit}"},
      {"label": "Approach C", "description": "{name} - {key benefit}"},
      {"label": "Discuss more", "description": "Explore hybrid or other options"}
    ]
  }]
}
```

### 3.5c. What-if Exploration (Interactive)

```json
{
  "questions": [{
    "header": "What-if",
    "question": "Which technical scenarios should we account for in the design?",
    "multiSelect": true,
    "options": [
      {"label": "Concurrency safety", "description": "Handle race conditions and concurrent access"},
      {"label": "Data growth", "description": "Query performance at scale, indexing strategy"},
      {"label": "Failure resilience", "description": "External dependency failures, timeout handling"},
      {"label": "Add your own", "description": "Describe a custom technical scenario"}
    ]
  }]
}
```

### 3.5d. Open Discussion (Interactive)

```json
{
  "questions": [{
    "header": "Discussion",
    "question": "Continue brainstorming or move to architecture decisions?",
    "multiSelect": false,
    "options": [
      {"label": "Continue discussing", "description": "I have more ideas to explore"},
      {"label": "End brainstorming", "description": "Ready to proceed to architecture Q&A"},
      {"label": "Revisit approaches", "description": "Go back to approach comparison"},
      {"label": "Generate design docs", "description": "Start design with current info"}
    ]
  }]
}
```

---

## Step 4: Architecture Questions

### Question 3: System Integration (multiSelect)

Options dynamically generated from config.integrations (max 3), or defaults:
- Database, External API, None

### Question 4: Data Storage

Options dynamically generated from config.storage, or defaults:
- SQLite, Memory only, File-based

### Question 5: API Endpoints

- Yes REST API, No, Redo previous question

---

## Step 5: Functional Design (Required)

### Question 8: Core Use Cases (multiSelect)
- CRUD operations, Data processing, User interaction

### Question 9: Interface Specification
- Detailed spec, Standard patterns, Minimal

### Question 10: Error Handling Strategy
- Comprehensive, Standard pattern, Basic

### Question 11: Security & Validation (Optional, multiSelect)
- Authentication, Authorization, Input validation

---

## Step 5.5a: Feature Size Assessment

Auto-assess feature size. If large (score 15+), ask user to split or continue.

## Step 5.5: Implementation Pattern Selection

Propose patterns based on architecture + requirements.

## Step 6: Auto Phase Proposal

Analyze and suggest 3-7 phase structure.

## Step 6.5: Risk Analysis

Identify risks and rollback strategies per phase.

## Step 7: Details (Optional)

Priority and scheduling questions.

## Step 7.5: Validation

Check completeness and consistency before preview.

---

## Special Behaviors

### When "Other" selected then "redo previous question" entered
- Show previous question again
- Ignore previous answer and collect new one

### When "Generate design docs" selected
- Generate documents immediately with current information
- Use reasonable defaults for missing information
