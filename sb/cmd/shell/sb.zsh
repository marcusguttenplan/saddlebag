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

# Track last directory to detect cd and re-resolve desk
_saddlebag_last_dir=""

_saddlebag_refresh() {
  # $SADDLEBAG_DESK is the shell pin (only set by `sb use`)
  # If pinned, never re-resolve — the user explicitly chose this desk
  if [[ -n "$SADDLEBAG_DESK" ]]; then
    return
  fi

  # Re-resolve on every directory change (workdir/local tier may change)
  if [[ "$PWD" != "$_saddlebag_last_dir" ]]; then
    eval "$(command sb env 2>/dev/null)"
    _saddlebag_last_dir="$PWD"
  fi
}

# Hook into zsh precmd to refresh env vars on each prompt
if [[ -n "$precmd_functions" ]]; then
  precmd_functions+=(_saddlebag_refresh)
else
  precmd_functions=(_saddlebag_refresh)
fi

# Initial load
eval "$(command sb env 2>/dev/null)"
_saddlebag_last_dir="$PWD"
