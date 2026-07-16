// Package cli implements the tfvault command line
// interface, including the Terraform credentials helper protocol
// (get/store/forget) and a few auxiliary commands.
package cli

import (
	"errors"
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
  install [-f]        link the helper into ~/.terraform.d/plugins
                      (-f/--force replaces an unrecognized existing file)
  status              show plugin link, terraformrc and profile resolution
  profiles            list configured profiles
  list                list hostnames with stored credentials (never tokens)
  version             print version information
  completion <shell>  print a completion script (bash, zsh, fish)
  help [topic]        print usage (topics: config)

Command groups (run without a subcommand for details):
  config              inspect and edit the tfvault configuration

Flags:
  --profile <name>    profile to use (default: config default_profile, else "default")
  --config <path>     config file path (default: $TFVAULT_CONFIG, else ~/.config/tfvault/config.yaml)
  --no-color          disable colored output (also: NO_COLOR env, "color: false" in config)
  --version           print version information
`

// resolveBackend resolves the backend for a profile. It is a variable so
// tests can substitute a fake backend.
var resolveBackend = defaultResolveBackend

// Run executes the CLI and returns the process exit code. All I/O goes
// through the provided streams so the full program is testable.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tfvault", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// Usage printing is handled on the error paths below; a no-op here
	// avoids flag's implicit call printing it a second time on -h.
	fs.Usage = func() {}
	profile := fs.String("profile", "", "profile name")
	configPath := fs.String("config", "", "config file path")
	noColor := fs.Bool("no-color", false, "disable colored output")
	versionFlag := fs.Bool("version", false, "print version information")
	if err := fs.Parse(args); err != nil {
		// Requested help is not a failure: usage goes to stdout, exit 0.
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprint(stderr, usage)
		return 1
	}
	if *versionFlag {
		return runVersion(stdout)
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
	case "config":
		return runConfig(verbArgs, *configPath, *profile, *noColor, pal, stdin, stdout, stderr)
	case "profiles":
		return runProfiles(*configPath, pal, stdout, stderr)
	case "list":
		return runList(*configPath, *profile, stdout, stderr)
	case "version":
		return runVersion(stdout)
	case "completion":
		return runCompletion(verbArgs, stdout, stderr)
	case "help":
		return runHelp(verbArgs, stdout, stderr)
	default:
		// Unknown verbs must fail so future protocol extensions are not
		// silently misinterpreted.
		fmt.Fprintf(stderr, "tfvault: unknown command %q\n", verb)
		return 1
	}
}

// runHelp prints usage for the CLI or for a command group topic.
func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}
	switch args[0] {
	case "config":
		fmt.Fprint(stdout, configUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "tfvault: help: unknown topic %q\n", args[0])
		return 1
	}
}
