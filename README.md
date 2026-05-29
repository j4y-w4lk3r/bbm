# bbm — Backblaze B2 manager

Focused CLI for the [`bu`](https://github.com/j4y-w4lk3r/bu) encrypted-bundle workflow. Built on B2's S3-compatible API, so it works with Wasabi, Cloudflare R2, and AWS S3 by changing one config line.

Sister tool to [`rui`](https://github.com/j4y-w4lk3r/rui) (router TUI) and [`ykw`](https://github.com/j4y-w4lk3r/ykw) (YubiKey OpenPGP workflow). Same packaging shape, same release cadence, same single-tag-triggers-three-channels distribution.

## Why not rclone?

[rclone](https://rclone.org) is a great general-purpose object-store client. `bbm` is not trying to compete with it — it's purpose-built for the `bu` bundle:

- one bucket, a handful of small encrypted blobs
- `bbm push --encrypt` shells out to `ykw` for one-line GPG-encrypted uploads
- `op://` references in the config so the B2 application key never lands on disk

Use rclone for "sync 200GB of photos to B2." Use `bbm` for "fetch the encrypted secrets bundle and pipe it into ykw."

## Install

### macOS / Linux (Homebrew)

```bash
brew tap j4y-w4lk3r/bbm
brew install bbm
```

### Arch Linux (AUR)

```bash
yay -S bbm-bin   # or paru, or any AUR helper
```

### From source

```bash
git clone https://github.com/j4y-w4lk3r/bbm
cd bbm
go install ./cmd/bbm
```

## First run

```bash
bbm init
```

Walks through the config interactively and writes `~/.config/bbm/config.toml` (mode 600). For the `app_key` field, paste either a literal value or an `op://` reference (e.g. `op://Personal/Backblaze/credential`) — the latter resolves at runtime via the [1Password CLI](https://developer.1password.com/docs/cli/) so the secret never lands on disk.

Verify with:

```bash
bbm ls
```

## Usage

```text
bbm init                          interactively write ~/.config/bbm/config.toml
bbm ls [PREFIX]                   list objects in the bucket
bbm pull KEY [DEST]               download an object
bbm push [--encrypt] FILE         upload (--encrypt pipes through `ykw encrypt`)
bbm cat KEY                       stream an object to stdout
bbm rm [--yes] KEY                delete an object
bbm bucket create NAME            create a new bucket (account-level)
bbm bucket list                   list every bucket the credentials see
bbm bucket delete [--yes] NAME    delete an EMPTY bucket
```

### Examples

```bash
# List everything under bu/
bbm ls bu/

# Download the encrypted bundle to ./
bbm pull bu/secret.txt.gpg

# Encrypt + upload in one shot
bbm push --encrypt secret.txt bu/secret.txt
# (uploads secret.txt.gpg → bu/secret.txt.gpg)

# Pipe out of bbm straight into gpg
bbm cat bu/secret.txt.gpg | gpg -d

# Account-level bucket admin (handy for one-shot encrypted backups —
# spin up a dedicated bucket, push the blob, retire it later)
bbm bucket list
bbm bucket create j4y-backup-2026-q2
bbm --bucket j4y-backup-2026-q2 push --encrypt big-archive.tar j4y-backup-2026-q2/big-archive.tar.gpg
bbm --bucket j4y-backup-2026-q2 rm j4y-backup-2026-q2/big-archive.tar.gpg
bbm bucket delete --yes j4y-backup-2026-q2
```

## Configuration

`bbm` reads config from a cascade. Per-field, the first non-empty wins:

1. CLI flags (`--bucket`, `--key-id`, `--app-key`, `--endpoint`, `--region`)
2. Process env vars (`B2_KEY_ID`, `B2_APP_KEY`, `B2_BUCKET`, `B2_ENDPOINT`, `B2_REGION`)
3. `~/.config/bbm/config.toml` (or `$XDG_CONFIG_HOME/bbm/config.toml`)

See [config.example.toml](./config.example.toml) for the full schema.

## Other providers

`bbm` talks S3 end-to-end. To point it at Wasabi, R2, or AWS S3:

```toml
# Wasabi
provider = "wasabi"
endpoint = "https://s3.us-east-1.wasabisys.com"
region   = "us-east-1"

# Cloudflare R2
provider = "r2"
endpoint = "https://<account-id>.r2.cloudflarestorage.com"
region   = "auto"

# AWS S3 proper
provider = "s3"
endpoint = ""        # leave empty to use SDK default URL resolution
region   = "us-east-1"
```

The v0.1.0 label says only B2 is "tested," but the code path is identical — these should Just Work.

## Releasing

```bash
git tag v0.1.x
git push origin v0.1.x
```

Triggers `.github/workflows/release.yml`, which:

1. Runs goreleaser → 7 build targets (darwin + linux + windows × amd64/arm64, plus linux/armv7), tarballed/zipped with LICENSE + README + config.example.toml.
2. Creates the GitHub release with all artifacts attached.
3. Renders `Formula/bbm.rb` and pushes to `j4y-w4lk3r/homebrew-bbm`.
4. Renders `bbm-bin` PKGBUILD + .SRCINFO and pushes to `aur.archlinux.org/bbm-bin.git`.

Three channels, one tag. See `arch/aur-bootstrap.sh` for the one-shot AUR repo bootstrap (only needed once for v0.1.0).

## License

[MIT](./LICENSE)
