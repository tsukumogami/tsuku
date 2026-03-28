_tsuku_hook() {
  eval "$(tsuku hook-env zsh)"
}
if (( ! ${precmd_functions[(I)_tsuku_hook]} )); then
  precmd_functions=(_tsuku_hook $precmd_functions)
fi
