# Security Review: shell-env-activation

## Dimension Analysis

### External Artifact Handling
**Applies:** Yes (reads .tsuku.toml which is an external artifact in cloned repos)
The activation reads .tsuku.toml and uses it to modify PATH. However, it only constructs paths within $TSUKU_HOME/tools/ -- it doesn't download, execute, or process external binaries. The tools referenced must already be installed.

### Permission Scope
**Applies:** Yes (modifies PATH, which affects which binaries execute)
PATH modification is the core function. All paths are constrained to $TSUKU_HOME/tools/{name}-{version}/bin. No sudo, no system directory modification.

### Supply Chain or Dependency Trust
**Applies:** No. Activation only references already-installed tools. It doesn't fetch or install anything. The trust boundary is at install time (Block 4), not activation time.

### Data Exposure
**Applies:** No. No new data transmission. Tool names and versions stay local.

## Recommended Outcome

**OPTION 2 - Document considerations.** The design already has a Security Considerations section that adequately covers PATH manipulation, prompt hook safety, and untrusted repo config risks. No design changes needed.

## Summary

The security posture is sound. Activation constrains all PATH entries to $TSUKU_HOME/tools/, doesn't trigger downloads, and prompt hooks are opt-in. The main residual risk (auto-activation in cloned repos) is mitigated by hooks being opt-in and only referencing installed tools.
