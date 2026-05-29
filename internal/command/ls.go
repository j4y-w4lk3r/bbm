package command

import (
	"context"
	"flag"
	"fmt"
	"os"
)

// RunLs lists objects in the configured bucket, optionally filtered by
// a prefix. Output is one row per object: timestamp, human size, key.
//
// Pagination: List() returns a continuation token; we keep walking
// until --limit is hit (default 1000) or the listing is exhausted.
func RunLs(g *Globals, args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm ls [--limit N] [PREFIX]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "List objects in the configured bucket whose key starts with PREFIX.")
		fmt.Fprintln(fs.Output(), "PREFIX is optional; omit to list everything.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	limit := fs.Int("limit", 1000, "max objects to print")
	if err := fs.Parse(args); err != nil {
		return err
	}

	prefix := ""
	if fs.NArg() > 0 {
		prefix = fs.Arg(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, _, err := loadBackend(ctx, g)
	if err != nil {
		return err
	}

	printed := 0
	token := ""
	for printed < *limit {
		page := *limit - printed
		if page > 1000 {
			page = 1000
		}
		objs, next, err := b.List(ctx, prefix, token, page)
		if err != nil {
			return err
		}
		for _, o := range objs {
			if printed >= *limit {
				break
			}
			ts := o.LastModified.UTC().Format("2006-01-02 15:04:05")
			fmt.Fprintf(os.Stdout, "%s  %7s  %s\n", ts, HumanSize(o.Size), o.Key)
			printed++
		}
		if next == "" {
			break
		}
		token = next
	}

	if printed == 0 {
		if prefix != "" {
			fmt.Fprintf(os.Stderr, "(no objects matching prefix %q)\n", prefix)
		} else {
			fmt.Fprintln(os.Stderr, "(bucket is empty)")
		}
	}
	return nil
}
