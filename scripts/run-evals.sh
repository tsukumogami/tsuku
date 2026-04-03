#!/usr/bin/env bash
# run-evals.sh - Run skill evals using /skill-creator
#
# Usage:
#   scripts/run-evals.sh <skill-name>             Run evals for one skill
#   scripts/run-evals.sh --all                    Run evals for all skills
#   scripts/run-evals.sh --list                   List skills with evals
#   scripts/run-evals.sh --validate <skill>       Re-validate existing results
#   scripts/run-evals.sh --prep-only <skill>      Prepare workspace only (for /skill-creator)
#
# Skills are discovered from plugins/*/skills/*/.
# Each skill's evals live at plugins/<plugin>/skills/<name>/evals/evals.json.
# Results go to plugins/<plugin>/skills/<name>/evals/workspace/iteration-<N>/.
#
# Exit codes:
#   0  All assertions passed
#   1  One or more assertions failed
#   2  No results produced (infrastructure failure)
#   3  Missing prerequisites
#
# Prerequisites: claude CLI, python3, skill-creator plugin installed

set -uo pipefail
# Note: no set -e; we handle errors explicitly for --all resilience

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLUGINS_DIR="$REPO_ROOT/plugins"

# Prerequisite checks
command -v claude >/dev/null 2>&1 || { echo "Error: claude CLI not found"; exit 3; }
command -v python3 >/dev/null 2>&1 || { echo "Error: python3 not found"; exit 3; }

usage() {
  echo "Usage: $0 <skill-name> | --all | --list | --validate <skill> | --prep-only <skill>"
  echo ""
  echo "  <skill-name>       Run evals for a specific skill (prep + execute + validate)"
  echo "  --all              Run evals for all skills that have evals/"
  echo "  --list             List skills that have evals"
  echo "  --validate <skill> Re-validate the latest iteration without re-running"
  echo "  --prep-only <skill>     Prepare workspace only (use with /skill-creator in Claude Code)"
  exit 1
}

# Resolve a skill name to its directory under plugins/*/skills/<name>/.
# Returns the first match. Exits with error if not found.
resolve_skill_dir() {
  local skill_name="$1"
  for plugin_dir in "$PLUGINS_DIR"/*/; do
    local candidate="${plugin_dir%/}/skills/$skill_name"
    if [ -d "$candidate" ]; then
      echo "$candidate"
      return 0
    fi
  done
  echo "Error: skill '$skill_name' not found under $PLUGINS_DIR/*/skills/" >&2
  return 3
}

list_skills_with_evals() {
  local found=0
  for plugin_dir in "$PLUGINS_DIR"/*/; do
    local plugin_name
    plugin_name=$(basename "$plugin_dir")
    for skill_dir in "$plugin_dir"/skills/*/; do
      [ -d "$skill_dir" ] || continue
      local name
      name=$(basename "$skill_dir")
      if [ -f "$skill_dir/evals/evals.json" ]; then
        local count
        count=$(python3 -c "import json; print(len(json.load(open('$skill_dir/evals/evals.json'))['evals']))" 2>/dev/null || echo "?")
        echo "  $name ($count evals) [$plugin_name]"
        found=$((found + 1))
      fi
    done
  done
  if [ "$found" -eq 0 ]; then
    echo "  (no skills have evals)"
  fi
}

next_iteration() {
  local workspace="$1"
  local n=1
  while [ -d "$workspace/iteration-$n" ]; do
    n=$((n + 1))
  done
  echo "$n"
}

latest_iteration() {
  local workspace="$1"
  local n=0
  while [ -d "$workspace/iteration-$((n + 1))" ]; do
    n=$((n + 1))
  done
  echo "$n"
}

