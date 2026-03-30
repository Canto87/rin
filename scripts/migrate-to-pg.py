#!/usr/bin/env python3
"""Migrate rin-memory data from SQLite+LanceDB to PostgreSQL.

Usage:
    python3 scripts/migrate-to-pg.py setup       # Create DB + apply schema
    python3 scripts/migrate-to-pg.py check       # Validate data, show stats
    python3 scripts/migrate-to-pg.py dry-run     # Simulate migration
    python3 scripts/migrate-to-pg.py migrate     # Execute migration
    python3 scripts/migrate-to-pg.py rollback    # Clear PostgreSQL data

Env:
    RIN_MEMORY_DSN  PostgreSQL DSN (default: dbname=rin_memory)
"""
import argparse
import json
import os
import sqlite3
import sys

try:
    import psycopg2
except ImportError:
    print("Error: psycopg2 required. Install: pip install psycopg2-binary")
    sys.exit(1)

try:
    import lancedb
except ImportError:
    lancedb = None

SQLITE_PATH = os.path.expanduser("~/.rin/memory.db")
LANCE_PATH = os.path.expanduser("~/.rin/vectors")
SCHEMA_PATH = os.path.join(os.path.dirname(__file__), "..", "src", "rin_memory_go", "schema.sql")
PG_DSN = os.environ.get("RIN_MEMORY_DSN", "dbname=rin_memory")
PG_DBNAME = "rin_memory"


def get_sqlite():
    conn = sqlite3.connect(SQLITE_PATH)
    conn.row_factory = sqlite3.Row
    return conn


def get_pg():
    return psycopg2.connect(PG_DSN)


def parse_tags(tags_json):
    """Convert JSON text array to Python list for PG TEXT[]."""
    if not tags_json:
        return None
    try:
        tags = json.loads(tags_json)
        return tags if tags else None
    except (json.JSONDecodeError, TypeError):
        return None


def cmd_setup(args):
    """Create PostgreSQL database and apply schema."""
    import subprocess

    # Create database (ignore if exists)
    result = subprocess.run(
        ["createdb", PG_DBNAME],
        capture_output=True, text=True,
    )
    if result.returncode == 0:
        print(f"Created database '{PG_DBNAME}'")
    elif "already exists" in result.stderr:
        print(f"Database '{PG_DBNAME}' already exists")
    else:
        print(f"createdb failed: {result.stderr.strip()}")
        sys.exit(1)

    # Apply schema
    schema = os.path.abspath(SCHEMA_PATH)
    if not os.path.exists(schema):
        print(f"Schema not found: {schema}")
        sys.exit(1)

    result = subprocess.run(
        ["psql", PG_DBNAME, "-f", schema],
        capture_output=True, text=True,
    )
    if result.returncode == 0:
        print(f"Schema applied from {schema}")
    else:
        # Schema may partially fail (e.g. extensions already exist) — show but don't abort
        if result.stderr:
            for line in result.stderr.strip().split("\n"):
                if "NOTICE" in line or "already exists" in line.lower():
                    continue
                print(f"  Warning: {line}")
        print("Schema applied (check warnings above)")


def cmd_check(args):
    """Validate SQLite data and show stats."""
    sq = get_sqlite()
    c = sq.cursor()

    c.execute("SELECT COUNT(*) FROM documents")
    total = c.fetchone()[0]
    c.execute("SELECT COUNT(*) FROM documents WHERE archived = 0")
    active = c.fetchone()[0]
    c.execute("SELECT COUNT(*) FROM relations")
    rels = c.fetchone()[0]

    print(f"=== SQLite: {SQLITE_PATH} ===")
    print(f"Documents: {total} (active: {active}, archived: {total - active})")
    print(f"Relations: {rels}")

    c.execute("SELECT kind, COUNT(*) cnt FROM documents GROUP BY kind ORDER BY cnt DESC")
    print("\nBy kind:")
    for row in c.fetchall():
        print(f"  {row[0]}: {row[1]}")

    c.execute("SELECT COALESCE(project, '(null)') p, COUNT(*) cnt FROM documents GROUP BY project ORDER BY cnt DESC")
    print("\nBy project:")
    for row in c.fetchall():
        print(f"  {row[0]}: {row[1]}")

    # Check LanceDB vectors
    if lancedb and os.path.exists(LANCE_PATH):
        try:
            db = lancedb.connect(LANCE_PATH)
            tbl = db.open_table("vectors")
            vec_count = tbl.count_rows()
            sample = tbl.head(1).to_pydict()
            dim = len(sample["vector"][0]) if sample.get("vector") else "?"
            all_data = tbl.to_arrow().to_pydict()
            unique_docs = len(set(all_data["doc_id"]))
            print(f"\n=== LanceDB: {LANCE_PATH} ===")
            print(f"Vectors: {vec_count} chunks, {unique_docs} unique docs, dim={dim}")
        except Exception as e:
            print(f"\nLanceDB: {e}")
    else:
        print(f"\nLanceDB: not available (path={LANCE_PATH}, lib={'yes' if lancedb else 'no'})")

    # Check PG
    try:
        pg = get_pg()
        pc = pg.cursor()
        pc.execute("SELECT COUNT(*) FROM documents")
        pg_total = pc.fetchone()[0]
        pc.execute("SELECT COUNT(*) FROM relations")
        pg_rels = pc.fetchone()[0]
        pc.execute("SELECT COUNT(*) FROM document_vectors")
        pg_vecs = pc.fetchone()[0]
        print("\n=== PostgreSQL ===")
        print(f"Documents: {pg_total}, Relations: {pg_rels}, Vectors: {pg_vecs}")
        pg.close()
    except Exception as e:
        print(f"\nPostgreSQL: {e}")

    sq.close()


