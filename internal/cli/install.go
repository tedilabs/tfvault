package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
func runInstall(stdout, stderr io.Writer) int {
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
	msg, err := installLink(exe, dir)
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: install: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, msg)
	fmt.Fprintln(stdout, `Run "tfvault status" to verify your Terraform CLI setup.`)
	return 0
}

// installLink creates dir/terraform-credentials-tfvault -> exe. An
// existing symlink pointing elsewhere is updated; a regular file is
// never clobbered (it is likely a binary copied by an old installer).
func installLink(exe, dir string) (string, error) {
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
		return "", fmt.Errorf("%s exists and is not a symlink (an old install?); remove it first, then re-run tfvault install", link)
	}
	target, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	if target == exe {
		return fmt.Sprintf("Already installed: %s -> %s", link, exe), nil
	}
	if err := os.Remove(link); err != nil {
		return "", err
	}
	if err := os.Symlink(exe, link); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated %s -> %s (was %s)", link, exe, target), nil
}
