package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RunPull downloads <key> from the configured bucket. With no <dest>,
// the file lands in $PWD as the basename of <key>. With <dest> a
// directory, $dest/$(basename key). With <dest> a path, that exact path.
//
// The download is streamed; bbm never buffers the whole object in RAM,
// which matters for the bu-bundle workflow (small today, but might grow
// to encrypted home-dir tarballs later).
func RunPull(g *Globals, args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm pull [--force] KEY [DEST]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Download KEY into DEST. DEST may be a file path or a directory.")
		fmt.Fprintln(fs.Output(), "When omitted, defaults to ./$(basename KEY).")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	force := fs.Bool("force", false, "overwrite DEST if it already exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing KEY")
	}

	key := fs.Arg(0)
	dest := ""
	if fs.NArg() >= 2 {
		dest = fs.Arg(1)
	}
	if dest == "" {
		dest = filepath.Base(key)
	}
	if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
		dest = filepath.Join(dest, filepath.Base(key))
	}

	if _, err := os.Stat(dest); err == nil && !*force {
		return fmt.Errorf("%s already exists (pass --force to overwrite)", dest)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, _, err := loadBackend(ctx, g)
	if err != nil {
		return err
	}

	body, size, err := b.Get(ctx, key)
	if err != nil {
		return err
	}
	defer body.Close()

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer out.Close()

	n, err := io.Copy(out, body)
	if err != nil {
		return fmt.Errorf("download %s → %s: %w", key, dest, err)
	}

	if size >= 0 && n != size {
		return fmt.Errorf("short read: got %d of %d bytes for %s", n, size, key)
	}

	fmt.Fprintf(os.Stderr, "pulled %s → %s (%s)\n", key, dest, HumanSize(n))
	return nil
}