def cmd_dry_run(args):
    """Show sample data transformation."""
    sq = get_sqlite()
    c = sq.cursor()

    c.execute("SELECT * FROM documents LIMIT 5")
    print("=== Sample Documents ===")
    for row in c.fetchall():
        tags = parse_tags(row["tags"])
        print(f"  [{row['id']}] {row['kind']}: {row['title'][:60]}")
        print(f"    tags={tags}, project={row['project']}, archived={bool(row['archived'])}")

    c.execute("SELECT * FROM relations LIMIT 5")
    print("\n=== Sample Relations ===")
    for row in c.fetchall():
        print(f"  {row['from_id']} --{row['type']}--> {row['to_id']}")

    sq.close()
    print("\nNo changes made.")


def cmd_migrate(args):
    """Execute migration."""
    sq = get_sqlite()
    sc = sq.cursor()
    pg = get_pg()
    pg.autocommit = False
    pc = pg.cursor()

    # --- Documents ---
    sc.execute(
        "SELECT id, kind, title, content, summary, tags, source, "
        "created_at, updated_at, archived, project FROM documents"
    )
    doc_ok = 0
    doc_skip = 0
    for row in sc.fetchall():
        tags = parse_tags(row["tags"])
        archived = bool(row["archived"])
        try:
            pc.execute(
                "INSERT INTO documents "
                "(id, kind, title, content, summary, tags, source, created_at, updated_at, archived, project) "
                "VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s) "
                "ON CONFLICT (id) DO NOTHING",
                (
                    row["id"], row["kind"], row["title"], row["content"],
                    row["summary"], tags, row["source"],
                    row["created_at"], row["updated_at"], archived, row["project"],
                ),
            )
            doc_ok += 1
        except Exception as e:
            print(f"  Skip doc {row['id']}: {e}")
            pg.rollback()
            doc_skip += 1

    pg.commit()
    print(f"Documents: {doc_ok} migrated, {doc_skip} skipped")

    # --- Relations ---
    sc.execute("SELECT from_id, to_id, type FROM relations")
    rel_ok = 0
    for row in sc.fetchall():
        try:
            pc.execute(
                "INSERT INTO relations (from_id, to_id, rel_type, created_at) "
                "VALUES (%s,%s,%s,NOW()) ON CONFLICT (from_id, to_id) DO NOTHING",
                (row["from_id"], row["to_id"], row["type"]),
            )
            rel_ok += 1
        except Exception as e:
            print(f"  Skip relation: {e}")
            pg.rollback()

    pg.commit()
    print(f"Relations: {rel_ok} migrated")

    # --- AGE Graph ---
    print("Reconstructing AGE graph...")
    # Use autocommit for AGE operations — each Cypher MERGE is independent
    pg.autocommit = True
    try:
        pc.execute("LOAD 'age'")
        pc.execute('SET search_path = ag_catalog, "$user", public')

        # Vertices: all document IDs involved in relations
        pc.execute("SELECT DISTINCT id FROM documents WHERE id IN "
                    "(SELECT from_id FROM relations UNION SELECT to_id FROM relations)")
        vertex_ids = [r[0] for r in pc.fetchall()]
        v_ok = 0
        for doc_id in vertex_ids:
            try:
                pc.execute(
                    f"SELECT * FROM ag_catalog.cypher('rin_memory', $$ "
                    f"MERGE (n:doc {{id: '{doc_id}'}}) RETURN n "
                    f"$$) AS (v ag_catalog.agtype)"
                )
                v_ok += 1
            except Exception as e:
                print(f"  Skip vertex {doc_id}: {e}")

        print(f"  Vertices: {v_ok}/{len(vertex_ids)}")

        # Edges
        pc.execute("SELECT from_id, to_id, rel_type FROM relations")
        edges = pc.fetchall()
        edge_ok = 0
        for from_id, to_id, rel_type in edges:
            try:
                pc.execute(
                    f"SELECT * FROM ag_catalog.cypher('rin_memory', $$ "
                    f"MATCH (a:doc {{id: '{from_id}'}}), (b:doc {{id: '{to_id}'}}) "
                    f"MERGE (a)-[r:{rel_type}]->(b) RETURN r "
                    f"$$) AS (r ag_catalog.agtype)"
                )
                edge_ok += 1
            except Exception as e:
                print(f"  Skip edge {from_id}->{to_id}: {e}")

        print(f"  Edges: {edge_ok}/{len(edges)}")
    except Exception as e:
        print(f"AGE graph failed (non-fatal): {e}")

    pg.autocommit = False

    # --- Vectors from LanceDB ---
    pc.execute("SET search_path = public")
    vec_ok = 0
    if lancedb and os.path.exists(LANCE_PATH):
        print("Migrating vectors from LanceDB...")
        try:
            db = lancedb.connect(LANCE_PATH)
            tbl = db.open_table("vectors")
            data = tbl.to_arrow().to_pydict()

            # Verify all doc_ids exist in PG before inserting vectors
            pc.execute("SELECT id FROM documents")
            pg_doc_ids = {r[0] for r in pc.fetchall()}

            batch = []
            skipped_orphan = 0
            n = len(data["doc_id"])
            for i in range(n):
                doc_id = data["doc_id"][i]
                if doc_id not in pg_doc_ids:
                    skipped_orphan += 1
                    continue
                chunk_idx = int(data["chunk_index"][i])
                vec = data["vector"][i]
                # Convert to pgvector literal: [0.1,0.2,...]
                vec_literal = "[" + ",".join(str(float(v)) for v in vec) + "]"
                batch.append((doc_id, chunk_idx, vec_literal))

            # Bulk insert with execute_batch for performance
            from psycopg2.extras import execute_batch
            execute_batch(
                pc,
                "INSERT INTO document_vectors (doc_id, chunk_index, embedding) "
                "VALUES (%s, %s, %s::vector) "
                "ON CONFLICT (doc_id, chunk_index) DO NOTHING",
                batch,
                page_size=100,
            )
            pg.commit()
            vec_ok = len(batch)
            if skipped_orphan:
                print(f"  Skipped {skipped_orphan} orphan vectors (doc_id not in PG)")
            print(f"Vectors: {vec_ok} migrated")
        except Exception as e:
            pg.rollback()
            print(f"Vector migration failed: {e}")
            print("  Fallback: cd src/rin_memory_go && go run . reembed")
    else:
        print("LanceDB not available, skipping vector migration.")
        print("  Fallback: cd src/rin_memory_go && go run . reembed")

    # --- Verify ---
    pc.execute("SET search_path = public")
    pc.execute("SELECT COUNT(*) FROM documents")
    pg_docs = pc.fetchone()[0]
    pc.execute("SELECT COUNT(*) FROM relations")
    pg_rels = pc.fetchone()[0]
    pc.execute("SELECT COUNT(*) FROM document_vectors")
    pg_vecs = pc.fetchone()[0]
    print(f"\nPostgreSQL totals: {pg_docs} documents, {pg_rels} relations, {pg_vecs} vectors")

    sq.close()
    pg.close()


