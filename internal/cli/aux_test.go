package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tedilabs/tfvault/internal/backend"
	"github.com/tedilabs/tfvault/internal/backend/fake"
)

// listingBackend wraps the fake backend with a Lister implementation.
type listingBackend struct {
	*fake.Backend
	hosts []string
}

func (b *listingBackend) List() ([]string, error) { return b.hosts, nil }

func TestProfilesOutput(t *testing.T) {
	path := writeTestConfig(t, `
default_profile: work
profiles:
  work:
    backend: testbe
    options:
      service: svc-work
  personal:
    backend: testbe
`)
	var out, errOut bytes.Buffer
	code := Run([]string{"--config", path, "profiles"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, "* work") {
		t.Errorf("default profile not marked: %q", got)
	}
	if !strings.Contains(got, "personal") || !strings.Contains(got, `service="svc-work"`) {
		t.Errorf("output = %q", got)
	}
}

func TestProfilesZeroConfig(t *testing.T) {
	t.Setenv("TFVAULT_CONFIG", "/nonexistent/tfvault/config.yaml")
	var out, errOut bytes.Buffer
	code := Run([]string{"profiles"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "keyring") {
		t.Errorf("output = %q", out.String())
	}
}

func TestListWithListerBackend(t *testing.T) {
	b := &listingBackend{Backend: fake.New(), hosts: []string{"app.terraform.io", "spacelift.io"}}
	orig := resolveBackend
	resolveBackend = func(string, string, io.Writer) (backend.Backend, error) { return b, nil }
	t.Cleanup(func() { resolveBackend = orig })

	var out, errOut bytes.Buffer
	code := Run([]string{"list"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if out.String() != "app.terraform.io\nspacelift.io\n" {
		t.Errorf("output = %q", out.String())
	}
}

func TestListUnsupportedBackend(t *testing.T) {
	orig := resolveBackend
	resolveBackend = func(string, string, io.Writer) (backend.Backend, error) { return fake.New(), nil }
	t.Cleanup(func() { resolveBackend = orig })

	var out, errOut bytes.Buffer
	code := Run([]string{"list"}, strings.NewReader(""), &out, &errOut)
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	if !strings.Contains(errOut.String(), "does not support listing") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestListResolutionError(t *testing.T) {
	orig := resolveBackend
	resolveBackend = func(string, string, io.Writer) (backend.Backend, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { resolveBackend = orig })

	var out, errOut bytes.Buffer
	if code := Run([]string{"list"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
}

func TestHelpFlagExitsZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var out, errOut bytes.Buffer
		code := Run([]string{flag}, strings.NewReader(""), &out, &errOut)
		if code != 0 {
			t.Errorf("%s: exit code = %d, want 0", flag, code)
		}
		if !strings.Contains(out.String(), "Usage: tfvault") {
			t.Errorf("%s: stdout = %q, want usage", flag, out.String())
		}
		if errOut.Len() != 0 {
			t.Errorf("%s: stderr = %q, want empty", flag, errOut.String())
		}
	}
}

func TestVersionFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--version"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "tfvault") {
		t.Errorf("output = %q", out.String())
	}
}

func TestHelpVerb(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"help"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Errorf("help: exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Usage: tfvault") {
		t.Errorf("help: stdout = %q", out.String())
	}

	out.Reset()
	if code := Run([]string{"help", "config"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Errorf("help config: exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "config <subcommand>") {
		t.Errorf("help config: stdout = %q", out.String())
	}

	if code := Run([]string{"help", "bogus"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Error("help bogus: exit code = 0, want nonzero")
	}
}

func TestUnknownFlagPrintsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"--bogus"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	if !strings.Contains(errOut.String(), "Usage: tfvault") {
		t.Errorf("stderr = %q, want usage after flag error", errOut.String())
	}
}

func TestVersionOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"version"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "tfvault") {
		t.Errorf("output = %q", out.String())
	}
}
