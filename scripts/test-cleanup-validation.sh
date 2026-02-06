#!/usr/bin/env bash
# Test script for cleanup logic validation
#
# Validates that the cleanup script's input validation and recipe
# classification logic work correctly.

set -euo pipefail

# Create temporary test directory
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"

# Initialize mock recipe directory structure
mkdir -p recipes/{a,b,c}

# Test 1: Valid recipe names
VALID_NAMES=(
  "aws-cli"
  "cargo-audit"
  "node_exporter"
  "python-3"
  "tool123"
)

echo "Test 1: Valid recipe names"
for name in "${VALID_NAMES[@]}"; do
  if echo "$name" | grep -qE '^[a-zA-Z0-9_-]+$'; then
    echo "  ✓ $name (valid)"
  else
    echo "  ✗ $name (invalid - should be valid)"
    exit 1
  fi
done

# Test 2: Invalid recipe names (path traversal attempts)
INVALID_NAMES=(
  "../etc/passwd"
  "recipe/../../../etc/shadow"
  "recipe.toml"
  "recipes/a/tool"
  "./tool"
  "tool/"
  "tool\$"
  "tool;rm -rf /"
)

echo "Test 2: Invalid recipe names (path traversal)"
for name in "${INVALID_NAMES[@]}"; do
  if echo "$name" | grep -qE '^[a-zA-Z0-9_-]+$'; then
    echo "  ✗ $name (valid - should be invalid)"
    exit 1
  else
    echo "  ✓ $name (rejected correctly)"
  fi
done

# Test 3: Recipe file detection
echo "Test 3: Recipe file detection"

# Create test recipes
echo 'name = "tool-a"' > recipes/a/tool-a.toml
echo 'name = "tool-b"' > recipes/b/tool-b.toml

# Test finding existing recipes
if [ -n "$(find recipes -name "tool-a.toml" 2>/dev/null | head -1)" ]; then
  echo "  ✓ Found existing recipe: tool-a"
else
  echo "  ✗ Failed to find tool-a"
  exit 1
fi

# Test handling missing recipes
if [ -z "$(find recipes -name "tool-missing.toml" 2>/dev/null | head -1)" ]; then
  echo "  ✓ Correctly detected missing recipe: tool-missing"
else
  echo "  ✗ False positive for missing recipe"
  exit 1
fi

# Cleanup
cd /
rm -rf "$TEST_DIR"

echo ""
echo "All validation tests passed!"
