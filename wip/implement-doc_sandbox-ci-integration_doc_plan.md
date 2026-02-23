# Documentation Plan: sandbox-ci-integration

Generated from: docs/designs/DESIGN-sandbox-ci-integration.md
Issues analyzed: 6
Total entries: 3

---

## doc-1: README.md
**Section**: Sandbox Testing
**Prerequisite issues**: #1942, #1943, #1944
**Update type**: modify
**Status**: pending
**Details**: Update the "Sandbox Testing" section to document three new capabilities. (1) The sandbox now runs the recipe's verify command after installation and reports pass/fail based on both install and verification -- update the bullet point "Verifies the tool installs and runs correctly" to reflect that verification uses the recipe's `[verification]` section. (2) Add `--env KEY=VALUE` flag usage examples showing how to pass environment variables (like `GITHUB_TOKEN`) into the sandbox container, and note that `--env KEY` reads from the host environment. (3) Add `--json` flag usage example showing structured output for scripting and CI consumption, with a brief description of the JSON schema fields (`passed`, `verified`, `install_exit_code`, `verify_exit_code`, `duration_ms`, `error`).

---

## doc-2: docs/ENVIRONMENT.md
**Section**: Sandbox Environment (new section)
**Prerequisite issues**: #1943
**Update type**: modify
**Status**: pending
**Details**: Add a "Sandbox" section documenting the environment variables that the sandbox hardcodes inside the container (`TSUKU_SANDBOX`, `TSUKU_HOME`, `HOME`, `DEBIAN_FRONTEND`, `PATH`) and their behavior. Explain that these five variables can't be overridden via `--env` -- user-provided keys matching these are silently dropped. Add `TSUKU_SANDBOX` to the summary table (it's set inside sandbox containers to indicate sandbox mode). Note that `TSUKU_REGISTRY_URL` is consumed on the host during plan generation, not inside the container, so it doesn't need `--env` passthrough.

---

## doc-3: docs/GUIDE-plan-based-installation.md
**Section**: Plan Format
**Prerequisite issues**: #1942
**Update type**: modify
**Status**: pending
**Details**: The plan format version bumps from 4 to 5 with the addition of `exit_code` to the `PlanVerify` struct. If this guide references the plan format version or the verify fields in the plan JSON, update to reflect the new `exit_code` field in the verify section and the version bump. If the guide doesn't currently cover plan internals at this level, skip this entry.
