# Exploration Decisions: tool-lifecycle-hooks

## Round 1
- Start with declarative hooks (Level 1), not imperative scripts: security research shows tsuku's current trust model (no post-install code execution) is a strength; declarative actions with a limited vocabulary preserve this while enabling the niwa use case
- shell.d directory model for composition: ecosystem patterns (mise, asdf) and tsuku's existing hook machinery support this; cached combined scripts keep startup cost under 5ms
- Extend existing action system rather than new recipe sections: the WhenClause/Step infrastructure is proven; adding a phase qualifier is lower risk than a schema redesign
- Post-install shell integration is the priority: 8-12 tools can't function without it; completions and services are secondary
- Store cleanup instructions in state at install time: remove flow doesn't load recipes today; storing what was installed ensures reliable cleanup
- Hooks fail gracefully, not fatally: hook failure should warn, not block installation or removal
