# tsuku command-not-found handler for fish
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
end
