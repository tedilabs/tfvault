package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tedilabs/tfvault/internal/config"
)

// runProfiles prints the configured profiles, their backend and options.
// Option values may contain paths or service names but never tokens.
func runProfiles(configPath string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "terraform-credentials-tfvault: %v\n", err)
		return 1
	}
	if cfg == nil {
		path, _ := config.ResolvePath(configPath)
		fmt.Fprintf(stdout, "no config file at %s (zero-config default)\n", path)
		fmt.Fprintf(stdout, "* default  %s\n", zeroConfigBackend)
		return 0
	}
	for _, w := range cfg.Warnings {
		fmt.Fprintf(stderr, "terraform-credentials-tfvault: warning: %s\n", w)
	}

	defaultName := cfg.DefaultProfile
	if defaultName == "" {
		defaultName = "default"
	}
	for _, name := range cfg.ProfileNames() {
		p := cfg.Profiles[name]
		marker := " "
		if name == defaultName {
			marker = "*"
		}
		fmt.Fprintf(stdout, "%s %s  %s%s\n", marker, name, p.Backend, formatOptions(p.Options))
	}
	return 0
}

func formatOptions(opts map[string]string) string {
	if len(opts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, opts[k]))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}
