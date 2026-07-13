// Package cli implements the terraform-credentials-tfvault command line
// interface, including the Terraform credentials helper protocol
// (get/store/forget) and a few auxiliary commands.
package cli

import (
	"flag"
	"fmt"
	"io"
)

const usage = `Usage: terraform-credentials-tfvault [flags] <command> [hostname]

Terraform credentials helper commands (invoked by Terraform):
  get <hostname>      print stored credentials for hostname as JSON
  store <hostname>    store credentials read from stdin as JSON
  forget <hostname>   remove stored credentials for hostname

Auxiliary commands:
  version             print version information

Flags:
  --profile <name>    profile to use (default: config default_profile, else "default")
  --config <path>     config file path (default: $TFVAULT_CONFIG, else ~/.config/tfvault/config.hcl)
`

// resolveBackend resolves the backend for a profile. It is a variable so
// tests can substitute a fake backend.
var resolveBackend = defaultResolveBackend

// Run executes the CLI and returns the process exit code. All I/O goes
// through the provided streams so the full program is testable.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("terraform-credentials-tfvault", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	profile := fs.String("profile", "", "profile name")
	configPath := fs.String("config", "", "config file path")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	verb, verbArgs := rest[0], rest[1:]

	switch verb {
	case "get", "store", "forget":
		return runProtocol(verb, verbArgs, *configPath, *profile, stdin, stdout, stderr)
	case "version":
		return runVersion(stdout)
	default:
		// Unknown verbs must fail so future protocol extensions are not
		// silently misinterpreted.
		fmt.Fprintf(stderr, "terraform-credentials-tfvault: unknown command %q\n", verb)
		return 1
	}
}
