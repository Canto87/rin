#!/usr/bin/env python3
"""Sync project_rin MCP servers into ~/.claude.json global config."""

import json
import os
import sys

RIN_HOME = os.environ.get(
    "RIN_HOME", os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
)
HOME = os.path.expanduser("~")
CLAUDE_JSON = os.path.join(HOME, ".claude.json")
MCP_SOURCE = os.path.join(RIN_HOME, "config", "mcp-servers.json")

if not os.path.exists(MCP_SOURCE):
    print(f"[RIN] Error: {MCP_SOURCE} not found.")
    sys.exit(1)

# Read source MCP servers
with open(MCP_SOURCE) as f:
    mcp_servers = json.load(f)

# Replace path placeholders
raw = json.dumps(mcp_servers)
raw = raw.replace("__RIN_HOME__", RIN_HOME)
raw = raw.replace("__HOME__", HOME)
mcp_servers = json.loads(raw)

# Read or create ~/.claude.json
if os.path.exists(CLAUDE_JSON):
    with open(CLAUDE_JSON) as f:
        claude_config = json.load(f)
else:
    claude_config = {}

# Merge MCP servers
existing = claude_config.get("mcpServers", {})
added = []
updated = []
for name, config in mcp_servers.items():
    if name not in existing:
        added.append(name)
    elif existing[name] != config:
        updated.append(name)
    existing[name] = config

claude_config["mcpServers"] = existing

with open(CLAUDE_JSON, "w") as f:
    json.dump(claude_config, f, indent=2, ensure_ascii=False)

if added:
    print(f"[RIN] MCP added: {', '.join(added)}")
if updated:
    print(f"[RIN] MCP updated: {', '.join(updated)}")
if not added and not updated:
    print("[RIN] MCP servers already up to date.")
