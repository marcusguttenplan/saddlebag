# sb — Saddlebag CLI companion

# Shell wrapper: intercepts commands that need to modify the calling shell
sb() {
  case "$1" in
    use)
      # sb use must eval in the calling shell to set env vars
      eval "$(command sb use "${@:2}" 2>/dev/null)"
      ;;
    *)
      command sb "$@"
      ;;
  esac
}

_saddlebag_refresh() {
  # Skip global sync if this session has a pinned desk
  if [[ -z "$SADDLEBAG_DESK" ]]; then
    eval "$(command sb env 2>/dev/null)"
  fi
}

# Hook into zsh precmd to refresh env vars on each prompt
if [[ -n "$precmd_functions" ]]; then
  precmd_functions+=(_saddlebag_refresh)
else
  precmd_functions=(_saddlebag_refresh)
fi

# Initial load
_saddlebag_refresh
