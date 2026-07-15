// Package cli implements the tfvault command line
// interface, including the Terraform credentials helper protocol
// (get/store/forget) and a few auxiliary commands.
package cli

import (
	"flag"
	"fmt"
	"io"
)

const usage = `Usage: tfvault [flags] <command> [hostname]

Terraform credentials helper commands (invoked by Terraform):
  get <hostname>      print stored credentials for hostname as JSON
  store <hostname>    store credentials read from stdin as JSON
  forget <hostname>   remove stored credentials for hostname

Auxiliary commands:
  install [-f]        symlink the helper into ~/.terraform.d/plugins
                      (-f/--force replaces an existing non-symlink file)
  status              show plugin link, terraformrc and profile resolution
  profiles            list configured profiles
  list                list hostnames with stored credentials (never tokens)
  version             print version information

Flags:
  --profile <name>    profile to use (default: config default_profile, else "default")
  --config <path>     config file path (default: $TFVAULT_CONFIG, else ~/.config/tfvault/config.yaml)
  --no-color          disable colored output (also: NO_COLOR env, "color: false" in config)
`

// resolveBackend resolves the backend for a profile. It is a variable so
// tests can substitute a fake backend.
var resolveBackend = defaultResolveBackend

// Run executes the CLI and returns the process exit code. All I/O goes
// through the provided streams so the full program is testable.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tfvault", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	profile := fs.String("profile", "", "profile name")
	configPath := fs.String("config", "", "config file path")
	noColor := fs.Bool("no-color", false, "disable colored output")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	verb, verbArgs := rest[0], rest[1:]

	// Color applies to auxiliary command output only; protocol and list
	// output are consumed by other programs and stay plain.
	pal := newPalette(*noColor, *configPath, stdout)

	switch verb {
	case "get", "store", "forget":
		return runProtocol(verb, verbArgs, *configPath, *profile, stdin, stdout, stderr)
	case "install":
		return runInstall(verbArgs, pal, stdout, stderr)
	case "status":
		return runStatus(*configPath, *profile, pal, stdout, stderr)
	case "profiles":
		return runProfiles(*configPath, pal, stdout, stderr)
	case "list":
		return runList(*configPath, *profile, stdout, stderr)
	case "version":
		return runVersion(stdout)
	default:
		// Unknown verbs must fail so future protocol extensions are not
		// silently misinterpreted.
		fmt.Fprintf(stderr, "tfvault: unknown command %q\n", verb)
		return 1
	}
}
