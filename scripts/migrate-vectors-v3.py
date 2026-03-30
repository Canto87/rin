#!/usr/bin/env python3
"""Migration: re-chunk and re-embed all documents for hybrid search (v3).

Drops old 1-doc-1-vector LanceDB table, re-creates with chunk-level vectors.
FTS5 table is handled by schema migration in init_db().

Usage:
    .venv/bin/python scripts/migrate-vectors-v3.py
    # or: make migrate-v3
"""

import asyncio
import sys
from pathlib import Path

# Add project to path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "src"))

from rin_memory.chunking import chunk_document
from rin_memory.embedding import embed_batch
from rin_memory.schema import init_db

import aiosqlite
import lancedb

DATA_DIR = Path.home() / ".rin"
DB_PATH = DATA_DIR / "memory.db"
LANCE_PATH = DATA_DIR / "vectors"


async def main():
    if not DB_PATH.exists():
        print(f"No database found at {DB_PATH}")
        return

    # 1. Init DB (applies schema v3 migration including FTS5)
    print("Applying schema migration...")
    db = await init_db(str(DB_PATH))

    # 2. Fetch all documents
    db.row_factory = aiosqlite.Row
    cursor = await db.execute("SELECT id, title, content FROM documents WHERE archived = 0")
    rows = await cursor.fetchall()
    print(f"Found {len(rows)} documents to re-embed")

    if not rows:
        await db.close()
        return

    # 3. Drop and recreate LanceDB vectors table
    lance_db = lancedb.connect(str(LANCE_PATH))
    try:
        lance_db.drop_table("vectors")
        print("Dropped old vectors table")
    except Exception:
        print("No existing vectors table to drop")

    # 4. Chunk and embed all documents
    batch_texts = []
    batch_meta = []  # (doc_id, chunk_index)

    for row in rows:
        chunks = chunk_document(row["title"], row["content"])
        for chunk in chunks:
            batch_texts.append(chunk["text"])
            batch_meta.append((row["id"], chunk["chunk_index"]))

    print(f"Total chunks: {len(batch_texts)}")

    # Embed in batches of 32
    BATCH_SIZE = 32
    all_vectors = []
    for i in range(0, len(batch_texts), BATCH_SIZE):
        batch = batch_texts[i : i + BATCH_SIZE]
        vectors = await embed_batch(batch)
        all_vectors.extend(vectors)
        done = min(i + BATCH_SIZE, len(batch_texts))
        print(f"  Embedded {done}/{len(batch_texts)}")

    # 5. Write to LanceDB
    data = [
        {
            "doc_id": meta[0],
            "chunk_index": meta[1],
            "vector": vec,
        }
        for meta, vec in zip(batch_meta, all_vectors)
    ]

    lance_db.create_table("vectors", data=data)
    print(f"Created new vectors table with {len(data)} chunk vectors")

    await db.close()
    print("Migration complete!")


if __name__ == "__main__":
    asyncio.run(main())
