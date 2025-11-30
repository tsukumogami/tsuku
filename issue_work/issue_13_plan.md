# Issue 13 Implementation Plan

## Summary

Add a `completion` subcommand that generates shell completion scripts for bash, zsh, and fish using Cobra's built-in completion generation.

## Approach

Use Cobra's built-in shell completion methods (GenBashCompletionV2, GenZshCompletion, GenFishCompletion) to generate completion scripts. The command will take a shell name as an argument and output the script to stdout.

## Files to Create
- `cmd/tsuku/completion.go` - Completion command implementation

## Files to Modify
- `cmd/tsuku/main.go` - Register completion command

## Implementation Steps
- [x] Create completion.go with subcommands for bash, zsh, fish
- [x] Register completion command in main.go
- [x] Test generated completions work

## Testing Strategy
- Generate completions and verify they're valid shell scripts
- Manual verification with actual shells

## Success Criteria
- [ ] `tsuku completion bash` outputs valid bash completion script
- [ ] `tsuku completion zsh` outputs valid zsh completion script
- [ ] `tsuku completion fish` outputs valid fish completion script
- [ ] Help text provides usage examples
