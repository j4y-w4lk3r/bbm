// Package command implements the bbm subcommands. Each subcommand is a
// `Run<Name>(g *Globals, args []string) error` function plus a small
// `flag.NewFlagSet` for its own flags. main.go dispatches by name.
//
// Why no cobra/urfave: the surface area is six small commands and a
// global "-c" flag. Stdlib flag + a switch in main keeps the binary
// small and the cognitive load tiny — same call as rui.
package command

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/j4y-w4lk3r/bbm/internal/config"
	"github.com/j4y-w4lk3r/bbm/internal/storage"
)

// Globals are the bbm-wide flag values main.go threads into every
// subcommand. Subcommand-local flags live inside each Run<Name> body.
type Globals struct {
	ConfigPath string
	Overrides  config.Overrides
}

// loadBackend resolves config and returns a connected S3-compatible
// backend. Common to every subcommand except `init` (which doesn't need
// network access — its job is to write the config that other commands
// will then load).
func loadBackend(ctx context.Context, g *Globals) (storage.Backend, *config.Config, error) {
	cfg, err := config.Load(g.ConfigPath, g.Overrides)
	if err != nil {
		return nil, nil, err
	}
	b, err := storage.NewS3(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return b, cfg, nil
}

// loadAccountBackend uses [admin] credentials when configured, for
// account-level calls like bucket create/list/delete.
func loadAccountBackend(ctx context.Context, g *Globals) (storage.Backend, *config.Config, error) {
	cfg, err := config.Load(g.ConfigPath, g.Overrides)
	if err != nil {
		return nil, nil, err
	}
	keyID, appKey := cfg.AccountCredentials()
	adminCfg := cfg.CloneWithCredentials(keyID, appKey)
	b, err := storage.NewS3(ctx, adminCfg)
	if err != nil {
		return nil, nil, err
	}
	return b, cfg, nil
}

// HumanSize formats a byte count for `bbm ls`. Mirrors `du -h`-style
// output: 1023 → "1023", 1024 → "1.0K", 1536 → "1.5K", and so on.
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for n2 := n / unit; n2 >= unit; n2 /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTPE"[exp])
}

// confirm prompts y/N on stderr and reads stdin. Returns true only on
// an explicit "y" / "yes". `--yes` flags in subcommands skip this.
func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// promptLine reads one trimmed line from stdin. Used by init's
// interactive setup.
func promptLine(r *bufio.Reader, label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptLineDefault is promptLine but renders [default] inline and
// returns the default when the user hits enter.
func promptLineDefault(r *bufio.Reader, label, def string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	v := strings.TrimSpace(line)
	if v == "" {
		return def, nil
	}
	return v, nil
}

// ensureDir creates parent directories for a path if missing, mode 0700
// (matches what `bbm init` and friends want for ~/.config/bbm/).
func ensureDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o700)
}
