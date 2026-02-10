# Issue 1577 Baseline

## Issue Summary
Migrate `validate-golden-execution.yml` from hardcoded Alpine package installation to recipe-driven dependency extraction.

## Current State
The `validate-linux-containers` job has hardcoded packages:
```yaml
if [ "$FAMILY" = "alpine" ]; then
  apk add --no-cache libstdc++ yaml-dev openssl-dev
fi
```

## Target State
Replace with recipe-driven installation using `.github/scripts/install-recipe-deps.sh` for each recipe being validated.

## Key Considerations
1. The job validates multiple recipes per family - each recipe needs its own deps installed
2. The file list format is `file|tool|version` where `tool` is the recipe name
3. Need to call the helper script once per unique recipe being validated
4. Alpine needs special handling for bootstrap deps (libgcc, libstdc++ for Rust tooling)
