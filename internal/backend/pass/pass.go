// Package pass stores credentials in a pass(1) or gopass password store
// by executing the CLI. gopass is command-line compatible with pass, so
// one backend covers both via the "binary" option.
//
// Tokens are always exchanged with the child process over stdin/stdout,
// never argv, so they cannot leak through the process table.
package pass

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tedilabs/tfvault/internal/backend"
)

// DefaultPrefix is the store path prefix under which entries are kept:
// <prefix>/<hostname>.
const DefaultPrefix = "terraform"

func init() {
	backend.Register("pass", New)
}

// Backend implements backend.Backend by executing pass/gopass.
type Backend struct {
	binary   string
	prefix   string
	storeDir string // sets PASSWORD_STORE_DIR for the child when non-empty
}

// New builds the pass backend from profile options.
func New(opts map[string]string) (backend.Backend, error) {
	b := &Backend{binary: "pass", prefix: DefaultPrefix}
	for k, v := range opts {
		switch k {
		case "binary":
			if v == "" {
				return nil, errors.New(`pass backend: option "binary" must not be empty`)
			}
			b.binary = v
		case "prefix":
			cleaned := path.Clean(v)
			if v == "" || path.IsAbs(cleaned) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
				return nil, fmt.Errorf("pass backend: invalid prefix %q (must be a relative store path)", v)
			}
			b.prefix = cleaned
		case "store_dir":
			dir, err := expandHome(v)
			if err != nil {
				return nil, fmt.Errorf("pass backend: store_dir: %w", err)
			}
			b.storeDir = dir
		default:
			return nil, fmt.Errorf("pass backend: unknown option %q (supported: binary, prefix, store_dir)", k)
		}
	}
	return b, nil
}

func expandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
	}
	return p, nil
}

func (b *Backend) Name() string { return "pass" }

// Check verifies the pass binary is on PATH and any configured store
// directory exists, without touching stored secrets or triggering a
// gpg pinentry prompt.
func (b *Backend) Check() error {
	if _, err := exec.LookPath(b.binary); err != nil {
		return fmt.Errorf("pass backend: %w", err)
	}
	if b.storeDir != "" {
		info, err := os.Stat(b.storeDir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("pass backend: store_dir %q is not a directory", b.storeDir)
		}
	}
	return nil
}

func (b *Backend) entry(hostname string) string {
	return path.Join(b.prefix, hostname)
}

// run executes the pass binary with the given stdin and returns stdout.
func (b *Backend) run(stdin string, args ...string) (stdout string, stderr string, err error) {
	cmd := exec.Command(b.binary, args...)
	cmd.Env = os.Environ()
	if b.storeDir != "" {
		cmd.Env = append(cmd.Env, "PASSWORD_STORE_DIR="+b.storeDir)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), errBuf.String(), err
}

// isNotFound reports whether pass/gopass stderr indicates a missing
// entry. pass prints "... is not in the password store."; gopass prints
// "entry is not in the password store" on show and
// `Secret "..." does not exist` on rm (verified against gopass 1.15).
func isNotFound(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "not in the password store") ||
		strings.Contains(s, "entry not found") ||
		strings.Contains(s, "does not exist")
}

// failure formats a child process error, capping stderr so unexpected
// verbose output cannot flood the caller.
func (b *Backend) failure(op string, err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if len(msg) > 500 {
		msg = msg[:500] + "..."
	}
	if msg != "" {
		return fmt.Errorf("%s %s: %w: %s", b.binary, op, err, msg)
	}
	return fmt.Errorf("%s %s: %w", b.binary, op, err)
}

func (b *Backend) Get(hostname string) (string, bool, error) {
	stdout, stderr, err := b.run("", "show", b.entry(hostname))
	if err != nil {
		if isNotFound(stderr) {
			return "", false, nil
		}
		// Fail closed: a broken GPG setup must not look like "no
		// credentials stored".
		return "", false, b.failure("show", err, stderr)
	}
	token, _, _ := strings.Cut(stdout, "\n")
	token = strings.TrimRight(token, "\r")
	if token == "" {
		return "", false, fmt.Errorf("entry %s exists but its first line is empty", b.entry(hostname))
	}
	return token, true, nil
}

func (b *Backend) Store(hostname, token string) error {
	// -m: read from stdin until EOF without the interactive double
	// prompt; -f: overwrite an existing entry.
	_, stderr, err := b.run(token+"\n", "insert", "-m", "-f", b.entry(hostname))
	if err != nil {
		return b.failure("insert", err, stderr)
	}
	return nil
}

func (b *Backend) Forget(hostname string) error {
	_, stderr, err := b.run("", "rm", "-f", b.entry(hostname))
	if err != nil {
		if isNotFound(stderr) {
			return nil
		}
		return b.failure("rm", err, stderr)
	}
	return nil
}

// List enumerates hostnames by walking the password store directory
// under the configured prefix. It resolves the store directory the same
// way pass does: store_dir option, then $PASSWORD_STORE_DIR, then
// ~/.password-store.
func (b *Backend) List() ([]string, error) {
	dir := b.storeDir
	if dir == "" {
		dir = os.Getenv("PASSWORD_STORE_DIR")
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".password-store")
	}
	root := filepath.Join(dir, filepath.FromSlash(b.prefix))
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".gpg") {
			continue
		}
		hosts = append(hosts, strings.TrimSuffix(e.Name(), ".gpg"))
	}
	sort.Strings(hosts)
	return hosts, nil
}
