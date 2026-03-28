# tsuku command-not-found handler for bash
# The eval below is safe: it applies declare -f output (the shell's own function
# serialization), not user input. See DESIGN-command-not-found.md for rationale.
# Guard against double-sourcing: if the hook is already registered, skip setup.
if [ "${_TSUKU_BASH_HOOK_LOADED:-}" = "1" ]; then
    return 0 2>/dev/null || true
fi
if command -v tsuku > /dev/null 2>&1; then
    if declare -f command_not_found_handle > /dev/null 2>&1; then
        eval "$(declare -f command_not_found_handle | sed 's/^command_not_found_handle/__tsuku_original_command_not_found_handle/')"
        command_not_found_handle() {
            if command -v tsuku > /dev/null 2>&1; then
                if tsuku run "$1" -- "${@:2}"; then
                    return 0
                fi
            fi
            __tsuku_original_command_not_found_handle "$@"
            return $?
        }
    else
        command_not_found_handle() {
            if command -v tsuku > /dev/null 2>&1; then
                if tsuku run "$1" -- "${@:2}"; then
                    return 0
                fi
            fi
            return 127
        }
    fi
    _TSUKU_BASH_HOOK_LOADED=1
fi
