package command

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunPush uploads <file> to the configured bucket. With no <key>, the
// object lands at $(basename file). With --encrypt, the file is first
// piped through `ykw encrypt` (which produces $file.gpg) and the .gpg
// is uploaded under the corresponding .gpg key.
//
// --encrypt is the ergonomic path for the bu-bundle flow — keeps you
// from having to remember the literal `ykw encrypt && bbm push` two-step
// every time. ykw and bbm stay decoupled at the Go-level (no shared
// imports), composed via PATH.
func RunPush(g *Globals, args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: bbm push [--encrypt] [--keep-encrypted] FILE [KEY]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Upload FILE to KEY in the configured bucket.")
		fmt.Fprintln(fs.Output(), "When KEY is omitted, defaults to $(basename FILE).")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "--encrypt runs `ykw encrypt FILE` first and uploads the resulting")
		fmt.Fprintln(fs.Output(), ".gpg, with KEY auto-suffixed to .gpg. By default the local .gpg is")
		fmt.Fprintln(fs.Output(), "removed after a successful upload; pass --keep-encrypted to keep it.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	encrypt := fs.Bool("encrypt", false, "GPG-encrypt with `ykw encrypt` before uploading")
	keepEnc := fs.Bool("keep-encrypted", false, "keep the local .gpg after a successful upload (--encrypt only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing FILE")
	}

	file := fs.Arg(0)
	key := ""
	if fs.NArg() >= 2 {
		key = fs.Arg(1)
	}

	if _, err := os.Stat(file); err != nil {
		return fmt.Errorf("stat %s: %w", file, err)
	}

	if *encrypt {
		gpgPath, err := runYkwEncrypt(file)
		if err != nil {
			return err
		}
		if !*keepEnc {
			defer func() {
				_ = os.Remove(gpgPath)
			}()
		}
		file = gpgPath
		if key == "" {
			key = filepath.Base(file)
		} else if !strings.HasSuffix(key, ".gpg") {
			key += ".gpg"
		}
	}

	if key == "" {
		key = filepath.Base(file)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, _, err := loadBackend(ctx, g)
	if err != nil {
		return err
	}

	in, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("open %s: %w", file, err)
	}
	defer in.Close()

	fi, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", file, err)
	}

	if err := b.Put(ctx, key, in, fi.Size()); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "pushed %s → %s (%s)\n", file, key, HumanSize(fi.Size()))
	return nil
}

// runYkwEncrypt shells out to `ykw encrypt <file>`. The convention
// (which ykw documents) is that the resulting ciphertext lands at
// <file>.gpg in the same directory. We don't try to teach bbm anything
// about the encryption mechanism — that's ykw's job.
func runYkwEncrypt(file string) (string, error) {
	if _, err := exec.LookPath("ykw"); err != nil {
		return "", fmt.Errorf("--encrypt requires `ykw` on PATH (install via brew/aur or set up locally)")
	}
	cmd := exec.Command("ykw", "encrypt", file)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ykw encrypt %s: %w", file, err)
	}
	gpgPath := file + ".gpg"
	if _, err := os.Stat(gpgPath); err != nil {
		return "", fmt.Errorf("ykw encrypt produced no %s (expected at $FILE.gpg): %w", gpgPath, err)
	}
	return gpgPath, nil
}
