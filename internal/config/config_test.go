package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFullConfig(t *testing.T) {
	path := writeConfig(t, `
default_profile = "personal"

profile "personal" {
  keyring {
    service = "tfvault-personal"
  }
}

profile "customer-a" {
  pass {
    binary    = "gopass"
    prefix    = "customers/a/terraform"
    store_dir = "~/.password-store-customer-a"
  }
}

profile "ci" {
  env {
    prefix = "CI_TF_TOKEN_"
  }
}
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
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Fatalf("cfg = %+v, want nil for missing file", cfg)
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
			`profile "a" {
  keyring {}
}
profile "a" {
  keyring {}
}`,
			"duplicate profile",
		},
		{
			"no backend block",
			`profile "a" {}`,
			"exactly one backend block",
		},
		{
			"two backend blocks",
			`profile "a" {
  keyring {}
  env {}
}`,
			"exactly one backend block",
		},
		{
			"attribute in profile",
			`profile "a" { service = "x" }`,
			"backend options belong inside a backend block",
		},
		{
			"unsupported top-level attribute",
			`something = "x"`,
			"unsupported attribute",
		},
		{
			"unsupported top-level block",
			`credentials "x" {}`,
			"unsupported block type",
		},
		{
			"default_profile without matching profile",
			`default_profile = "missing"
profile "a" {
  keyring {}
}`,
			"does not match any profile",
		},
		{
			"nested block in backend",
			`profile "a" {
  keyring {
    nested {}
  }
}`,
			"must not contain nested blocks",
		},
		{
			"non-string option",
			`profile "a" {
  keyring {
    service = []
  }
}`,
			"must be a string",
		},
		{
			"invalid HCL",
			`profile "a" {`,
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
	path := writeConfig(t, `profile "a" {
  keyring {}
}`)
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
	t.Setenv("TFVAULT_CONFIG", "/from/env.hcl")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	if got, _ := ResolvePath("/from/flag.hcl"); got != "/from/flag.hcl" {
		t.Errorf("flag precedence: got %q", got)
	}
	if got, _ := ResolvePath(""); got != "/from/env.hcl" {
		t.Errorf("env precedence: got %q", got)
	}

	t.Setenv("TFVAULT_CONFIG", "")
	if got, _ := ResolvePath(""); got != filepath.Join("/xdg", "tfvault", "config.hcl") {
		t.Errorf("xdg: got %q", got)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	if got, _ := ResolvePath(""); got != filepath.Join(home, ".config", "tfvault", "config.hcl") {
		t.Errorf("home fallback: got %q", got)
	}
}
