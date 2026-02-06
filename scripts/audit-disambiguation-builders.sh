#!/bin/bash
# Audit disambiguation entries against existing recipes

set -euo pipefail

echo "=== Disambiguation Builder Audit ==="
echo ""

# Read disambiguation tools
tools=(
  "age" "bat" "buf" "delta" "dive" "dust" "fd" "fzf"
  "gh" "gum" "hub" "jq" "just" "ko" "rg" "sd" "sk"
  "step" "task" "yq"
)

echo "Checking 20 disambiguation tools..."
echo ""

found_with_recipe=0
mismatches=0

for tool in "${tools[@]}"; do
  # Find recipe by searching for the tool name
  recipe_path=""
  first_letter="${tool:0:1}"

  # Special case for go-task (task)
  if [ "$tool" = "task" ]; then
    recipe_path="recipes/g/go-task.toml"
  else
    potential_path="recipes/$first_letter/$tool.toml"
    if [ -f "$potential_path" ]; then
      recipe_path="$potential_path"
    fi
  fi

  if [ -n "$recipe_path" ] && [ -f "$recipe_path" ]; then
    found_with_recipe=$((found_with_recipe + 1))

    # Extract the action from recipe
    action=$(grep -m 1 'action = ' "$recipe_path" | sed 's/.*action = "\([^"]*\)".*/\1/')

    # Get current builder from disambiguations.json
    current_builder=$(jq -r ".entries[] | select(.name == \"$tool\") | .builder" data/discovery-seeds/disambiguations.json)

    # Determine expected builder based on action
    expected_builder=""
    if [ "$action" = "homebrew" ]; then
      expected_builder="homebrew"
    elif [ "$action" = "github_archive" ]; then
      expected_builder="github"  # github_archive is deterministic, github builder can use it
    else
      expected_builder="unknown"
    fi

    # Check for mismatch
    status="OK"
    if [ "$action" = "homebrew" ] && [ "$current_builder" != "homebrew" ]; then
      status="MISMATCH"
      mismatches=$((mismatches + 1))
    fi

    printf "%-10s %-20s %-15s %-15s %s\n" "$tool" "$recipe_path" "$action" "$current_builder" "$status"
  else
    echo "$tool - NO RECIPE (github builder is appropriate)"
  fi
done

echo ""
echo "=== Summary ==="
echo "Tools with recipes: $found_with_recipe"
echo "Builder mismatches: $mismatches"
echo ""

if [ $mismatches -gt 0 ]; then
  echo "RECOMMENDATION: Update tools with 'homebrew' action to use 'homebrew' builder"
  echo "This ensures they work with --deterministic-only mode."
fi
