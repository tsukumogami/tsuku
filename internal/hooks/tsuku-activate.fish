function _tsuku_hook --on-event fish_prompt
  tsuku hook-env fish | source
end
