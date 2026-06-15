#!/usr/bin/env bash
set -euo pipefail
LABEL="com.j4y.rclone-j4y-bu"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
UID="$(id -u)"
launchctl bootout "gui/$UID" "$PLIST" 2>/dev/null || true
rm -f "$PLIST"
echo "Removed launchd agent $LABEL"
