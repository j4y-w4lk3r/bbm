#!/usr/bin/env bash
# Show B2 mount health and what to do next.
set -euo pipefail
MOUNT="$HOME/mnt/j4y-bu"
RCLONE="${RCLONE_BIN:-/usr/local/bin/rclone}"

echo "=== B2 mount status ==="
echo "mount point: $MOUNT"
echo ""

if mount | grep -q " on ${MOUNT} "; then
  echo "mount table:  YES (listed)"
else
  echo "mount table:  NO"
fi

if pgrep -f "rclone.*mount.*j4y-bu" >/dev/null; then
  echo "rclone proc:  YES ($(pgrep -f 'rclone.*mount.*j4y-bu'))"
else
  echo "rclone proc:  NO  <-- mount is dead or never started"
fi

if ls "$MOUNT" >/dev/null 2>&1; then
  echo "ls mount:     OK"
  echo "contents:     $(ls "$MOUNT" | tr '\n' ' ')"
else
  echo "ls mount:     FAILED (Device not configured or not mounted)"
fi

echo ""
echo "=== B2 via rclone (no mount needed) ==="
"$RCLONE" lsf lsybb0:j4y-bu/bu --format "p" 2>&1 || true

echo ""
echo "=== To mount (run in a tab, KEEP IT OPEN) ==="
echo "  bash ~/.config/yazi/plugins/bbm.yazi/mount-b2-term.sh"
echo ""
echo "=== To list without mount ==="
echo "  bash ~/.config/yazi/plugins/bbm.yazi/b2-ls.sh"
