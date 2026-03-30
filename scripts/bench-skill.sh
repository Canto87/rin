#!/bin/bash
# Benchmark a skill.md file using LLM-as-judge (Claude Opus).
# Usage: ./scripts/bench-skill.sh <skill.md path>
# Exit 1 if required keywords missing. Otherwise outputs quality score (0-100, higher=better).

set -euo pipefail

FILE="${1:-.claude/skills/auto-research/skill.md}"

if [ ! -f "$FILE" ]; then
  echo "FAIL: file not found: $FILE" >&2
  exit 1
fi

# --- Step 1: Hard keyword check ---
REQUIRED=(
  "Phase 0"
  "Phase 1"
  "HYPOTHESIZE"
  "IMPLEMENT"
  "MEASURE"
  "DECIDE"
  "KEEP"
  "DISCARD"
  "results.tsv"
  "code-edit"
  "simplicity"
  "git reset"
  "--continue"
  "--unattended"
  "metric"
)

for keyword in "${REQUIRED[@]}"; do
  if ! grep -qiF -- "$keyword" "$FILE"; then
    echo "FAIL: missing required keyword: $keyword" >&2
    exit 1
  fi
done

# --- Step 2: LLM-as-judge via Claude ---
CONTENT=$(cat "$FILE")
WORDS=$(wc -w < "$FILE" | tr -d ' ')

PROMPT="You are evaluating an AI agent skill definition file. This file instructs an autonomous coding agent on how to run an experiment loop (modify code, measure metrics, keep/discard changes).

Rate the file on 0-100 based on these criteria:

- Protocol Completeness (25pts): Are all phases clearly defined? Setup, experiment loop, completion? Are edge cases (crash, timeout, resume) covered?
- Decision Clarity (25pts): Can an agent follow the KEEP/DISCARD logic without ambiguity? Are thresholds and conditions explicit?
- Error Handling (15pts): Are failure scenarios covered with clear recovery actions?
- Conciseness (15pts): Is every sentence necessary? No redundancy or filler?
- Actionability (20pts): Are there concrete commands, templates, and formats the agent can directly execute?

Word count: ${WORDS} words.
Fewer words for the same quality = higher Conciseness score.

<file>
${CONTENT}
</file>

Output ONLY a single integer 0-100. Nothing else."

SCORE=$(claude -p "$PROMPT" --model opus 2>/dev/null | grep -oE '[0-9]+' | head -1)

if [ -z "$SCORE" ]; then
  echo "FAIL: judge returned no score" >&2
  exit 1
fi

echo "$SCORE"
