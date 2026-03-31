#!/bin/bash
# sync-harness.sh — Sync harness files from project-rin to target project
# Usage: ./scripts/sync-harness.sh [--dry-run] [--init] [--global] <target-project-path>
#
# Syncs:
#   agents/*.md                → target/.claude/agents/          (overwrite)
#   skills/*/*.md              → target/.claude/skills/*/        (overwrite)
#   skills/*/templates/*       → target/.claude/skills/*/templates/ (overwrite)
#   commands/*.md              → target/.claude/commands/         (overwrite)
#
# --global:
#   Deploy to ~/.claude/ instead of a project directory.
#   Makes agents/skills/commands available across all projects.
#   No <target-project-path> needed.
#
# --init:
#   Also copies config templates (config.yaml, config.project.yaml) for skills
#   that need them. Skips if config already exists in target.
#
# Does NOT sync:
#   config.yaml, config.project.yaml (project-specific, unless --init)
#   commands/rin/* (rin-specific commands)
#   Any files only in target (preserves project-specific files)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SOURCE_DIR="$(dirname "$SCRIPT_DIR")/.claude"
SOURCE_ROOT="$(dirname "$SCRIPT_DIR")"
SYNC_MARKER="<!-- Synced from project-rin. Do not edit. -->"
SYNC_STATE_FILE=".sync-state"

DRY_RUN=false
INIT=false
GLOBAL=false
TARGET=""

# Parse args
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run) DRY_RUN=true; shift ;;
        --init) INIT=true; shift ;;
        --global) GLOBAL=true; shift ;;
        *) TARGET="$1"; shift ;;
    esac
done

if $GLOBAL; then
    TARGET="$HOME"
    TARGET_CLAUDE="$HOME/.claude"
elif [[ -z "$TARGET" ]]; then
    echo "Usage: $0 [--dry-run] [--init] [--global] <target-project-path>"
    exit 1
else
    TARGET_CLAUDE="$TARGET/.claude"
    if [[ ! -d "$TARGET" ]]; then
        echo "Error: Target directory does not exist: $TARGET"
        exit 1
    fi
fi

# Counters
synced=0
skipped=0
created_dirs=0
configs_created=0

sync_file() {
    # Temporarily disable pipefail inside this function — head/tail close pipes
    # early causing SIGPIPE (exit 141) with pipefail enabled.
    set +o pipefail
    local src="$1"
    local dst="$2"
    local dst_dir="$(dirname "$dst")"

    if [[ ! -d "$dst_dir" ]]; then
        if $DRY_RUN; then
            echo "  [mkdir] $dst_dir"
        else
            mkdir -p "$dst_dir"
        fi
        created_dirs=$((created_dirs + 1))
    fi

    if $DRY_RUN; then
        if [[ -f "$dst" ]]; then
            echo "  [update] $dst"
        else
            echo "  [create] $dst"
        fi
    else
        # Strip existing marker from source
        local clean
        clean="$(sed '1{/^<!-- Synced from project-rin/d;}' "$src" | sed '1{/^<!-- Managed by project-rin/d;}')"

        local first_line_src first_line_clean
        first_line_src="$(head -1 "$src")"
        first_line_clean="$(printf '%s' "$clean" | head -1)"
        if [[ "$first_line_src" == "---" ]] || [[ "$first_line_clean" == "---" ]]; then
            # File has YAML frontmatter — insert marker after closing ---
            # to avoid breaking frontmatter parser
            local in_front=true
            local front_end=0
            local line_num=0
            while IFS= read -r line; do
                line_num=$((line_num + 1))
                if [[ $line_num -eq 1 ]]; then
                    continue  # skip opening ---
                fi
                if $in_front && [[ "$line" == "---" ]]; then
                    front_end=$line_num
                    break
                fi
            done <<< "$clean"

            if [[ $front_end -gt 0 ]]; then
                {
                    printf '%s\n' "$clean" | head -n "$front_end" || true
                    echo "$SYNC_MARKER"
                    printf '%s\n' "$clean" | tail -n +"$((front_end + 1))" || true
                } > "$dst"
            else
                # Malformed frontmatter — prepend as fallback
                { echo "$SYNC_MARKER"; echo "$clean"; } > "$dst"
            fi
        else
            # No frontmatter — prepend marker as before
            { echo "$SYNC_MARKER"; echo "$clean"; } > "$dst"
        fi
    fi
    synced=$((synced + 1))
    set -o pipefail
}

