// Package config loads bbm credentials from a TOML file, the process
// environment, or both, with CLI flags layered on top by main.go.
//
// The lookup order is intentionally identical in spirit to rui's:
//
//	1. CLI flags                       (highest, applied by main.go)
//	2. Process env vars                (B2_KEY_ID, B2_APP_KEY, B2_BUCKET, ...)
//	3. $XDG_CONFIG_HOME/bbm/config.toml or ~/.config/bbm/config.toml
//	4. .config-next-to-binary fallback (source-tree / portable installs)
//
// Per-field precedence: CLI flag overrides env, env overrides file. A
// value of "" is treated as "not set" so leaving a field blank in the
// TOML lets the env override fill it in.
//
// op:// references in app_key are resolved at runtime by shelling out to
// the 1Password CLI. The resolved secret never lands on disk; it lives
// in process memory only for the duration of the bbm invocation.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the resolved (post-cascade) credential + endpoint bundle
// the storage layer needs to talk to a bucket.
type Config struct {
	Provider string `toml:"provider"`
	Endpoint string `toml:"endpoint"`
	Region   string `toml:"region"`
	Bucket   string `toml:"bucket"`
	KeyID    string `toml:"key_id"`
	AppKey   string `toml:"app_key"`
	// Admin holds optional credentials with writeBuckets (bucket create/delete).
	// When set, `bbm bucket` uses these instead of the main key.
	Admin *AdminConfig `toml:"admin"`
}

// AdminConfig is an optional second B2 application key (account-level).
type AdminConfig struct {
	KeyID  string `toml:"key_id"`
	AppKey string `toml:"app_key"`
}

// ErrNoConfig is returned by Load when no config.toml exists in any of
// the search locations AND no B2_* env vars are exported. main() catches
// this and prints a friendly `bbm init` walkthrough.
var ErrNoConfig = errors.New("no config.toml and no B2_* env vars in environment")

// Overrides are CLI-flag values main.go threads in. An empty string is
// "not set" — it does NOT shadow a value set further down the cascade.
type Overrides struct {
	Endpoint string
	Region   string
	Bucket   string
	KeyID    string
	AppKey   string
}

// Load resolves credentials from the cascade. configPath, when non-empty,
// short-circuits the file-search portion (similar to rui's -env <path>).
//
// app_key fields starting with "op://" are resolved on the fly by
// invoking `op read <ref>`. If `op` is missing or the lookup fails, the
// caller gets the underlying error — there's deliberately no silent
// fallback to plaintext.
func Load(configPath string, ov Overrides) (*Config, error) {
	cands := configCandidates(configPath)
	cfg := &Config{}
	loaded := ""
	for _, p := range cands {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if _, err := toml.DecodeFile(p, cfg); err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		loaded = p
		break
	}

	envSet := overlayEnv(cfg)
	overlayOverrides(cfg, ov)
	applyProviderDefaults(cfg)

	// Both empty AND no file loaded AND no env set → fresh install.
	if loaded == "" && !envSet && cfg.KeyID == "" && cfg.AppKey == "" && cfg.Bucket == "" {
		return nil, ErrNoConfig
	}

	source := loaded
	if source == "" {
		source = "process environment"
	}

	if cfg.Bucket == "" {
		return nil, fmt.Errorf("missing 'bucket' in %s (searched: %v)", source, cands)
	}
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("missing 'key_id' in %s (searched: %v)", source, cands)
	}
	if cfg.AppKey == "" {
		return nil, fmt.Errorf("missing 'app_key' in %s (searched: %v)", source, cands)
	}
	if cfg.Endpoint == "" && cfg.Provider != "s3" {
		return nil, fmt.Errorf("missing 'endpoint' in %s — required for provider=%q", source, cfg.Provider)
	}

	if err := resolveConfigSecrets(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func resolveConfigSecrets(cfg *Config) error {
	if strings.HasPrefix(cfg.AppKey, "op://") {
		resolved, err := ResolveSecret(cfg.AppKey)
		if err != nil {
			return fmt.Errorf("resolve %q: %w", cfg.AppKey, err)
		}
		cfg.AppKey = resolved
	}
	if cfg.Admin != nil && strings.HasPrefix(cfg.Admin.AppKey, "op://") {
		resolved, err := ResolveSecret(cfg.Admin.AppKey)
		if err != nil {
			return fmt.Errorf("resolve admin %q: %w", cfg.Admin.AppKey, err)
		}
		cfg.Admin.AppKey = resolved
	}
	return nil
}

// AccountCredentials returns key material for account-level bucket admin.
// Falls back to the main key when no [admin] block is configured.
func (c *Config) AccountCredentials() (keyID, appKey string) {
	if c.Admin != nil && c.Admin.KeyID != "" && c.Admin.AppKey != "" {
		return c.Admin.KeyID, c.Admin.AppKey
	}
	return c.KeyID, c.AppKey
}

// CloneWithCredentials returns a copy using alternate key material (same endpoint/region/bucket).
func (c *Config) CloneWithCredentials(keyID, appKey string) *Config {
	out := *c
	out.KeyID = keyID
	out.AppKey = appKey
	out.Admin = nil
	return &out
}

// Candidates returns the list of locations Load will search, for use in
// user-facing help text from main.go.
func Candidates(configPath string) []string {
	return configCandidates(configPath)
}

// SuggestedPath is the XDG-friendly per-user path that `bbm init`
// writes to and that the no-config help recommends.
func SuggestedPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "bbm", "config.toml")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "bbm", "config.toml")
	}
	return "~/.config/bbm/config.toml"
}

