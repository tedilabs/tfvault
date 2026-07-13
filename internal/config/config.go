// Package config loads the tfvault profile configuration from a YAML
// file. Each profile names exactly one backend; the profile's options
// map is handed opaquely to the backend factory, so this package needs
// no knowledge of individual backend schemas.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Profile is one named credentials profile.
type Profile struct {
	Name    string
	Backend string
	Options map[string]string
}

// Config is the parsed configuration file.
type Config struct {
	DefaultProfile string
	Profiles       map[string]*Profile
	// Path is the file the config was loaded from.
	Path string
	// Warnings are non-fatal issues (e.g. loose file permissions) the
	// caller should surface on stderr.
	Warnings []string
}

// ResolvePath returns the config file path to use: the explicit flag
// value, then $TFVAULT_CONFIG, then $XDG_CONFIG_HOME/tfvault/config.yaml
// (with ~/.config as the XDG fallback).
func ResolvePath(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	if p := os.Getenv("TFVAULT_CONFIG"); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving config path: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tfvault", "config.yaml"), nil
}

// Load resolves the config path and parses the file. When the file does
// not exist it returns (nil, nil) so the caller can apply zero-config
// defaults.
func Load(flagPath string) (*Config, error) {
	path, err := ResolvePath(flagPath)
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg, err := parse(src, path)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
		cfg.Warnings = append(cfg.Warnings,
			fmt.Sprintf("config file %s is readable by other users (mode %04o); consider chmod 0600", path, info.Mode().Perm()))
	}
	return cfg, nil
}

// yamlConfig mirrors the on-disk YAML schema:
//
//	default_profile: personal
//	profiles:
//	  personal:
//	    backend: keyring
//	    options:
//	      service: tfvault-personal
type yamlConfig struct {
	DefaultProfile string                 `yaml:"default_profile"`
	Profiles       map[string]yamlProfile `yaml:"profiles"`
}

type yamlProfile struct {
	Backend string            `yaml:"backend"`
	Options map[string]string `yaml:"options"`
}

func parse(src []byte, path string) (*Config, error) {
	var raw yamlConfig
	dec := yaml.NewDecoder(bytes.NewReader(src))
	// Reject unknown keys so typos fail loudly instead of being ignored.
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	cfg := &Config{Profiles: map[string]*Profile{}, Path: path, DefaultProfile: raw.DefaultProfile}

	for name, p := range raw.Profiles {
		if p.Backend == "" {
			return nil, fmt.Errorf("%s: profile %q: must specify a backend", path, name)
		}
		opts := p.Options
		if opts == nil {
			opts = map[string]string{}
		}
		cfg.Profiles[name] = &Profile{Name: name, Backend: p.Backend, Options: opts}
	}

	if cfg.DefaultProfile != "" {
		if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
			return nil, fmt.Errorf("%s: default_profile %q does not match any profile (available: %v)",
				path, cfg.DefaultProfile, cfg.ProfileNames())
		}
	}
	return cfg, nil
}

// ProfileNames returns the configured profile names, sorted.
func (c *Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
