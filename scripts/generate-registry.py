#!/usr/bin/env python3
"""Generate recipes.json from TOML recipe files.

This script parses all recipe TOML files in recipes/*/*.toml,
validates metadata, and outputs a JSON file for the recipe browser.

Requirements:
- Python 3.11+ (uses tomllib from standard library)
- No external dependencies

Output:
- _site/recipes.json with schema_version, generated_at, and recipes array
"""

import json
import re
import sys
import tomllib
from datetime import datetime, timezone
from pathlib import Path

SCHEMA_VERSION = "1.0.0"
MAX_DESCRIPTION_LENGTH = 200
MAX_FILE_SIZE = 100 * 1024  # 100KB
RECIPES_DIR = Path("recipes")
OUTPUT_DIR = Path("_site")
OUTPUT_FILE = OUTPUT_DIR / "recipes.json"

# Validation patterns
NAME_PATTERN = re.compile(r"^[a-z0-9-]+$")
PATH_PATTERN = re.compile(r"^recipes/[a-z]/[a-z0-9-]+\.toml$")


class ValidationError:
    """Represents a validation error for a recipe."""

    def __init__(self, file_path: str, message: str):
        self.file_path = file_path
        self.message = message

    def __str__(self) -> str:
        return f"{self.file_path}: {self.message}"


def discover_recipes() -> list[Path]:
    """Find all recipe TOML files in recipes/*/*.toml."""
    return sorted(RECIPES_DIR.glob("*/*.toml"))


def validate_path(file_path: Path) -> list[ValidationError]:
    """Validate the file path matches expected pattern."""
    errors = []
    path_str = str(file_path)

    if not PATH_PATTERN.match(path_str):
        errors.append(ValidationError(path_str, f"path does not match pattern recipes/[a-z]/[a-z0-9-]+.toml"))

    # Check file is within recipes directory (path traversal protection)
    try:
        resolved = file_path.resolve()
        recipes_resolved = RECIPES_DIR.resolve()
        if not str(resolved).startswith(str(recipes_resolved)):
            errors.append(ValidationError(path_str, "path traversal detected"))
    except Exception as e:
        errors.append(ValidationError(path_str, f"could not resolve path: {e}"))

    return errors


def validate_file_size(file_path: Path) -> list[ValidationError]:
    """Validate file size is under limit."""
    errors = []
    try:
        size = file_path.stat().st_size
        if size > MAX_FILE_SIZE:
            errors.append(ValidationError(
                str(file_path),
                f"file size {size} bytes exceeds limit of {MAX_FILE_SIZE} bytes"
            ))
    except Exception as e:
        errors.append(ValidationError(str(file_path), f"could not check file size: {e}"))
    return errors


def validate_metadata(file_path: Path, metadata: dict) -> list[ValidationError]:
    """Validate recipe metadata fields."""
    errors = []
    path_str = str(file_path)
    expected_name = file_path.stem  # filename without .toml

    # Check required fields exist
    for field in ["name", "description", "homepage"]:
        if field not in metadata:
            errors.append(ValidationError(path_str, f"missing required field: {field}"))

    if "name" in metadata:
        name = metadata["name"]
        # Name must match filename
        if name != expected_name:
            errors.append(ValidationError(
                path_str,
                f"name '{name}' does not match filename '{expected_name}'"
            ))
        # Name must match pattern
        if not NAME_PATTERN.match(name):
            errors.append(ValidationError(
                path_str,
                f"name '{name}' contains invalid characters (must be lowercase alphanumeric and hyphens)"
            ))

    if "description" in metadata:
        desc = metadata["description"]
        # Check length
        if len(desc) > MAX_DESCRIPTION_LENGTH:
            errors.append(ValidationError(
                path_str,
                f"description length {len(desc)} exceeds limit of {MAX_DESCRIPTION_LENGTH}"
            ))
        # Check for control characters (U+0000-U+001F)
        if any(ord(c) < 32 for c in desc):
            errors.append(ValidationError(path_str, "description contains control characters"))

    if "homepage" in metadata:
        homepage = metadata["homepage"]
        # Must start with https://
        if not homepage.startswith("https://"):
            errors.append(ValidationError(
                path_str,
                f"homepage must start with https:// (got: {homepage[:50]}...)"
            ))
        # Reject dangerous schemes that might be embedded
        dangerous = ["javascript:", "data:", "vbscript:"]
        lower_homepage = homepage.lower()
        for scheme in dangerous:
            if scheme in lower_homepage:
                errors.append(ValidationError(
                    path_str,
                    f"homepage contains dangerous scheme: {scheme}"
                ))

    return errors


def parse_recipe(file_path: Path) -> tuple[dict | None, list[ValidationError]]:
    """Parse a recipe TOML file and validate it."""
    errors = []

    # Validate path first
    errors.extend(validate_path(file_path))
    errors.extend(validate_file_size(file_path))

    if errors:
        return None, errors

    # Parse TOML
    try:
        with open(file_path, "rb") as f:
            data = tomllib.load(f)
    except tomllib.TOMLDecodeError as e:
        errors.append(ValidationError(str(file_path), f"invalid TOML: {e}"))
        return None, errors
    except Exception as e:
        errors.append(ValidationError(str(file_path), f"could not read file: {e}"))
        return None, errors

    # Check metadata section exists
    if "metadata" not in data:
        errors.append(ValidationError(str(file_path), "missing [metadata] section"))
        return None, errors

    metadata = data["metadata"]
    errors.extend(validate_metadata(file_path, metadata))

    if errors:
        return None, errors

    # Return extracted metadata
    return {
        "name": metadata["name"],
        "description": metadata["description"],
        "homepage": metadata["homepage"],
    }, []


def generate_json(recipes: list[dict]) -> dict:
    """Generate the output JSON structure."""
    return {
        "schema_version": SCHEMA_VERSION,
        "generated_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "recipes": sorted(recipes, key=lambda r: r["name"].lower()),
    }


def main() -> int:
    """Main entry point."""
    # Discover recipe files
    recipe_files = discover_recipes()
    print(f"Found {len(recipe_files)} recipe files")

    # Parse and validate all recipes
    recipes = []
    all_errors: list[ValidationError] = []

    for file_path in recipe_files:
        recipe, errors = parse_recipe(file_path)
        if errors:
            all_errors.extend(errors)
        elif recipe:
            recipes.append(recipe)

    # Report errors
    if all_errors:
        print(f"\nValidation failed with {len(all_errors)} error(s):", file=sys.stderr)
        for error in all_errors:
            print(f"  - {error}", file=sys.stderr)
        return 1

    # Generate output
    output = generate_json(recipes)

    # Create output directory
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    # Write JSON
    with open(OUTPUT_FILE, "w") as f:
        json.dump(output, f, indent=2)
        f.write("\n")  # Trailing newline

    print(f"Generated {OUTPUT_FILE} with {len(recipes)} recipes")
    return 0


if __name__ == "__main__":
    sys.exit(main())
