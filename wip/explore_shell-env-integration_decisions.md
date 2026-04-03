# Exploration Decisions: shell-env-integration

## Round 1

- Update static env file (not switch to eval): avoids subprocess overhead on every shell start; migration path via EnsureEnvFile already exists
- Fix in tsuku repo (not user dotfiles): the problem is in EnsureEnvFile + install.sh, not in user configuration
- Three-part fix scope: (1) env file content, (2) doctor --rebuild-cache + staleness check, (3) TSUKU_NO_TELEMETRY preservation bug
- Niwa recipe fix is out of scope: separate PR in niwa repo, already identified
- Fish shell deferred: handle bash/zsh first; fish shell integration for env file is a follow-on
