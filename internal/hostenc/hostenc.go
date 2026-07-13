// Package hostenc normalizes and encodes Terraform service hostnames.
//
// Hostnames are used as keyring account names, pass entry path components
// and environment variable suffixes, so normalization also acts as an
// injection guard (path traversal, argv injection).
package hostenc

import (
	"fmt"
	"strings"
)

// Normalize lowercases and validates a hostname received from Terraform.
// It rejects anything that is not a plausible host[:port] so hostnames are
// safe to embed in filesystem paths, argv and environment variable names.
func Normalize(hostname string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(hostname))
	if h == "" {
		return "", fmt.Errorf("hostname is empty")
	}
	if strings.HasPrefix(h, "-") || strings.HasPrefix(h, ".") {
		return "", fmt.Errorf("invalid hostname %q", hostname)
	}
	if strings.Contains(h, "..") {
		return "", fmt.Errorf("invalid hostname %q", hostname)
	}
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == ':':
		default:
			return "", fmt.Errorf("invalid character %q in hostname %q", r, hostname)
		}
	}
	return h, nil
}

// EnvSuffix converts a normalized hostname to the encoding Terraform uses
// for its native TF_TOKEN_* environment variables: periods become single
// underscores and dashes become double underscores. Ports (colons) also
// become single underscores.
func EnvSuffix(hostname string) string {
	r := strings.NewReplacer("-", "__", ".", "_", ":", "_")
	return r.Replace(hostname)
}
