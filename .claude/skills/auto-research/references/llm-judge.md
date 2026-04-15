# LLM-as-Judge Metric Mode

## When to Use

Use `--metric-type llm-judge` when:
- The quality you're measuring can't be expressed as a single shell command output
- Keyword matching produces too many false negatives (e.g., checking nuance, tone, style)
- You need multi-criteria evaluation (e.g., perspective + naturalness + variety)

## Eval Config YAML

```yaml
# Required
metric_type: llm-judge
eval_command: "go run ./cmd/eval/ {capture} {game_title}"  # {capture} is replaced per-sample
captures_dir: testdata/captures/                            # directory with input samples

# Optional (defaults shown)
judge_runs: 3              # N runs per criterion per sample, averaged to reduce variance
judge_model: haiku         # model for judging (haiku = cheap + fast)
sample_count: 7            # max samples from captures_dir (0 = all)
direction: higher          # which direction is "better" for the final score

# Template variables (available in eval_command via {key})
game_title: "Hollow Knight: Silksong"

# Criteria (at least 1 required)
criteria:
  - name: perspective
    weight: 5
    prompt: |
      Rate this game commentary response on a 1-10 scale.
      Is it a 2nd-person reaction to a friend's gameplay, or a 3rd-person broadcast?
      10=talking to a friend  5=mixed  1=broadcasting
      Reply with a single number.

  - name: naturalness
    weight: 3
    prompt: |
      Rate this response on naturalness.
      Would a real friend say this while watching someone play?
      10=completely natural  5=somewhat stiff  1=robotic
      Reply with a single number.

  - name: variety
    weight: 2
    prompt: |
      Rate the variety of this set of responses.
      Are sentence endings, exclamations, and patterns diverse?
      10=always different  5=somewhat repetitive  1=templated
      Reply with a single number.
```

## Config Fields

### eval_command (required)
Shell command that generates the response to be judged.
- `{capture}` is replaced with the absolute path to each sample file
- Other `{key}` placeholders are replaced from top-level YAML fields
- Must output the response text to stdout
- Non-zero exit = skip that sample (logged as warning). If ALL samples fail, treat as crash.

### captures_dir (required)
Directory containing input sample files (any format).
- Files are sorted alphabetically for reproducibility
- If `sample_count` < total files, the first N are used (deterministic)

### criteria (required, 1+)
Each criterion has:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Identifier for results.tsv columns |
| `weight` | yes | Relative weight in final score |
| `prompt` | yes | Judge prompt. The response text is prepended before this prompt. Must ask for a 1-10 integer score. |

### judge_runs (default: 3)
Number of times each criterion is evaluated per sample.
- Averaging N runs stabilizes LLM non-determinism
- Empirical: 3 runs reduces variance from ±6 to ±2-3 points
- Higher = more stable but more expensive

### judge_model (default: haiku)
Model used for `claude -p` judge calls.
- `haiku` — cheapest, fast, sufficient for simple rubric scoring
- `sonnet` — better for nuanced criteria, 10x cost
- Any model alias supported by `claude -p --model`

## Scoring Formula

```
For each sample:
  For each criterion:
    scores = [judge_run_1, judge_run_2, ..., judge_run_N]  # each clamped to [1,10]
    criterion_avg = mean(scores)

  capture_score = Σ(criterion_avg × weight) / Σ(weights)   # weighted average, range [1,10]

final_score = mean(capture_scores) × 10                     # normalize to [10,100]
```

The final_score range is [10,100] (since criterion scores are clamped to [1,10]).
This is what gets compared across experiments in the DECIDE step.

**Parse failure**: If a judge run returns no parseable integer, retry once. If still unparseable, exclude that run from the average. If all runs for a criterion fail, use score 5 (conservative fallback).

## Results TSV

In llm-judge mode, the results.tsv `metric` column contains the final_score (10-100 scale).
Per-criterion breakdowns are logged to stderr for debugging.

## Cost Estimation

Total `claude -p` calls per measurement:
```
sample_count × criteria_count × judge_runs
```

Example: 7 samples × 3 criteria × 3 runs = 63 calls per experiment.
At ~$0.001/call (haiku): ~$0.06 per experiment, ~$3 for 50 experiments.

If total calls > 100, the skill warns before first measurement (skipped in `--unattended`).

## Timeout

The `budget` parameter (minutes per experiment) applies to llm-judge mode too.
If the judge pipeline hasn't finished all samples within the budget, score with
samples completed so far (minimum 1 sample required; if 0 completed, treat as crash).

## Compatibility

- `--metric "shell command"` continues to work unchanged (shell mode is default)
- `--metric-type llm-judge` activates this mode
- Both modes use the same experiment loop (hypothesize → implement → measure → decide)
- `--direction` defaults to `higher` in llm-judge mode (higher score = better quality)
