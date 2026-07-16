package cli

import (
	"bytes"
	"os"
	"os/exec"
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

func TestInstallLinkForceSurvivesStaleTmp(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, pluginBinary)
	if err := os.WriteFile(link, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A leftover .tmp from a crashed earlier run must not break the
	// atomic replacement.
	if err := os.WriteFile(link+".tmp", []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installLink("/usr/local/bin/tfvault", dir, true); err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(link); target != "/usr/local/bin/tfvault" {
		t.Errorf("target = %q", target)
	}
	if _, err := os.Lstat(link + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp left behind: %v", err)
	}
}

// fakeMiseExe lays out a minimal mise data dir (installs/ + shims/) in
// a temp dir and returns the versioned executable path and shim path.
func fakeMiseExe(t *testing.T) (exe, shim string) {
	t.Helper()
	data := t.TempDir()
	exe = filepath.Join(data, "mise", "installs", "github-tedilabs-tfvault", "1.0.0", "tfvault")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("fake binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	shim = filepath.Join(data, "mise", "shims", "tfvault")
	if err := os.MkdirAll(filepath.Dir(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shim, []byte("fake shim"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe, shim
}

func TestInstallMiseWritesWrapper(t *testing.T) {
	dir := t.TempDir()
	exe, shim := fakeMiseExe(t)

	msg, err := installLink(exe, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Created") || !strings.Contains(msg, "wraps mise shim") {
		t.Errorf("msg = %q", msg)
	}
	link := filepath.Join(dir, pluginBinary)
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("wrapper is a symlink, want regular file")
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Errorf("wrapper mode = %04o, want 0755", perm)
	}
	if got := readWrapperShim(link); got != shim {
		t.Errorf("wrapper shim = %q, want %q", got, shim)
	}

	// Second run is idempotent.
	msg, err = installLink(exe, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Already installed") {
		t.Errorf("msg = %q", msg)
	}
}

func TestInstallMiseReplacesSymlink(t *testing.T) {
	dir := t.TempDir()
	if _, err := installLink("/old/tfvault", dir, false); err != nil {
		t.Fatal(err)
	}

	exe, _ := fakeMiseExe(t)
	msg, err := installLink(exe, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Replaced") || !strings.Contains(msg, "was symlink to /old/tfvault") {
		t.Errorf("msg = %q", msg)
	}
	if readWrapperShim(filepath.Join(dir, pluginBinary)) == "" {
		t.Error("symlink was not replaced by a wrapper")
	}
}

func TestInstallSymlinkReplacesWrapperWithoutForce(t *testing.T) {
	dir := t.TempDir()
	exe, _ := fakeMiseExe(t)
	if _, err := installLink(exe, dir, false); err != nil {
		t.Fatal(err)
	}

	// Moving from a mise install to a plain binary must not demand
	// --force: the wrapper is ours.
	msg, err := installLink("/usr/local/bin/tfvault", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Replaced") || !strings.Contains(msg, "was a wrapper for mise shim") {
		t.Errorf("msg = %q", msg)
	}
	if target, err := os.Readlink(filepath.Join(dir, pluginBinary)); err != nil || target != "/usr/local/bin/tfvault" {
		t.Errorf("target = %q, err = %v", target, err)
	}
}

func TestInstallMiseWithoutShimFallsBackToSymlink(t *testing.T) {
	dir := t.TempDir()
	exe, shim := fakeMiseExe(t)
	if err := os.Remove(shim); err != nil {
		t.Fatal(err)
	}

	if _, err := installLink(exe, dir, false); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(dir, pluginBinary)); err != nil || target != exe {
		t.Errorf("target = %q, err = %v (want plain symlink fallback)", target, err)
	}
}

func TestInstallMiseWrapperRefusesForeignFile(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, pluginBinary)
	if err := os.WriteFile(link, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	exe, _ := fakeMiseExe(t)
	if _, err := installLink(exe, dir, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("want --force error, got %v", err)
	}
	msg, err := installLink(exe, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "Updated") {
		t.Errorf("msg = %q", msg)
	}
}

// TestInstallMiseNewlinePathFallsBackToSymlink: a newline in the shim
// path cannot be represented in the wrapper's "# shim:" header, so
// install must fall back to the symlink, which handles any byte.
func TestInstallMiseNewlinePathFallsBackToSymlink(t *testing.T) {
	base := filepath.Join(t.TempDir(), "new\nline")
	exe := filepath.Join(base, "mise", "installs", "github-tedilabs-tfvault", "1.0.0", "tfvault")
	shim := filepath.Join(base, "mise", "shims", "tfvault")
	for _, p := range []string{filepath.Dir(exe), filepath.Dir(shim)} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{exe, shim} {
		if err := os.WriteFile(p, []byte("fake"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	if _, err := installLink(exe, dir, false); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(dir, pluginBinary)); err != nil || target != exe {
		t.Errorf("target = %q, err = %v (want symlink fallback for newline path)", target, err)
	}
}

// TestInstallMiseWrapperQuoting builds a mise layout under a directory
// whose name contains shell-active characters and runs the generated
// wrapper through a real sh: expansion or injection would either fail
// the exec or change the output.
func TestInstallMiseWrapperQuoting(t *testing.T) {
	base := filepath.Join(t.TempDir(), "we`ird $HOME it's")
	exe := filepath.Join(base, "mise", "installs", "github-tedilabs-tfvault", "1.0.0", "tfvault")
	shim := filepath.Join(base, "mise", "shims", "tfvault")
	for _, p := range []string{filepath.Dir(exe), filepath.Dir(shim)} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exe, []byte("fake binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shim, []byte("#!/bin/sh\necho shim-ok \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if _, err := installLink(exe, dir, false); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, pluginBinary)
	if got := readWrapperShim(link); got != shim {
		t.Fatalf("wrapper shim = %q, want %q", got, shim)
	}

	out, err := exec.Command(link, "get").CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed to exec shim: %v\n%s", err, out)
	}
	if string(out) != "shim-ok get\n" {
		t.Errorf("wrapper output = %q, want %q", out, "shim-ok get\n")
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
		// Remove any symlink left by the previous iteration first:
		// WriteFile would follow it and try to write the running test
		// executable itself (ETXTBSY on Linux).
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
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
