# Documentation Plan: Secrets Manager

Generated from: docs/designs/DESIGN-secrets-manager.md
Issues analyzed: 5
Total entries: 4

---

## doc-1: README.md
**Section**: LLM-Powered Recipe Generation
**Prerequisite issues**: #1735, #1737
**Update type**: modify
**Status**: updated
**Details**: Update the API key setup instructions under "LLM-Powered Recipe Generation" to show both methods: environment variables (existing) and `tsuku config set secrets.*` via stdin. Replace the bare `export` examples with a note that keys can also be stored in config.toml. Keep the env var examples since they're the quick-start path, but add the config alternative so users know it exists.

---

## doc-2: README.md
**Section**: Usage (new subsection: Secrets Management)
**Prerequisite issues**: #1737
**Update type**: modify
**Status**: updated
**Details**: Add a "Secrets Management" subsection under Usage documenting the `tsuku config` secrets workflow: setting secrets via stdin (`tsuku config set secrets.anthropic_api_key`), checking secret status (`tsuku config get secrets.anthropic_api_key` shows set/not-set), and the secrets status display in `tsuku config` output. Mention that stdin input avoids shell history exposure and that piping is supported.

---

## doc-3: docs/ENVIRONMENT.md
**Section**: GITHUB_TOKEN
**Prerequisite issues**: #1736
**Update type**: modify
**Status**: updated
**Details**: Update the GITHUB_TOKEN section to mention that the token can also be stored in `$TSUKU_HOME/config.toml` under `[secrets]` via `tsuku config set secrets.github_token`. Note the resolution order (env var checked first, then config file). Keep the existing env var instructions intact since they still work.

---

## doc-4: docs/ENVIRONMENT.md
**Section**: Development and Debugging (new entries for API keys)
**Prerequisite issues**: #1735, #1736
**Update type**: modify
**Status**: updated
**Details**: Add entries for ANTHROPIC_API_KEY, GOOGLE_API_KEY/GEMINI_API_KEY, TAVILY_API_KEY, and BRAVE_API_KEY to the environment variables reference. Each entry should note that these can also be set via `tsuku config set secrets.<name>` as a config file alternative. Update the Summary Table to include these new entries.
