#!/usr/bin/env bash
# Bump the bbm-bin AUR package to the version of the latest GitHub release.
#
# Usage:  ./arch/aur-bump.sh <version>     # e.g. 0.1.0 (no leading 'v')
#
# This is a manual fallback. Normal release flow auto-pushes the AUR via
# the `aurs:` block in .goreleaser.yaml on every tag. Use this script if
# the goreleaser AUR push ever fails and you need to re-publish without
# re-tagging.
#
# Requires:
#   - SSH access to aur.archlinux.org as your AUR user
#     (~/.ssh/config Host aur.archlinux.org → User aur, IdentityFile ...)
#   - The matching release already published at
#       https://github.com/j4y-w4lk3r/bbm/releases/tag/v<version>
#     with bbm_<version>_Linux_x86_64.tar.gz attached (goreleaser does this).
#
# What it does:
#   1. Clones (or refreshes) the AUR bbm-bin repo into /tmp/bbm-bin-aur
#   2. Updates pkgver, pkgrel, and the source SHA256 by curl'ing the
#      published Linux x86_64 tarball from the GitHub release
#   3. Regenerates .SRCINFO via `makepkg --printsrcinfo` (or sed fallback
#      when running outside Arch)
#   4. Commits + pushes back to the AUR
#
# Idempotent: re-running with the same version is a no-op apart from a
# pkgrel bump.

set -euo pipefail

VER="${1:-${VER:-}}"
if [[ -z "$VER" ]]; then
    echo "usage: $0 <version>     # e.g. 0.1.0 (no leading 'v')" >&2
    exit 1
fi

REPO="https://github.com/j4y-w4lk3r/bbm"
AUR="ssh://aur@aur.archlinux.org/bbm-bin.git"
WORK="/tmp/bbm-bin-aur"

echo ">>> Cloning AUR repo into $WORK"
rm -rf "$WORK"
git clone "$AUR" "$WORK"

cd "$WORK"

echo ">>> Fetching release SHA256 for v${VER} (Linux x86_64)"
url="${REPO}/releases/download/v${VER}/bbm_${VER}_Linux_x86_64.tar.gz"
sha=$(curl -fsSL "$url" | sha256sum | awk '{print $1}')
echo "    sha256: $sha"

echo ">>> Patching PKGBUILD"
sed -i.bak "s|^pkgver=.*|pkgver=${VER}|" PKGBUILD
sed -i.bak "s|^pkgrel=.*|pkgrel=1|"      PKGBUILD
sed -i.bak "s|^sha256sums=.*|sha256sums=('${sha}')|" PKGBUILD
rm -f PKGBUILD.bak

echo ">>> Regenerating .SRCINFO"
if command -v makepkg >/dev/null 2>&1; then
    makepkg --printsrcinfo > .SRCINFO
else
    echo "    (no makepkg on PATH — falling back to sed)" >&2
    sed -i.bak "s|^\([[:space:]]*\)pkgver = .*|\1pkgver = ${VER}|"       .SRCINFO
    sed -i.bak "s|^\([[:space:]]*\)pkgrel = .*|\1pkgrel = 1|"            .SRCINFO
    sed -i.bak "s|^\([[:space:]]*\)sha256sums = .*|\1sha256sums = ${sha}|" .SRCINFO
    sed -i.bak "s|/v[0-9.]*/bbm_[0-9.]*_Linux_x86_64\.tar\.gz|/v${VER}/bbm_${VER}_Linux_x86_64.tar.gz|" .SRCINFO
    rm -f .SRCINFO.bak
fi

echo ">>> Committing + pushing"
git add PKGBUILD .SRCINFO
if git diff --cached --quiet; then
    echo "    (no changes — already at v${VER})"
else
    git commit -m "bbm-bin: bump to ${VER}"
    git push origin HEAD:master
fi

echo
echo "[OK] bbm-bin bumped to v${VER} on AUR."
echo "     https://aur.archlinux.org/packages/bbm-bin"
