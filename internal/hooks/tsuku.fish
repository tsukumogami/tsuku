# tsuku command-not-found handler for fish
# Guard against double-sourcing: if the hook is already registered, skip setup.
if set -q _TSUKU_FISH_HOOK_LOADED
    exit 0
end
if command -q tsuku
    if functions --query fish_command_not_found
        functions --copy fish_command_not_found __tsuku_original_fish_command_not_found
        function fish_command_not_found
            if command -q tsuku
                tsuku suggest $argv[1]
            end
            __tsuku_original_fish_command_not_found $argv
        end
    else
        function fish_command_not_found
            if command -q tsuku
                tsuku suggest $argv[1]
            end
        end
    end
    set -g _TSUKU_FISH_HOOK_LOADED 1
end
