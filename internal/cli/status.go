package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/tedilabs/tfvault/internal/backend"
	"github.com/tedilabs/tfvault/internal/config"
)

// terraformRC is the subset of the Terraform CLI configuration that
// concerns credentials: the helper registration and any explicit
// credentials blocks (which take precedence over the helper).
type terraformRC struct {
	Path       string
	HelperName string // "" when no credentials_helper block exists
	HelperArgs []string
	CredHosts  []string
}

// terraformRCPath mirrors Terraform's CLI config lookup on unix:
// $TF_CLI_CONFIG_FILE, else ~/.terraformrc.
func terraformRCPath() (string, error) {
	if p := os.Getenv("TF_CLI_CONFIG_FILE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".terraformrc"), nil
}

// parseTerraformRC extracts credentials_helper and credentials blocks.
// Unrelated blocks and attributes (provider_installation, plugin_cache_dir,
// ...) are ignored: this is a diagnostic reader, not a validator.
func parseTerraformRC(src []byte, path string) (*terraformRC, error) {
	file, diags := hclparse.NewParser().ParseHCL(src, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing %s: %w", path, diags)
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("parsing %s: unexpected body type", path)
	}

	rc := &terraformRC{Path: path}
	for _, block := range body.Blocks {
		switch block.Type {
		case "credentials_helper":
			if rc.HelperName != "" || len(block.Labels) != 1 {
				continue
			}
			rc.HelperName = block.Labels[0]
			if attr, ok := block.Body.Attributes["args"]; ok {
				val, diags := attr.Expr.Value(nil)
				if diags.HasErrors() || !val.CanIterateElements() {
					continue
				}
				for _, v := range val.AsValueSlice() {
					if v.Type() == cty.String && !v.IsNull() {
						rc.HelperArgs = append(rc.HelperArgs, v.AsString())
					}
				}
			}
		case "credentials":
			if len(block.Labels) == 1 {
				rc.CredHosts = append(rc.CredHosts, block.Labels[0])
			}
		}
	}
	return rc, nil
}

// flagValue extracts the value of --<name> or --<name>=<value> from a
// helper args list.
func flagValue(args []string, name string) string {
	for i, a := range args {
		trimmed := strings.TrimLeft(a, "-")
		if trimmed == a {
			continue // not a flag
		}
		if trimmed == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(trimmed, name+"=") {
			return strings.TrimPrefix(trimmed, name+"=")
		}
	}
	return ""
}

// runStatus reports how Terraform will reach tfvault: the plugin
// symlink, the terraformrc helper registration, and which profile and
// backend requests will resolve to. Exit code is nonzero when the
// helper is not fully wired up.
func runStatus(configPath, profileFlag string, pal *palette, stdout, stderr io.Writer) int {
	healthy := true

	// Plugin symlink.
	pal.sectionf(stdout, "Plugin link:")
	dir, err := pluginDir()
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: status: %v\n", err)
		return 1
	}
	link := filepath.Join(dir, pluginBinary)
	healthy = reportLink(pal, stdout, link) && healthy

	// Terraform CLI config.
	fmt.Fprintln(stdout)
	pal.sectionf(stdout, "Terraform CLI config:")
	rc := &terraformRC{}
	rcPath, err := terraformRCPath()
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: status: %v\n", err)
		return 1
	}
	src, err := os.ReadFile(rcPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fmt.Fprintf(stdout, "  %s %s does not exist\n", pal.fail("missing:"), rcPath)
		healthy = false
	case err != nil:
		fmt.Fprintf(stdout, "  %s %v\n", pal.fail("error:"), err)
		healthy = false
	default:
		rc, err = parseTerraformRC(src, rcPath)
		if err != nil {
			fmt.Fprintf(stdout, "  %s %v\n", pal.fail("error:"), err)
			healthy = false
			rc = &terraformRC{}
			break
		}
		switch {
		case rc.HelperName == "":
			fmt.Fprintf(stdout, "  %s %s has no credentials_helper block\n", pal.warn("warning:"), rcPath)
			healthy = false
		case rc.HelperName != "tfvault":
			fmt.Fprintf(stdout, "  %s %s registers credentials_helper %q, not \"tfvault\"\n", pal.warn("warning:"), rcPath, rc.HelperName)
			healthy = false
		default:
			fmt.Fprintf(stdout, "  %s %s registers credentials_helper \"tfvault\" (args: %q)\n", pal.ok("ok:"), rcPath, rc.HelperArgs)
		}
		for _, h := range rc.CredHosts {
			fmt.Fprintf(stdout, "  %s credentials %q block takes precedence over the helper for that host\n", pal.warn("note:"), h)
		}
	}

	// Token sources Terraform consults before the helper. These may be
	// intentional, so they are notes rather than failures.
	fmt.Fprintln(stdout)
	pal.sectionf(stdout, "Shadowing token sources:")
	reportShadowing(pal, stdout)

	// Profile and backend resolution, following the same precedence as
	// a real helper invocation driven by this terraformrc.
	fmt.Fprintln(stdout)
	pal.sectionf(stdout, "Profile:")
	if configPath == "" {
		configPath = flagValue(rc.HelperArgs, "config")
	}
	profile := profileFlag
	source := "--profile flag"
	if profile == "" {
		profile = flagValue(rc.HelperArgs, "profile")
		source = "terraformrc helper args"
	}

	b := reportProfile(pal, stdout, configPath, profile, source, stderr)
	if b == nil {
		return 1
	}
	if c, ok := b.(backend.Checker); ok {
		if err := c.Check(); err != nil {
			fmt.Fprintf(stdout, "  %s %v\n", pal.fail("check:"), err)
			healthy = false
		} else {
			fmt.Fprintf(stdout, "  %s backend prerequisites present\n", pal.ok("check:"))
		}
	}

	// Stored hosts, when the backend can enumerate them.
	fmt.Fprintln(stdout)
	pal.sectionf(stdout, "Hosts with stored credentials:")
	if lister, ok := b.(backend.Lister); ok {
		hosts, err := lister.List()
		switch {
		case err != nil:
			// A backend that cannot even list (locked vault, dead daemon)
			// will not serve credentials either; the report must fail.
			fmt.Fprintf(stdout, "  %s %v\n", pal.fail("error:"), err)
			healthy = false
		case len(hosts) == 0:
			fmt.Fprintln(stdout, pal.dim("  (none)"))
		default:
			for _, h := range hosts {
				fmt.Fprintf(stdout, "  %s\n", h)
			}
		}
	} else {
		fmt.Fprintf(stdout, "%s\n", pal.dim(fmt.Sprintf("  (the %q backend cannot enumerate entries)", b.Name())))
	}

	if !healthy {
		return 1
	}
	return 0
}

