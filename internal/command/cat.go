package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

// RunCat streams an object's body to stdout. Useful for piping into
// gpg, jq, etc. without leaving a file on disk:
//
//	bbm cat secret.txt.gpg | gpg -d
//	bbm cat machines.json | jq .[].hostname
//
// Stderr is reserved for diagnostics so the stdout stream stays clean
// for downstream pipes.
func RunCat(g *Globals, args []string) error {
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm cat KEY")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Stream the object KEY to stdout.")
	}
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

	b, _, err := loadBackend(ctx, g)
	if err != nil {
		return err
	}

	body, _, err := b.Get(ctx, key)
	if err != nil {
		return err
	}
	defer body.Close()

	if _, err := io.Copy(os.Stdout, body); err != nil {
		return fmt.Errorf("stream %s: %w", key, err)
	}
	return nil
}
