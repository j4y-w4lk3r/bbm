package command

import (
	"context"
	"flag"
	"fmt"
	"os"
)

// RunRm deletes a single object. By default it prompts for confirmation
// because deleting an object the bu-bundle workflow depends on is the
// kind of mistake you only make once before adding a confirm prompt.
//
// --yes skips the prompt for scripted use. Multi-key deletion is out of
// scope for v0.1.0; a future `bbm rm --prefix` could land if useful.
func RunRm(g *Globals, args []string) error {
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm rm [--yes] KEY")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Delete KEY from the configured bucket.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing KEY")
	}

	key := fs.Arg(0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, cfg, err := loadBackend(ctx, g)
	if err != nil {
		return err
	}

	if !*yes {
		if !confirm(fmt.Sprintf("Delete %q from %s?", key, cfg.Bucket)) {
			fmt.Fprintln(os.Stderr, "(cancelled)")
			return nil
		}
	}

	if err := b.Delete(ctx, key); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "deleted %s\n", key)
	return nil
}
