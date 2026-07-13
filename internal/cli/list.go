package cli

import (
	"fmt"
	"io"

	"github.com/tedilabs/tfvault/internal/backend"
)

// runList prints the hostnames the profile's backend has credentials
// for. Hostnames only — token values are never printed.
func runList(configPath, profile string, stdout, stderr io.Writer) int {
	b, err := resolveBackend(configPath, profile, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "terraform-credentials-tfvault: list: %v\n", err)
		return 1
	}
	lister, ok := b.(backend.Lister)
	if !ok {
		fmt.Fprintf(stderr, "terraform-credentials-tfvault: list: the %q backend does not support listing\n", b.Name())
		return 1
	}
	hosts, err := lister.List()
	if err != nil {
		fmt.Fprintf(stderr, "terraform-credentials-tfvault: list: %v\n", err)
		return 1
	}
	for _, h := range hosts {
		fmt.Fprintln(stdout, h)
	}
	return 0
}
