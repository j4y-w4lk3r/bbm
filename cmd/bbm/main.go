// Command bbm is the Backblaze B2 manager — a focused CLI for the
// `bu` encrypted-bundle workflow built on top of B2's S3-compatible API
// (so it works with Wasabi/R2/AWS-S3-proper too by pointing at a
// different endpoint).
//
// Subcommands:
//
//	bbm init                      interactively write ~/.config/bbm/config.toml
//	bbm ls [PREFIX]               list objects
//	bbm pull KEY [DEST]           download an object
//	bbm push [--encrypt] FILE     upload (--encrypt pipes through `ykw encrypt`)
//	bbm cat KEY                   stream an object to stdout
//	bbm rm [--yes] KEY            delete an object
//	bbm bucket <create|list|delete>  account-level bucket admin
//
// Global flags (BEFORE the subcommand name):
//
//	-c PATH    config file (overrides ~/.config/bbm/config.toml)
//	-v         print version and exit
//	-h         show help
//
// Per-subcommand flags follow the subcommand name (e.g. `bbm ls --limit 50`).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/j4y-w4lk3r/bbm/internal/command"
	"github.com/j4y-w4lk3r/bbm/internal/config"
)

// Stamped at link time by goreleaser via -ldflags
// "-X main.version=... -X main.commit=... -X main.date=...". Defaults
// fire on `go run` / `go build` without ldflags so we still print
// something useful in dev.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	// Parse global flags up to the first non-flag argument, which is
	// the subcommand. Use ContinueOnError so we can render our own
	// usage on errors (the default ExitOnError is too quick to bow out
	// before we've shown the subcommand list).
	gfs := flag.NewFlagSet("bbm", flag.ContinueOnError)
	gfs.SetOutput(os.Stderr)
	cfgPath := gfs.String("c", "", "config file path (overrides cascade)")
	showVer := gfs.Bool("v", false, "print version and exit")
	showVerLong := gfs.Bool("version", false, "print version and exit")
	showHelp := gfs.Bool("h", false, "show help")
	showHelpLong := gfs.Bool("help", false, "show help")

	// Per-subcommand credential overrides, parsed at the top level so
	// they work regardless of which subcommand the user lands in.
	bucket := gfs.String("bucket", "", "override bucket")
	keyID := gfs.String("key-id", "", "override key_id")
	appKey := gfs.String("app-key", "", "override app_key")
	endpoint := gfs.String("endpoint", "", "override endpoint")
	region := gfs.String("region", "", "override region")

	if err := gfs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	if *showVer || *showVerLong {
		fmt.Printf("bbm %s (commit %s, built %s)\n", version, commit, date)
		return
	}
	if *showHelp || *showHelpLong {
		printUsage(os.Stdout)
		return
	}

	rest := gfs.Args()
	if len(rest) == 0 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	g := &command.Globals{
		ConfigPath: *cfgPath,
		Overrides: config.Overrides{
			Bucket:   *bucket,
			KeyID:    *keyID,
			AppKey:   *appKey,
			Endpoint: *endpoint,
			Region:   *region,
		},
	}

	sub := rest[0]
	subArgs := rest[1:]

	var err error
	switch sub {
	case "init":
		err = command.RunInit(g, subArgs)
	case "ls", "list":
		err = command.RunLs(g, subArgs)
	case "pull", "get", "download":
		err = command.RunPull(g, subArgs)
	case "push", "put", "upload":
		err = command.RunPush(g, subArgs)
	case "cat":
		err = command.RunCat(g, subArgs)
	case "rm", "delete":
		err = command.RunRm(g, subArgs)
	case "bucket":
		err = command.RunBucket(g, subArgs)
	case "help":
		printUsage(os.Stdout)
		return
	default:
		fmt.Fprintf(os.Stderr, "bbm: unknown command %q\n\n", sub)
		printUsage(os.Stderr)
		os.Exit(2)
	}

	if err != nil {
		// ErrNoConfig gets the long-form walkthrough; everything else
		// is a one-liner. Same UX shape as rui.
		if errors.Is(err, config.ErrNoConfig) {
			printNoConfigHelp(g.ConfigPath)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "bbm:", err)
		os.Exit(1)
	}
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, "bbm — Backblaze B2 manager (S3-compatible CLI)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bbm [GLOBAL FLAGS] <subcommand> [SUBCOMMAND FLAGS] [ARGS...]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  init                  interactively write ~/.config/bbm/config.toml")
	fmt.Fprintln(w, "  ls [PREFIX]           list objects in the bucket")
	fmt.Fprintln(w, "  pull KEY [DEST]       download an object")
	fmt.Fprintln(w, "  push [--encrypt] FILE upload a file (optionally GPG-encrypted via ykw)")
	fmt.Fprintln(w, "  cat KEY               stream an object to stdout")
	fmt.Fprintln(w, "  rm [--yes] KEY        delete an object")
	fmt.Fprintln(w, "  bucket <verb> ...     account-level admin (create | list | delete)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Global flags:")
	fmt.Fprintln(w, "  -c PATH               config file (default: $XDG_CONFIG_HOME/bbm/config.toml)")
	fmt.Fprintln(w, "  -v / --version        print version and exit")
	fmt.Fprintln(w, "  -h / --help           show this help")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  --bucket NAME         override config: bucket")
	fmt.Fprintln(w, "  --key-id ID           override config: key_id")
	fmt.Fprintln(w, "  --app-key VALUE       override config: app_key (literal or op:// reference)")
	fmt.Fprintln(w, "  --endpoint URL        override config: endpoint")
	fmt.Fprintln(w, "  --region REGION       override config: region")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run `bbm <subcommand> -h` for subcommand-specific flags.")
}

// printNoConfigHelp is what a fresh `brew install bbm && bbm ls` user
// sees when they haven't configured anything yet.
func printNoConfigHelp(configPath string) {
	cands := config.Candidates(configPath)
	fmt.Fprintln(os.Stderr, "bbm: no configuration found.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Searched (in order):")
	for _, p := range cands {
		fmt.Fprintln(os.Stderr, "  -", p)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "To get started, pick ONE of:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  (a) interactive setup — RECOMMENDED")
	fmt.Fprintln(os.Stderr, "      bbm init")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  (b) one-shot via env vars")
	fmt.Fprintln(os.Stderr, "      B2_KEY_ID=... B2_APP_KEY=... B2_BUCKET=my-bucket \\")
	fmt.Fprintln(os.Stderr, "        B2_ENDPOINT=https://s3.us-west-002.backblazeb2.com bbm ls")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  (c) one-shot via flags")
	fmt.Fprintln(os.Stderr, "      bbm --bucket my-bucket --key-id ... --app-key ... \\")
	fmt.Fprintln(os.Stderr, "        --endpoint https://s3.us-west-002.backblazeb2.com ls")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "See `bbm -h` or https://github.com/j4y-w4lk3r/bbm#first-run for details.")
}
