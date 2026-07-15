package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wireHealthyStatus prepares a fake HOME with a config for the given
// backend, a terraformrc registering the helper, and the plugin link,
// so status tests start from an otherwise healthy setup.
func wireHealthyStatus(t *testing.T, backendYAML string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("profiles:\n  work:\n"+backendYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TFVAULT_CONFIG", cfgPath)

	rcPath := filepath.Join(home, "terraformrc")
	if err := os.WriteFile(rcPath, []byte(`
credentials_helper "tfvault" {
  args = ["--profile", "work"]
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", rcPath)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installLink(exe, filepath.Join(home, ".terraform.d", "plugins"), false); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestStatusShadowingSources(t *testing.T) {
	home := wireHealthyStatus(t, "    backend: testbe\n")
	t.Setenv("TF_TOKEN_spacelift_io", "tok")
	credPath := filepath.Join(home, ".terraform.d", "credentials.tfrc.json")
	if err := os.WriteFile(credPath, []byte(`{"credentials":{"app.terraform.io":{"token":"x"},"empty.example.com":{"token":""}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"status"}, strings.NewReader(""), &out, &errOut)
	got := out.String()
	// Notes must not fail the health check.
	if code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, got)
	}
	for _, want := range []string{
		"TF_TOKEN_spacelift_io is set",
		"plaintext tokens in " + credPath + " for: app.terraform.io",
		`"terraform logout <hostname>"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "empty.example.com") {
		t.Errorf("tokenless host reported as plaintext:\n%s", got)
	}
}

func TestStatusNoShadowingSources(t *testing.T) {
	wireHealthyStatus(t, "    backend: testbe\n")

	var out, errOut bytes.Buffer
	code := Run([]string{"status"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Shadowing token sources:") {
		t.Errorf("section missing:\n%s", out.String())
	}
}

func TestStatusBackendCheckFails(t *testing.T) {
	wireHealthyStatus(t, `    backend: pass
    options:
      binary: tfvault-test-definitely-missing
`)

	var out, errOut bytes.Buffer
	code := Run([]string{"status"}, strings.NewReader(""), &out, &errOut)
	got := out.String()
	if code == 0 {
		t.Errorf("exit code = 0, want nonzero when backend check fails\n%s", got)
	}
	if !strings.Contains(got, "check:") || !strings.Contains(got, "executable file not found") {
		t.Errorf("output missing check failure:\n%s", got)
	}
}
