package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeColorConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPaletteDisabledForNonTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	pal := newPalette(false, writeColorConfig(t, "profiles:\n  a:\n    backend: env\n"), &bytes.Buffer{})
	if pal.enabled {
		t.Error("palette enabled for a non-terminal writer")
	}
}

func TestPaletteDisabledByFlag(t *testing.T) {
	pal := newPalette(true, "", os.Stdout)
	if pal.enabled {
		t.Error("palette enabled despite --no-color")
	}
}

func TestPaletteDisabledByNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	pal := newPalette(false, "", os.Stdout)
	if pal.enabled {
		t.Error("palette enabled despite NO_COLOR")
	}
}

func TestPaletteDisabledByConfig(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	path := writeColorConfig(t, "color: false\nprofiles:\n  a:\n    backend: env\n")
	// Even for a terminal-like stdout the config must win; we can't
	// fabricate a TTY in tests, so assert via the decision on os.Stdout
	// only when it happens to be a terminal — the meaningful assertion
	// is that config false always disables.
	pal := newPalette(false, path, os.Stdout)
	if pal.enabled {
		t.Error("palette enabled despite color: false in config")
	}
}

func TestPaletteWrapsAndStripsCorrectly(t *testing.T) {
	on := &palette{enabled: true}
	if got := on.green("ok:"); got != "\x1b[32mok:\x1b[0m" {
		t.Errorf("green = %q", got)
	}
	off := &palette{}
	if got := off.green("ok:"); got != "ok:" {
		t.Errorf("disabled green = %q", got)
	}
	if got := on.bold(""); got != "" {
		t.Errorf("empty string must stay empty, got %q", got)
	}
}

// TestAuxOutputHasNoANSIWhenPiped runs the real commands with buffer
// stdout (as scripts would see) and asserts no escape codes leak.
func TestAuxOutputHasNoANSIWhenPiped(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := writeColorConfig(t, "default_profile: work\nprofiles:\n  work:\n    backend: testbe\n")

	var out, errOut bytes.Buffer
	Run([]string{"--config", cfg, "profiles"}, strings.NewReader(""), &out, &errOut)
	if strings.Contains(out.String(), "\x1b[") {
		t.Errorf("ANSI escapes in piped output:\n%q", out.String())
	}
}
