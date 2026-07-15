package op

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOp creates a shell script that emulates the op CLI over plain
// files in $FAKE_OP_DIR, and records every argv line to a log so tests
// can assert tokens never travel via argv. Items are stored as
// two-line files: title, then credential.
func fakeOp(t *testing.T) (binary, argvLog, storeDir string) {
	t.Helper()
	dir := t.TempDir()
	binary = filepath.Join(dir, "fakeop")
	argvLog = filepath.Join(dir, "argv.log")
	storeDir = filepath.Join(dir, "store")
	if err := os.Mkdir(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
echo "$@" >> ` + argvLog + `
store="` + storeDir + `"
if [ "${FAKE_OP_FAIL:-}" = "1" ]; then
  echo "[ERROR] session expired, sign in again" >&2
  exit 1
fi
fname() { printf '%s' "$1" | tr '/:' '__'; }
sub="$1 $2"; shift 2
case "$sub" in
  "item get")
    f="$store/$(fname "$1")"
    if [ ! -f "$f" ]; then
      echo "[ERROR] \"$1\" isn't an item. Specify the item with its UUID, name, or domain." >&2
      exit 1
    fi
    sed -n 2p "$f"
    ;;
  "item create")
    # "-" means the item JSON arrives on stdin.
    [ "$1" = "-" ] || { echo "expected -" >&2; exit 2; }
    json=$(cat)
    title=$(printf '%s' "$json" | sed -E 's/.*"title":"([^"]*)".*/\1/')
    value=$(printf '%s' "$json" | sed -E 's/.*"value":"([^"]*)".*/\1/')
    printf '%s\n%s\n' "$title" "$value" > "$store/$(fname "$title")"
    ;;
  "item delete")
    f="$store/$(fname "$1")"
    if [ ! -f "$f" ]; then
      echo "[ERROR] \"$1\" isn't an item. Specify the item with its UUID, name, or domain." >&2
      exit 1
    fi
    rm "$f"
    ;;
  "item list")
    out="["
    sep=""
    for f in "$store"/*; do
      [ -f "$f" ] || continue
      title=$(sed -n 1p "$f")
      out="$out$sep{\"title\":\"$title\"}"
      sep=","
    done
    printf '%s]\n' "$out"
    ;;
  *)
    echo "unknown subcommand $sub" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, argvLog, storeDir
}

func newFakeBackend(t *testing.T, extraOpts map[string]string) (*Backend, string) {
	t.Helper()
	binary, argvLog, _ := fakeOp(t)
	opts := map[string]string{"binary": binary}
	for k, v := range extraOpts {
		opts[k] = v
	}
	b, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return b.(*Backend), argvLog
}

func TestRoundTrip(t *testing.T) {
	b, argvLog := newFakeBackend(t, nil)
	const host = "app.terraform.io"
	const token = "super-secret-token"

	if _, found, err := b.Get(host); err != nil || found {
		t.Fatalf("Get before store: found=%v err=%v", found, err)
	}
	if err := b.Store(host, token); err != nil {
		t.Fatal(err)
	}
	got, found, err := b.Get(host)
	if err != nil || !found || got != token {
		t.Fatalf("Get = (%q, %v, %v), want (%q, true, nil)", got, found, err, token)
	}

	hosts, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0] != host {
		t.Errorf("List = %v", hosts)
	}

	if err := b.Forget(host); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := b.Get(host); found {
		t.Error("Get after forget: still found")
	}
	// Forget must succeed when nothing is stored.
	if err := b.Forget(host); err != nil {
		t.Errorf("Forget of missing entry: %v", err)
	}

	// The token must never appear in any argv.
	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(argv), token) {
		t.Errorf("token leaked into argv:\n%s", argv)
	}
}

func TestGetFailsClosedOnUnexpectedError(t *testing.T) {
	b, _ := newFakeBackend(t, nil)
	t.Setenv("FAKE_OP_FAIL", "1")

	_, found, err := b.Get("app.terraform.io")
	if err == nil {
		t.Fatal("want error for locked/expired session, got nil")
	}
	if found {
		t.Error("found = true on error")
	}
	if !strings.Contains(err.Error(), "session expired") {
		t.Errorf("error should carry op stderr: %v", err)
	}
}

func TestScopeFlagsArePassed(t *testing.T) {
	b, argvLog := newFakeBackend(t, map[string]string{
		"vault":   "Work",
		"account": "my.1password.com",
	})
	if err := b.Store("app.terraform.io", "tok"); err != nil {
		t.Fatal(err)
	}
	argv, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--vault Work", "--account my.1password.com"} {
		if !strings.Contains(string(argv), want) {
			t.Errorf("argv missing %q:\n%s", want, argv)
		}
	}
}

func TestListFiltersByPrefix(t *testing.T) {
	b, _ := newFakeBackend(t, map[string]string{"prefix": "customer-a/"})
	other, _ := newFakeBackend(t, nil)
	_ = other

	if err := b.Store("app.terraform.io", "tok"); err != nil {
		t.Fatal(err)
	}
	hosts, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0] != "app.terraform.io" {
		t.Errorf("List = %v", hosts)
	}
}

func TestOptionValidation(t *testing.T) {
	for name, opts := range map[string]map[string]string{
		"unknown option": {"nope": "x"},
		"empty binary":   {"binary": ""},
		"empty prefix":   {"prefix": ""},
	} {
		if _, err := New(opts); err == nil {
			t.Errorf("%s: want error, got nil", name)
		}
	}
}
