//go:build integration

package op

import (
	"os"
	"os/exec"
	"slices"
	"testing"
)

// TestRealCLIRoundTrip exercises the backend against the real op CLI.
// It requires an authenticated session (desktop-app integration,
// OP_SERVICE_ACCOUNT_TOKEN or `op signin`) and is skipped otherwise.
// It creates and removes a single item titled
// tfvault-integration-test/<host> in the default vault, or in
// $TFVAULT_TEST_OP_VAULT when set.
func TestRealCLIRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("op"); err != nil {
		t.Skip("op CLI not installed")
	}
	if err := exec.Command("op", "whoami").Run(); err != nil {
		t.Skip("op CLI not signed in")
	}

	opts := map[string]string{"prefix": "tfvault-integration-test/"}
	if v := os.Getenv("TFVAULT_TEST_OP_VAULT"); v != "" {
		opts["vault"] = v
	}
	be, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	b := be.(*Backend)

	const host = "integration.tfvault.example"
	const token = "integration-test-token"
	t.Cleanup(func() { _ = b.Forget(host) })

	if _, found, err := b.Get(host); err != nil || found {
		t.Fatalf("Get before store: found=%v err=%v", found, err)
	}
	if err := b.Store(host, token); err != nil {
		t.Fatal(err)
	}
	// Storing twice must not create a duplicate item.
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
	if !slices.Contains(hosts, host) {
		t.Errorf("List = %v, missing %q", hosts, host)
	}
	if err := b.Forget(host); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := b.Get(host); found {
		t.Error("Get after forget: still found")
	}
	if err := b.Forget(host); err != nil {
		t.Errorf("Forget of missing entry: %v", err)
	}
}
