#!/usr/bin/env bash
# check-evals-exist.sh - CI check: every skill must have at least 1 eval
#
# Usage: scripts/check-evals-exist.sh
# Exit code: 0 if all skills have evals, 1 if any are missing
#
# Skills are discovered from plugins/*/skills/*/.
# Skills with disable-model-invocation: true are exempt (reference-only skills).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLUGINS_DIR="$REPO_ROOT/plugins"

missing=()
exempt=()
passing=()

for plugin_dir in "$PLUGINS_DIR"/*/; do
  [ -d "$plugin_dir" ] || continue
  plugin_name=$(basename "$plugin_dir")

  for skill_dir in "$plugin_dir"/skills/*/; do
    [ -d "$skill_dir" ] || continue
    name=$(basename "$skill_dir")
    skill_md="$skill_dir/SKILL.md"

    if [ ! -f "$skill_md" ]; then
      continue
    fi

    # Check if skill is exempt (disable-model-invocation: true)
    if grep -q "disable-model-invocation: true" "$skill_md" 2>/dev/null; then
      exempt+=("$name [$plugin_name]")
      continue
    fi

    # Check for evals
    evals_file="$skill_dir/evals/evals.json"
    if [ ! -f "$evals_file" ]; then
      missing+=("$name [$plugin_name] -- needs $skill_dir/evals/evals.json")
      continue
    fi

    # Check evals file has at least 1 eval
    count=$(python3 -c "import json; print(len(json.load(open('$evals_file')).get('evals', [])))" 2>/dev/null || echo "0")
    if [ "$count" -eq 0 ]; then
      missing+=("$name [$plugin_name] -- evals.json has 0 evals")
    else
      passing+=("$name ($count evals) [$plugin_name]")
    fi
  done
done

echo "=== Skill Eval Check ==="
echo ""

if [ ${#passing[@]} -gt 0 ]; then
  echo "Passing:"
  for s in "${passing[@]}"; do
    echo "  + $s"
  done
fi

if [ ${#exempt[@]} -gt 0 ]; then
  echo ""
  echo "Exempt (disable-model-invocation):"
  for s in "${exempt[@]}"; do
    echo "  ~ $s"
  done
fi

if [ ${#missing[@]} -gt 0 ]; then
  echo ""
  echo "MISSING EVALS:"
  for s in "${missing[@]}"; do
    echo "  ! $s"
  done
  echo ""
  echo "Every user-invocable skill must have at least 1 eval scenario."
  exit 1
fi

echo ""
echo "All skills have evals."
exit 0
