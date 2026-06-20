# bbm.yazi — Yazi plugin for Backblaze B2

One-click encrypt/push, pull/decrypt, and in-Yazi bucket browsing via [`bbm`](../../README.md) + [`ykw`](https://github.com/j4y-w4lk3r/ykw).

Yazi does not yet ship a native S3/B2 VFS provider (only SFTP in `vfs.toml` nightly). This plugin fills the gap with a `bbm ls`-backed browser UI, similar to `mount.yazi`.

## Install

```bash
ln -sfn ~/px/x-j4y/bbm/plugins/bbm.yazi ~/.config/yazi/plugins/bbm.yazi
```

Requires `bbm`, `ykw`, `tar`, and `gpg` on `PATH`. `bbm` must be configured (`bbm init`).

Optional `~/.config/yazi/init.lua` setup:

```lua
require("bbm"):setup({
    prefix = "bu/",  -- default B2 key prefix for uploads
})
```

## Keybindings

Add to `keymap.toml`:

```toml
[[mgr.prepend_keymap]]
on   = [ "b", "m" ]
run  = "plugin bbm mount"
desc = "Mount B2 bucket and cd into it"

[[mgr.prepend_keymap]]
on   = [ "b", "b" ]
run  = "plugin bbm browse"
desc = "Browse B2 bucket (modal)"

[[mgr.prepend_keymap]]
on   = [ "b", "p" ]
run  = "plugin bbm push"
desc = "Encrypt + push to B2"

[[mgr.prepend_keymap]]
on   = [ "b", "P" ]
run  = "plugin bbm pull"
desc = "Pull + decrypt from B2"
```

Configure the mount in `init.lua` (uses your existing `rclone` remote):

```lua
require("bbm"):setup({
    prefix = "bu/",
    mount = {
        remote = "lsybb0:j4y-bu",   -- rclone remote:bucket
        path = "~/mnt/j4y-bu",      -- local mount point
    },
})
```

Requires [macFUSE](https://macfuse.io/) (macOS) or FUSE (Linux) for `rclone mount`.

**macOS caveat:** Homebrew's `rclone` cannot mount. Install the official binary from [rclone.org/downloads](https://rclone.org/downloads/) and put it on your `PATH` before Homebrew's, or use `bb` (modal browser) instead — that works with Homebrew rclone + `bbm`.

## Two ways to browse

### `bm` — Mount + browse (recommended)

Press `bm` and Yazi **changes directory** into `~/mnt/j4y-bu`. The bucket appears in the normal file manager — left pane, preview, yank/paste, the works. Uses `rclone mount` under the hood.

This is the closest thing to "B2 as a Yazi remote" today.

### `bb` — Modal browser (no mount)

Press `bb` and a **popup overlay** opens on top of Yazi. You navigate with `j/k/h/l` inside the popup; your underlying cwd does not change. Listings come from `bbm ls`. Good for a quick peek without mounting.

**Bucket picker:** the browser opens on a bucket list (`bbm bucket list`). Press `l`/`Enter` to enter a bucket, `h` at the bucket root to go back to the list, `B` to switch buckets anytime. Inside a bucket, `h` navigates up through prefixes as usual.

## Actions

| Key | Action |
|-----|--------|
| `j`/`k` | Move |
| `l`/`Enter` | Enter prefix / open dir |
| `h` | Go up one prefix |
| `p` | Pull file to current Yazi cwd |
| `P` | Pull + decrypt (`.gpg` → `ykw decrypt` → `tar xf` if tarball) |
| `r` | Refresh listing |
| `q`/`Esc` | Close |

### `bp` — Encrypt + push

On the selected (or hovered) file or directory:

- **File** → `bbm push --encrypt FILE bu/FILE.gpg`
- **Directory** → `tar -czf` → `bbm push --encrypt`

Prompts for the B2 object key (defaults to `bu/<name>`).

### `bP` — Pull + decrypt

On a local `.gpg` file: `ykw decrypt` and auto-extract `.tar.gz` archives.

From the B2 browser, use `P` instead.

## Future: native VFS

When Yazi adds an S3 provider to `vfs.toml`, you can browse with `cd s3://...` directly. Until then, this plugin uses `bbm ls` as the remote listing backend.