def cmd_rollback(args):
    """Clear all PostgreSQL data."""
    pg = get_pg()
    pc = pg.cursor()

    pc.execute("SELECT COUNT(*) FROM documents")
    count = pc.fetchone()[0]
    if count == 0:
        print("PostgreSQL is already empty.")
        pg.close()
        return

    if not args.yes:
        ans = input(f"Delete {count} documents from PostgreSQL? (yes/no): ")
        if ans != "yes":
            print("Aborted.")
            pg.close()
            return

    pc.execute("DELETE FROM document_vectors")
    pc.execute("DELETE FROM relations")
    pc.execute("DELETE FROM documents")
    pg.commit()

    # Reset AGE graph
    try:
        pc.execute("LOAD 'age'")
        pc.execute('SET search_path = ag_catalog, "$user", public')
        pc.execute("SELECT ag_catalog.drop_graph('rin_memory', true)")
        pc.execute("SELECT ag_catalog.create_graph('rin_memory')")
        pg.commit()
    except Exception:
        pg.rollback()

    print(f"Deleted {count} documents + relations + vectors + graph.")
    pg.close()


def main():
    p = argparse.ArgumentParser(description="Migrate rin-memory SQLite → PostgreSQL")
    sub = p.add_subparsers(dest="command")

    sub.add_parser("setup", help="Create DB and apply schema")
    sub.add_parser("check", help="Validate data and show stats")
    sub.add_parser("dry-run", help="Simulate migration")
    sub.add_parser("migrate", help="Execute migration")
    rb = sub.add_parser("rollback", help="Clear PostgreSQL data")
    rb.add_argument("-y", "--yes", action="store_true", help="Skip confirmation")

    args = p.parse_args()
    cmds = {
        "setup": cmd_setup,
        "check": cmd_check,
        "dry-run": cmd_dry_run,
        "migrate": cmd_migrate,
        "rollback": cmd_rollback,
    }

    if args.command in cmds:
        cmds[args.command](args)
    else:
        p.print_help()


if __name__ == "__main__":
    main()
