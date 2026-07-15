package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionOutput(t *testing.T) {
	wants := map[string]string{
		"bash": "complete -F _tfvault tfvault",
		"zsh":  "#compdef tfvault",
		"fish": "complete -c tfvault",
	}
	for shell, want := range wants {
		var out, errOut bytes.Buffer
		code := Run([]string{"completion", shell}, strings.NewReader(""), &out, &errOut)
		if code != 0 {
			t.Errorf("%s: exit code = %d, stderr = %q", shell, code, errOut.String())
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("%s: output missing %q", shell, want)
		}
	}
}

func TestCompletionBadArgs(t *testing.T) {
	for _, args := range [][]string{
		{"completion"},
		{"completion", "powershell"},
		{"completion", "bash", "extra"},
	} {
		var out, errOut bytes.Buffer
		if code := Run(args, strings.NewReader(""), &out, &errOut); code == 0 {
			t.Errorf("%v: exit code = 0, want nonzero", args)
		}
	}
}

// TestCompletionSyntax parses each script with its shell when the shell
// is installed, so a quoting mistake fails in CI rather than in users'
// dotfiles.
func TestCompletionSyntax(t *testing.T) {
	checks := []struct {
		shell string
		args  []string
	}{
		{"bash", []string{"-n"}},
		{"zsh", []string{"-n"}},
		{"fish", []string{"--no-execute"}},
	}
	for _, c := range checks {
		bin, err := exec.LookPath(c.shell)
		if err != nil {
			t.Logf("%s not installed; skipping syntax check", c.shell)
			continue
		}
		var out, errOut bytes.Buffer
		if code := Run([]string{"completion", c.shell}, strings.NewReader(""), &out, &errOut); code != 0 {
			t.Fatalf("completion %s: exit code = %d", c.shell, code)
		}
		script := filepath.Join(t.TempDir(), "completion."+c.shell)
		if err := os.WriteFile(script, out.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, append(c.args, script)...)
		if combined, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s syntax check failed: %v\n%s", c.shell, err, combined)
		}
	}
}
