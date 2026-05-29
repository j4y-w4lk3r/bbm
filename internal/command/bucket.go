package command

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/j4y-w4lk3r/bbm/internal/storage"
)

// RunBucket dispatches to the create / list / delete sub-subcommands.
//
// Why a sub-subcommand and not three top-level commands (`bbm mkbucket`,
// `bbm lsbucket`, `bbm rmbucket`)? Bucket admin is a separate
// conceptual layer from object operations — keeping it under a single
// `bucket` namespace keeps `bbm -h` readable and lets us add future
// admin verbs (versioning, lifecycle rules, lock policies) without
// crowding the top-level help.
func RunBucket(g *Globals, args []string) error {
	if len(args) == 0 {
		printBucketUsage(os.Stderr)
		return errors.New("bbm bucket: missing subcommand (create | list | delete)")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "create", "mk", "new":
		return runBucketCreate(g, rest)
	case "list", "ls":
		return runBucketList(g, rest)
	case "delete", "rm":
		return runBucketDelete(g, rest)
	case "-h", "--help", "help":
		printBucketUsage(os.Stdout)
		return nil
	default:
		printBucketUsage(os.Stderr)
		return fmt.Errorf("bbm bucket: unknown subcommand %q", sub)
	}
}

func printBucketUsage(w *os.File) {
	fmt.Fprintln(w, "Usage: bbm bucket <create|list|delete> [args...]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  bucket create NAME [--region R]   create a new bucket")
	fmt.Fprintln(w, "  bucket list                       list buckets the credentials can see")
	fmt.Fprintln(w, "  bucket delete [--yes] NAME        delete an EMPTY bucket")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Notes:")
	fmt.Fprintln(w, "  - `create` defaults --region to the bbm config region.")
	fmt.Fprintln(w, "  - `delete` refuses non-empty buckets (S3 semantics). Run")
	fmt.Fprintln(w, "    `bbm rm` for every key first, or use --bucket NAME for the")
	fmt.Fprintln(w, "    listing.")
}

// runBucketCreate is what materializes a fresh B2/Wasabi/R2/S3 bucket.
// The configured cfg.Bucket is irrelevant here — we use the credentials
// + region only.
func runBucketCreate(g *Globals, args []string) error {
	fs := flag.NewFlagSet("bucket create", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm bucket create NAME [--region R]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Create a new bucket with the configured credentials. If --region")
		fmt.Fprintln(fs.Output(), "is omitted, the region from the bbm config is used.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	region := fs.String("region", "", "region for the new bucket (default: same as bbm config)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("bbm bucket create: NAME is required")
	}
	name := fs.Arg(0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, cfg, err := loadBackend(ctx, g)
	if err != nil {
		return err
	}
	r := *region
	if r == "" {
		r = cfg.Region
	}

	if err := b.CreateBucket(ctx, name, r); err != nil {
		if errors.Is(err, storage.ErrBucketExists) {
			return fmt.Errorf("bucket %q already exists (you may already own it — try `bbm bucket list`)", name)
		}
		return err
	}
	fmt.Fprintf(os.Stderr, "created bucket %q in region %q\n", name, r)
	return nil
}

// runBucketList prints every bucket the credentials can see (one row
// per bucket, with creation timestamp).
func runBucketList(g *Globals, args []string) error {
	fs := flag.NewFlagSet("bucket list", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm bucket list")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "List every bucket the configured credentials can see. The")
		fmt.Fprintln(fs.Output(), "configured `bucket` field is irrelevant — this is an")
		fmt.Fprintln(fs.Output(), "account-level call.")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return errors.New("bbm bucket list: takes no positional arguments")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, _, err := loadBackend(ctx, g)
	if err != nil {
		return err
	}
	buckets, err := b.ListBuckets(ctx)
	if err != nil {
		return err
	}
	if len(buckets) == 0 {
		fmt.Fprintln(os.Stderr, "(no buckets visible to these credentials)")
		return nil
	}
	for _, bk := range buckets {
		ts := bk.CreatedAt.UTC().Format("2006-01-02 15:04:05")
		fmt.Fprintf(os.Stdout, "%s  %s\n", ts, bk.Name)
	}
	return nil
}

// runBucketDelete deletes an EMPTY bucket. Refuses without --yes unless
// stdin is non-interactive (mirrors `bbm rm`).
func runBucketDelete(g *Globals, args []string) error {
	fs := flag.NewFlagSet("bucket delete", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm bucket delete [--yes] NAME")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Delete a bucket. The bucket must be EMPTY (S3 refuses to remove")
		fmt.Fprintln(fs.Output(), "non-empty buckets). Clear it via `bbm --bucket NAME rm <key>` for")
		fmt.Fprintln(fs.Output(), "every object first.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("bbm bucket delete: NAME is required")
	}
	name := fs.Arg(0)

	if !*yes {
		if !confirm(fmt.Sprintf("Delete bucket %q? It must be EMPTY.", name)) {
			return errors.New("aborted")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, _, err := loadBackend(ctx, g)
	if err != nil {
		return err
	}
	if err := b.DeleteBucket(ctx, name); err != nil {
		if errors.Is(err, storage.ErrBucketNotEmpty) {
			return fmt.Errorf("bucket %q is not empty — run `bbm --bucket %s ls` to inspect, then delete its objects first", name, name)
		}
		return err
	}
	fmt.Fprintf(os.Stderr, "deleted bucket %q\n", name)
	return nil
}