prep_skill_evals() {
  local skill_name="$1"
  local skill_dir
  skill_dir=$(resolve_skill_dir "$skill_name") || return $?
  local evals_file="$skill_dir/evals/evals.json"

  if [ ! -f "$evals_file" ]; then
    echo "Error: no evals found at $evals_file"
    return 3
  fi

  if [ ! -f "$skill_dir/SKILL.md" ]; then
    echo "Error: no SKILL.md found at $skill_dir/SKILL.md"
    return 3
  fi

  local workspace="$skill_dir/evals/workspace"
  mkdir -p "$workspace"

  local iteration
  iteration=$(next_iteration "$workspace")
  local iter_dir="$workspace/iteration-$iteration"

  local eval_count
  eval_count=$(python3 -c "import json; print(len(json.load(open('$evals_file'))['evals']))")

  echo "=== Preparing evals for skill: $skill_name ==="
  echo "  Evals file: $evals_file"
  echo "  Eval count: $eval_count"
  echo "  Iteration: $iteration"
  echo "  Output: $iter_dir"
  echo ""

  python3 << PYEOF
import json, os, shutil

with open("$evals_file") as f:
    data = json.load(f)

iter_dir = "$iter_dir"
evals_dir = os.path.dirname("$evals_file")

for eval_item in data["evals"]:
    eval_id = eval_item["id"]
    eval_name = eval_item.get("name", f"eval-{eval_id}")
    prompt = eval_item["prompt"]

    eval_dir = os.path.join(iter_dir, eval_name)
    os.makedirs(os.path.join(eval_dir, "with_skill", "outputs"), exist_ok=True)
    os.makedirs(os.path.join(eval_dir, "without_skill", "outputs"), exist_ok=True)

    metadata = {
        "eval_id": eval_id,
        "eval_name": eval_name,
        "prompt": prompt,
        "assertions": eval_item.get("assertions", [])
    }
    with open(os.path.join(eval_dir, "eval_metadata.json"), "w") as f:
        json.dump(metadata, f, indent=2)

    # Copy fixture files to inputs/ if fixture_dir is specified
    fixture_dir_rel = eval_item.get("fixture_dir")
    if fixture_dir_rel:
        fixture_dir = os.path.join(evals_dir, fixture_dir_rel)
        if os.path.isdir(fixture_dir):
            inputs_dir = os.path.join(eval_dir, "inputs")
            if os.path.exists(inputs_dir):
                shutil.rmtree(inputs_dir)
            shutil.copytree(fixture_dir, inputs_dir)
            metadata["has_fixtures"] = True
            with open(os.path.join(eval_dir, "eval_metadata.json"), "w") as f:
                json.dump(metadata, f, indent=2)
            print(f"  Prepared: {eval_name} (with fixtures from {fixture_dir_rel})")
        else:
            print(f"  WARNING: fixture_dir not found: {fixture_dir}")
            print(f"  Prepared: {eval_name}")
    else:
        print(f"  Prepared: {eval_name}")

print(f"\nPrepared {len(data['evals'])} eval directories.")
PYEOF

  # Return values for callers
  echo "$iter_dir" > /tmp/run-evals-iter-dir
  echo "$eval_count" > /tmp/run-evals-eval-count
  echo "$iteration" > /tmp/run-evals-iteration
}

