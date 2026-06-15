#!/usr/bin/env bash
# Install launchd agent to keep lsybb0:j4y-bu mounted at ~/mnt/j4y-bu
set -euo pipefail

LABEL="com.j4y.rclone-j4y-bu"
RCLONE="${RCLONE_BIN:-/usr/local/bin/rclone}"
MOUNT="$HOME/mnt/j4y-bu"
STATE="$HOME/.local/state/yazi"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
TEMPLATE="$(dirname "$0")/com.j4y.rclone-j4y-bu.plist.template"

if [[ ! -x "$RCLONE" ]]; then
  echo "Need official rclone at $RCLONE" >&2
  exit 1
fi

mkdir -p "$MOUNT" "$STATE" "$(dirname "$PLIST")"

sed \
  -e "s|__RCLONE__|$RCLONE|g" \
  -e "s|__MOUNT__|$MOUNT|g" \
  -e "s|__LOG__|$STATE/rclone-j4y-bu.log|g" \
  -e "s|__ERR__|$STATE/rclone-j4y-bu.err|g" \
  -e "s|__OUT__|$STATE/rclone-j4y-bu.out|g" \
  "$TEMPLATE" > "$PLIST"

USER_ID="$(id -u)"
launchctl bootout "gui/$USER_ID" "$PLIST" 2>/dev/null || true
launchctl bootstrap "gui/$USER_ID" "$PLIST"
launchctl enable "gui/$USER_ID/$LABEL"
launchctl kickstart -k "gui/$USER_ID/$LABEL"

echo "Installed $PLIST"
echo "Mount should appear at $MOUNT within a few seconds."
echo "Check:  bash $(dirname "$0")/b2-status.sh"
