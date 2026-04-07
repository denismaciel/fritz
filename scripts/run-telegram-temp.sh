#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
workspace="${WORKSPACE:-$(mktemp -d -t fritz-telegram-XXXXXX)}"
pairing_token="${PAIRING_TOKEN:-secret}"
binary="${BINARY_PATH:-$workspace/bin/fritz}"
telegram_bot_token="${TELEGRAM_BOT_TOKEN:-8772278315:AAH6EL8vrMPQtPP7MBO5F9igQwAIoigwO34}"

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    printf 'missing env: %s\n' "$name" >&2
    exit 1
  fi
}

require_env GEMINI_API_KEY

mkdir -p "$workspace/.fritz" "$workspace/memory" "$(dirname "$binary")"

if [[ ! -f "$workspace/AGENTS.md" ]]; then
  cat >"$workspace/AGENTS.md" <<'EOF'
Be terse. Prefer direct answers.
EOF
fi

if [[ ! -f "$workspace/MEMORY.md" ]]; then
  : >"$workspace/MEMORY.md"
fi

if [[ ! -f "$workspace/HEARTBEAT.md" ]]; then
  cat >"$workspace/HEARTBEAT.md" <<'EOF'
If nothing needs attention, reply HEARTBEAT_OK.
EOF
fi

printf 'building: %s\n' "$binary"
(
  cd "$repo_root"
  go build -o "$binary" ./cmd/fritz
)

printf 'workspace: %s\n' "$workspace"
printf 'bot: %s\n' "${TELEGRAM_BOT_USERNAME:-unknown}"
printf 'pairing token: %s\n' "$pairing_token"
printf 'dm first: /start %s\n' "$pairing_token"

cd "$workspace"
export TELEGRAM_BOT_TOKEN="$telegram_bot_token"
exec "$binary" --telegram-pairing-token "$pairing_token" telegram "$@"
