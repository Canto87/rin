"""Model routing with experience-based suggestions."""

import json
import logging
from datetime import datetime, timezone

log = logging.getLogger(__name__)

# ── Level classification ─────────────────────────────────────


def classify_level(
    file_count: int = 1,
    has_dependencies: bool = False,
    needs_design: bool = False,
) -> str:
    """Classify task complexity into L1/L2/L3.

    L1: 1 file, no dependencies, no design needed
    L2: 2-3 files, no dependencies
    L3: 3+ files OR has dependencies OR needs design
    """
    if has_dependencies or needs_design or file_count > 3:
        return "L3"
    if file_count >= 2:
        return "L2"
    return "L1"


# ── Default routing table ────────────────────────────────────

_DEFAULTS = {
    "L1": {"model": "glm-5", "mode": "solo", "agent_count": 1},
    "L2": {"model": "glm-5", "mode": "solo", "agent_count": 1},
    "L3": {"model": "glm-5", "mode": "team", "agent_count": 2},
}


# ── Suggest ──────────────────────────────────────────────────


async def suggest(
    store,
    task: str,
    file_count: int | None = None,
    has_dependencies: bool = False,
    needs_design: bool = False,
    project: str | None = None,
) -> dict:
    """Suggest optimal model routing based on past experience.

    Returns dict with: level, model, mode, agent_count, confidence, reason, history.
    """
    level = classify_level(file_count or 1, has_dependencies, needs_design)
    default = _DEFAULTS[level]

    # Search for similar routing logs (need content for JSON parsing)
    similar = await store.search(
        task, kind="routing_log", project=project, limit=20, detail="full"
    )

    if not similar:
        return {
            "level": level,
            **default,
            "confidence": 0.0,
            "reason": "no history — using defaults",
            "history": [],
        }

    # Aggregate by model: success rate + avg duration
    model_stats: dict[str, dict] = {}
    for doc in similar:
        content = doc.get("content", "")
        try:
            data = json.loads(content)
        except (json.JSONDecodeError, TypeError):
            continue

        model = data.get("model", "unknown")
        if model not in model_stats:
            model_stats[model] = {"success": 0, "fail": 0, "durations": []}

        if data.get("success"):
            model_stats[model]["success"] += 1
        else:
            model_stats[model]["fail"] += 1

        duration = data.get("duration_s")
        if duration and data.get("success"):
            model_stats[model]["durations"].append(duration)

    if not model_stats:
        return {
            "level": level,
            **default,
            "confidence": 0.0,
            "reason": "no parseable history — using defaults",
            "history": [],
        }

    # Score each model: success_rate * 0.7 + speed_score * 0.3
    best_model = None
    best_score = -1.0

    for model, stats in model_stats.items():
        total = stats["success"] + stats["fail"]
        if total == 0:
            continue

        success_rate = stats["success"] / total
        avg_duration = (
            sum(stats["durations"]) / len(stats["durations"])
            if stats["durations"]
            else 999
        )
        # Normalize speed: faster = higher score (cap at 300s)
        speed_score = max(0, 1.0 - avg_duration / 300)
        score = success_rate * 0.7 + speed_score * 0.3

        if score > best_score:
            best_score = score
            best_model = model

    total_logs = sum(s["success"] + s["fail"] for s in model_stats.values())
    confidence = min(total_logs / 10, 1.0)

    # Build history summary
    history = []
    for model, stats in model_stats.items():
        total = stats["success"] + stats["fail"]
        rate = stats["success"] / total if total else 0
        avg_d = (
            int(sum(stats["durations"]) / len(stats["durations"]))
            if stats["durations"]
            else None
        )
        history.append(
            {
                "model": model,
                "total": total,
                "success_rate": round(rate, 2),
                "avg_duration_s": avg_d,
            }
        )

    if best_model:
        best_stats = model_stats[best_model]
        best_total = best_stats["success"] + best_stats["fail"]
        best_rate = best_stats["success"] / best_total if best_total else 0
        reason = f"{best_model}: {best_rate:.0%} success ({best_total} logs)"
    else:
        best_model = default["model"]
        reason = "no clear winner — using defaults"

    return {
        "level": level,
        "model": best_model,
        "mode": default["mode"],
        "agent_count": default["agent_count"],
        "confidence": round(confidence, 2),
        "reason": reason,
        "history": history,
    }


# ── Log ──────────────────────────────────────────────────────

_CONSECUTIVE_FAIL_THRESHOLD = 3


