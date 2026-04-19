#!/usr/bin/env bash
set -euo pipefail

# MnemosyneOS - macOS background service installer (launchd)
# Usage:
#   ./scripts/install-macos-service.sh [--addr :8080] [--label com.mnemosyneos.agent] [--workspace-root <path>] [--runtime-root <path>]
#
# After install:
#   launchctl list | grep com.mnemosyneos.agent
#   curl http://127.0.0.1:8080/health
#   go run ./cmd/mnemosynectl chat
#

ADDR=":8080"
LABEL="com.mnemosyneos.agent"
WORKSPACE_ROOT="$(pwd)"
RUNTIME_ROOT="$HOME/.mnemosyneos/runtime"
BIN_DIR="$HOME/.mnemosyneos/bin"
BIN_PATH="$BIN_DIR/mnemosynectl"
PLIST_PATH="$HOME/Library/LaunchAgents/${LABEL}.plist"
LOG_DIR="$HOME/Library/Logs"
OUT_LOG="$LOG_DIR/mnemosyneos.out.log"
ERR_LOG="$LOG_DIR/mnemosyneos.err.log"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --addr) ADDR="${2:?}"; shift 2 ;;
    --label) LABEL="${2:?}"; PLIST_PATH="$HOME/Library/LaunchAgents/${LABEL}.plist"; shift 2 ;;
    --workspace-root) WORKSPACE_ROOT="$(cd "$2" && pwd)"; shift 2 ;;
    --runtime-root) RUNTIME_ROOT="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

echo "Installing MnemosyneOS launchd service:"
echo "- Label:           $LABEL"
echo "- Address:         $ADDR"
echo "- Workspace root:  $WORKSPACE_ROOT"
echo "- Runtime root:    $RUNTIME_ROOT"
echo "- Binary path:     $BIN_PATH"
echo "- Plist path:      $PLIST_PATH"
echo

mkdir -p "$BIN_DIR" "$(dirname "$PLIST_PATH")" "$LOG_DIR" "$RUNTIME_ROOT"

echo "Building mnemosynectl binary..."
GO111MODULE=on go build -o "$BIN_PATH" ./cmd/mnemosynectl
chmod +x "$BIN_PATH"

# Bootstrap runtime if missing
if [[ ! -f "$RUNTIME_ROOT/state/runtime.json" ]]; then
  echo "Bootstrapping runtime at $RUNTIME_ROOT ..."
  "$BIN_PATH" init --runtime-root "$RUNTIME_ROOT" || {
    echo "Failed to bootstrap runtime. Please run: $BIN_PATH init --runtime-root \"$RUNTIME_ROOT\"" >&2
    exit 1
  }
fi

echo "Writing launchd plist..."
cat >"$PLIST_PATH" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>${LABEL}</string>
    <key>ProgramArguments</key>
    <array>
      <string>${BIN_PATH}</string>
      <string>serve</string>
      <string>--ui</string>
      <string>web</string>
      <string>--addr</string>
      <string>${ADDR}</string>
    </array>
    <key>WorkingDirectory</key>
    <string>${WORKSPACE_ROOT}</string>
    <key>EnvironmentVariables</key>
    <dict>
      <key>MNEMOSYNE_RUNTIME_ROOT</key><string>${RUNTIME_ROOT}</string>
      <key>MNEMOSYNE_ADDR</key><string>${ADDR}</string>
      <key>MNEMOSYNE_WORKSPACE_ROOT</key><string>${WORKSPACE_ROOT}</string>
    </dict>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>${OUT_LOG}</string>
    <key>StandardErrorPath</key><string>${ERR_LOG}</string>
  </dict>
</plist>
PLIST

echo "Reloading launchd service..."
if launchctl list | grep -q "$LABEL"; then
  launchctl unload -w "$PLIST_PATH" || true
fi
launchctl load -w "$PLIST_PATH"
launchctl start "$LABEL" || true

echo
echo "Installed. Health check:"
echo "  curl http://127.0.0.1${ADDR#/}/health"
echo
echo "Logs:"
echo "  tail -f \"$OUT_LOG\""
echo "  tail -f \"$ERR_LOG\""
echo
echo "CLI chat:"
echo "  go run ./cmd/mnemosynectl chat"
echo
echo "To uninstall:"
echo "  ./scripts/uninstall-macos-service.sh --label \"$LABEL\""

