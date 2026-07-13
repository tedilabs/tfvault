package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tedilabs/tfvault/internal/backend"
)

// testBackend records the options it was built with.
type testBackend struct {
	opts map[string]string
}

func (b *testBackend) Name() string                     { return "testbe" }
func (b *testBackend) Get(string) (string, bool, error) { return "", false, nil }
func (b *testBackend) Store(string, string) error       { return nil }
func (b *testBackend) Forget(string) error              { return nil }

func init() {
	backend.Register("testbe", func(opts map[string]string) (backend.Backend, error) {
		return &testBackend{opts: opts}, nil
	})
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveNamedProfile(t *testing.T) {
	path := writeTestConfig(t, `
profile "customer-a" {
  testbe {
    service = "svc-a"
  }
}
`)
	var stderr bytes.Buffer
	b, err := defaultResolveBackend(path, "customer-a", &stderr)
	if err != nil {
		t.Fatal(err)
	}
	tb := b.(*testBackend)
	if tb.opts["service"] != "svc-a" {
		t.Errorf("options = %v", tb.opts)
	}
}

func TestResolveDefaultProfileFromConfig(t *testing.T) {
	path := writeTestConfig(t, `
default_profile = "personal"
profile "personal" {
  testbe {}
}
profile "work" {
  testbe {}
}
`)
	var stderr bytes.Buffer
	if _, err := defaultResolveBackend(path, "", &stderr); err != nil {
		t.Fatal(err)
	}
}

func TestResolveProfileNotFound(t *testing.T) {
	path := writeTestConfig(t, `profile "personal" {
  testbe {}
}`)
	var stderr bytes.Buffer
	_, err := defaultResolveBackend(path, "missing", &stderr)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "personal") {
		t.Errorf("error should list available profiles: %v", err)
	}
}

func TestResolveNamedProfileWithoutConfigFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.hcl")
	var stderr bytes.Buffer
	_, err := defaultResolveBackend(missing, "customer-a", &stderr)
	if err == nil {
		t.Fatal("named profile without config must fail, not fall back to a shared default")
	}
	if !strings.Contains(err.Error(), "customer-a") {
		t.Errorf("error = %v", err)
	}
}

func TestResolveUnknownBackend(t *testing.T) {
	path := writeTestConfig(t, `profile "a" {
  doesnotexist {}
}`)
	var stderr bytes.Buffer
	_, err := defaultResolveBackend(path, "a", &stderr)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "unknown backend") || !strings.Contains(err.Error(), "testbe") {
		t.Errorf("error should name available backends: %v", err)
	}
}

func TestResolvePrintsWarnings(t *testing.T) {
	path := writeTestConfig(t, `profile "a" {
  testbe {}
}`)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if _, err := defaultResolveBackend(path, "a", &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "warning") {
		t.Errorf("stderr = %q, want permission warning", stderr.String())
	}
}
