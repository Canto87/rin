"""Tests for hybrid search (vector + FTS5) and RRF merging."""

from unittest.mock import AsyncMock, patch

import pytest

from rin_memory.store import MemoryStore


@pytest.fixture
async def store(tmp_path):
    """Create a MemoryStore with temp DB and mocked embedding."""
    db_path = str(tmp_path / "test.db")
    lance_path = str(tmp_path / "vectors")

    s = MemoryStore(db_path, lance_path)
    # Mock embed and embed_batch to return deterministic vectors
    with patch("rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024):
        with patch(
            "rin_memory.store.embed_batch",
            new_callable=AsyncMock,
            return_value=[[0.1] * 1024],
        ):
            await s.initialize()
            yield s
    await s.close()


async def _store_doc(store, title, content, **kwargs):
    """Helper to store a document with mocked embeddings."""
    with patch("rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024):
        with patch(
            "rin_memory.store.embed_batch",
            new_callable=AsyncMock,
            return_value=[[0.1] * 1024],
        ):
            return await store.store_document(
                kind=kwargs.get("kind", "note"),
                title=title,
                content=content,
                **{k: v for k, v in kwargs.items() if k != "kind"},
            )


# ── FTS5 setup ──────────────────────────────────────────────────


async def test_fts5_table_created(store):
    """FTS5 virtual table should exist after init."""
    assert await store._has_fts()


async def test_fts5_keyword_search(store):
    """FTS5 should find documents by keyword."""
    await _store_doc(store, "AI named itself", "A story I read on a blog")
    await _store_doc(store, "Today's journal", "Had an ordinary day")

    results = await store._fts_search("blog", None, None, 10)
    assert len(results) > 0


async def test_fts5_no_match(store):
    """FTS5 should return empty for non-matching query."""
    await _store_doc(store, "Test doc", "Some content here")

    results = await store._fts_search("nonexistentwordxyz", None, None, 10)
    assert len(results) == 0


# ── RRF merge ──────────────────────────────────────────────────


def test_rrf_merge_both_sources():
    """Documents in both rankings should get higher score."""
    vec = {"doc_a": 1, "doc_b": 2, "doc_c": 3}
    fts = {"doc_a": 2, "doc_d": 1}

    merged = MemoryStore._rrf_merge(vec, fts)

    # doc_a appears in both → highest score
    assert merged["doc_a"] > merged["doc_b"]
    assert merged["doc_a"] > merged["doc_d"]


def test_rrf_merge_empty_fts():
    """With empty FTS, should still produce scores from vector."""
    vec = {"doc_a": 1, "doc_b": 2}
    merged = MemoryStore._rrf_merge(vec, {})
    assert "doc_a" in merged
    assert "doc_b" in merged
    assert merged["doc_a"] > merged["doc_b"]


def test_rrf_merge_empty_both():
    """Empty inputs should produce empty output."""
    assert MemoryStore._rrf_merge({}, {}) == {}


# ── Hybrid search integration ─────────────────────────────────


async def test_search_returns_results(store):
    """Hybrid search should return stored documents."""
    await _store_doc(store, "Memory system design", "Combining vector search and keyword search")

    with patch("rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024):
        results = await store.search("memory design")

    assert len(results) > 0
    assert results[0]["title"] == "Memory system design"


async def test_search_deduplicates_chunks(store):
    """A document with multiple chunks should appear once in results."""
    # Store a long document that will be chunked
    long_content = "## Part 1\n" + "keyword " * 200 + "\n## Part 2\n" + "keyword " * 200

    with patch("rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024):
        with patch(
            "rin_memory.store.embed_batch",
            new_callable=AsyncMock,
            return_value=[[0.1] * 1024, [0.1] * 1024],
        ):
            await store.store_document(kind="note", title="Chunked Doc", content=long_content)

    with patch("rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024):
        results = await store.search("keyword")

    # Should be exactly 1 result (deduplicated)
    titles = [r["title"] for r in results]
    assert titles.count("Chunked Doc") == 1


async def test_search_graceful_without_fts(store):
    """Search should work even if FTS table doesn't exist."""
    await _store_doc(store, "Test", "Content")

    # Simulate FTS not existing
    with patch.object(store, "_has_fts", new_callable=AsyncMock, return_value=False):
        with patch("rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024):
            results = await store.search("test")

    # Should still return results from vector search
    assert len(results) > 0


async def test_search_with_kind_filter(store):
    """Search with kind filter should only return matching kinds."""
    await _store_doc(store, "Arch decision", "Important choice", kind="arch_decision")
    await _store_doc(store, "Random note", "Not important", kind="note")

    with patch("rin_memory.store.embed", new_callable=AsyncMock, return_value=[0.1] * 1024):
        results = await store.search("important", kind="arch_decision")

    for r in results:
        assert r["kind"] == "arch_decision"
