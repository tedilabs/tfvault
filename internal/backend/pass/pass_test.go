package pass

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePass creates a shell script that emulates the pass CLI over plain
// files in $PASSWORD_STORE_DIR, and records every argv line to a log so
// tests can assert tokens never travel via argv.
func fakePass(t *testing.T) (binary, argvLog string) {
	t.Helper()
	dir := t.TempDir()
	binary = filepath.Join(dir, "fakepass")
	argvLog = filepath.Join(dir, "argv.log")
	script := `#!/bin/sh
set -eu
echo "$@" >> ` + argvLog + `
store="${PASSWORD_STORE_DIR}"
cmd="$1"; shift
case "$cmd" in
  show)
    f="$store/$1.gpg"
    if [ ! -f "$f" ]; then
      echo "Error: $1 is not in the password store." >&2
      exit 1
    fi
    cat "$f"
    ;;
  insert)
    while [ "${1#-}" != "$1" ]; do shift; done
    f="$store/$1.gpg"
    mkdir -p "$(dirname "$f")"
    cat > "$f"
    ;;
  rm)
    while [ "${1#-}" != "$1" ]; do shift; done
    f="$store/$1.gpg"
    if [ ! -f "$f" ]; then
      echo "Error: $1 is not in the password store." >&2
      exit 1
    fi
    rm "$f"
    ;;
  *)
    echo "unknown command $cmd" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, argvLog
}

func newFakeBackend(t *testing.T, extraOpts map[string]string) (*Backend, string, string) {
	t.Helper()
	binary, argvLog := fakePass(t)
	storeDir := t.TempDir()
	opts := map[string]string{"binary": binary, "store_dir": storeDir}
	for k, v := range extraOpts {
		opts[k] = v
	}
	b, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return b.(*Backend), storeDir, argvLog
}

func TestRoundTrip(t *testing.T) {
	b, storeDir, argvLog := newFakeBackend(t, map[string]string{"prefix": "customers/a"})

	if _, found, err := b.Get("app.terraform.io"); err != nil || found {
		t.Fatalf("initial get: found=%v err=%v", found, err)
	}
	if err := b.Store("app.terraform.io", "tok-secret-123"); err != nil {
		t.Fatal(err)
	}

	// Entry lands under the configured prefix.
	if _, err := os.Stat(filepath.Join(storeDir, "customers/a/app.terraform.io.gpg")); err != nil {
		t.Errorf("entry not at expected path: %v", err)
	}

	token, found, err := b.Get("app.terraform.io")
	if err != nil || !found || token != "tok-secret-123" {
		t.Fatalf("get: token=%q found=%v err=%v", token, found, err)
	}

	if err := b.Forget("app.terraform.io"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := b.Get("app.terraform.io"); found {
		t.Error("token still present after forget")
	}
	if err := b.Forget("app.terraform.io"); err != nil {
		t.Errorf("forget absent: %v", err)
	}

	// The token must never appear in argv.
	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(argv), "tok-secret-123") {
		t.Errorf("token leaked via argv: %s", argv)
	}
}

func TestGetReturnsFirstLineOnly(t *testing.T) {
	b, storeDir, _ := newFakeBackend(t, nil)
	entry := filepath.Join(storeDir, "terraform/multi.example.com.gpg")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("first-line-token\nurl: https://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, found, err := b.Get("multi.example.com")
	if err != nil || !found || token != "first-line-token" {
		t.Fatalf("token=%q found=%v err=%v", token, found, err)
	}
}

func TestGetFailsClosedOnUnexpectedError(t *testing.T) {
	// A binary that always fails with an unrelated error must surface an
	// error, not read as "no credentials".
	dir := t.TempDir()
	binary := filepath.Join(dir, "brokenpass")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'gpg: decryption failed' >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := New(map[string]string{"binary": binary, "store_dir": dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Get("app.terraform.io"); err == nil {
		t.Fatal("want error, got nil (must fail closed)")
	} else if !strings.Contains(err.Error(), "decryption failed") {
		t.Errorf("error should include child stderr: %v", err)
	}
}

func TestOptionValidation(t *testing.T) {
	for _, opts := range []map[string]string{
		{"prefix": "/absolute"},
		{"prefix": "../escape"},
		{"prefix": ".."},
		{"prefix": ""},
		{"binary": ""},
		{"unknown": "x"},
	} {
		if _, err := New(opts); err == nil {
			t.Errorf("New(%v): want error, got nil", opts)
		}
	}
}

func TestList(t *testing.T) {
	b, storeDir, _ := newFakeBackend(t, nil)
	if err := b.Store("app.terraform.io", "a"); err != nil {
		t.Fatal(err)
	}
	if err := b.Store("spacelift.io", "b"); err != nil {
		t.Fatal(err)
	}
	hosts, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0] != "app.terraform.io" || hosts[1] != "spacelift.io" {
		t.Errorf("List() = %v", hosts)
	}
	_ = storeDir
}

func TestListMissingStoreIsEmpty(t *testing.T) {
	b, err := New(map[string]string{"store_dir": filepath.Join(t.TempDir(), "nope")})
	if err != nil {
		t.Fatal(err)
	}
	hosts, err := b.(*Backend).List()
	if err != nil || hosts != nil {
		t.Errorf("hosts=%v err=%v, want empty", hosts, err)
	}
}
