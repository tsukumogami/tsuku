---
summary:
  constraints:
    - --from flag must accept same builder:source syntax as tsuku create --from
    - Must forward --yes and --deterministic-only to create pipeline
    - Generated recipe saved to $TSUKU_HOME/recipes/ for subsequent installs
  integration_points:
    - cmd/tsuku/install.go - add --from flag and create pipeline forwarding
    - cmd/tsuku/create.go - reuse runCreate() or extract shared logic
  risks:
    - runCreate() uses package-level vars (createFrom, createAutoApprove, etc.) - need to handle that
  approach_notes: |
    Add --from flag to install command. When set, call into create pipeline
    then install the generated recipe. The tricky part is that create.go uses
    package-level flag vars. May need to either set those vars or extract
    the create logic into a callable function.
---
