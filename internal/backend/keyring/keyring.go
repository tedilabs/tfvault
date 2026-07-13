// Package keyring stores credentials in the operating system keyring
// (macOS Keychain, Linux Secret Service) via zalando/go-keyring.
//
// The keyring entry service name is configurable per profile, which is
// the isolation mechanism for keeping accounts of different customers
// (or personal vs. work) apart on one machine.
package keyring

import (
	"errors"
	"fmt"
	"runtime"

	gokeyring "github.com/zalando/go-keyring"

	"github.com/tedilabs/tfvault/internal/backend"
)

// DefaultService is the keyring service name used when a profile does
// not override it.
const DefaultService = "tfvault"

func init() {
	backend.Register("keyring", New)
}

// Backend implements backend.Backend on the OS keyring.
type Backend struct {
	service string
}

// New builds the keyring backend from profile options.
func New(opts map[string]string) (backend.Backend, error) {
	b := &Backend{service: DefaultService}
	for k, v := range opts {
		switch k {
		case "service":
			if v == "" {
				return nil, errors.New(`keyring backend: option "service" must not be empty`)
			}
			b.service = v
		default:
			return nil, fmt.Errorf("keyring backend: unknown option %q (supported: service)", k)
		}
	}
	return b, nil
}

func (b *Backend) Name() string { return "keyring" }

func (b *Backend) Get(hostname string) (string, bool, error) {
	token, err := gokeyring.Get(b.service, hostname)
	if errors.Is(err, gokeyring.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, b.wrap(err)
	}
	return token, true, nil
}

func (b *Backend) Store(hostname, token string) error {
	if err := gokeyring.Set(b.service, hostname, token); err != nil {
		return b.wrap(err)
	}
	return nil
}

func (b *Backend) Forget(hostname string) error {
	err := gokeyring.Delete(b.service, hostname)
	if errors.Is(err, gokeyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return b.wrap(err)
	}
	return nil
}

func (b *Backend) wrap(err error) error {
	hint := ""
	if runtime.GOOS == "linux" {
		hint = " (the keyring backend needs a running Secret Service daemon such as gnome-keyring; on headless machines consider the pass backend)"
	}
	return fmt.Errorf("os keyring (service %q): %w%s", b.service, err, hint)
}
