#!/usr/bin/env bash
# install-codex-skill.sh — Install the Postbrain skill into a project's .codex/skills/ directory.
#
# Usage:
#   ./scripts/install-codex-skill.sh [TARGET_DIR]
#
# TARGET_DIR defaults to the current working directory.
# The script creates .codex/skills/ inside TARGET_DIR if it doesn't exist,
# then copies .codex/skills/postbrain/SKILL.md from the Postbrain source tree.
#
# Environment variables honoured:
#   POSTBRAIN_URL    — written into TARGET_DIR/AGENTS.md hint block if not already present
#   POSTBRAIN_SCOPE  — written into TARGET_DIR/AGENTS.md hint block if not already present

set -euo pipefail

TARGET_DIR="${1:-${PWD}}"

# Prefer installed CLI when available so users do not need this repository
# checked out to install the skill.
if command -v postbrain-cli >/dev/null 2>&1; then
  exec postbrain-cli install-codex-skill --target "${TARGET_DIR}"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_SKILL="${SCRIPT_DIR}/../.codex/skills/postbrain/SKILL.md"
DEST_DIR="${TARGET_DIR}/.codex/skills"
DEST_FILE="${DEST_DIR}/postbrain/SKILL.md"
AGENTS_FILE="${TARGET_DIR}/AGENTS.md"

# ── Resolve source ────────────────────────────────────────────────────────────
if [[ ! -f "${SOURCE_SKILL}" ]]; then
  echo "error: skill source not found at ${SOURCE_SKILL}" >&2
  exit 1
fi

# ── Install skill file ────────────────────────────────────────────────────────
mkdir -p "$(dirname "${DEST_FILE}")"
cp "${SOURCE_SKILL}" "${DEST_FILE}"
echo "installed: ${DEST_FILE}"

# ── Optionally annotate AGENTS.md ─────────────────────────────────────────────
POSTBRAIN_URL="${POSTBRAIN_URL:-http://localhost:7433}"
POSTBRAIN_SCOPE="${POSTBRAIN_SCOPE:-}"

HINT_MARKER="<!-- postbrain-config -->"

if [[ -f "${AGENTS_FILE}" ]] && grep -q "${HINT_MARKER}" "${AGENTS_FILE}"; then
  echo "skipped: ${AGENTS_FILE} already contains Postbrain config block"
else
  {
    echo ""
    echo "${HINT_MARKER}"
    echo "## Postbrain"
    echo ""
    echo "The \`.codex/skills/postbrain/SKILL.md\` skill is active for this project."
    echo ""
    echo "\`\`\`"
    echo "POSTBRAIN_URL=${POSTBRAIN_URL}"
    if [[ -n "${POSTBRAIN_SCOPE}" ]]; then
      echo "POSTBRAIN_SCOPE=${POSTBRAIN_SCOPE}"
    else
      echo "# POSTBRAIN_SCOPE=project:your-org/your-repo   ← set this to skip the scope prompt"
    fi
    echo "\`\`\`"
  } >> "${AGENTS_FILE}"
  echo "updated:   ${AGENTS_FILE}"
fi
