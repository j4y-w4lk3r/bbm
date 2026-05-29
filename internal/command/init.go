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
// The op:// suggestion for app_key is by design: piling secrets on
// disk in plaintext is the failure mode this whole tool was built to
// reduce. Power users who don't run 1Password just paste the literal
// value when prompted; everyone else gets the secret-ref default.
func RunInit(g *Globals, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm init [--force] [--path PATH]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Interactively write ~/.config/bbm/config.toml.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	force := fs.Bool("force", false, "overwrite an existing config.toml")
	custom := fs.String("path", "", "write to this path instead of $XDG_CONFIG_HOME/bbm/config.toml")
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

	// 0600 because the file MAY contain a plaintext app_key — and even
	// when it doesn't (op:// reference), the bucket name + keyID half
	// of an Application Key is enough info to scope an attack.
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
	return b.String()
}
