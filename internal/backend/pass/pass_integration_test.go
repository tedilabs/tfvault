//go:build integration

package pass

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRealCLIRoundTrip exercises the backend against the real pass and
// gopass CLIs with a throwaway GPG key and password store. Binaries
// that are not installed are skipped.
func TestRealCLIRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg not installed")
	}
	// GNUPGHOME must be short: gpg-agent creates unix sockets inside it
	// and socket paths are capped around 104 chars (macOS t.TempDir()
	// paths under /var/folders are too long).
	gnupgHome, err := os.MkdirTemp("/tmp", "tfv-gpg-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("gpgconf", "--homedir", gnupgHome, "--kill", "gpg-agent").Run()
		_ = os.RemoveAll(gnupgHome)
	})
	if err := os.Chmod(gnupgHome, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GNUPGHOME", gnupgHome)

	keySpec := `%no-protection
Key-Type: RSA
Key-Length: 2048
Subkey-Type: RSA
Subkey-Length: 2048
Name-Email: tfvault-test@example.com
Expire-Date: 0
%commit
`
	gen := exec.Command("gpg", "--batch", "--generate-key")
	gen.Stdin = strings.NewReader(keySpec)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("gpg key generation failed: %v\n%s", err, out)
	}

	for _, binary := range []string{"pass", "gopass"} {
		t.Run(binary, func(t *testing.T) {
			if _, err := exec.LookPath(binary); err != nil {
				t.Skipf("%s not installed", binary)
			}
			storeDir := t.TempDir()
			t.Setenv("PASSWORD_STORE_DIR", storeDir)
			// Isolate gopass from any user-level configuration.
			t.Setenv("GOPASS_HOMEDIR", t.TempDir())

			if out, err := exec.Command(binary, "init", "tfvault-test@example.com").CombinedOutput(); err != nil {
				t.Fatalf("%s init failed: %v\n%s", binary, err, out)
			}

			b, err := New(map[string]string{"binary": binary, "store_dir": storeDir, "prefix": "terraform"})
			if err != nil {
				t.Fatal(err)
			}

			if _, found, err := b.Get("app.terraform.io"); err != nil || found {
				t.Fatalf("initial get: found=%v err=%v", found, err)
			}
			if err := b.Store("app.terraform.io", "integration-token"); err != nil {
				t.Fatal(err)
			}
			token, found, err := b.Get("app.terraform.io")
			if err != nil || !found || token != "integration-token" {
				t.Fatalf("get: token=%q found=%v err=%v", token, found, err)
			}
			if err := b.Forget("app.terraform.io"); err != nil {
				t.Fatal(err)
			}
			if err := b.Forget("app.terraform.io"); err != nil {
				t.Errorf("forget of absent entry must succeed: %v", err)
			}
		})
	}
}
