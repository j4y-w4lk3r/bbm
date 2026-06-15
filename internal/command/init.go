package command

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/j4y-w4lk3r/bbm/internal/config"
)

// RunInit walks the user through writing ~/.config/bbm/config.toml from
// scratch. No network calls — this is purely "ask, write, chmod 600".
//
// With --from-op, credentials are discovered from 1Password and written
// as op:// references (plus plaintext key_id). An optional [admin] block
// is added when a second key with writeBuckets is found (e.g. k4i).
func RunInit(g *Globals, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm init [--force] [--path PATH] [--from-op]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Write ~/.config/bbm/config.toml interactively, or pull Backblaze")
		fmt.Fprintln(fs.Output(), "credentials from 1Password with --from-op.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	force := fs.Bool("force", false, "overwrite an existing config.toml")
	custom := fs.String("path", "", "write to this path instead of $XDG_CONFIG_HOME/bbm/config.toml")
	fromOp := fs.Bool("from-op", false, "discover B2 keys from 1Password and write op:// references")
	opVault := fs.String("op-vault", "", "limit 1Password search to this vault name or ID")
	region := fs.String("region", "eu-central-003", "B2 region (used with --from-op)")
	endpoint := fs.String("endpoint", "", "S3 endpoint (default: derived from --region)")
	bucket := fs.String("bucket", "j4y-bu", "default bucket for object operations (used with --from-op)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	path := *custom
	if path == "" {
		path = config.SuggestedPath()
	}

	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s already exists (pass --force to overwrite)", path)
	}

	if *fromOp {
		return runInitFromOp(path, *opVault, *region, *endpoint, *bucket)
	}

	return runInitInteractive(path)
}

func runInitFromOp(path, opVault, region, endpoint, bucket string) error {
	if !config.OpAvailable() {
		return fmt.Errorf("1Password CLI (`op`) not found on PATH — install it or run `bbm init` without --from-op")
	}

	fmt.Fprintln(os.Stderr, "bbm init --from-op — discovering Backblaze keys in 1Password…")

	items, err := config.OpFindBackblazeItems(opVault)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no Backblaze/B2 items found in 1Password (try --op-vault)")
	}

	fmt.Fprintln(os.Stderr, "Found:")
	fmt.Fprint(os.Stderr, config.OpItemTitles(items))

	opsItem, ok := config.OpPickItem(items, "bu bundle", "bbm", "j4y-bu")
	if !ok {
		return fmt.Errorf("could not find an ops key (looked for titles containing \"bu bundle\", \"bbm\", or \"j4y-bu\")\n%s", config.OpItemTitles(items))
	}
	opsKey, err := config.OpKeyFromItem(opsItem)
	if err != nil {
		return fmt.Errorf("ops key %q: %w", opsItem.Title, err)
	}
	if err := config.OpVerifyKey(opsKey); err != nil {
		return fmt.Errorf("ops key %q: %w", opsItem.Title, err)
	}
	fmt.Fprintf(os.Stderr, "ops:   %s (key_id %s…)\n", opsItem.Title, truncID(opsKey.KeyID))

	var admin *config.AdminConfig
	if adminItem, ok := config.OpPickItem(items, "application key k4i", "k4i", "writebuckets", "admin"); ok && adminItem.ID != opsItem.ID {
		adminKey, err := config.OpKeyFromItem(adminItem)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: admin key %q: %v (skipping [admin])\n", adminItem.Title, err)
		} else if err := config.OpVerifyKey(adminKey); err != nil {
			fmt.Fprintf(os.Stderr, "warn: admin key %q: %v (skipping [admin])\n", adminItem.Title, err)
		} else {
			admin = &config.AdminConfig{
				KeyID:  adminKey.KeyID,
				AppKey: adminKey.AppKeyRef,
			}
			fmt.Fprintf(os.Stderr, "admin: %s (key_id %s…)\n", adminItem.Title, truncID(adminKey.KeyID))
		}
	} else {
		fmt.Fprintln(os.Stderr, "admin: (none — bucket create/delete will use the ops key)")
	}

	if endpoint == "" {
		endpoint = "https://s3." + region + ".backblazeb2.com"
	}
	if bucket == "" {
		return fmt.Errorf("--bucket is required with --from-op")
	}

	cfg := &config.Config{
		Provider: "b2",
		Endpoint: endpoint,
		Region:   region,
		Bucket:   bucket,
		KeyID:    opsKey.KeyID,
		AppKey:   opsKey.AppKeyRef,
		Admin:    admin,
	}

	if err := ensureDir(path); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	body := renderTOML(cfg)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Wrote %s (mode 600).\n", path)
	fmt.Fprintln(os.Stderr, "app_key values are op:// references — `op` is invoked at runtime.")
	if admin != nil {
		fmt.Fprintln(os.Stderr, "[admin] is set — `bbm bucket` uses the admin key automatically.")
	}
	fmt.Fprintln(os.Stderr, "Next: `bbm ls` and `bbm bucket list` to verify.")
	return nil
}

