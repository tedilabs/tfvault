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

// runInstall symlinks the running executable into Terraform's plugin
// directory under the terraform-credentials-tfvault name.
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

// installLink creates dir/terraform-credentials-tfvault -> exe. An
// existing symlink pointing elsewhere is updated. Anything that is not
// a symlink (e.g. a binary copied by an old installer) is replaced
// only when force is set.
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
