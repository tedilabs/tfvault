package cli

import (
	"fmt"
	"io"

	"github.com/tedilabs/tfvault/internal/backend"
	"github.com/tedilabs/tfvault/internal/config"
)

// zeroConfigBackend is the backend synthesized when no config file
// exists: the OS keyring with its default service name.
const zeroConfigBackend = "keyring"

func defaultResolveBackend(configPath, profileFlag string, stderr io.Writer) (backend.Backend, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	if cfg == nil {
		// Zero-config: a bare install must work with the default profile,
		// but a named profile implies per-account isolation the user set
		// up on purpose — falling back to a shared default there could
		// leak tokens across accounts, so fail instead.
		if profileFlag == "" || profileFlag == "default" {
			return backend.New(zeroConfigBackend, nil)
		}
		path, _ := config.ResolvePath(configPath)
		return nil, fmt.Errorf("profile %q requested but no config file found at %s", profileFlag, path)
	}

	for _, w := range cfg.Warnings {
		fmt.Fprintf(stderr, "tfvault: warning: %s\n", w)
	}

	name := profileFlag
	if name == "" {
		name = cfg.DefaultProfile
	}
	if name == "" {
		name = "default"
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found in %s (available: %v)", name, cfg.Path, cfg.ProfileNames())
	}
	return backend.New(p.Backend, p.Options)
}
