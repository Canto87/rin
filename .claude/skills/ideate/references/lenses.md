<!-- Managed by project-rin harness. Source: .claude/skills/ideate/references/lenses.md -->
# Cognitive Lenses

Structured frameworks for divergent exploration. Each lens forces a different angle on the same problem.

## Lens: Default

Best for: **New features**, greenfield ideas.

Generate 4-6 approaches from distinct strategic directions:

| Direction | Prompt |
|-----------|--------|
| Conservative | "Minimal viable version using only existing patterns" |
| Bold | "Ambitious version — what if we had no constraints?" |
| Speed | "Fastest path to value — what can ship in days?" |
| Extensibility | "Designed to grow — what foundation enables future features?" |
| Lateral | "Unexpected angle — solve the problem by reframing it" |
| Hybrid | "Combine the best parts of 2+ directions above" |

Output per approach:
```
## Approach: {Name}
**Direction**: {direction}
**Summary**: 2-3 sentences
**Key steps**: 3-5 numbered
**Strengths**: 2-3 bullets
**Risks**: 2-3 bullets
**Effort**: Low / Medium / High
```

## Lens: SCAMPER

Best for: **Improving existing features**, rethinking current implementations.

Apply 7 transformation operators to the current feature/system:

| Operator | Question |
|----------|----------|
| **S**ubstitute | What component/process/data can be replaced? |
| **C**ombine | What features/modules can be merged? |
| **A**dapt | What existing pattern from elsewhere can be borrowed? |
| **M**odify | What can be enlarged, shrunk, or reshaped? |
| **P**ut to other uses | Can this serve a different purpose than originally intended? |
| **E**liminate | What can be removed without losing core value? |
| **R**everse | What if the flow/order/dependency were inverted? |

Apply each operator to the target. Skip operators that don't produce meaningful insight (minimum 3, maximum 7).

Output per operator:
```
### {Operator}: {Idea Name}
**Transformation**: What changes
**Before → After**: Concrete comparison
**Value**: Why this is better
**Risk**: What could go wrong
```

## Lens: Six Thinking Hats (optional)

Best for: **Balanced decision-making**, team-oriented evaluation.

Force 6 perspectives on the same idea:

| Hat | Color | Focus |
|-----|-------|-------|
| Facts | White | Data, numbers, what we know and don't know |
| Emotions | Red | Gut feelings, intuitions, user sentiment |
| Caution | Black | Risks, dangers, worst cases |
| Optimism | Yellow | Benefits, best cases, opportunities |
| Creativity | Green | Alternatives, wild ideas, provocations |
| Process | Blue | Summary, next steps, meta-view |

Use when: user selects it, or when the problem has significant human/organizational dimensions.

## Lens: Adjacent Possible (optional)

Best for: **Technology selection**, exploring what recently became feasible.

1. Identify 3-5 key constraints or assumptions of the current approach
2. For each constraint, WebSearch for recent developments (last 12 months) that may have changed the landscape
3. Generate approaches that leverage newly available capabilities

Output:
```
### Constraint: {assumption}
**Recent change**: {what changed, with source}
**New approach**: {how this enables a different solution}
**Maturity**: Experimental / Emerging / Stable
```

Use when: user selects it, or when the problem involves technology choices.

---

## Deep Mode: Parallel Dispatch

In Deep mode, each facet of the selected lens is dispatched as a separate Agent():

```
Agent({
  description: "[lens] [facet] perspective",
  prompt: "You are generating ONE approach to: {problem}.
Your perspective: {facet description}.
Context: {frame summary}.

Generate exactly ONE approach. Format:
## Approach: [Name]
**Perspective**: [facet]
**Summary**: 2-3 sentences
**Key steps**: 3-5 numbered
**Strengths**: 2-3 bullets
**Risks**: 2-3 bullets
**Effort**: Low/Medium/High

Stay under 150 lines. Be concrete and specific.",
  subagent_type: "general-purpose",
  model: "sonnet"
})
```

Dispatch all facets in parallel, then collect and synthesize results.
