package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFullConfig(t *testing.T) {
	path := writeConfig(t, `
default_profile: personal

profiles:
  personal:
    backend: keyring
    options:
      service: tfvault-personal
  customer-a:
    backend: pass
    options:
      binary: gopass
      prefix: customers/a/terraform
      store_dir: ~/.password-store-customer-a
  ci:
    backend: env
    options:
      prefix: CI_TF_TOKEN_
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProfile != "personal" {
		t.Errorf("DefaultProfile = %q", cfg.DefaultProfile)
	}
	if len(cfg.Profiles) != 3 {
		t.Fatalf("got %d profiles", len(cfg.Profiles))
	}
	p := cfg.Profiles["customer-a"]
	if p.Backend != "pass" {
		t.Errorf("backend = %q", p.Backend)
	}
	if p.Options["binary"] != "gopass" || p.Options["prefix"] != "customers/a/terraform" {
		t.Errorf("options = %v", p.Options)
	}
	if cfg.Profiles["ci"].Backend != "env" {
		t.Errorf("ci backend = %q", cfg.Profiles["ci"].Backend)
	}
}

func TestLoadMissingFileReturnsNil(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Fatalf("cfg = %+v, want nil for missing file", cfg)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	cfg, err := Load(writeConfig(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || len(cfg.Profiles) != 0 {
		t.Fatalf("cfg = %+v, want empty config", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			"duplicate profile",
			`profiles:
  a:
    backend: keyring
  a:
    backend: keyring`,
			"already defined",
		},
		{
			"no backend",
			`profiles:
  a:
    options:
      service: x`,
			"must specify a backend",
		},
		{
			"empty profile",
			`profiles:
  a: {}`,
			"must specify a backend",
		},
		{
			"option outside options map",
			`profiles:
  a:
    backend: keyring
    service: x`,
			"not found",
		},
		{
			"unsupported top-level key",
			`something: x`,
			"not found",
		},
		{
			"default_profile without matching profile",
			`default_profile: missing
profiles:
  a:
    backend: keyring`,
			"does not match any profile",
		},
		{
			"nested value in options",
			`profiles:
  a:
    backend: keyring
    options:
      service:
        nested: x`,
			"cannot unmarshal",
		},
		{
			"non-string option",
			`profiles:
  a:
    backend: keyring
    options:
      service: []`,
			"cannot unmarshal",
		},
		{
			"invalid YAML",
			`profiles: [`,
			"parsing config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.content))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestPermissionWarning(t *testing.T) {
	path := writeConfig(t, `profiles:
  a:
    backend: keyring
`)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "chmod 0600") {
		t.Errorf("Warnings = %v", cfg.Warnings)
	}
}

func TestResolvePathPrecedence(t *testing.T) {
	t.Setenv("TFVAULT_CONFIG", "/from/env.yaml")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	if got, _ := ResolvePath("/from/flag.yaml"); got != "/from/flag.yaml" {
		t.Errorf("flag precedence: got %q", got)
	}
	if got, _ := ResolvePath(""); got != "/from/env.yaml" {
		t.Errorf("env precedence: got %q", got)
	}

	t.Setenv("TFVAULT_CONFIG", "")
	if got, _ := ResolvePath(""); got != filepath.Join("/xdg", "tfvault", "config.yaml") {
		t.Errorf("xdg: got %q", got)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	if got, _ := ResolvePath(""); got != filepath.Join(home, ".config", "tfvault", "config.yaml") {
		t.Errorf("home fallback: got %q", got)
	}
}
