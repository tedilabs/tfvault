package cli

import (
	"fmt"
	"io"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func runVersion(stdout io.Writer) int {
	fmt.Fprintf(stdout, "tfvault %s (commit %s, built %s)\n", version, commit, date)
	return 0
}
