package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigShowOutput(t *testing.T) {
	path := writeTestConfig(t, `
default_profile: work
editor: myeditor
profiles:
  work:
    backend: testbe
    options:
      service: svc-work
  personal:
    backend: testbe
`)
	var out, errOut bytes.Buffer
	code := Run([]string{"--config", path, "config", "show"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, path) {
		t.Errorf("config path not shown: %q", got)
	}
	if !strings.Contains(got, "profile: work (from config default_profile)") {
		t.Errorf("profile source missing: %q", got)
	}
	if !strings.Contains(got, "editor: myeditor (from config editor)") {
		t.Errorf("editor source missing: %q", got)
	}
	if !strings.Contains(got, "* work") || !strings.Contains(got, `service="svc-work"`) {
		t.Errorf("profiles listing missing: %q", got)
	}
}

func TestConfigShowProfileFlagOverride(t *testing.T) {
	path := writeTestConfig(t, `
default_profile: work
profiles:
  work:
    backend: testbe
  personal:
    backend: testbe
`)
	var out, errOut bytes.Buffer
	code := Run([]string{"--config", path, "--profile", "personal", "config", "show"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, "profile: personal (from --profile flag)") {
		t.Errorf("flag override not reflected: %q", got)
	}
	if !strings.Contains(got, "* personal") {
		t.Errorf("effective profile not marked: %q", got)
	}
}

func TestConfigShowEditorFromEnv(t *testing.T) {
	path := writeTestConfig(t, `
profiles:
  work:
    backend: testbe
`)
	t.Setenv("EDITOR", "enved")
	var out, errOut bytes.Buffer
	code := Run([]string{"--config", path, "config", "show"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "editor: enved (from $EDITOR)") {
		t.Errorf("output = %q", out.String())
	}
}

func TestConfigShowZeroConfig(t *testing.T) {
	t.Setenv("TFVAULT_CONFIG", "/nonexistent/tfvault/config.yaml")
	var out, errOut bytes.Buffer
	code := Run([]string{"config", "show"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, "zero-config") || !strings.Contains(got, "keyring") {
		t.Errorf("output = %q", got)
	}
}

func TestConfigEditCreatesFileAndRunsEditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	t.Setenv("TFVAULT_CONFIG", path)
	t.Setenv("EDITOR", `echo 'default_profile: ""' >`)

	var out, errOut bytes.Buffer
	code := Run([]string{"config", "edit"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "default_profile") {
		t.Errorf("editor did not receive the config path, content = %q", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("created config mode = %04o, want 0600", perm)
	}
}

func TestConfigEditConfigEditorWinsOverEnv(t *testing.T) {
	path := writeTestConfig(t, `
editor: 'echo "color: false" >'
profiles:
  work:
    backend: testbe
`)
	// If $EDITOR were used it would fail the command.
	t.Setenv("EDITOR", "false")

	var out, errOut bytes.Buffer
	code := Run([]string{"--config", path, "config", "edit"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "color: false") {
		t.Errorf("config editor not used, content = %q", content)
	}
}

func TestConfigEditWarnsOnInvalidResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("TFVAULT_CONFIG", path)
	t.Setenv("EDITOR", `echo 'not_a_key: 1' >`)

	var out, errOut bytes.Buffer
	code := Run([]string{"config", "edit"}, strings.NewReader(""), &out, &errOut)
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero for invalid saved config")
	}
	if !strings.Contains(errOut.String(), "invalid") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestConfigUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"config", "frobnicate"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	if code := Run([]string{"config"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Fatal("exit code = 0, want nonzero for missing subcommand")
	}
}
