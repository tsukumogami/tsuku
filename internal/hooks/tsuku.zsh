# tsuku command-not-found handler for zsh
# Guard against double-sourcing: if the hook is already registered, skip setup.
if [[ "${_TSUKU_ZSH_HOOK_LOADED:-}" = "1" ]]; then
    return 0 2>/dev/null || true
fi
if command -v tsuku > /dev/null 2>&1; then
    if (( ${+functions[command_not_found_handler]} )); then
        functions -c command_not_found_handler __tsuku_original_command_not_found_handler
        command_not_found_handler() {
            if command -v tsuku > /dev/null 2>&1; then
                tsuku suggest "$1"
            fi
            __tsuku_original_command_not_found_handler "$@"
            return 127
        }
    else
        command_not_found_handler() {
            if command -v tsuku > /dev/null 2>&1; then
                tsuku suggest "$1"
            fi
            return 127
        }
    fi
    _TSUKU_ZSH_HOOK_LOADED=1
fi
