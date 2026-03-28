_tsuku_hook() {
  local previous_exit_status=$?
  eval "$(tsuku hook-env bash)"
  return $previous_exit_status
}
if [[ ";${PROMPT_COMMAND[*]:-};" != *";_tsuku_hook;"* ]]; then
  PROMPT_COMMAND=("_tsuku_hook" "${PROMPT_COMMAND[@]}")
fi