async def log_routing(
    store,
    task: str,
    model: str,
    duration_s: int,
    success: bool,
    level: str | None = None,
    mode: str = "solo",
    agent_count: int = 1,
    files_changed: int | None = None,
    files_list: list[str] | None = None,
    fallback_used: bool = False,
    fallback_from: str | None = None,
    error_type: str | None = None,
    project: str | None = None,
) -> str:
    """Record routing result and detect failure patterns."""
    if level is None:
        level = classify_level(files_changed or 1)

    data = {
        "task_description": task,
        "level": level,
        "model": model,
        "mode": mode,
        "agent_count": agent_count,
        "duration_s": duration_s,
        "success": success,
        "files_changed": files_changed,
        "files_list": files_list,
        "fallback_used": fallback_used,
        "fallback_from": fallback_from,
        "error_type": error_type,
    }

    tags = [
        f"model:{model}",
        f"level:{level}",
        "success" if success else "failure",
        f"mode:{mode}",
    ]
    if fallback_used:
        tags.append("fallback")
    if error_type:
        tags.append(f"error:{error_type}")

    doc_id = await store.store_document(
        kind="routing_log",
        title=f"routing:{model}:{level}:{'ok' if success else 'fail'}",
        content=json.dumps(data, ensure_ascii=False),
        tags=tags,
        source=f"routing:{datetime.now(timezone.utc).strftime('%Y-%m-%d')}",
        project=project,
    )

    # Detect consecutive failures for same model
    if not success:
        await _check_failure_pattern(store, model, project)

    return doc_id


async def _check_failure_pattern(
    store, model: str, project: str | None
) -> None:
    """If model has N consecutive failures, create a team_pattern warning."""
    recent = await store.lookup(
        kind="routing_log",
        tags=[f"model:{model}"],
        project=project,
        limit=_CONSECUTIVE_FAIL_THRESHOLD,
        detail="full",
    )

    if len(recent) < _CONSECUTIVE_FAIL_THRESHOLD:
        return

    all_failed = True
    for doc in recent:
        try:
            data = json.loads(doc.get("content", "{}"))
            if data.get("success"):
                all_failed = False
                break
        except (json.JSONDecodeError, TypeError):
            continue

    if all_failed:
        log.warning("Consecutive failure pattern detected for model %s", model)
        await store.store_document(
            kind="team_pattern",
            title=f"{model} consecutive failure detected — fallback recommended",
            content=(
                f"{model} has failed {_CONSECUTIVE_FAIL_THRESHOLD} times consecutively. "
                f"Consider using a fallback model instead."
            ),
            tags=["routing", "failure_pattern", f"model:{model}"],
            project=project,
        )


# ── Stats ────────────────────────────────────────────────────


async def stats(
    store,
    model: str | None = None,
    level: str | None = None,
    days: int = 30,
    project: str | None = None,
) -> dict:
    """Aggregate routing statistics from SQLite directly."""
    db = store._sqlite

    conditions = ["kind = 'routing_log'", "archived = 0"]
    params: list = []

    if days:
        from datetime import timedelta

        cutoff_date = (datetime.now(timezone.utc) - timedelta(days=days)).isoformat()
        conditions.append("created_at >= ?")
        params.append(cutoff_date)

    if project:
        conditions.append("(project = ? OR project IS NULL)")
        params.append(project)

    where = " AND ".join(conditions)
    cursor = await db.execute(
        f"SELECT content, tags FROM documents WHERE {where} ORDER BY created_at DESC",
        params,
    )
    rows = await cursor.fetchall()

    # Aggregate
    agg: dict[str, dict[str, dict]] = {}  # model -> level -> stats

    for row in rows:
        try:
            data = json.loads(row["content"])
        except (json.JSONDecodeError, TypeError):
            continue

        row_model = data.get("model", "unknown")
        row_level = data.get("level", "?")

        # Apply filters
        if model and row_model != model:
            continue
        if level and row_level != level:
            continue

        if row_model not in agg:
            agg[row_model] = {}
        if row_level not in agg[row_model]:
            agg[row_model][row_level] = {
                "success": 0,
                "fail": 0,
                "durations": [],
            }

        bucket = agg[row_model][row_level]
        if data.get("success"):
            bucket["success"] += 1
        else:
            bucket["fail"] += 1

        d = data.get("duration_s")
        if d is not None:
            bucket["durations"].append(d)

    # Format output
    result = {}
    for m, levels in agg.items():
        model_result = {}
        for lv, s in levels.items():
            total = s["success"] + s["fail"]
            durations = sorted(s["durations"])
            p90 = durations[int(len(durations) * 0.9)] if durations else None
            avg = int(sum(durations) / len(durations)) if durations else None

            model_result[lv] = {
                "total": total,
                "success": s["success"],
                "success_rate": round(s["success"] / total, 2) if total else 0,
                "avg_duration_s": avg,
                "p90_duration_s": p90,
            }
        result[m] = model_result

    return result
