package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// OpItem is a 1Password item discovered for Backblaze setup.
type OpItem struct {
	ID    string
	Title string
	Vault string
	VaultID string
}

// OpKeyMaterial is key_id + an op:// reference for the secret (never plaintext on disk).
type OpKeyMaterial struct {
	KeyID      string
	AppKeyRef  string
	ItemTitle  string
	VaultID    string
	Field      string // credential or password
}

// OpAvailable reports whether the 1Password CLI is on PATH.
func OpAvailable() bool {
	_, err := exec.LookPath("op")
	return err == nil
}

// OpListItems returns every item title in a vault (empty vault = all vaults).
func OpListItems(vault string) ([]OpItem, error) {
	args := []string{"item", "list", "--format", "json"}
	if vault != "" {
		args = append(args, "--vault", vault)
	}
	out, err := exec.Command("op", args...).Output()
	if err != nil {
		return nil, opErr("item list", err)
	}
	var raw []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Vault struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"vault"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse op item list: %w", err)
	}
	items := make([]OpItem, 0, len(raw))
	for _, r := range raw {
		items = append(items, OpItem{
			ID:      r.ID,
			Title:   r.Title,
			Vault:   r.Vault.Name,
			VaultID: r.Vault.ID,
		})
	}
	return items, nil
}

// OpFindBackblazeItems filters items whose titles look like B2 / bbm keys.
func OpFindBackblazeItems(vault string) ([]OpItem, error) {
	all, err := OpListItems(vault)
	if err != nil {
		return nil, err
	}
	var out []OpItem
	for _, it := range all {
		t := strings.ToLower(it.Title)
		if strings.Contains(t, "backblaze") || strings.Contains(t, "blackblaze") ||
			strings.Contains(t, " b2") || strings.HasPrefix(t, "b2 ") ||
			strings.Contains(t, "bbm") {
			out = append(out, it)
		}
	}
	return out, nil
}

// OpKeyFromItem reads key_id (username) and builds an op:// ref for the secret field.
func OpKeyFromItem(it OpItem) (OpKeyMaterial, error) {
	out, err := exec.Command("op", "item", "get", it.ID, "--format", "json").Output()
	if err != nil {
		return OpKeyMaterial{}, opErr("item get "+it.ID, err)
	}
	var doc struct {
		Title  string `json:"title"`
		Vault  struct {
			ID string `json:"id"`
		} `json:"vault"`
		Fields []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return OpKeyMaterial{}, fmt.Errorf("parse op item: %w", err)
	}

	vaultID := doc.Vault.ID
	if vaultID == "" {
		vaultID = it.VaultID
	}
	title := doc.Title
	if title == "" {
		title = it.Title
	}

	var keyID string
	field := ""
	for _, f := range doc.Fields {
		lbl := strings.ToLower(strings.TrimSpace(f.Label))
		if lbl == "" {
			lbl = strings.ToLower(f.ID)
		}
		switch lbl {
		case "username", "key id", "keyid", "key_id":
			keyID = cleanOpValue(f.Value)
		case "credential":
			field = "credential"
		case "password":
			if field == "" {
				field = "password"
			}
		}
	}
	if keyID == "" {
		return OpKeyMaterial{}, fmt.Errorf("item %q: no username/key_id field", title)
	}
	if field == "" {
		return OpKeyMaterial{}, fmt.Errorf("item %q: no credential or password field", title)
	}

	ref := fmt.Sprintf("op://%s/%s/%s", vaultID, title, field)
	return OpKeyMaterial{
		KeyID:     keyID,
		AppKeyRef: ref,
		ItemTitle: title,
		VaultID:   vaultID,
		Field:     field,
	}, nil
}

// OpVerifyKey resolves the secret and checks key_id length (no network).
func OpVerifyKey(mat OpKeyMaterial) error {
	secret, err := ResolveSecret(mat.AppKeyRef)
	if err != nil {
		return err
	}
	if len(secret) < 20 {
		return fmt.Errorf("resolved secret for %q looks too short (%d chars)", mat.ItemTitle, len(secret))
	}
	return nil
}

// ResolveSecret fetches a literal value or resolves an op:// reference.
// Supports credential fields (op read) and login password fields (op reveal).
func ResolveSecret(ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty secret reference")
	}
	if !strings.HasPrefix(ref, "op://") {
		return ref, nil
	}
	s, err := opRead(ref)
	if err == nil && !looksLikeOpRevealHint(s) && len(s) >= 20 {
		return s, nil
	}
	vault, item, field, perr := parseOpRef(ref)
	if perr != nil {
		if err != nil {
			return "", err
		}
		return "", perr
	}
	return opItemFieldReveal(vault, item, field)
}

func opRead(ref string) (string, error) {
	if _, err := exec.LookPath("op"); err != nil {
		return "", fmt.Errorf("1Password CLI (`op`) not found on PATH")
	}
	out, err := exec.Command("op", "read", ref).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("op read %s: %s", ref, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("op read %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func opItemFieldReveal(vault, item, field string) (string, error) {
	if _, err := exec.LookPath("op"); err != nil {
		return "", fmt.Errorf("1Password CLI (`op`) not found on PATH")
	}
	args := []string{"item", "get", item, "--reveal", "--fields", field}
	if vault != "" {
		args = append(args, "--vault", vault)
	}
	out, err := exec.Command("op", args...).Output()
	if err != nil {
		return "", opErr(fmt.Sprintf("item get %q field %q", item, field), err)
	}
	v := cleanOpValue(string(out))
	if v == "" {
		return "", fmt.Errorf("empty %q from 1Password item %q", field, item)
	}
	return v, nil
}

func parseOpRef(ref string) (vault, item, field string, err error) {
	rest := strings.TrimPrefix(ref, "op://")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid op:// reference %q (want op://VAULT/ITEM/FIELD)", ref)
	}
	return parts[0], parts[1], parts[2], nil
}

func looksLikeOpRevealHint(s string) bool {
	return strings.Contains(s, "op item get") || strings.Contains(s, "--reveal")
}

func cleanOpValue(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	return strings.TrimSpace(s)
}

func opErr(action string, err error) error {
	if exitErr, ok := err.(*exec.ExitError); ok {
		msg := strings.TrimSpace(string(exitErr.Stderr))
		return fmt.Errorf("op %s: %s", action, msg)
	}
	return fmt.Errorf("op %s: %w", action, err)
}

// OpPickItem chooses the best matching item title for a role.
func OpPickItem(items []OpItem, matchers ...string) (OpItem, bool) {
	for _, m := range matchers {
		m = strings.ToLower(m)
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Title), m) {
				return it, true
			}
		}
	}
	return OpItem{}, false
}

// OpItemTitles returns a human-readable bullet list for stderr.
func OpItemTitles(items []OpItem) string {
	var b bytes.Buffer
	for _, it := range items {
		fmt.Fprintf(&b, "  - %s (vault %s)\n", it.Title, it.Vault)
	}
	return b.String()
}
