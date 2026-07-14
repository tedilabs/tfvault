package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallLinkCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugins")
	exe := "/usr/local/bin/tfvault"

	msg, err := installLink(exe, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Created") {
		t.Errorf("msg = %q", msg)
	}
	target, err := os.Readlink(filepath.Join(dir, pluginBinary))
	if err != nil {
		t.Fatal(err)
	}
	if target != exe {
		t.Errorf("target = %q, want %q", target, exe)
	}
}

func TestInstallLinkResolvesRelativeExecutablePath(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)

	dir := t.TempDir()
	if _, err := installLink("./tfvault", dir, false); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(dir, pluginBinary))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(work, "tfvault"); target != want {
		t.Errorf("target = %q, want %q", target, want)
	}
}

func TestInstallLinkIdempotent(t *testing.T) {
	dir := t.TempDir()
	exe := "/usr/local/bin/tfvault"

	if _, err := installLink(exe, dir, false); err != nil {
		t.Fatal(err)
	}
	msg, err := installLink(exe, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Already installed") {
		t.Errorf("msg = %q", msg)
	}
}

func TestInstallLinkUpdatesStaleSymlink(t *testing.T) {
	dir := t.TempDir()
	if _, err := installLink("/old/tfvault", dir, false); err != nil {
		t.Fatal(err)
	}

	msg, err := installLink("/new/tfvault", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Updated") {
		t.Errorf("msg = %q", msg)
	}
	target, _ := os.Readlink(filepath.Join(dir, pluginBinary))
	if target != "/new/tfvault" {
		t.Errorf("target = %q", target)
	}
}

func TestInstallLinkRefusesRegularFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, pluginBinary), []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := installLink("/usr/local/bin/tfvault", dir, false)
	if err == nil {
		t.Fatal("want error for existing regular file")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should point at --force: %v", err)
	}
	// The old file must be untouched.
	data, _ := os.ReadFile(filepath.Join(dir, pluginBinary))
	if string(data) != "old binary" {
		t.Errorf("existing file was modified: %q", data)
	}
}

func TestInstallLinkForceReplacesRegularFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, pluginBinary), []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	msg, err := installLink("/usr/local/bin/tfvault", dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Replaced") {
		t.Errorf("msg = %q", msg)
	}
	target, err := os.Readlink(filepath.Join(dir, pluginBinary))
	if err != nil {
		t.Fatal(err)
	}
	if target != "/usr/local/bin/tfvault" {
		t.Errorf("target = %q", target)
	}
}

// TestInstallForceFlagParsing drives the full command: -f and --force
// are accepted, unknown flags and stray arguments fail.
func TestInstallForceFlagParsing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	link := filepath.Join(home, ".terraform.d", "plugins", pluginBinary)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, flag := range []string{"-f", "--force"} {
		if err := os.WriteFile(link, []byte("old binary"), 0o755); err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		if code := Run([]string{"install", flag}, strings.NewReader(""), &out, &errOut); code != 0 {
			t.Fatalf("%s: exit code = %d, stderr = %q", flag, code, errOut.String())
		}
		if _, err := os.Readlink(link); err != nil {
			t.Errorf("%s: link not replaced: %v", flag, err)
		}
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"install", "--bogus"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Error("unknown flag: exit code = 0, want nonzero")
	}
	if code := Run([]string{"install", "extra"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Error("stray argument: exit code = 0, want nonzero")
	}
}
