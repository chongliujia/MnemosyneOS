#!/usr/bin/env bash
set -euo pipefail

# MnemosyneOS - macOS background service uninstaller (launchd)
# Usage:
#   ./scripts/uninstall-macos-service.sh [--label com.mnemosyneos.agent]

LABEL="com.mnemosyneos.agent"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --label) LABEL="${2:?}"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

PLIST_PATH="$HOME/Library/LaunchAgents/${LABEL}.plist"

echo "Uninstalling MnemosyneOS launchd service:"
echo "- Label:      $LABEL"
echo "- Plist path: $PLIST_PATH"

if [[ -f "$PLIST_PATH" ]]; then
  launchctl unload -w "$PLIST_PATH" || true
  rm -f "$PLIST_PATH"
  echo "Removed $PLIST_PATH"
else
  echo "No plist found at $PLIST_PATH"
fi

echo "Done."

