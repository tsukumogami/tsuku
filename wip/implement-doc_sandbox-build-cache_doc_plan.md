# Documentation Plan: sandbox-build-cache

Generated from: docs/designs/DESIGN-sandbox-build-cache.md
Issues analyzed: 6
Total entries: 1

---

## doc-1: docs/GUIDE-system-dependencies.md
**Section**: Sandbox Testing with System Dependencies
**Prerequisite issues**: #1961
**Update type**: modify
**Status**: updated
**Details**: Add a subsection or paragraph about ecosystem toolchain caching to the existing "How It Works" explanation. After #1961, the sandbox pre-installs ecosystem dependencies (Rust, Node.js, etc.) as Docker image layers when a recipe has InstallTime dependencies. Subsequent sandbox runs for recipes that need the same toolchain at the same version skip the installation entirely. This complements the existing system dependency image caching described in the guide. Mention that this caching is automatic and requires no user action.