run_skill_evals() {
  local skill_name="$1"
  local skill_dir
  skill_dir=$(resolve_skill_dir "$skill_name") || return $?
  local evals_file="$skill_dir/evals/evals.json"

  # Step 1: Prepare
  prep_skill_evals "$skill_name" || return $?

  local iter_dir eval_count iteration
  iter_dir=$(cat /tmp/run-evals-iter-dir)
  eval_count=$(cat /tmp/run-evals-eval-count)
  iteration=$(cat /tmp/run-evals-iteration)

  # Step 2: Build tier-specific instructions for each eval
  local fixtures_bin="$skill_dir/evals/fixtures/bin"
  local tier_instructions
  tier_instructions=$(python3 << PYEOF
import json

with open("$evals_file") as f:
    data = json.load(f)

lines = []
for ev in data["evals"]:
    tier = ev.get("tier", 1)
    name = ev.get("name", f"eval-{ev['id']}")
    if tier == 2:
        scenario = ev.get("scenario", "")
        lines.append(f"- {name}: TIER 2 (execute) — set EVAL_SCENARIO={scenario}, prepend $fixtures_bin to PATH. "
                     f"Instruct agent: 'Execute the workflow. gh and tsuku are available on PATH.'")
    else:
        lines.append(f"- {name}: TIER 1 (plan_only) — "
                     f"Instruct agent: 'Read the skill file and describe the exact sequence of commands you would run. Do NOT execute any commands.'")

print("\n".join(lines))
PYEOF
)

  # Step 3: Run evals via claude -p with /skill-creator
  echo ""
  echo "Invoking claude with /skill-creator to run evals..."
  echo "(this may take several minutes)"
  echo ""

  local claude_exit=0
  claude -p "$(cat <<PROMPT
Invoke /skill-creator. You already have an existing skill with evals ready to run.

The skill is at: $skill_dir/SKILL.md
The evals are at: $evals_file
The eval workspace is prepared at: $iter_dir

Each eval directory in the workspace has:
- eval_metadata.json with the prompt and assertions
- with_skill/outputs/ (empty, for you to fill)
- without_skill/outputs/ (empty, for you to fill)

TIER-SPECIFIC INSTRUCTIONS:
Evals are split into two tiers. For each eval, apply the matching tier instruction below.

$tier_instructions

For tier 2 evals, before spawning the with-skill agent:
1. Set the EVAL_SCENARIO environment variable as specified above.
2. Prepend $fixtures_bin to PATH so the agent uses shimmed binaries.
These environment variables must be passed to the spawned agent process.

For tier 1 evals, the agent must NOT execute any commands. It should only read the
skill file and describe its planned execution sequence.

Follow the skill-creator's "Running and evaluating test cases" workflow:
- Step 1: For each eval, spawn a with-skill agent (reads the skill SKILL.md then executes the prompt) and a without-skill baseline agent (same prompt, no skill). Save outputs to the respective outputs/ directories.
  - IMPORTANT: If eval_metadata.json contains "has_fixtures": true, an inputs/ directory exists alongside it with pre-defined artifact files. Before running the with-skill agent for that eval, treat those files as already present -- the skill should read them rather than improvising fixture content.
- Step 2: Grade each with-skill run against the assertions in eval_metadata.json. Write grading.json in each with_skill/ directory.
- Step 3: Capture timing data (total_tokens, duration_ms) to timing.json in each run directory.
- Step 4: Run the aggregation and generate the viewer to /tmp/${skill_name}-eval-review.html using --static mode.

This is iteration $iteration for the $skill_name skill.
PROMPT
)" 2>&1 || claude_exit=$?

  if [ "$claude_exit" -ne 0 ]; then
    echo ""
    echo "Warning: claude -p exited with status $claude_exit"
  fi

  # Step 3: Validate results
  echo ""
  echo "=== Validating results ==="
  validate_results "$iter_dir" "$eval_count"

  # Step 4: Open viewer if it was generated
  local viewer="/tmp/${skill_name}-eval-review.html"
  if [ -f "$viewer" ]; then
    echo ""
    echo "Open the eval viewer:"
    echo "  xdg-open $viewer"
  fi
}

validate_results() {
  local iter_dir="$1"
  local expected_count="$2"
  local graded=0
  local missing_outputs=()
  local missing_grading=()
  local total_assertions=0
  local passed_assertions=0
  local failed_assertions=0

  for eval_dir in "$iter_dir"/*/; do
    [ -d "$eval_dir" ] || continue
    local name
    name=$(basename "$eval_dir")
    # Skip non-eval entries
    [[ "$name" == *.json ]] && continue
    [[ "$name" == *.html ]] && continue
    [[ "$name" == *.md ]] && continue

    # Check with_skill outputs exist
    if [ ! -d "$eval_dir/with_skill/outputs" ] || [ -z "$(ls -A "$eval_dir/with_skill/outputs" 2>/dev/null)" ]; then
      missing_outputs+=("$name/with_skill")
    fi

    # Check without_skill outputs exist
    if [ ! -d "$eval_dir/without_skill/outputs" ] || [ -z "$(ls -A "$eval_dir/without_skill/outputs" 2>/dev/null)" ]; then
      missing_outputs+=("$name/without_skill")
    fi

    # Check grading exists and tally
    # Only with_skill is graded against assertions; without_skill is the baseline
    if [ -f "$eval_dir/with_skill/grading.json" ]; then
      graded=$((graded + 1))
      local counts
      counts=$(python3 -c "
import json
with open('$eval_dir/with_skill/grading.json') as f:
    g = json.load(f)
# Handle both formats: {expectations: [...]} and bare [...]
exps = g if isinstance(g, list) else g.get('expectations', [])
p = sum(1 for e in exps if e.get('passed', False))
print(f'{len(exps)} {p}')
" 2>/dev/null || echo "0 0")
      local total passed
      total=$(echo "$counts" | cut -d' ' -f1)
      passed=$(echo "$counts" | cut -d' ' -f2)
      total_assertions=$((total_assertions + total))
      passed_assertions=$((passed_assertions + passed))
      failed_assertions=$((failed_assertions + total - passed))
    else
      missing_grading+=("$name")
    fi
  done

  echo "  Evals expected: $expected_count"
  echo "  Evals graded:   $graded"
  echo "  Assertions:     $passed_assertions/$total_assertions passed"

  if [ ${#missing_outputs[@]} -gt 0 ]; then
    echo ""
    echo "  Missing outputs:"
    for m in "${missing_outputs[@]}"; do
      echo "    - $m"
    done
  fi

  if [ ${#missing_grading[@]} -gt 0 ]; then
    echo ""
    echo "  Missing grading:"
    for m in "${missing_grading[@]}"; do
      echo "    - $m"
    done
  fi

  if [ "$failed_assertions" -gt 0 ]; then
    echo ""
    echo "  FAILED ASSERTIONS: $failed_assertions"
    for eval_dir in "$iter_dir"/*/; do
      [ -d "$eval_dir" ] || continue
      local gfile="$eval_dir/with_skill/grading.json"
      [ -f "$gfile" ] || continue
      local ename
      ename=$(basename "$eval_dir")
      python3 -c "
import json
with open('$gfile') as f:
    g = json.load(f)
exps = g if isinstance(g, list) else g.get('expectations', [])
for e in exps:
    if not e.get('passed', False):
        print(f'    [$ename] FAIL: {e.get(\"text\", \"unknown\")}')
        if e.get('evidence'):
            print(f'           {e[\"evidence\"]}')
" 2>/dev/null
    done
    return 1
  fi

  if [ "$graded" -eq 0 ]; then
    echo ""
    echo "  WARNING: No evals were graded. The claude session may not have produced results."
    echo "  Re-run or check the workspace: $iter_dir"
    return 2
  fi

  echo ""
  echo "  All assertions passed."
  return 0
}

