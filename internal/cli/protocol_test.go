package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tedilabs/tfvault/internal/backend"
	"github.com/tedilabs/tfvault/internal/backend/fake"
)

// trackingReader reports whether the underlying reader was fully consumed.
type trackingReader struct {
	r io.Reader
}

func (t *trackingReader) Read(p []byte) (int, error) { return t.r.Read(p) }

func (t *trackingReader) fullyConsumed() bool {
	var buf [1]byte
	n, err := t.r.Read(buf[:])
	return n == 0 && err == io.EOF
}

// run executes the CLI against the given fake backend and returns exit
// code, stdout, stderr and the stdin tracker.
func run(t *testing.T, b backend.Backend, stdin string, args ...string) (int, string, string, *trackingReader) {
	t.Helper()
	orig := resolveBackend
	resolveBackend = func(configPath, profile string) (backend.Backend, error) {
		if b == nil {
			return nil, errors.New("backend resolution failed")
		}
		return b, nil
	}
	t.Cleanup(func() { resolveBackend = orig })

	in := &trackingReader{r: strings.NewReader(stdin)}
	var out, errOut bytes.Buffer
	code := Run(args, in, &out, &errOut)
	return code, out.String(), errOut.String(), in
}

func TestGetFound(t *testing.T) {
	b := fake.New()
	b.Tokens["app.terraform.io"] = "secret-token"
	code, out, errOut, _ := run(t, b, "", "get", "app.terraform.io")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if out != `{"token":"secret-token"}`+"\n" {
		t.Errorf("stdout = %q", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
}

func TestGetNotFound(t *testing.T) {
	code, out, errOut, _ := run(t, fake.New(), "", "get", "app.terraform.io")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if out != "{}\n" {
		t.Errorf("stdout = %q, want {}", out)
	}
}

func TestGetBackendError(t *testing.T) {
	b := fake.New()
	b.Err = errors.New("keyring locked")
	code, out, errOut, _ := run(t, b, "", "get", "app.terraform.io")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero (fail closed)")
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty on error", out)
	}
	if !strings.Contains(errOut, "keyring locked") {
		t.Errorf("stderr = %q, want backend error", errOut)
	}
}

func TestGetTokenJSONEscaping(t *testing.T) {
	b := fake.New()
	b.Tokens["app.terraform.io"] = `we"ird\to<ken>` + " "
	code, out, _, _ := run(t, b, "", "get", "app.terraform.io")
	if code != 0 {
		t.Fatal("exit code nonzero")
	}
	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (out=%q)", err, out)
	}
	if parsed.Token != b.Tokens["app.terraform.io"] {
		t.Errorf("token round-trip mismatch: %q", parsed.Token)
	}
}

func TestStore(t *testing.T) {
	b := fake.New()
	code, out, errOut, in := run(t, b, `{"token":"new-token"}`, "store", "app.terraform.io")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty on store", out)
	}
	if b.Tokens["app.terraform.io"] != "new-token" {
		t.Errorf("stored token = %q", b.Tokens["app.terraform.io"])
	}
	if !in.fullyConsumed() {
		t.Error("stdin not fully consumed")
	}
}

func TestStoreRejectsExtraProperties(t *testing.T) {
	b := fake.New()
	code, out, errOut, in := run(t, b, `{"token":"x","expiration":123}`, "store", "app.terraform.io")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	if !strings.Contains(errOut, "expiration") {
		t.Errorf("stderr = %q, want mention of offending property", errOut)
	}
	if len(b.Tokens) != 0 {
		t.Error("token must not be stored when object is rejected")
	}
	if !in.fullyConsumed() {
		t.Error("stdin not fully consumed on rejection path")
	}
}

func TestStoreMalformedJSON(t *testing.T) {
	code, _, _, in := run(t, fake.New(), `{not json`, "store", "app.terraform.io")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	if !in.fullyConsumed() {
		t.Error("stdin not fully consumed on malformed JSON path")
	}
}

func TestStoreMissingToken(t *testing.T) {
	code, _, errOut, _ := run(t, fake.New(), `{}`, "store", "app.terraform.io")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	if !strings.Contains(errOut, "token") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestStoreNonStringToken(t *testing.T) {
	code, _, _, _ := run(t, fake.New(), `{"token":42}`, "store", "app.terraform.io")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
}

func TestStoreReadOnlyBackend(t *testing.T) {
	b := fake.New()
	b.ReadOnly = true
	code, _, errOut, in := run(t, b, `{"token":"x"}`, "store", "app.terraform.io")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	if !strings.Contains(errOut, "read-only") {
		t.Errorf("stderr = %q, want read-only message", errOut)
	}
	if !in.fullyConsumed() {
		t.Error("stdin not fully consumed")
	}
}

func TestStoreBackendResolutionFailureConsumesStdin(t *testing.T) {
	code, _, _, in := run(t, nil, `{"token":"x"}`, "store", "app.terraform.io")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	if !in.fullyConsumed() {
		t.Error("stdin not fully consumed when backend resolution fails")
	}
}

func TestForget(t *testing.T) {
	b := fake.New()
	b.Tokens["app.terraform.io"] = "x"
	code, out, errOut, _ := run(t, b, "", "forget", "app.terraform.io")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	if _, ok := b.Tokens["app.terraform.io"]; ok {
		t.Error("token not removed")
	}
}

func TestForgetAbsentSucceeds(t *testing.T) {
	code, _, errOut, _ := run(t, fake.New(), "", "forget", "never.stored.example.com")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q (forget must succeed when absent)", code, errOut)
	}
}

func TestUnknownVerb(t *testing.T) {
	code, out, errOut, _ := run(t, fake.New(), "", "frobnicate", "app.terraform.io")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero for unknown verb")
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	if !strings.Contains(errOut, "frobnicate") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestInvalidHostname(t *testing.T) {
	for _, host := range []string{"../etc/passwd", "host/slash", "-dash.example.com"} {
		code, out, _, _ := run(t, fake.New(), "", "get", host)
		if code == 0 {
			t.Errorf("get %q: exit code = 0, want nonzero", host)
		}
		if out != "" {
			t.Errorf("get %q: stdout = %q, want empty", host, out)
		}
	}
}

func TestMissingHostnameArgument(t *testing.T) {
	code, _, _, _ := run(t, fake.New(), "", "get")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero")
	}
	code, _, _, _ = run(t, fake.New(), "", "get", "a.example.com", "b.example.com")
	if code == 0 {
		t.Fatal("exit code = 0, want nonzero for extra args")
	}
}

func TestFlagsBeforeVerb(t *testing.T) {
	// Terraform invokes: helper [args-from-terraformrc...] verb hostname.
	b := fake.New()
	b.Tokens["app.terraform.io"] = "tok"
	code, out, errOut, _ := run(t, b, "", "--profile", "customer-a", "get", "app.terraform.io")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "tok") {
		t.Errorf("stdout = %q", out)
	}
}
