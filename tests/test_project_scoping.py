"""Tests for project-scoped memory."""

import os
from unittest.mock import AsyncMock, patch

import pytest

from rin_memory.schema import SCHEMA_VERSION, init_db
from rin_memory.server import _resolve_project
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


# ── Schema migration ─────────────────────────────────────────


async def test_fresh_db_has_project_column(tmp_path):
    """Fresh DB should have project column and version 2."""
    import aiosqlite

    db_path = str(tmp_path / "fresh.db")
    db = await init_db(db_path)

    cursor = await db.execute("PRAGMA table_info(documents)")
    columns = {row[1] for row in await cursor.fetchall()}
    assert "project" in columns

    cursor = await db.execute("SELECT value FROM metadata WHERE key = 'schema_version'")
    row = await cursor.fetchone()
    assert int(row[0]) == SCHEMA_VERSION

    await db.close()


async def test_migration_v1_to_v2(tmp_path):
    """Existing v1 DB should get project column via migration."""
    import aiosqlite

    db_path = str(tmp_path / "v1.db")

    # Create a v1 database manually
    db = await aiosqlite.connect(db_path)
    await db.executescript("""
        CREATE TABLE IF NOT EXISTS documents (
            id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL,
            content TEXT NOT NULL, summary TEXT, tags TEXT, source TEXT,
            created_at TEXT NOT NULL DEFAULT (datetime('now')),
            updated_at TEXT, archived INTEGER DEFAULT 0
        );
        CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
        INSERT INTO metadata (key, value) VALUES ('schema_version', '1');
        INSERT INTO documents (id, kind, title, content, created_at)
        VALUES ('test123', 'session_journal', 'Old doc', 'Content', '2026-01-01');
    """)
    await db.commit()
    await db.close()

    # Run init_db which should migrate
    db = await init_db(db_path)

    cursor = await db.execute("PRAGMA table_info(documents)")
    columns = {row[1] for row in await cursor.fetchall()}
    assert "project" in columns

    # Old doc should still be there with NULL project
    cursor = await db.execute("SELECT project FROM documents WHERE id = 'test123'")
    row = await cursor.fetchone()
    assert row[0] is None

    cursor = await db.execute("SELECT value FROM metadata WHERE key = 'schema_version'")
    row = await cursor.fetchone()
    assert int(row[0]) == SCHEMA_VERSION

    await db.close()


# ── Store with project ────────────────────────────────────────


async def test_store_with_project(store):
    """Documents stored with project should have it set."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        doc_id = await store.store_document(
            kind="arch_decision",
            title="Test doc",
            content="Content",
            project="project_rin",
        )

    cursor = await store._sqlite.execute(
        "SELECT project FROM documents WHERE id = ?", (doc_id,)
    )
    row = await cursor.fetchone()
    assert row["project"] == "project_rin"


async def test_store_without_project(store):
    """Documents stored without project should have NULL."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        doc_id = await store.store_document(
            kind="arch_decision",
            title="Global doc",
            content="Content",
        )

    cursor = await store._sqlite.execute(
        "SELECT project FROM documents WHERE id = ?", (doc_id,)
    )
    row = await cursor.fetchone()
    assert row["project"] is None


# ── Lookup with project filter ────────────────────────────────


async def test_lookup_filters_by_project(store):
    """lookup() with project should return matching + NULL docs."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await store.store_document(
            kind="note", title="A", content="a", project="proj_a"
        )
        await store.store_document(
            kind="note", title="B", content="b", project="proj_b"
        )
        await store.store_document(kind="note", title="C", content="c")  # NULL project

    results = await store.lookup(project="proj_a")
    titles = {r["title"] for r in results}
    assert titles == {"A", "C"}  # proj_a + NULL, not proj_b


async def test_lookup_without_project_returns_all(store):
    """lookup() without project should return all docs."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await store.store_document(
            kind="note", title="A", content="a", project="proj_a"
        )
        await store.store_document(
            kind="note", title="B", content="b", project="proj_b"
        )
        await store.store_document(kind="note", title="C", content="c")

    results = await store.lookup()
    assert len(results) == 3


async def test_lookup_project_and_kind_combined(store):
    """lookup() with project + kind should filter both."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await store.store_document(
            kind="arch_decision", title="A", content="a", project="proj"
        )
        await store.store_document(kind="note", title="B", content="b", project="proj")
        await store.store_document(
            kind="arch_decision", title="C", content="c", project="other"
        )

    results = await store.lookup(kind="arch_decision", project="proj")
    assert len(results) == 1
    assert results[0]["title"] == "A"


# ── Format includes project ──────────────────────────────────


async def test_format_document_includes_project(store):
    """_format_document should include project at summary level."""
    with patch(
        "rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024
    ):
        await store.store_document(
            kind="note", title="T", content="C", project="my_proj"
        )

    results = await store.lookup(project="my_proj", detail="summary")
    assert len(results) == 1
    assert results[0]["project"] == "my_proj"


# ── _resolve_project ─────────────────────────────────────────


def test_resolve_project_explicit():
    """Explicit project should take priority."""
    assert _resolve_project("my_project") == "my_project"


def test_resolve_project_star_means_all():
    """'*' should return None (no filter)."""
    assert _resolve_project("*") is None


def test_resolve_project_env_fallback():
    """Should fall back to RIN_PROJECT env var."""
    with patch.dict(os.environ, {"RIN_PROJECT": "env_project"}):
        assert _resolve_project() == "env_project"


def test_resolve_project_no_env():
    """No explicit, no env → None."""
    with patch.dict(os.environ, {}, clear=True):
        result = _resolve_project()
    assert result is None
