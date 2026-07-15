package pass

import (
	"strings"
	"testing"

	"github.com/tedilabs/tfvault/internal/backend"
)

func TestCheck(t *testing.T) {
	newChecker := func(opts map[string]string) backend.Checker {
		t.Helper()
		b, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		return b.(backend.Checker)
	}

	if err := newChecker(map[string]string{"binary": "sh"}).Check(); err != nil {
		t.Errorf("Check() with present binary: %v", err)
	}
	if err := newChecker(map[string]string{"binary": "tfvault-test-definitely-missing"}).Check(); err == nil {
		t.Error("Check() with missing binary: want error")
	}
	err := newChecker(map[string]string{"binary": "sh", "store_dir": t.TempDir() + "/nope"}).Check()
	if err == nil || !strings.Contains(err.Error(), "store_dir") {
		t.Errorf("Check() with missing store_dir = %v, want store_dir error", err)
	}
	if err := newChecker(map[string]string{"binary": "sh", "store_dir": t.TempDir()}).Check(); err != nil {
		t.Errorf("Check() with existing store_dir: %v", err)
	}
}