# Copy config template if target doesn't have one
init_config() {
    local src="$1"
    local dst="$2"
    local dst_dir="$(dirname "$dst")"

    if [[ -f "$dst" ]]; then
        echo "  [skip] $dst (already exists)"
        skipped=$((skipped + 1))
        return
    fi

    if [[ ! -d "$dst_dir" ]]; then
        if $DRY_RUN; then
            echo "  [mkdir] $dst_dir"
        else
            mkdir -p "$dst_dir"
        fi
        created_dirs=$((created_dirs + 1))
    fi

    if $DRY_RUN; then
        echo "  [init] $dst"
    else
        cp "$src" "$dst"
    fi
    configs_created=$((configs_created + 1))
}

echo "=== sync-harness ==="
echo "Source: $SOURCE_DIR"
echo "Target: $TARGET_CLAUDE"
$DRY_RUN && echo "Mode: DRY RUN"
$INIT && echo "Mode: INIT (config templates)"
echo ""

# 1. Sync agents
echo "--- Agents ---"
for f in "$SOURCE_DIR"/agents/*.md; do
    [[ -f "$f" ]] || continue
    basename="$(basename "$f")"
    sync_file "$f" "$TARGET_CLAUDE/agents/$basename"
done

# 2. Sync skills (*.md + templates/*, skip config files)
echo "--- Skills ---"
for skill_dir in "$SOURCE_DIR"/skills/*/; do
    [[ -d "$skill_dir" ]] || continue
    skill_name="$(basename "$skill_dir")"

    # Sync all .md files (skill.md, questions.md, etc.) — skip config*
    for f in "$skill_dir"*.md; do
        [[ -f "$f" ]] || continue
        sync_file "$f" "$TARGET_CLAUDE/skills/$skill_name/$(basename "$f")"
    done

    # Sync templates/ subdirectory if exists
    if [[ -d "$skill_dir/templates" ]]; then
        for f in "$skill_dir"templates/*; do
            [[ -f "$f" ]] || continue
            sync_file "$f" "$TARGET_CLAUDE/skills/$skill_name/templates/$(basename "$f")"
        done
    fi
done

# 3. Sync commands (exclude rin/ subdirectory)
echo "--- Commands ---"
for f in "$SOURCE_DIR"/commands/*.md; do
    [[ -f "$f" ]] || continue
    basename="$(basename "$f")"
    sync_file "$f" "$TARGET_CLAUDE/commands/$basename"
done

# 4. Init config templates (--init only)
if $INIT; then
    echo ""
    echo "--- Config Init ---"
    for skill_dir in "$SOURCE_DIR"/skills/*/; do
        [[ -d "$skill_dir" ]] || continue
        skill_name="$(basename "$skill_dir")"

        for f in "$skill_dir"config*.yaml "$skill_dir"config*.yml; do
            [[ -f "$f" ]] || continue
            init_config "$f" "$TARGET_CLAUDE/skills/$skill_name/$(basename "$f")"
        done
    done
fi

# 5. Write sync state to target
if ! $DRY_RUN; then
    source_commit="$(git -C "$SOURCE_ROOT" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    cat > "$TARGET_CLAUDE/$SYNC_STATE_FILE" <<EOF
# Harness sync state — auto-generated by sync-harness.sh
source: project-rin
commit: $source_commit
synced_at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
files_synced: $synced
init: $INIT
EOF
    echo ""
    echo "--- Sync State ---"
    echo "  [write] $TARGET_CLAUDE/$SYNC_STATE_FILE (commit: $source_commit)"
fi

echo ""
echo "=== Summary ==="
echo "Synced: $synced files"
if $INIT; then echo "Configs created: $configs_created (skipped: $skipped)"; fi
echo "Directories created: $created_dirs"
if $DRY_RUN; then echo "(dry run — no files were actually modified)"; fi
