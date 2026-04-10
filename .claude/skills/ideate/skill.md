<!-- Managed by project-rin harness. Source: .claude/skills/ideate/skill.md -->
---
name: ideate
description: >
  Pre-planning ideation skill for new features and improvements.
  Divergent exploration → convergent evaluation → Feature Brief.
  Use before plan-feature when the idea is still vague or multiple directions are possible.
  Triggers: "ideate", "brainstorm", "explore idea", "what should we build", "how to improve"
allowed-tools: Read, Write, Glob, Grep, Agent, AskUserQuestion, WebSearch, WebFetch, Bash
user_invocable: true
---

# Ideation Skill

Transforms vague ideas into concrete Feature Briefs that feed into `/plan-feature`.

## When to Use

- "I want to add something like X"
- "How can we improve Y?"
- "I have a rough idea for Z"
- When the user doesn't yet know *what* to build, only *why* something feels needed
- Before `/plan-feature` — this skill answers "what and why", plan-feature answers "how"

## When NOT to Use

- Idea is already concrete with clear scope → go directly to `/plan-feature`
- Architecture/technology decisions without a feature context → separate discussion
- Bug fixes or known issues → `/troubleshoot` or direct fix

## Configuration

Read `config.yaml` in the same folder:

```yaml
defaults:
  depth: standard            # quick | standard | deep
  confidence_threshold: 80   # percentage

lenses:
  enabled: [default, scamper]
  optional: [six-hats, adjacent-possible]

paths:
  output: "docs/ideation"
```

If `config.yaml` is absent, use defaults above.

## Input Parsing

Extract from user message:
- **topic**: The idea or area to explore (required)
- **context**: Why this came up, what pain point triggered it (optional)
- **options**: `--depth quick|standard|deep`, `--lens <name>`, `--resume`

Examples:
- `/ideate memory search improvements` → topic: memory search improvements
- `/ideate "session handoff" --depth deep` → topic: session handoff, depth: deep
- `/ideate --resume` → resume previous session

---

## Execution Flow

```
0. Session Check       → Resume or fresh start
      ↓
1. Frame (Required)    → 5W1H questions, confidence scoring
      ↓
   Confidence Gate     → ≥ threshold? proceed : ask more
      ↓
2. Explore             → Divergent idea generation with cognitive lenses
      ↓
3. Evaluate            → Convergent scoring + adversarial review
      ↓
4. Synthesize          → Feature Brief generation
      ↓
   → /plan-feature handoff
```

---

## Phase 0: Session Check

Check for existing state at `.ideate/{topic}/state.md`.

If exists, ask:
```json
{
  "question": "Found existing ideation session for '{topic}'. How to proceed?",
  "options": [
    {"label": "Resume", "description": "Continue from Phase {N}"},
    {"label": "Start fresh", "description": "Delete state and restart"},
    {"label": "View state", "description": "Show current state before deciding"}
  ]
}
```

If not exists, proceed to Phase 1.

---

## Phase 1: Frame

**Goal**: Define the problem clearly enough to explore solutions.

### Step 1a: Initial Understanding

Read the user's raw input. Generate 5W1H questions automatically:
- **Who** is affected?
- **What** is the problem or opportunity?
- **When** does it occur?
- **Where** in the system/workflow?
- **Why** does it matter?
- **How** is it currently handled (workaround)?

Present the most critical unanswered questions (max 2 at a time) via `AskUserQuestion`.

### Step 1b: Codebase Context (if applicable)

If the topic relates to existing code:
- Use Glob/Grep to find relevant modules
- Summarize what exists today (3-5 lines)
- Identify patterns and constraints from current implementation

### Step 1c: Confidence Scoring

After each Q&A round, score 4 dimensions per `references/confidence-rubric.md`:

```
Round {n} | Confidence: {score}% | Weakest: {dimension} | Why: {rationale}

{Targeted question for weakest dimension}
```

**Threshold behavior**:
- `< 50%`: Ask broad, open questions
- `50-79%`: Ask targeted questions for weakest dimension
- `≥ 80%` (threshold): Proceed to Phase 2, note remaining gaps as Open Questions

**Rules**:
- One question at a time (AskUserQuestion)
- Target the weakest dimension
- Maximum 8 rounds — if threshold not met after 8 rounds, proceed anyway with gaps noted
- Anti-sycophancy: never praise the idea, stay neutral, challenge vague claims
- Always include "Skip to Explore" option for users who want to move faster

### Step 1d: Frame Summary

Before proceeding, display a compact summary:

```
## Frame Summary
**Problem**: {1-2 sentences}
**Goal**: {1-2 sentences}
**Scope**: {in/out bullets}
**Confidence**: {score}%
**Open Questions**: {if any}
```

Ask user to confirm or adjust.

---

## Phase 2: Explore

**Goal**: Generate multiple approaches from different angles.

### Step 2a: Depth & Lens Selection

If not specified via `--depth` or `--lens`, ask:

```json
{
  "question": "How deep should we explore?",
  "options": [
    {"label": "Quick", "description": "3 inline approaches, ~2 min. No lens selection."},
    {"label": "Standard", "description": "1 lens, sequential exploration, ~5 min."},
    {"label": "Deep", "description": "1+ lenses, parallel sub-agents, ~10 min."}
  ]
}
```

