"""Tests for model routing with experience-based suggestions."""

import json
from unittest.mock import AsyncMock, patch

import pytest

from rin_memory.routing import classify_level, log_routing, stats, suggest
from rin_memory.store import MemoryStore


@pytest.fixture
async def store(tmp_path):
    """Create a MemoryStore with temp DB and mocked embedding."""
    db_path = str(tmp_path / "test.db")
    lance_path = str(tmp_path / "vectors")

    s = MemoryStore(db_path, lance_path)
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await s.initialize()
        yield s
    await s.close()


# ── classify_level ───────────────────────────────────────────


def test_classify_level_l1():
    assert classify_level(file_count=1) == "L1"


def test_classify_level_l1_default():
    assert classify_level() == "L1"


def test_classify_level_l2():
    assert classify_level(file_count=2) == "L2"


def test_classify_level_l2_boundary():
    assert classify_level(file_count=3) == "L2"


def test_classify_level_l3_by_files():
    assert classify_level(file_count=4) == "L3"


def test_classify_level_l3_by_dependencies():
    assert classify_level(file_count=1, has_dependencies=True) == "L3"


def test_classify_level_l3_by_design():
    assert classify_level(file_count=1, needs_design=True) == "L3"


def test_classify_level_l3_combined():
    assert classify_level(file_count=5, has_dependencies=True, needs_design=True) == "L3"


# ── routing log ──────────────────────────────────────────────


async def test_routing_log_creates_document(store):
    """Log should create a routing_log document with correct tags."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        doc_id = await log_routing(
            store,
            task="Add vector delete method",
            model="glm-5",
            duration_s=45,
            success=True,
            level="L1",
        )

    assert doc_id  # non-empty string

    docs = await store.lookup(kind="routing_log", detail="full")
    assert len(docs) == 1

    tags = docs[0]["tags"]
    assert "model:glm-5" in tags
    assert "level:L1" in tags
    assert "success" in tags
    assert "mode:solo" in tags

    data = json.loads(docs[0]["content"])
    assert data["task_description"] == "Add vector delete method"
    assert data["duration_s"] == 45
    assert data["success"] is True


async def test_routing_log_failure_tags(store):
    """Failed log should have 'failure' tag."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await log_routing(
            store,
            task="Fix parser bug",
            model="glm-5",
            duration_s=120,
            success=False,
            error_type="timeout",
        )

    docs = await store.lookup(kind="routing_log", detail="full")
    assert len(docs) == 1
    assert "failure" in docs[0]["tags"]
    assert "error:timeout" in docs[0]["tags"]


async def test_routing_log_auto_classifies_level(store):
    """Level should auto-classify based on files_changed when not provided."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await log_routing(
            store,
            task="Multi-file refactor",
            model="glm-5",
            duration_s=90,
            success=True,
            files_changed=3,
        )

    docs = await store.lookup(kind="routing_log", detail="full")
    data = json.loads(docs[0]["content"])
    assert data["level"] == "L2"  # 3 files -> L2


# ── suggest ──────────────────────────────────────────────────


async def test_suggest_no_history(store):
    """With no history, suggest should return defaults with 0 confidence."""
    result = await suggest(store, task="Fix bug in parser", file_count=1)

    assert result["level"] == "L1"
    assert result["confidence"] == 0.0
    assert result["model"] == "glm-5"
    assert result["reason"] == "no history — using defaults"
    assert result["history"] == []


async def test_suggest_with_history(store):
    """With history, suggest should return experience-based recommendation."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        for i in range(5):
            await log_routing(
                store,
                task=f"Fix parser issue {i}",
                model="glm-5",
                duration_s=30 + i,
                success=True,
                level="L1",
            )

        result = await suggest(store, task="Fix parser bug", file_count=1)

    assert result["confidence"] > 0
    assert result["model"] == "glm-5"
    assert len(result["history"]) > 0


async def test_suggest_prefers_higher_success_rate(store):
    """Suggest should prefer model with higher success rate."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        # glm-5: 2 success, 3 fail = 40%
        for i in range(2):
            await log_routing(store, f"task {i}", "glm-5", 30, True, "L1")
        for i in range(3):
            await log_routing(store, f"task fail {i}", "glm-5", 30, False, "L1")

        # sonnet: 4 success, 1 fail = 80%
        for i in range(4):
            await log_routing(store, f"task s {i}", "sonnet-code-edit", 50, True, "L1")
        await log_routing(store, "task s fail", "sonnet-code-edit", 50, False, "L1")

        result = await suggest(store, task="similar task", file_count=1)

    assert result["model"] == "sonnet-code-edit"


# ── stats ────────────────────────────────────────────────────


async def test_routing_stats_aggregation(store):
    """Stats should aggregate by model and level."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await log_routing(store, "task1", "glm-5", 30, True, "L1")
        await log_routing(store, "task2", "glm-5", 45, True, "L2")
        await log_routing(store, "task3", "glm-5", 60, False, "L2")
        await log_routing(store, "task4", "sonnet-code-edit", 40, True, "L1")

    result = await stats(store, days=30)

    assert "glm-5" in result
    assert "sonnet-code-edit" in result

    glm = result["glm-5"]
    assert "L1" in glm
    assert "L2" in glm
    assert glm["L1"]["total"] == 1
    assert glm["L1"]["success"] == 1
    assert glm["L2"]["total"] == 2
    assert glm["L2"]["success_rate"] == 0.5


async def test_routing_stats_model_filter(store):
    """Stats with model filter should only return that model."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await log_routing(store, "task1", "glm-5", 30, True, "L1")
        await log_routing(store, "task2", "sonnet-code-edit", 40, True, "L1")

    result = await stats(store, model="glm-5", days=30)

    assert "glm-5" in result
    assert "sonnet-code-edit" not in result


# ── failure pattern detection ────────────────────────────────


async def test_failure_pattern_detection(store):
    """3 consecutive failures should create a team_pattern warning."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        for i in range(3):
            await log_routing(
                store,
                task=f"failing task {i}",
                model="glm-5",
                duration_s=120,
                success=False,
                error_type="timeout",
            )

    patterns = await store.lookup(kind="team_pattern", detail="full")
    assert len(patterns) >= 1
    assert any("consecutive failure" in p["title"] for p in patterns)


async def test_no_pattern_on_mixed_results(store):
    """Mixed success/failure should not trigger pattern."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await log_routing(store, "task1", "glm-5", 30, True, "L1")
        await log_routing(store, "task2", "glm-5", 120, False, "L1")
        await log_routing(store, "task3", "glm-5", 120, False, "L1")

    patterns = await store.lookup(kind="team_pattern")
    assert len(patterns) == 0