func configCandidates(configPath string) []string {
	if configPath != "" {
		return []string{configPath}
	}
	var out []string
	out = append(out, SuggestedPath())
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		out = append(out, filepath.Join(filepath.Dir(exe), "config.toml"))
	}
	return out
}

// overlayEnv applies B2_* environment variables on top of the loaded
// file. Returns true if at least one variable was actually set in the
// environment, which is the signal Load uses to decide between
// "fresh install, friendly help" and "missing field, terse error".
func overlayEnv(cfg *Config) bool {
	any := false
	if v := os.Getenv("B2_KEY_ID"); v != "" {
		cfg.KeyID = v
		any = true
	}
	if v := os.Getenv("B2_APP_KEY"); v != "" {
		cfg.AppKey = v
		any = true
	}
	if v := os.Getenv("B2_BUCKET"); v != "" {
		cfg.Bucket = v
		any = true
	}
	if v := os.Getenv("B2_ENDPOINT"); v != "" {
		cfg.Endpoint = v
		any = true
	}
	if v := os.Getenv("B2_REGION"); v != "" {
		cfg.Region = v
		any = true
	}
	return any
}

func overlayOverrides(cfg *Config, ov Overrides) {
	if ov.Endpoint != "" {
		cfg.Endpoint = ov.Endpoint
	}
	if ov.Region != "" {
		cfg.Region = ov.Region
	}
	if ov.Bucket != "" {
		cfg.Bucket = ov.Bucket
	}
	if ov.KeyID != "" {
		cfg.KeyID = ov.KeyID
	}
	if ov.AppKey != "" {
		cfg.AppKey = ov.AppKey
	}
}

// applyProviderDefaults fills in known endpoint/region defaults when the
// user only specified `provider = "..."`. Keeps `bbm init` output short
// for the common case while still letting people override per-field.
func applyProviderDefaults(cfg *Config) {
	if cfg.Provider == "" {
		cfg.Provider = "b2"
	}
	switch strings.ToLower(cfg.Provider) {
	case "b2":
		// Region defaults to us-west-002 (Backblaze's oldest US-West
		// region; the exact one matters because the endpoint hostname
		// embeds it). If you're on us-east-005 or eu-central-003 just
		// override `endpoint` + `region` in config.toml.
		if cfg.Region == "" {
			cfg.Region = "us-west-002"
		}
		if cfg.Endpoint == "" {
			cfg.Endpoint = "https://s3." + cfg.Region + ".backblazeb2.com"
		}
	case "wasabi":
		if cfg.Region == "" {
			cfg.Region = "us-east-1"
		}
		if cfg.Endpoint == "" {
			cfg.Endpoint = "https://s3." + cfg.Region + ".wasabisys.com"
		}
	case "r2":
		if cfg.Region == "" {
			cfg.Region = "auto"
		}
		// R2 endpoint is per-account: https://<acct>.r2.cloudflarestorage.com
		// — no sane default; user must set it explicitly.
	case "s3":
		if cfg.Region == "" {
			cfg.Region = "us-east-1"
		}
		// AWS S3 proper: empty endpoint = SDK default URL resolution.
	}
}

