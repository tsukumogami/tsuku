#!/usr/bin/env python3
"""Tests for generate-registry.py validation logic.

Requires Python 3.11+ (uses tomllib from standard library).
Run with: python3 -m pytest scripts/test_generate_registry.py -v
Or: python3 scripts/test_generate_registry.py
"""

import sys
import unittest
from pathlib import Path

# Import the module under test
sys.path.insert(0, str(Path(__file__).parent))
import importlib

generate_registry = importlib.import_module("generate-registry")

ValidationError = generate_registry.ValidationError
validate_metadata = generate_registry.validate_metadata


class TestCanonicalNameCollision(unittest.TestCase):
    """Test that satisfies entries conflicting with canonical recipe names are rejected.

    Context: libcurl previously declared satisfies.homebrew = ["curl"], but "curl"
    is a canonical recipe name (recipes/c/curl.toml). The cross-recipe validation
    in main() detects this collision and rejects the manifest. This test verifies
    that the validation logic catches the libcurl/curl case and similar conflicts.

    See: generate-registry.py lines 339-345 for the canonical name collision check.
    """

    def _run_cross_recipe_validation(self, recipes):
        """Run the cross-recipe satisfies validation from main().

        This replicates the validation logic from generate-registry.py main()
        (lines 322-345) to test it in isolation.
        """
        errors = []
        recipe_names = {r["name"] for r in recipes}
        satisfies_claims = {}

        for recipe in recipes:
            satisfies = recipe.get("satisfies", {})
            for ecosystem, pkg_names in satisfies.items():
                for pkg_name in pkg_names:
                    # Duplicate claim check
                    if pkg_name in satisfies_claims:
                        errors.append(ValidationError(
                            f"recipe '{recipe['name']}'",
                            f"duplicate satisfies entry: '{pkg_name}' is already claimed "
                            f"by recipe '{satisfies_claims[pkg_name]}' "
                            f"(in ecosystem '{ecosystem}')"
                        ))
                    else:
                        satisfies_claims[pkg_name] = recipe["name"]

                    # Canonical name collision check
                    if pkg_name in recipe_names and pkg_name != recipe["name"]:
                        errors.append(ValidationError(
                            f"recipe '{recipe['name']}'",
                            f"satisfies entry '{pkg_name}' conflicts with existing "
                            f"recipe canonical name '{pkg_name}'"
                        ))

        return errors

    def test_libcurl_claiming_curl_rejected(self):
        """libcurl cannot claim 'curl' because curl is a canonical recipe name."""
        recipes = [
            {
                "name": "curl",
                "description": "Command line tool for transferring data with URLs",
                "homepage": "https://curl.se/",
                "dependencies": [],
                "runtime_dependencies": [],
            },
            {
                "name": "libcurl",
                "description": "Multi-protocol file transfer library",
                "homepage": "https://curl.se/libcurl/",
                "dependencies": [],
                "runtime_dependencies": [],
                "satisfies": {
                    "homebrew": ["curl"],
                },
            },
        ]

        errors = self._run_cross_recipe_validation(recipes)

        self.assertEqual(len(errors), 1, f"expected 1 error, got {len(errors)}: {errors}")
        self.assertIn("curl", str(errors[0]))
        self.assertIn("conflicts with existing recipe canonical name", str(errors[0]))

    def test_satisfies_non_canonical_name_allowed(self):
        """A satisfies entry that doesn't match any canonical name is allowed."""
        recipes = [
            {
                "name": "sqlite",
                "description": "SQLite database engine",
                "homepage": "https://sqlite.org",
                "dependencies": [],
                "runtime_dependencies": [],
                "satisfies": {
                    "homebrew": ["sqlite3"],
                },
            },
        ]

        errors = self._run_cross_recipe_validation(recipes)
        self.assertEqual(len(errors), 0, f"unexpected errors: {errors}")

    def test_duplicate_satisfies_across_recipes_rejected(self):
        """Two recipes cannot claim the same satisfies name."""
        recipes = [
            {
                "name": "recipe-a",
                "description": "Recipe A",
                "homepage": "https://example.com",
                "dependencies": [],
                "runtime_dependencies": [],
                "satisfies": {
                    "homebrew": ["shared-name"],
                },
            },
            {
                "name": "recipe-b",
                "description": "Recipe B",
                "homepage": "https://example.com",
                "dependencies": [],
                "runtime_dependencies": [],
                "satisfies": {
                    "homebrew": ["shared-name"],
                },
            },
        ]

        errors = self._run_cross_recipe_validation(recipes)

        self.assertEqual(len(errors), 1, f"expected 1 error, got {len(errors)}: {errors}")
        self.assertIn("duplicate satisfies entry", str(errors[0]))
        self.assertIn("shared-name", str(errors[0]))


if __name__ == "__main__":
    unittest.main()
