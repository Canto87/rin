#!/usr/bin/env python3
"""PostToolUse hook — auto routing_log for Task completions.

Runs on every Task tool completion. Extracts model, subagent_type,
and success from the hook payload, then writes a routing_log entry
to PostgreSQL via the rin-memory-go binary (insert-log subcommand).

Errors are silently caught — hook failure must not block Claude Code.
"""

import json
import os
import subprocess
import sys

raw = sys.stdin.read()
try:
    data = json.loads(raw)
except json.JSONDecodeError:
    json.dump({"continue": True}, sys.stdout)
    sys.exit(0)

tool_name = data.get("tool_name", "")
if tool_name != "Task":
    json.dump({"continue": True}, sys.stdout)
    sys.exit(0)

# Extract from tool_input
tool_input = data.get("tool_input", {})
if isinstance(tool_input, str):
    try:
        tool_input = json.loads(tool_input)
    except json.JSONDecodeError:
        tool_input = {}

model = tool_input.get("model", "sonnet")
subagent_type = tool_input.get("subagent_type", "general-purpose")
prompt = tool_input.get("prompt", "")[:100]
run_in_bg = tool_input.get("run_in_background", False)

# Detect success from tool_response
tool_response = data.get("tool_response", "")
response_str = str(tool_response).lower()[:500]
success = "error" not in response_str and "failed" not in response_str

# Build routing log content
log_content = json.dumps(
    {
        "task_description": prompt,
        "model": model,
        "level": "L1",
        "mode": "solo",
        "agent_count": 1,
        "success": success,
        "subagent_type": subagent_type,
        "duration_s": None,
        "background": run_in_bg,
        "source": "auto:hook",
    },
    ensure_ascii=False,
)

# Write to PostgreSQL via Go binary
go_bin = os.path.join(
    os.environ.get("RIN_HOME", os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
    "src", "rin_memory_go", "rin-memory-go",
)
if os.path.exists(go_bin):
    try:
        env = dict(os.environ)
        subprocess.run(
            [go_bin, "insert-log"],
            input=log_content.encode(),
            timeout=5,
            capture_output=True,
            env=env,
        )
    except Exception:
        pass

json.dump({"continue": True}, sys.stdout)
