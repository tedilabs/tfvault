// Package env reads credentials from environment variables. It is
// read-only: tokens are provided by the environment (CI, direnv, shell
// profiles), never written by the helper.
//
// Variable names follow Terraform's native TF_TOKEN_* encoding
// ('.' -> '_', '-' -> '__'), with a configurable prefix so multiple
// profiles can coexist (e.g. CUSTOMER_A_TF_TOKEN_app_terraform_io).
package env

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tedilabs/tfvault/internal/backend"
	"github.com/tedilabs/tfvault/internal/hostenc"
)

// DefaultPrefix matches Terraform's native token environment variables.
const DefaultPrefix = "TF_TOKEN_"

func init() {
	backend.Register("env", New)
}

// Backend implements a read-only backend.Backend over the environment.
type Backend struct {
	prefix string
}

// New builds the env backend from profile options.
func New(opts map[string]string) (backend.Backend, error) {
	b := &Backend{prefix: DefaultPrefix}
	for k, v := range opts {
		switch k {
		case "prefix":
			if v == "" {
				return nil, fmt.Errorf(`env backend: option "prefix" must not be empty`)
			}
			b.prefix = v
		default:
			return nil, fmt.Errorf("env backend: unknown option %q (supported: prefix)", k)
		}
	}
	return b, nil
}

func (b *Backend) Name() string { return "env" }

func (b *Backend) Get(hostname string) (string, bool, error) {
	v, ok := os.LookupEnv(b.prefix + hostenc.EnvSuffix(hostname))
	if !ok || v == "" {
		return "", false, nil
	}
	return v, true, nil
}

func (b *Backend) Store(string, string) error { return backend.ErrReadOnly }
func (b *Backend) Forget(string) error        { return backend.ErrReadOnly }

// List enumerates hostnames from matching environment variables. The
// decoding is best-effort: the TF_TOKEN encoding is lossy (ports and
// dots both map to '_'), so '__' becomes '-' and '_' becomes '.'.
func (b *Backend) List() ([]string, error) {
	var hosts []string
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(name, b.prefix) || value == "" {
			continue
		}
		suffix := strings.TrimPrefix(name, b.prefix)
		host := strings.ReplaceAll(suffix, "__", "\x00")
		host = strings.ReplaceAll(host, "_", ".")
		host = strings.ReplaceAll(host, "\x00", "-")
		hosts = append(hosts, strings.ToLower(host))
	}
	sort.Strings(hosts)
	return hosts, nil
}
