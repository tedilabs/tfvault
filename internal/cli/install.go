package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// wrapperMarker identifies wrapper scripts written by tfvault install,
// so later runs (and status) can tell them apart from foreign files.
const wrapperMarker = "# tfvault-install wrapper"

// wrapperScript is the plugin entry written for mise installs: it execs
// the mise shim so the helper follows mise's active tfvault version
// (including per-directory pins) instead of pinning one release binary
// that goes stale on every upgrade.
func wrapperScript(shim string) string {
	return fmt.Sprintf(`#!/bin/sh
%s
# Execs the mise shim so the helper follows mise's active tfvault
# version. Regenerate with "tfvault install".
exec %q "$@"
`, wrapperMarker, shim)
}

// miseShimTarget returns the mise shim to wrap when exe lives inside a
// mise install directory (<data-dir>/mise/installs/<tool>/<version>/...),
// or "" when it does not or the shim is missing.
func miseShimTarget(exe string) string {
	sep := string(filepath.Separator)
	marker := sep + "mise" + sep + "installs" + sep
	i := strings.Index(exe, marker)
	if i < 0 {
		// The binary may be reached through a symlink into the mise tree.
		resolved, err := filepath.EvalSymlinks(exe)
		if err != nil {
			return ""
		}
		exe = resolved
		if i = strings.Index(exe, marker); i < 0 {
			return ""
		}
	}
	shim := filepath.Join(exe[:i], "mise", "shims", filepath.Base(exe))
	if info, err := os.Stat(shim); err != nil || info.IsDir() {
		return ""
	}
	return shim
}

// readWrapperShim returns the shim path a tfvault wrapper at path execs,
// or "" when path is not a tfvault wrapper. Reads are capped since the
// path may hold an arbitrarily large foreign binary.
func readWrapperShim(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	src, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil || !strings.Contains(string(src), wrapperMarker) {
		return ""
	}
	for _, line := range strings.Split(string(src), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), `exec "`); ok {
			if shim, _, ok := strings.Cut(rest, `"`); ok {
				return shim
			}
		}
	}
	return ""
}

// pluginBinary is the name Terraform discovers credentials helpers by:
// terraform-credentials-<name> inside the CLI plugin directory. The
// distributed binary is called tfvault, so install symlinks it under
// this name.
const pluginBinary = "terraform-credentials-tfvault"

// pluginDir returns Terraform's user plugin directory.
func pluginDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".terraform.d", "plugins"), nil
}

// runInstall links the running executable into Terraform's plugin
// directory under the terraform-credentials-tfvault name: a symlink
// normally, or a shim wrapper for mise-managed installs.
func runInstall(args []string, pal *palette, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("tfvault install", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var force bool
	flags.BoolVar(&force, "f", false, "replace whatever exists at the link path")
	flags.BoolVar(&force, "force", false, "replace whatever exists at the link path")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "tfvault: install: unexpected argument %q\n", flags.Arg(0))
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: install: %v\n", err)
		return 1
	}
	dir, err := pluginDir()
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: install: %v\n", err)
		return 1
	}
	msg, err := installLink(exe, dir, force)
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: install: %v\n", err)
		return 1
	}
	// Color the leading action word (Created/Updated/Replaced/Already
	// installed) so the outcome reads at a glance.
	if word, rest, found := strings.Cut(msg, " "); found {
		msg = pal.ok(word) + " " + rest
	}
	fmt.Fprintln(stdout, msg)
	fmt.Fprintln(stdout, pal.dim(`Run "tfvault status" to verify your Terraform CLI setup.`))
	return 0
}

// installLink creates dir/terraform-credentials-tfvault for exe: a
// wrapper execing the mise shim when exe is a mise-managed install,
// else a symlink to exe. An existing symlink or tfvault wrapper is
// updated freely; anything else (e.g. a binary copied by an old
// installer) is replaced only when force is set.
func installLink(exe, dir string, force bool) (string, error) {
	// os.Executable does not guarantee an absolute path; a relative
	// target would resolve against the plugins directory and break.
	exe, err := filepath.Abs(exe)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(dir, pluginBinary)
	if shim := miseShimTarget(exe); shim != "" {
		return installWrapper(shim, link, force)
	}
	info, err := os.Lstat(link)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.Symlink(exe, link); err != nil {
			return "", err
		}
		return fmt.Sprintf("Created %s -> %s", link, exe), nil
	case err != nil:
		return "", err
	case info.Mode()&fs.ModeSymlink == 0:
		if wasShim := readWrapperShim(link); wasShim != "" {
			if err := replaceLink(exe, link); err != nil {
				return "", err
			}
			return fmt.Sprintf("Replaced %s -> %s (was a wrapper for mise shim %s)", link, exe, wasShim), nil
		}
		if !force {
			return "", fmt.Errorf("%s exists and is not a symlink (an old install?); re-run with --force to replace it", link)
		}
		if err := replaceLink(exe, link); err != nil {
			return "", err
		}
		return fmt.Sprintf("Replaced %s -> %s (was not a symlink)", link, exe), nil
	}
	target, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	if target == exe {
		return fmt.Sprintf("Already installed: %s -> %s", link, exe), nil
	}
	if err := replaceLink(exe, link); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated %s -> %s (was %s)", link, exe, target), nil
}

// installWrapper writes link as a wrapper script execing shim. Existing
// symlinks and tfvault wrappers are replaced freely; foreign files need
// force, exactly like installLink.
func installWrapper(shim, link string, force bool) (string, error) {
	content := wrapperScript(shim)
	info, err := os.Lstat(link)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := writeWrapper(link, content); err != nil {
			return "", err
		}
		return fmt.Sprintf("Created %s (wraps mise shim %s)", link, shim), nil
	case err != nil:
		return "", err
	case info.Mode()&fs.ModeSymlink != 0:
		target, _ := os.Readlink(link)
		if err := writeWrapper(link, content); err != nil {
			return "", err
		}
		return fmt.Sprintf("Replaced %s (wraps mise shim %s; was symlink to %s)", link, shim, target), nil
	}
	switch readWrapperShim(link) {
	case shim:
		return fmt.Sprintf("Already installed: %s wraps mise shim %s", link, shim), nil
	case "":
		if !force {
			return "", fmt.Errorf("%s exists and is not a symlink (an old install?); re-run with --force to replace it", link)
		}
	}
	if err := writeWrapper(link, content); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated %s (wraps mise shim %s)", link, shim), nil
}

// writeWrapper atomically replaces link with an executable wrapper by
// renaming a temporary file over it. Writing via a temp file also means
// an existing symlink is replaced rather than followed — a direct write
// through a stale symlink could clobber whatever it points at.
func writeWrapper(link, content string) error {
	tmp := link + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(tmp, []byte(content), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// replaceLink atomically replaces link with a symlink to exe by
// renaming a temporary symlink over it, so a failure never leaves the
// plugin without any link at all.
func replaceLink(exe, link string) error {
	tmp := link + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Symlink(exe, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
