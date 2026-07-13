package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallLinkCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugins")
	exe := "/usr/local/bin/tfvault"

	msg, err := installLink(exe, dir)
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
	if _, err := installLink("./tfvault", dir); err != nil {
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

	if _, err := installLink(exe, dir); err != nil {
		t.Fatal(err)
	}
	msg, err := installLink(exe, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Already installed") {
		t.Errorf("msg = %q", msg)
	}
}

func TestInstallLinkUpdatesStaleSymlink(t *testing.T) {
	dir := t.TempDir()
	if _, err := installLink("/old/tfvault", dir); err != nil {
		t.Fatal(err)
	}

	msg, err := installLink("/new/tfvault", dir)
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

	_, err := installLink("/usr/local/bin/tfvault", dir)
	if err == nil {
		t.Fatal("want error for existing regular file")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Errorf("error = %v", err)
	}
	// The old file must be untouched.
	data, _ := os.ReadFile(filepath.Join(dir, pluginBinary))
	if string(data) != "old binary" {
		t.Errorf("existing file was modified: %q", data)
	}
}
