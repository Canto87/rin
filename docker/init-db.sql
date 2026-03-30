-- Initialize rin_memory database with required extensions
-- Note: AGE is loaded via shared_preload_libraries in postgres CMD
CREATE DATABASE rin_memory;
\c rin_memory

-- pgvector for semantic search
CREATE EXTENSION IF NOT EXISTS vector;

-- trigram for fuzzy text search
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- AGE for knowledge graph (already preloaded)
CREATE EXTENSION IF NOT EXISTS age;
SET search_path = ag_catalog, "$user", public;
LOAD 'age';