# Main
if [ $# -eq 0 ]; then
  usage
fi

case "$1" in
  --list)
    echo "Skills with evals:"
    list_skills_with_evals
    ;;
  --all)
    failed_skills=()
    infra_failed=()
    for plugin_dir in "$PLUGINS_DIR"/*/; do
      for skill_dir in "$plugin_dir"/skills/*/; do
        [ -d "$skill_dir" ] || continue
        name=$(basename "$skill_dir")
        if [ -f "$skill_dir/evals/evals.json" ]; then
          if ! run_skill_evals "$name"; then
            rc=$?
            if [ "$rc" -eq 2 ] || [ "$rc" -eq 3 ]; then
              infra_failed+=("$name")
            else
              failed_skills+=("$name")
            fi
          fi
          echo ""
        fi
      done
    done
    echo "=== Summary ==="
    if [ ${#failed_skills[@]} -gt 0 ]; then
      echo "  Failed assertions: ${failed_skills[*]}"
    fi
    if [ ${#infra_failed[@]} -gt 0 ]; then
      echo "  Infrastructure failures: ${infra_failed[*]}"
    fi
    if [ ${#failed_skills[@]} -eq 0 ] && [ ${#infra_failed[@]} -eq 0 ]; then
      echo "  All skills passed."
    fi
    [ ${#failed_skills[@]} -gt 0 ] && exit 1
    [ ${#infra_failed[@]} -gt 0 ] && exit 2
    exit 0
    ;;
  --prep-only)
    if [ $# -lt 2 ]; then
      echo "Usage: $0 --prep-only <skill-name>"
      exit 1
    fi
    prep_skill_evals "$2"
    skill_dir=$(resolve_skill_dir "$2")
    iter_dir=$(cat /tmp/run-evals-iter-dir)
    echo ""
    echo "Workspace ready. To run evals interactively:"
    echo "  Use /skill-creator in Claude Code with this workspace: $iter_dir"
    echo "  Skill path: $skill_dir/SKILL.md"
    echo ""
    echo "To validate results after running:"
    echo "  $0 --validate $2"
    ;;
  --validate)
    if [ $# -lt 2 ]; then
      echo "Usage: $0 --validate <skill-name>"
      exit 1
    fi
    skill_name="$2"
    skill_dir=$(resolve_skill_dir "$skill_name") || exit $?
    workspace="$skill_dir/evals/workspace"
    iteration=$(latest_iteration "$workspace")
    if [ "$iteration" -eq 0 ]; then
      echo "Error: no iterations found in $workspace"
      exit 2
    fi
    iter_dir="$workspace/iteration-$iteration"
    eval_count=$(python3 -c "import json; print(len(json.load(open('$skill_dir/evals/evals.json'))['evals']))")
    echo "=== Validating iteration $iteration for $skill_name ==="
    validate_results "$iter_dir" "$eval_count"
    ;;
  --help|-h)
    usage
    ;;
  *)
    run_skill_evals "$1"
    ;;
esac
