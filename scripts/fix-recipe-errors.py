#!/usr/bin/env python3
"""Fix validation errors in new recipe TOML files.

Reads the error output from generate-registry.py and applies fixes to
new (untracked) recipe files. Also resolves satisfies conflicts where
an existing recipe's satisfies entry conflicts with a new recipe's
canonical name. Error types handled:

1. homepage must start with https:// -> change http:// to https://
2. runtime_dependencies contains invalid name -> remove the entry
3. runtime_dependency references non-existent recipe -> remove the entry
   (if the dep matches a metadata.satisfies alias, rewrite it to the
   canonical recipe name instead of removing, e.g. "openssl@3" -> "openssl")
4. satisfies entry conflicts with new recipe name -> remove from old file

This script uses toml-aware line editing (not a TOML library) to preserve
the original file formatting as much as possible.
"""

import re
import subprocess
import sys
try:
    import tomllib
except ModuleNotFoundError:
    import tomli as tomllib
from pathlib import Path


def get_new_files() -> set[str]:
    """Get set of untracked recipe files from git status."""
    result = subprocess.run(
        ["git", "status", "--short", "recipes/"],
        capture_output=True, text=True, check=True,
    )
    new_files = set()
    for line in result.stdout.splitlines():
        parts = line.split()
        if len(parts) >= 2 and parts[0] == "??":
            new_files.add(parts[1])
    return new_files


def get_all_recipe_names() -> tuple[set[str], dict[str, str]]:
    """Build set of all recipe names and a satisfies-to-recipe-name mapping.

    Returns:
        A tuple of (recipe_names, satisfies_map) where satisfies_map maps
        package names from metadata.satisfies entries (e.g. "openssl@3") to
        the canonical recipe name (e.g. "openssl").
    """
    names = set()
    satisfies_map: dict[str, str] = {}
    for recipes_dir in [Path("recipes"), Path("internal/recipe/recipes")]:
        if not recipes_dir.exists():
            continue
        for toml_file in recipes_dir.rglob("*.toml"):
            try:
                with open(toml_file, "rb") as f:
                    data = tomllib.load(f)
                metadata = data.get("metadata", {})
                name = metadata.get("name")
                if name:
                    names.add(name)
                    # Build reverse mapping from satisfies entries to recipe name
                    satisfies = metadata.get("satisfies", {})
                    for _ecosystem, pkg_names in satisfies.items():
                        if isinstance(pkg_names, list):
                            for pkg_name in pkg_names:
                                satisfies_map[pkg_name] = name
            except Exception:
                # If we can't parse it, skip
                continue
    return names, satisfies_map


NAME_PATTERN = re.compile(r"^[a-z0-9@.-]+$")


def _resolve_deps(
    deps: list[str],
    all_recipe_names: set[str],
    satisfies_map: dict[str, str],
    dep_kind: str,
) -> tuple[list[str], list[str]]:
    """Resolve a dependency list, rewriting satisfies aliases and dropping invalid entries.

    Returns:
        A tuple of (resolved_deps, fix_descriptions) where resolved_deps is the
        cleaned list and fix_descriptions explains what changed.
    """
    resolved = []
    fixes = []
    for dep in deps:
        if not NAME_PATTERN.match(dep):
            fixes.append(f"removed invalid {dep_kind}: {dep}")
            continue
        if dep in all_recipe_names:
            resolved.append(dep)
            continue
        # Check if this is a satisfies alias (e.g. "openssl@3" -> "openssl")
        canonical = satisfies_map.get(dep)
        if canonical and canonical in all_recipe_names:
            resolved.append(canonical)
            fixes.append(f"rewrote {dep_kind}: {dep} -> {canonical}")
            continue
        # Not a known recipe or satisfies alias -- drop it
        fixes.append(f"removed non-existent {dep_kind}: {dep}")
    return resolved, fixes


