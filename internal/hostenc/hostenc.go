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
// The port must be numeric and the colon unique: EnvSuffix maps both ":"
// and "-" onto underscores, so loosely validated inputs like "a::443"
// would collide with the legitimate host "a-443".
func Normalize(hostname string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(hostname))
	if h == "" {
		return "", fmt.Errorf("hostname is empty")
	}
	host, port, hasPort := strings.Cut(h, ":")
	if hasPort {
		if port == "" {
			return "", fmt.Errorf("invalid port in hostname %q", hostname)
		}
		for _, r := range port {
			if r < '0' || r > '9' {
				return "", fmt.Errorf("invalid port in hostname %q", hostname)
			}
		}
	}
	if host == "" {
		return "", fmt.Errorf("invalid hostname %q", hostname)
	}
	// Label-wise checks: empty labels also cover leading/trailing dots
	// and "..", which would otherwise smuggle path traversal.
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", fmt.Errorf("invalid hostname %q", hostname)
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= '0' && r <= '9':
			case r == '-':
			default:
				return "", fmt.Errorf("invalid character %q in hostname %q", r, hostname)
			}
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
