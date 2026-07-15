package op

import (
	"testing"

	"github.com/tedilabs/tfvault/internal/backend"
)

func TestCheck(t *testing.T) {
	b, err := New(map[string]string{"binary": "sh"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.(backend.Checker).Check(); err != nil {
		t.Errorf("Check() with present binary: %v", err)
	}

	missing, err := New(map[string]string{"binary": "tfvault-test-definitely-missing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := missing.(backend.Checker).Check(); err == nil {
		t.Error("Check() with missing binary: want error")
	}
}
