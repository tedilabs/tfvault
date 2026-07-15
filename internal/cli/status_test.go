package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tedilabs/tfvault/internal/backend"
)

func TestParseTerraformRC(t *testing.T) {
	src := `
plugin_cache_dir = "$HOME/.terraform.d/plugin-cache"

credentials_helper "tfvault" {
  args = ["--profile", "customer-a"]
}

credentials "app.terraform.io" {
  token = "REDACTED"
}

provider_installation {
  direct {}
}
`
	rc, err := parseTerraformRC([]byte(src), "test.tfrc")
	if err != nil {
		t.Fatal(err)
	}
	if rc.HelperName != "tfvault" {
		t.Errorf("HelperName = %q", rc.HelperName)
	}
	if !reflect.DeepEqual(rc.HelperArgs, []string{"--profile", "customer-a"}) {
		t.Errorf("HelperArgs = %v", rc.HelperArgs)
	}
	if !reflect.DeepEqual(rc.CredHosts, []string{"app.terraform.io"}) {
		t.Errorf("CredHosts = %v", rc.CredHosts)
	}
}

func TestParseTerraformRCNoHelper(t *testing.T) {
	rc, err := parseTerraformRC([]byte(`plugin_cache_dir = "/tmp"`), "test.tfrc")
	if err != nil {
		t.Fatal(err)
	}
	if rc.HelperName != "" {
		t.Errorf("HelperName = %q, want empty", rc.HelperName)
	}
}

func TestFlagValue(t *testing.T) {
	tests := []struct {
		args []string
		name string
		want string
	}{
		{[]string{"--profile", "a"}, "profile", "a"},
		{[]string{"-profile", "a"}, "profile", "a"},
		{[]string{"--profile=a"}, "profile", "a"},
		{[]string{"--config", "/x", "--profile", "a"}, "config", "/x"},
		{[]string{"profile", "a"}, "profile", ""},
		{[]string{"--profile"}, "profile", ""},
		{nil, "profile", ""},
	}
	for _, tt := range tests {
		if got := flagValue(tt.args, tt.name); got != tt.want {
			t.Errorf("flagValue(%v, %q) = %q, want %q", tt.args, tt.name, got, tt.want)
		}
	}
}

// TestStatusEndToEnd wires a fake HOME, a terraformrc selecting a
// profile via helper args, and a tfvault config, then checks the full
// report.
func TestStatusEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`profiles:
  customer-a:
    backend: testbe
    options:
      service: svc-a
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TFVAULT_CONFIG", cfgPath)

	rcPath := filepath.Join(home, "terraformrc")
	if err := os.WriteFile(rcPath, []byte(`
credentials_helper "tfvault" {
  args = ["--profile", "customer-a"]
}
credentials "bypass.example.com" {
  token = "x"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", rcPath)

	// Link missing: unhealthy exit, but the report must still resolve
	// the profile from the terraformrc helper args.
	var out, errOut bytes.Buffer
	code := Run([]string{"status"}, strings.NewReader(""), &out, &errOut)
	if code == 0 {
		t.Errorf("exit code = 0, want nonzero while link is missing")
	}
	got := out.String()
	for _, want := range []string{
		`run "tfvault install"`,
		`credentials_helper "tfvault"`,
		"bypass.example.com",
		"customer-a (from terraformrc helper args",
		`backend: testbe (service="svc-a")`,
		"cannot enumerate",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}

	// After install the report is healthy.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installLink(exe, filepath.Join(home, ".terraform.d", "plugins"), false); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := Run([]string{"status"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "ok: ") {
		t.Errorf("output = %s", out.String())
	}
}

func TestStatusProfileFlagOverridesHelperArgs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`profiles:
  from-args:
    backend: testbe
  from-flag:
    backend: testbe
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TFVAULT_CONFIG", cfgPath)

	rcPath := filepath.Join(home, "terraformrc")
	if err := os.WriteFile(rcPath, []byte(`
credentials_helper "tfvault" {
  args = ["--profile", "from-args"]
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", rcPath)

	var out, errOut bytes.Buffer
	Run([]string{"--profile", "from-flag", "status"}, strings.NewReader(""), &out, &errOut)
	if !strings.Contains(out.String(), "from-flag (from --profile flag") {
		t.Errorf("output = %s", out.String())
	}
}

// errListBackend fails enumeration, exercising the status List error path.
type errListBackend struct{ testBackend }

func (errListBackend) List() ([]string, error) { return nil, errors.New("vault locked") }

func init() {
	backend.Register("errlistbe", func(map[string]string) (backend.Backend, error) {
		return &errListBackend{}, nil
	})
}

// TestStatusListErrorUnhealthy verifies that a backend which cannot
// enumerate its entries makes status exit nonzero even when the link
// and terraformrc are fully wired up.
func TestStatusListErrorUnhealthy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`profiles:
  broken:
    backend: errlistbe
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TFVAULT_CONFIG", cfgPath)

	rcPath := filepath.Join(home, "terraformrc")
	if err := os.WriteFile(rcPath, []byte(`
credentials_helper "tfvault" {
  args = ["--profile", "broken"]
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

	var out, errOut bytes.Buffer
	code := Run([]string{"status"}, strings.NewReader(""), &out, &errOut)
	if code == 0 {
		t.Errorf("exit code = 0, want nonzero when List fails\n%s", out.String())
	}
	if !strings.Contains(out.String(), "vault locked") {
		t.Errorf("output missing List error:\n%s", out.String())
	}
}

func TestStatusZeroConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TFVAULT_CONFIG", filepath.Join(home, "nope.yaml"))
	t.Setenv("TF_CLI_CONFIG_FILE", filepath.Join(home, "no-terraformrc"))

	var out, errOut bytes.Buffer
	code := Run([]string{"status"}, strings.NewReader(""), &out, &errOut)
	if code == 0 {
		t.Errorf("exit code = 0, want nonzero (no link, no terraformrc)")
	}
	got := out.String()
	for _, want := range []string{"missing", "zero-config", "backend: keyring"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}