// reportShadowing warns about token sources Terraform consults before
// any credentials helper: TF_TOKEN_* environment variables and the
// plaintext credentials file terraform login writes when no helper is
// configured.
func reportShadowing(pal *palette, stdout io.Writer) {
	found := false

	var names []string
	for _, kv := range os.Environ() {
		if name, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(name, "TF_TOKEN_") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(stdout, "  %s %s is set; Terraform uses it before consulting the helper\n", pal.warn("note:"), n)
		found = true
	}

	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".terraform.d", "credentials.tfrc.json")
		if hosts := plaintextCredHosts(path); len(hosts) > 0 {
			fmt.Fprintf(stdout, "  %s plaintext tokens in %s for: %s\n", pal.warn("note:"), path, strings.Join(hosts, ", "))
			fmt.Fprintf(stdout, "        they take precedence over the helper; remove with \"terraform logout <hostname>\"\n")
			found = true
		}
	}

	if !found {
		fmt.Fprintln(stdout, pal.dim("  (none)"))
	}
}

// plaintextCredHosts returns the hostnames with a token in Terraform's
// plaintext credentials file, or nil when the file is absent or not in
// the expected shape. Tokens themselves are never read out.
func plaintextCredHosts(path string) []string {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f struct {
		Credentials map[string]struct {
			Token string `json:"token"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(src, &f); err != nil {
		return nil
	}
	var hosts []string
	for h, c := range f.Credentials {
		if c.Token != "" {
			hosts = append(hosts, h)
		}
	}
	sort.Strings(hosts)
	return hosts
}

// reportLink prints the state of the plugin symlink and returns whether
// it is usable.
func reportLink(pal *palette, stdout io.Writer, link string) bool {
	info, err := os.Lstat(link)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fmt.Fprintf(stdout, "  %s %s — run \"tfvault install\"\n", pal.fail("missing:"), link)
		return false
	case err != nil:
		fmt.Fprintf(stdout, "  %s %v\n", pal.fail("error:"), err)
		return false
	case info.Mode()&fs.ModeSymlink == 0:
		if shim := readWrapperShim(link); shim != "" {
			if _, err := os.Stat(shim); err != nil {
				fmt.Fprintf(stdout, "  %s %s wraps mise shim %s (shim missing) — run \"tfvault install\"\n", pal.fail("broken:"), link, shim)
				return false
			}
			fmt.Fprintf(stdout, "  %s %s wraps mise shim %s\n", pal.ok("ok:"), link, shim)
			return true
		}
		fmt.Fprintf(stdout, "  %s %s is not a symlink (an old install?)\n", pal.warn("warning:"), link)
		return true // a real binary there still works for Terraform
	}
	target, err := os.Readlink(link)
	if err != nil {
		fmt.Fprintf(stdout, "  %s %v\n", pal.fail("error:"), err)
		return false
	}
	if _, err := os.Stat(link); err != nil {
		fmt.Fprintf(stdout, "  %s %s -> %s (target missing) — run \"tfvault install\"\n", pal.fail("broken:"), link, target)
		return false
	}
	fmt.Fprintf(stdout, "  %s %s -> %s\n", pal.ok("ok:"), link, target)
	return true
}

// reportProfile prints the resolved profile and backend and returns the
// instantiated backend, or nil when resolution fails.
func reportProfile(pal *palette, stdout io.Writer, configPath, profile, source string, stderr io.Writer) backend.Backend {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stdout, "  %s %v\n", pal.fail("error:"), err)
		return nil
	}

	if cfg == nil {
		path, _ := config.ResolvePath(configPath)
		if profile != "" && profile != "default" {
			fmt.Fprintf(stdout, "  %s profile %q requested but no config file found at %s\n", pal.fail("error:"), profile, path)
			return nil
		}
		fmt.Fprintf(stdout, "  default (zero-config: no file at %s)\n", path)
		fmt.Fprintf(stdout, "  backend: %s\n", pal.cyan(zeroConfigBackend))
		b, err := backend.New(zeroConfigBackend, nil)
		if err != nil {
			fmt.Fprintf(stdout, "  %s %v\n", pal.fail("error:"), err)
			return nil
		}
		return b
	}

	for _, w := range cfg.Warnings {
		fmt.Fprintf(stderr, "tfvault: warning: %s\n", w)
	}
	if profile == "" {
		profile, source = cfg.DefaultProfile, "config default_profile"
	}
	if profile == "" {
		profile, source = "default", "fallback"
	}
	p, ok := cfg.Profiles[profile]
	if !ok {
		fmt.Fprintf(stdout, "  %s profile %q not found in %s (available: %v)\n", pal.fail("error:"), profile, cfg.Path, cfg.ProfileNames())
		return nil
	}
	fmt.Fprintf(stdout, "  %s (from %s, config %s)\n", pal.bold(profile), source, cfg.Path)
	fmt.Fprintf(stdout, "  backend: %s%s\n", pal.cyan(p.Backend), formatOptions(p.Options))
	b, err := backend.New(p.Backend, p.Options)
	if err != nil {
		fmt.Fprintf(stdout, "  %s %v\n", pal.fail("error:"), err)
		return nil
	}
	return b
}
