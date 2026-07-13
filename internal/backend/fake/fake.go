// Package fake provides an in-memory backend for tests.
package fake

import "github.com/tedilabs/tfvault/internal/backend"

// Backend is an in-memory backend.Backend with error injection.
type Backend struct {
	Tokens   map[string]string
	ReadOnly bool
	// Err, when set, is returned by every operation.
	Err error
}

func New() *Backend {
	return &Backend{Tokens: map[string]string{}}
}

func (b *Backend) Name() string { return "fake" }

func (b *Backend) Get(hostname string) (string, bool, error) {
	if b.Err != nil {
		return "", false, b.Err
	}
	token, ok := b.Tokens[hostname]
	return token, ok, nil
}

func (b *Backend) Store(hostname, token string) error {
	if b.Err != nil {
		return b.Err
	}
	if b.ReadOnly {
		return backend.ErrReadOnly
	}
	b.Tokens[hostname] = token
	return nil
}

func (b *Backend) Forget(hostname string) error {
	if b.Err != nil {
		return b.Err
	}
	if b.ReadOnly {
		return backend.ErrReadOnly
	}
	delete(b.Tokens, hostname)
	return nil
}
