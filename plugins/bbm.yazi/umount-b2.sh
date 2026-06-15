#!/usr/bin/env bash
# Tear down a dead or live j4y-bu mount.
set -euo pipefail
MOUNT="$HOME/mnt/j4y-bu"

echo "killing rclone mount processes..."
pkill -9 -f "rclone.*mount.*j4y-bu" 2>/dev/null || true
sleep 0.5

echo "unmounting $MOUNT..."
umount -f "$MOUNT" 2>/dev/null || true
diskutil unmount force "$MOUNT" 2>/dev/null || true
sleep 1

if mount | grep -q " on ${MOUNT} "; then
  echo "Still in mount table. Try:  sudo umount -f $MOUNT"
  exit 1
fi

echo "clean. Run mount-b2-term.sh in a terminal tab to mount again."
