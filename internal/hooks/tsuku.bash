# tsuku command-not-found handler for bash
# The eval below is safe: it applies declare -f output (the shell's own function
# serialization), not user input. See DESIGN-command-not-found.md for rationale.
if command -v tsuku > /dev/null 2>&1; then
    if declare -f command_not_found_handle > /dev/null 2>&1; then
        eval "$(declare -f command_not_found_handle | sed 's/^command_not_found_handle/__tsuku_original_command_not_found_handle/')"
        command_not_found_handle() {
            if command -v tsuku > /dev/null 2>&1; then
                tsuku suggest "$1"
            fi
            __tsuku_original_command_not_found_handle "$@"
            return 127
        }
    else
        command_not_found_handle() {
            if command -v tsuku > /dev/null 2>&1; then
                tsuku suggest "$1"
            fi
            return 127
        }
    fi
fi
