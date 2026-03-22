# sb — Saddlebag CLI companion

_saddlebag_refresh() {
  eval "$(sb env 2>/dev/null)"
}

# Hook into zsh precmd to refresh env vars on each prompt
if [[ -n "$precmd_functions" ]]; then
  precmd_functions+=(_saddlebag_refresh)
else
  precmd_functions=(_saddlebag_refresh)
fi

# Initial load
_saddlebag_refresh
