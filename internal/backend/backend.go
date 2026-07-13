// Package backend defines the credential storage backend contract and the
// registry through which backends are looked up by name. Adding a new
// backend means adding one package that calls Register in its init().
package backend

import "errors"

// ErrReadOnly is returned by Store and Forget on backends that cannot
// write (e.g. the env backend).
var ErrReadOnly = errors.New("backend is read-only")

// Backend stores Terraform credentials keyed by service hostname.
type Backend interface {
	// Get returns the token for hostname. When no credentials are stored
	// it returns found=false with a nil error.
	Get(hostname string) (token string, found bool, err error)
	// Store saves the token for hostname, overwriting any existing one.
	Store(hostname, token string) error
	// Forget removes the token for hostname. It returns nil when no
	// credentials were stored (the protocol requires forget to succeed
	// even when there is nothing to forget).
	Forget(hostname string) error
	// Name returns the backend type name, for diagnostics.
	Name() string
}

// Lister is optionally implemented by backends that can enumerate the
// hostnames they have credentials for.
type Lister interface {
	List() ([]string, error)
}