func runInitInteractive(path string) error {
	r := bufio.NewReader(os.Stdin)
	fmt.Fprintln(os.Stderr, "bbm init — interactive setup")
	fmt.Fprintln(os.Stderr, "(Hit ENTER to accept the [default] in brackets.)")
	fmt.Fprintln(os.Stderr, "")

	provider, err := promptLineDefault(r, "provider (b2|wasabi|r2|s3)", "b2")
	if err != nil {
		return err
	}
	provider = strings.ToLower(provider)

	region, err := promptLineDefault(r, "region", defaultRegion(provider))
	if err != nil {
		return err
	}

	endpoint, err := promptLineDefault(r, "endpoint", defaultEndpoint(provider, region))
	if err != nil {
		return err
	}

	bucket, err := promptLine(r, "bucket")
	if err != nil {
		return err
	}
	if bucket == "" {
		return fmt.Errorf("bucket is required")
	}

	keyID, err := promptLine(r, "key_id (B2 application keyID — NOT account ID)")
	if err != nil {
		return err
	}
	if keyID == "" {
		return fmt.Errorf("key_id is required")
	}

	appKey, err := promptLineDefault(r, "app_key (literal value or op:// reference)", "op://Personal/Backblaze/credential")
	if err != nil {
		return err
	}
	if appKey == "" {
		return fmt.Errorf("app_key is required")
	}

	if err := ensureDir(path); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	body := renderTOML(&config.Config{
		Provider: provider,
		Endpoint: endpoint,
		Region:   region,
		Bucket:   bucket,
		KeyID:    keyID,
		AppKey:   appKey,
	})

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Wrote %s (mode 600).\n", path)
	if strings.HasPrefix(appKey, "op://") {
		fmt.Fprintln(os.Stderr, "Note: app_key is an op:// reference — `op` will be invoked at runtime.")
	}
	fmt.Fprintln(os.Stderr, "Next: `bbm ls` to verify.")
	return nil
}

func truncID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func defaultRegion(provider string) string {
	switch provider {
	case "wasabi":
		return "us-east-1"
	case "r2":
		return "auto"
	case "s3":
		return "us-east-1"
	default:
		return "us-west-002"
	}
}

func defaultEndpoint(provider, region string) string {
	switch provider {
	case "wasabi":
		return "https://s3." + region + ".wasabisys.com"
	case "r2":
		return ""
	case "s3":
		return ""
	default:
		return "https://s3." + region + ".backblazeb2.com"
	}
}

// renderTOML hand-renders the config rather than going through
// toml.Marshal because (a) we want comments and (b) the encoding/toml
// landscape in Go has a few footguns around how booleans/strings get
// quoted that aren't worth depending on here.
func renderTOML(c *config.Config) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# bbm configuration — written by `bbm init`. chmod 600.")
	fmt.Fprintln(&b, "")
	fmt.Fprintf(&b, "provider = %q\n", c.Provider)
	fmt.Fprintf(&b, "endpoint = %q\n", c.Endpoint)
	fmt.Fprintf(&b, "region   = %q\n", c.Region)
	fmt.Fprintf(&b, "bucket   = %q\n", c.Bucket)
	fmt.Fprintf(&b, "key_id   = %q\n", c.KeyID)
	fmt.Fprintf(&b, "app_key  = %q\n", c.AppKey)
	if c.Admin != nil {
		fmt.Fprintln(&b, "")
		fmt.Fprintln(&b, "# Optional account-level key for `bbm bucket` (writeBuckets).")
		fmt.Fprintln(&b, "[admin]")
		fmt.Fprintf(&b, "key_id  = %q\n", c.Admin.KeyID)
		fmt.Fprintf(&b, "app_key = %q\n", c.Admin.AppKey)
	}
	return b.String()
}
