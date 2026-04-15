<!-- Managed by project-rin harness. Source: .claude/skills/usage-report/skill.md -->
---
name: usage-report
description: >
  Claude Code session usage analysis with category classification.
  Extracts per-session time/token/cost statistics and classifies each session
  into work categories. Use for "usage report", "使用量レポート", "사용량 리포트",
  "usage stats", "how much did I use" requests.
allowed-tools: Read, Bash, Glob, Grep
user_invocable: true
---

# Usage Report — Session Analysis & Classification

## Role
Analyze Claude Code session transcripts across all projects. Extract time, token,
and cost statistics per session, classify each session into work categories, and
generate an aggregated report.

## Workflow

### Step 1: Parse user request
Extract from user message:
- **period**: `today`, `week` (default), `month`, or custom date range
- **project**: project name filter (optional)
- **format**: `text` (default), `csv`, `json`
- **detail**: whether to show minor categories (default: major only)
- **reclassify**: force re-classification of cached results

Map natural language: "今週" → week, "今日" → today, "今月" → month,
"이번 주" → week, "오늘" → today, "이번 달" → month

### Step 2: Run data extraction script
```bash
python3 ~/.claude/skills/usage-report/usage-report.py --period={period} [--project={project}] --format=json --min-turns=2
```

The script outputs JSON with:
- `summary`: totals for the period (sessions, time, tokens, cost, by_project, by_model)
- `sessions[]`: per-session data (timestamps, turns, tokens, cost, tools, first_prompts, facet summary)

### Step 3: Classify sessions
Read the config file for category definitions:
```
.claude/skills/usage-report/config.yaml
```

For each unclassified session, determine the best-fit category based on:
1. `first_prompts` — the actual user requests (highest signal)
2. `tools_used` — tool patterns indicate work type:
   - Edit/Write heavy → コーディング
   - Grep/Read/Glob only → 調査・理解
   - Agent(code-review) → レビュー・品質
   - Bash(git commit/push) → コミット・PR作成
3. `summary` / `goal_categories` — facet data if available
4. `project` — project context can hint at work type

Assign: `{ major: "...", minor: "..." }`

Classification rules:
- One major + one minor per session
- When ambiguous, prefer the category that best describes the **user's intent** (first prompt)
- Subagent/teammate sessions (first_prompt contains `<teammate-message>`) → classify by the delegated task content
- Very short sessions (2-3 turns) with only Read/Grep → 調査・理解 > 既存コード理解
- **Orchestration detection**: Long sessions (50+ turns) that heavily use Agent/TeamCreate/SendMessage
  and delegate to multiple subagents → オーケストレーション. These sessions coordinate design,
  implementation, review, and QA — they are NOT coding or review themselves. The actual coding/review
  work is already counted in the subagent sessions. The orchestrator cost is "coordination overhead".
  - Minor = 設計+実装指揮: feature implementation orchestration (plan → delegate → QA gate → merge)
  - Minor = ツール・ハーネス構築: building/configuring tools, skills, scripts, infrastructure

### Step 4: Generate report

#### Text format (default)
Output in the language specified in config.yaml `output.language`.

```
═══════════════════════════════════════════════
  Claude Code Usage Report — 2026-03-25 ~ 03-31
═══════════════════════════════════════════════

  期間: 2026-03-25 ~ 2026-03-31
  セッション数: 45
  Claude応答時間: 3h 42m
  セッション経過時間: 12h 15m
  トークン: 1.2M input / 890K output
  コスト: $156.23

  ─── カテゴリ別 ──────────────────────────────

  カテゴリ          セッション  応答時間    コスト    割合
  ────────────────  ──────────  ────────  ────────  ────
  コーディング              18   1h 45m   $89.20   57%
  調査・理解                12     42m    $28.50   18%
  レビュー・品質             8     35m    $22.10   14%
  設計・ドキュメント         4     28m    $12.30    8%
  運用・DevOps              2      8m     $3.10    2%
  その他                     1      4m     $1.03    1%

  ─── プロジェクト別 ──────────────────────────

  プロジェクト      セッション  応答時間    コスト
  ────────────────  ──────────  ────────  ────────
  acme-web              30   2h 30m  $112.00
  project-rin              10     52m    $35.20
  internal-api                 5     20m     $9.03

  ─── 日別推移 ────────────────────────────────

  日付        セッション  応答時間    コスト
  ──────────  ──────────  ────────  ────────
  03-25 (月)          8     38m    $24.50
  03-26 (火)         12     52m    $31.20
  ...
═══════════════════════════════════════════════
```

When `--detail` is specified, expand each major category to show minor breakdown.

#### Work summary section (always included after category table)
After the category breakdown, add a work summary that groups similar sessions and describes what was done:

```
  ─── 作業サマリー ────────────────────────────────

  ▸ レビュー・品質                          33件  $302.27
    feature-x Phase 1 Review + Validate      4件  $40.77
    feature-x Phase 2 Review + Validate      6件  $43.87
    feature-x Phase 3 Review + Validate      8件  $77.50
    feature-x Phase 4 Review + Validate     14件  $132.75
    acme-mobile PR #72 QA gate          1件   $7.38

  ▸ コーディング                            14件   $15.37
    feature-x Phase 2: UI実装 (React)        1件   $1.06
    feature-x Phase 3: Proto + Backend       3件   $4.42
    feature-x Phase 4: LiveChat統合         8件   $8.79
    feature-x Phase 4: QA修正               3件   $4.05

  ▸ 運用・DevOps                             1件  $51.26
    statusline使用量バグ修正 + usage-report作成   1件  $51.26
```

Rules for work summary:
- Group sessions by semantic similarity (same feature/phase/PR)
- Each group: one-line description (Japanese) + session count + total cost
- Order groups by cost descending within each category
- For teammate sessions: extract the task description from first_prompt
- Consolidate Review + Validate pairs of the same phase into one line
- Keep descriptions concise (under 50 chars)

#### CSV format
Run the script with `--format=csv` and append classification columns:
```
session_id,project,first_ts,turn_count,response_time_sec,elapsed_sec,input_tokens,output_tokens,cost_usd,model_tier,summary,first_prompt,major_category,minor_category
```

#### JSON format
Return the script output with classification data added to each session:
```json
{
  "summary": { ... },
  "sessions": [
    { ..., "classification": { "major": "コーディング", "minor": "新機能実装" } }
  ]
}
```

### Step 5: Present results
**CRITICAL: Always display the FULL report. Never summarize, omit, or editorialize.**
- Display the complete formatted report as-is — all sections (summary, categories, work summary, projects, daily)
- Do NOT add your own commentary, predictions, or judgments about the data
- Do NOT say things like "大したことない" or "相当なコストになっている" — just show the numbers
- If there are only 1-2 sessions, still show the full report format
- If format=csv, output as copyable code block
- If format=json, output as JSON code block

For work summary: use `first_prompts` to write specific descriptions of what was done,
not generic labels like "品質レビュー". Example:
- Bad: "acme-web 品質レビュー 2件"
- Good: "feature-x Final comprehensive R/V (90 commits) 2件"

## Notes
- All times are displayed in the user's local timezone
- Cost calculation uses model-specific pricing (Opus $15/$75, Sonnet $3/$15, Haiku $1/$5)
- Sessions with < 2 turns are excluded by default (configurable via min_turns)
- Classification is performed inline by the current Claude session — no external API calls needed
