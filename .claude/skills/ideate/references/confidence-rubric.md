<!-- Managed by project-rin harness. Source: .claude/skills/ideate/references/confidence-rubric.md -->
# Confidence Rubric

4-dimension scoring system. Each dimension 0-20 points, total 80.
Threshold: **80% (64/80)** to proceed to Explore phase.

## Dimensions

### 1. Problem Clarity (0-20)

| Score | Level | Criteria |
|-------|-------|----------|
| 0-5 | Vague | No clear problem articulated, only a wish or feeling |
| 6-10 | Emerging | Problem exists but who/when/how-often undefined |
| 11-15 | Clear | Problem, affected users, and frequency identified |
| 16-20 | Sharp | Quantified impact, root cause understood, prior attempts documented |

### 2. Goal Definition (0-20)

| Score | Level | Criteria |
|-------|-------|----------|
| 0-5 | Absent | No concrete goal, only vague aspiration |
| 6-10 | Directional | General direction but not measurable |
| 11-15 | Defined | Clear goal with rough success metric |
| 16-20 | SMART | Specific, measurable, time-bound goal with concrete metric |

### 3. Scope Boundaries (0-20)

| Score | Level | Criteria |
|-------|-------|----------|
| 0-5 | Unbounded | No sense of what's in or out |
| 6-10 | Implied | Some boundaries hinted but not explicit |
| 11-15 | Bounded | Clear in/out list, some gray areas remain |
| 16-20 | Crisp | Explicit in/out/future with rationale for each exclusion |

### 4. Internal Consistency (0-20)

| Score | Level | Criteria |
|-------|-------|----------|
| 0-5 | Contradictory | Goals conflict with constraints or scope |
| 6-10 | Tensions | Minor inconsistencies between stated goals |
| 11-15 | Aligned | Goals, scope, and constraints mostly coherent |
| 16-20 | Harmonized | All elements reinforce each other, no contradictions |

## Scoring Rules

- Score conservatively. When uncertain, round down.
- Each round: compute `confidence = sum(dimensions) / 80 * 100`.
- Target the **weakest dimension** with next question.
- Show score after each round:
  ```
  Round {n} | Confidence: {score}% | Weakest: {dimension} | Why: {rationale}
  ```
- Threshold behavior:
  - `<50%`: 3+ questions remain, ask broad questions
  - `50-64%`: 1-2 targeted questions
  - `>=64%` (threshold met): proceed to Explore, note remaining gaps as Open Questions

## Anti-Sycophancy

- Never say "interesting approach" or "great idea" — stay neutral.
- Challenge vague claims: "users want X" → "which users? how do you know?"
- Challenge undefined terms: "better UX" → "better how? measured by what?"
- If a challenge reveals a gap, adjust score downward.
