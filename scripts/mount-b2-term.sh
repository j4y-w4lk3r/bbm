#!/usr/bin/env bash
# Run this in a dedicated terminal tab and LEAVE IT OPEN.
# While it runs, open another tab/Yazi and:  ls ~/mnt/j4y-bu/bu
set -euo pipefail

REMOTE="lsybb0:j4y-bu"
MOUNT="$HOME/mnt/j4y-bu"
RCLONE="${RCLONE_BIN:-/usr/local/bin/rclone}"

if [[ ! -x "$RCLONE" ]]; then
  echo "Need official rclone at /usr/local/bin/rclone (not Homebrew)" >&2
  exit 1
fi

mkdir -p "$MOUNT"

echo "== cleaning old mounts =="
pkill -9 -f "rclone.*mount.*j4y-bu" 2>/dev/null || true
umount -f "$MOUNT" 2>/dev/null || true
diskutil unmount force "$MOUNT" 2>/dev/null || true
sleep 1

if mount | grep -q " on ${MOUNT} "; then
  echo "ERROR: stale mount still listed. Run:  sudo umount -f $MOUNT" >&2
  exit 1
fi

echo "== mounting $REMOTE -> $MOUNT =="
echo "Keep THIS terminal open. Ctrl-C unmounts."
echo "In another tab:  ls $MOUNT/bu"
echo ""

exec "$RCLONE" mount "$REMOTE" "$MOUNT" \
  --vfs-cache-mode full \
  --dir-cache-time 30s \
  --log-level INFO