For Standard/Deep, offer lens selection:
```json
{
  "question": "Which lens to apply?",
  "options": [
    {"label": "Default", "description": "6 strategic directions (conservative → bold → lateral)"},
    {"label": "SCAMPER", "description": "7 transformation operators on existing feature"},
    {"label": "Other", "description": "Six Hats, Adjacent Possible, or suggest your own"}
  ]
}
```

### Step 2b: Idea Generation

**Quick mode**:
- Generate 3 approaches inline (no sub-agents)
- Each: Name + Summary (2-3 sentences) + Key trade-off (1 sentence)
- Compact format, minimal detail

**Standard mode**:
- Apply selected lens sequentially
- Generate approaches per lens definition (see `references/lenses.md`)
- Full output format per approach

**Deep mode**:
- Apply selected lens(es)
- Dispatch each facet as a parallel `Agent()` call (sonnet model)
- Collect results and synthesize
- Agent prompt template in `references/lenses.md` → "Deep Mode: Parallel Dispatch"

### Step 2c: Prior Art Scan

For all modes, briefly scan the codebase for:
- Similar existing implementations (Grep for related keywords)
- Patterns that could be reused
- Constraints from current architecture

Note findings inline with approaches.

---

## Phase 3: Evaluate

**Goal**: Narrow down to one recommended approach.

### Step 3a: Evaluation Matrix

Score each approach on 4 criteria (1-5 each):

| Criterion | Description |
|-----------|-------------|
| Complexity | Implementation effort and risk |
| Value | Impact on the problem |
| Extensibility | How well it supports future growth |
| Alignment | Fit with existing codebase patterns |

Display as table:
```
| Approach | Complexity | Value | Extensibility | Alignment | Total |
```

### Step 3b: Adversarial Review

Apply exactly one challenge technique:

**Contrarian** — "What if the opposite were true?"
- Pick the leading approach
- Argue against it from 2-3 angles
- See if the ranking changes

**Pre-mortem** — "It's 3 months later and this failed. Why?"
- For the top 1-2 approaches
- List 3 failure modes each (likelihood + impact)
- Identify mitigations

Choose Contrarian for new features, Pre-mortem for improvements to existing systems.

### Step 3c: User Decision

Present evaluation summary and ask:

```json
{
  "question": "Which approach to pursue?",
  "options": [
    {"label": "Approach A (Recommended)", "description": "{name} — Score: {x}/20"},
    {"label": "Approach B", "description": "{name} — Score: {x}/20"},
    {"label": "Explore more", "description": "Back to Phase 2 with different lens"},
    {"label": "None — rethink", "description": "Back to Phase 1 to reframe"}
  ]
}
```

---

## Phase 4: Synthesize

**Goal**: Produce a Feature Brief document.

### Step 4a: Generate Brief

Write Feature Brief to `{config.paths.output}/{topic}/brief.md` using `templates/brief.md`.

Fill in all sections from Phases 1-3:
- Problem & Goal from Frame
- Approaches table from Explore
- Selected approach & rationale from Evaluate
- Risks & mitigations from Adversarial Review
- Open Questions from confidence gaps
- Confidence scores

### Step 4b: State Cleanup

Delete `.ideate/{topic}/` state directory if it exists.

### Step 4c: Handoff

Display completion summary:

```
## Ideation Complete

Brief: {output_path}/brief.md
Confidence: {score}%
Selected: {approach name}

### Next Steps
- `/plan-feature {topic}` → Detailed design from this brief
- Review brief and adjust before proceeding
```

---

## State Management

During the session, save state to `.ideate/{topic}/state.md`:

```markdown
# Ideation State: {topic}
Phase: {current_phase}
Updated: {ISO-8601}

## Frame
{frame summary}

## Confidence
{dimension scores}

## Approaches
{generated approaches, if Phase 2+ reached}

## Selection
{chosen approach, if Phase 3+ reached}
```

Update after each phase transition. Delete on successful completion (Phase 4b).

---

## Completion Output

```
## Ideation Complete

{config.paths.output}/{topic}/
└── brief.md    ({status})

### Summary
- Topic: {topic}
- Problem: {problem, 1 line}
- Selected: {approach name}
- Confidence: {score}%
- Depth: {quick|standard|deep}
- Lens: {lens used}

### Next Steps
→ `/plan-feature {topic}` to begin detailed design
```

**Status indicators:**
- (generated) — Fresh from this session
- (updated) — Modified from resumed session

---

## Related Skills

| Skill | Purpose | When to Use |
|-------|---------|-------------|
| `/plan-feature` | Detailed phase-based design | After brief is generated |
| `/auto-impl` | Implementation from design docs | After plan-feature |
| `/auto-research` | Autonomous experiment loop | When spike/PoC is needed during ideation |

## Limitations

- **AskUserQuestion: Max 4 options** — use "Other" for free-form input
- Quick mode skips lens selection — always uses inline generation
- Deep mode spawns sub-agents — higher token cost
- Maximum 8 Q&A rounds in Frame phase to prevent infinite loops