def fix_file(
    file_path: Path,
    all_recipe_names: set[str],
    satisfies_map: dict[str, str],
) -> tuple[bool, list[str]]:
    """Fix a single recipe file. Returns (changed, list_of_fixes)."""
    content = file_path.read_text()
    fixes = []
    changed = False

    # Parse to understand current state
    try:
        with open(file_path, "rb") as f:
            data = tomllib.load(f)
    except Exception as e:
        return False, [f"Could not parse {file_path}: {e}"]

    metadata = data.get("metadata", {})

    # Fix 1: homepage http -> https
    homepage = metadata.get("homepage", "")
    if homepage.startswith("http://") and not homepage.startswith("https://"):
        old_val = homepage
        new_val = "https://" + homepage[len("http://"):]
        content = content.replace(f'"{old_val}"', f'"{new_val}"', 1)
        fixes.append(f"homepage: {old_val} -> {new_val}")
        changed = True

    # Fix 2 & 3: Resolve runtime_dependencies (rewrite aliases, remove invalid)
    runtime_deps = metadata.get("runtime_dependencies", [])
    new_runtime_deps, runtime_fixes = _resolve_deps(
        runtime_deps, all_recipe_names, satisfies_map, "dep",
    )
    fixes.extend(runtime_fixes)

    if new_runtime_deps != runtime_deps:
        # Replace the runtime_dependencies line in the file
        # Match the TOML array format: runtime_dependencies = [...]
        dep_line_re = re.compile(
            r'(\s*runtime_dependencies\s*=\s*)\[([^\]]*)\]',
            re.DOTALL,
        )
        m = dep_line_re.search(content)
        if m:
            prefix = m.group(1)
            if new_runtime_deps:
                quoted = ", ".join(f'"{d}"' for d in new_runtime_deps)
                replacement = f"{prefix}[{quoted}]"
            else:
                replacement = f"{prefix}[]"
            content = content[:m.start()] + replacement + content[m.end():]
            changed = True

    # Also fix dependencies (not just runtime_dependencies)
    deps = metadata.get("dependencies", [])
    new_build_deps, build_fixes = _resolve_deps(
        deps, all_recipe_names, satisfies_map, "build dep",
    )
    fixes.extend(build_fixes)

    if new_build_deps != deps:
        dep_line_re = re.compile(
            r'(\s*dependencies\s*=\s*)\[([^\]]*)\]',
            re.DOTALL,
        )
        # Be careful not to match runtime_dependencies
        # We need to find 'dependencies = [...]' but NOT 'runtime_dependencies = [...]'
        for m in dep_line_re.finditer(content):
            # Check that the match is NOT preceded by 'runtime_'
            start = m.start()
            preceding = content[max(0, start - 10):start]
            if "runtime_" in preceding:
                continue
            prefix = m.group(1)
            if new_build_deps:
                quoted = ", ".join(f'"{d}"' for d in new_build_deps)
                replacement = f"{prefix}[{quoted}]"
            else:
                replacement = f"{prefix}[]"
            content = content[:m.start()] + replacement + content[m.end():]
            changed = True
            break

    if changed:
        file_path.write_text(content)

    return changed, fixes


def main() -> int:
    new_files = get_new_files()
    print(f"Found {len(new_files)} new recipe files")

    all_recipe_names, satisfies_map = get_all_recipe_names()
    print(f"Found {len(all_recipe_names)} total recipe names")
    print(f"Found {len(satisfies_map)} satisfies mappings")

    total_fixed = 0
    total_fixes = 0

    for fpath_str in sorted(new_files):
        fpath = Path(fpath_str)
        if not fpath.exists() or not fpath.suffix == ".toml":
            continue

        file_changed, fixes = fix_file(fpath, all_recipe_names, satisfies_map)
        if file_changed:
            total_fixed += 1
            total_fixes += len(fixes)
            for fix in fixes:
                print(f"  {fpath}: {fix}")

    # Fix satisfies conflicts: when a new recipe's canonical name
    # conflicts with an existing recipe's satisfies entry, remove
    # the conflicting entry from the existing recipe.
    new_recipe_names = set()
    for fpath_str in new_files:
        fpath = Path(fpath_str)
        if not fpath.exists() or fpath.suffix != ".toml":
            continue
        try:
            with open(fpath, "rb") as f:
                data = tomllib.load(f)
            name = data.get("metadata", {}).get("name")
            if name:
                new_recipe_names.add(name)
        except Exception:
            continue

    # Scan all existing (non-new) recipe files for conflicting satisfies
    for recipes_dir in [Path("recipes"), Path("internal/recipe/recipes")]:
        if not recipes_dir.exists():
            continue
        for toml_file in sorted(recipes_dir.rglob("*.toml")):
            rel_path = str(toml_file)
            if rel_path in new_files:
                continue
            try:
                with open(toml_file, "rb") as f:
                    data = tomllib.load(f)
            except Exception:
                continue
            metadata = data.get("metadata", {})
            satisfies = metadata.get("satisfies", {})
            if not satisfies:
                continue

            # Find entries that conflict with new recipe names
            conflicts = []
            for ecosystem, pkg_names in satisfies.items():
                for pkg_name in pkg_names:
                    if pkg_name in new_recipe_names:
                        conflicts.append((ecosystem, pkg_name))

            if not conflicts:
                continue

            content = toml_file.read_text()
            for ecosystem, pkg_name in conflicts:
                pkg_names = satisfies[ecosystem]
                remaining = [p for p in pkg_names if p not in new_recipe_names]
                if not remaining and len(satisfies) == 1:
                    # Remove entire [metadata.satisfies] section
                    # Match the section header and its content lines
                    section_re = re.compile(
                        r'\n\[metadata\.satisfies\]\n(?:[^\[]*?)(?=\n\[|\Z)',
                        re.DOTALL,
                    )
                    content = section_re.sub('\n', content)
                elif not remaining:
                    # Remove just this ecosystem line
                    line_re = re.compile(
                        rf'^{re.escape(ecosystem)}\s*=\s*\[.*?\]\s*$',
                        re.MULTILINE,
                    )
                    content = line_re.sub('', content)
                else:
                    # Update the line with remaining entries
                    quoted = ", ".join(f'"{p}"' for p in remaining)
                    line_re = re.compile(
                        rf'^({re.escape(ecosystem)}\s*=\s*)\[.*?\]',
                        re.MULTILINE,
                    )
                    content = line_re.sub(rf'\g<1>[{quoted}]', content)

                print(f"  {toml_file}: removed satisfies conflict '{pkg_name}' "
                      f"(ecosystem '{ecosystem}')")
                total_fixes += 1

            toml_file.write_text(content)
            total_fixed += 1

    print(f"\nFixed {total_fixed} files with {total_fixes} total fixes")
    return 0


if __name__ == "__main__":
    sys.exit(main())
