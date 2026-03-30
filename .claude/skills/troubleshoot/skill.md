---
name: troubleshoot
description: >
  Deep troubleshooting pipeline. Symptom analysis → Hypothesis formation → Code verification → Self-refutation → Resolution.
  Use for "troubleshoot", "debug", "find root cause" requests.
allowed-tools: Read, Glob, Grep, Bash, Task, AskUserQuestion, WebSearch
user_invocable: true
---

# /troubleshoot

Performs 5-step structured diagnosis for bugs/errors. **Diagnosis only** — code modifications proceed separately after user approval.

## Examples
- "I don't know why this error happens" → Start 5-step diagnosis pipeline
- "Build is broken, find the cause" → Step 1: Symptom Collection

## Rules

- Do not proceed to next step before completing current step
- No guessing — judge based on evidence from code and logs

## Step 1: Symptom Collection

Parse user description → Check logs/errors/stack traces → Organize reproduction conditions

**Output**: Symptom summary table (error message, location, reproduction conditions, impact scope)

## Step 2: Code Exploration

Up to 3 parallel Explore agents: call stack tracing, config/env variables, recent git changes

If a language-specific MCP server is available (Serena, gopls, etc.), prefer its tools for token-efficient exploration (symbol overview, find symbol, find references).

If no LSP MCP is available, use Read, Glob, Grep for equivalent exploration.

**Output**: Related code path map (`{file:line} — {role}`)

## Step 3: Hypothesis Formation + Verification

1. Form **minimum 3 hypotheses** based on symptoms + code paths
2. For each hypothesis: collect evidence/counter-evidence via Explore agent or Read/Grep
3. If fewer than 3 hypotheses → **return to Step 2**

**Output**: Hypothesis verification table (hypothesis, evidence, counter-evidence, verdict: probable/eliminated/inconclusive)

## Step 4: Self-Refutation

1. For probable hypotheses: "What if this isn't the cause?", "What are the side effects of the fix?"
2. Use web search: library issues, API changes, similar bug reports
3. If refutation is valid → **return to Step 3**

**Output**: Refutation content + verification method/results + conclusion (maintain/revise)

## Step 5: Diagnosis Report

Synthesize Steps 1-4: **Symptoms → Root cause (confirmed + eliminated hypotheses) → Solution → Self-refutation results → Prevention measures**

After output, confirm with user whether to proceed with fix.
