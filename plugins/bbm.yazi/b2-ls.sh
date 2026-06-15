#!/usr/bin/env bash
# List B2 bucket contents WITHOUT FUSE mount (always works).
set -euo pipefail
RCLONE="${RCLONE_BIN:-/usr/local/bin/rclone}"
REMOTE="${1:-lsybb0:j4y-bu/bu}"

"$RCLONE" lsf "$REMOTE" --format "p" 2>&1
