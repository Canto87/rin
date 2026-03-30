#!/usr/bin/env python3
"""Recall@5 benchmark for rin-memory search quality.

Golden test set: real usage patterns from Rin's workflow.
Each test has a query, optional filters, and expected document IDs.
Score = (hits in top 5 / total expected) * 100.

Deterministic: same DB + same query + same embedding = same results.
"""
import asyncio
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from rin_memory.store import MemoryStore

# Golden test set: {query, expected IDs, optional kind/project filters}
# Mix of currently-passing and currently-failing queries.
GOLDEN_TESTS = [
    # --- Exact keyword matches (should pass) ---
    {
        "query": "brain SQLite fallback environment variable",
        "expected": ["ea674c0a7d23"],
        "note": "error_pattern: Brain restart SQLite fallback",
    },
    {
        "query": "memory_store project slug mismatch search miss",
        "expected": ["f6c5248410b4"],
        "note": "error_pattern: project slug mismatch",
    },
    {
        "query": "auto-research autonomous experiment design",
        "expected": ["c9643bb140a3"],
        "note": "arch_decision: auto-research skill design",
    },
    {
        "query": "Gemini CLI flat-rate token saving",
        "expected": ["72fe4543ab92"],
        "note": "preference: Gemini CLI usage rules",
    },
    {
        "query": "rin-memory memory_update project parameter workaround",
        "expected": ["98a1041f7337"],
        "note": "domain_knowledge: memory_update workaround",
    },

    # --- Semantic / conceptual queries (currently failing) ---
    {
        "query": "Opus don't edit code directly delegate instead",
        "expected": ["e72a946bedc8"],
        "note": "arch_decision: Opus as orchestrator, delegate edits",
    },
    {
        "query": "RRF score cosine similarity threshold blocking",
        "expected": ["9286c73a29fe"],
        "note": "error_pattern: RRF score range mismatch",
    },

    # --- Kind-filtered queries ---
    {
        "query": "multi-agent routing Level system",
        "kind": "arch_decision",
        "expected": ["e72a946bedc8"],
        "note": "arch_decision: multi-agent routing rules",
    },

    # --- Combined semantic + keyword ---
    {
        "query": "thinking loop timeout round counter",
        "expected": ["1506976bfa45"],
        "note": "arch_decision: thinking loop timeout design",
    },
    {
        "query": "transcript auto-collection session harvest launchd",
        "expected": ["18a8897170d4"],
        "note": "arch_decision: session harvest pipeline",
    },
    {
        "query": "webhook bridge timeout SSE",
        "expected": ["b55f8a8b33f8"],
        "note": "session_summary: webhook bridge timeout fix",
    },
]

K = 5  # Recall@K


async def main():
    db_dir = os.environ.get("RIN_MEMORY_DIR", os.path.expanduser("~/.rin"))
    store = MemoryStore(
        db_path=os.path.join(db_dir, "memory.db"),
        lance_path=os.path.join(db_dir, "vectors"),
    )
    await store.initialize()

    hits = 0
    total = 0
    details = []

    for test in GOLDEN_TESTS:
        results = await store.search(
            test["query"],
            kind=test.get("kind"),
            project=test.get("project"),
            limit=K,
        )
        result_ids = {r["id"] for r in results}

        for expected_id in test["expected"]:
            total += 1
            found = expected_id in result_ids
            if found:
                hits += 1
            details.append((found, test["query"][:40], expected_id[:12]))

    await store.close()

    score = int(100 * hits / total) if total else 0

    # Verbose output to stderr for debugging
    if os.environ.get("VERBOSE"):
        for found, query, eid in details:
            status = "HIT " if found else "MISS"
            print(f"  {status} q={query:<40s} expected={eid}", file=sys.stderr)
        print(f"\nRecall@{K}: {hits}/{total} = {score}%", file=sys.stderr)

    # Single number to stdout (for auto-research metric)
    print(score)


if __name__ == "__main__":
    asyncio.run(main())
