package keyring

import (
	"strings"
	"testing"

	gokeyring "github.com/zalando/go-keyring"
)

func TestOptions(t *testing.T) {
	b, err := New(map[string]string{"service": "tfvault-customer-a"})
	if err != nil {
		t.Fatal(err)
	}
	if b.(*Backend).service != "tfvault-customer-a" {
		t.Errorf("service = %q", b.(*Backend).service)
	}

	b, err = New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.(*Backend).service != DefaultService {
		t.Errorf("default service = %q", b.(*Backend).service)
	}

	if _, err := New(map[string]string{"unknown": "x"}); err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Errorf("unknown option: err = %v", err)
	}
	if _, err := New(map[string]string{"service": ""}); err == nil {
		t.Error("empty service must be rejected")
	}
}

func TestRoundTrip(t *testing.T) {
	gokeyring.MockInit()
	b, err := New(map[string]string{"service": "tfvault-test"})
	if err != nil {
		t.Fatal(err)
	}

	if _, found, err := b.Get("app.terraform.io"); err != nil || found {
		t.Fatalf("initial get: found=%v err=%v", found, err)
	}
	if err := b.Store("app.terraform.io", "tok-123"); err != nil {
		t.Fatal(err)
	}
	token, found, err := b.Get("app.terraform.io")
	if err != nil || !found || token != "tok-123" {
		t.Fatalf("get after store: token=%q found=%v err=%v", token, found, err)
	}
	if err := b.Forget("app.terraform.io"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := b.Get("app.terraform.io"); found {
		t.Error("token still present after forget")
	}
	// forget of an absent entry must succeed (protocol requirement).
	if err := b.Forget("app.terraform.io"); err != nil {
		t.Errorf("forget absent: %v", err)
	}
}
