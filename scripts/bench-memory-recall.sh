#!/bin/bash
# Recall@5 benchmark for rin-memory search quality (Go).
# Runs Go test against production PostgreSQL DB.
# Outputs a single integer 0-100 (higher = better recall).
set -euo pipefail
cd "$(dirname "$0")/../src/rin_memory_go"
go test -run TestRecall -count=1 2>&1 > /dev/null
cat /tmp/rin-memory-recall-score.txt
