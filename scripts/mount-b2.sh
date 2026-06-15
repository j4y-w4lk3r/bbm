#!/usr/bin/env bash
# Mount lsybb0:j4y-bu for Yazi / Finder browsing.
# Usage: mount-b2.sh [subfolder]   (default subfolder: bu)
set -euo pipefail

REMOTE="lsybb0:j4y-bu"
MOUNT="$HOME/mnt/j4y-bu"
RCLONE="${RCLONE_BIN:-/usr/local/bin/rclone}"
SUB="${1:-bu}"
LOG="$HOME/.local/state/yazi/rclone-j4y-bu.log"

if [[ ! -x "$RCLONE" ]]; then
  echo "rclone not found at $RCLONE (install official binary, not Homebrew)" >&2
  exit 1
fi

mkdir -p "$MOUNT" "$(dirname "$LOG")"

alive() {
  ls "$MOUNT" >/dev/null 2>&1
}

in_mount_table() {
  mount | grep -q " on ${MOUNT} "
}

cleanup_stale() {
  echo "cleaning stale mount at $MOUNT" >&2
  pkill -9 -f "rclone.*mount.*j4y-bu" 2>/dev/null || true
  umount -f "$MOUNT" 2>/dev/null || true
  diskutil unmount force "$MOUNT" 2>/dev/null || true
  # Wait until macOS drops the dead FUSE entry from the mount table.
  for _ in {1..20}; do
    in_mount_table || return 0
    sleep 0.25
  done
  echo "warning: $MOUNT still in mount table after cleanup" >&2
}

if in_mount_table && ! alive; then
  cleanup_stale
fi

if ! in_mount_table || ! alive; then
  echo "mounting $REMOTE → $MOUNT" >&2
  "$RCLONE" mount "$REMOTE" "$MOUNT" \
    --vfs-cache-mode full \
    --dir-cache-time 30s \
    --daemon-timeout 30m \
    --log-file "$LOG" \
    --log-level INFO \
    --daemon
  for _ in {1..30}; do
    if in_mount_table && alive; then
      break
    fi
    sleep 0.25
  done
fi

if ! alive; then
  echo "mount failed — check $LOG and macFUSE" >&2
  tail -5 "$LOG" 2>/dev/null >&2 || true
  exit 1
fi

if ! pgrep -f "rclone.*mount.*j4y-bu" >/dev/null; then
  echo "warning: mount looks alive but rclone daemon is gone" >&2
fi

DEST="$MOUNT/$SUB"
if [[ ! -d "$DEST" ]] && [[ ! -f "$DEST" ]]; then
  # Prefix may be a virtual dir; still listable once mounted.
  ls "$DEST" >/dev/null 2>&1 || {
    echo "subfolder $DEST not reachable" >&2
    DEST="$MOUNT"
  }
fi

echo "$DEST"
