// Package config loads the tfvault profile configuration from an HCL
// file. Each profile contains exactly one backend block; the block's
// options are handed opaquely to the backend factory, so this package
// needs no knowledge of individual backend schemas.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
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
// value, then $TFVAULT_CONFIG, then $XDG_CONFIG_HOME/tfvault/config.hcl
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
	return filepath.Join(base, "tfvault", "config.hcl"), nil
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

func parse(src []byte, path string) (*Config, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(src, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing config: %w", diags)
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("parsing config %s: unexpected body type", path)
	}

	cfg := &Config{Profiles: map[string]*Profile{}, Path: path}

	for name, attr := range body.Attributes {
		if name != "default_profile" {
			return nil, fmt.Errorf("%s: unsupported attribute %q", path, name)
		}
		v, err := stringValue(attr.Expr)
		if err != nil {
			return nil, fmt.Errorf("%s: default_profile: %w", path, err)
		}
		cfg.DefaultProfile = v
	}

	for _, block := range body.Blocks {
		if block.Type != "profile" {
			return nil, fmt.Errorf("%s: unsupported block type %q", path, block.Type)
		}
		if len(block.Labels) != 1 {
			return nil, fmt.Errorf("%s: profile block requires exactly one name label", path)
		}
		name := block.Labels[0]
		if _, dup := cfg.Profiles[name]; dup {
			return nil, fmt.Errorf("%s: duplicate profile %q", path, name)
		}
		p, err := parseProfile(name, block.Body)
		if err != nil {
			return nil, fmt.Errorf("%s: profile %q: %w", path, name, err)
		}
		cfg.Profiles[name] = p
	}

	if cfg.DefaultProfile != "" {
		if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
			return nil, fmt.Errorf("%s: default_profile %q does not match any profile (available: %v)",
				path, cfg.DefaultProfile, cfg.ProfileNames())
		}
	}
	return cfg, nil
}

func parseProfile(name string, body *hclsyntax.Body) (*Profile, error) {
	for attrName := range body.Attributes {
		return nil, fmt.Errorf("unexpected attribute %q (backend options belong inside a backend block)", attrName)
	}
	if len(body.Blocks) != 1 {
		return nil, fmt.Errorf("must contain exactly one backend block, found %d", len(body.Blocks))
	}
	block := body.Blocks[0]
	if len(block.Labels) != 0 {
		return nil, fmt.Errorf("backend block %q takes no labels", block.Type)
	}
	if len(block.Body.Blocks) != 0 {
		return nil, fmt.Errorf("backend block %q must not contain nested blocks", block.Type)
	}

	opts := map[string]string{}
	for optName, attr := range block.Body.Attributes {
		v, err := stringValue(attr.Expr)
		if err != nil {
			return nil, fmt.Errorf("backend %q option %q: %w", block.Type, optName, err)
		}
		opts[optName] = v
	}
	return &Profile{Name: name, Backend: block.Type, Options: opts}, nil
}

func stringValue(expr hclsyntax.Expression) (string, error) {
	val, diags := expr.Value(nil)
	if diags.HasErrors() {
		return "", fmt.Errorf("evaluating value: %w", diags)
	}
	val, err := convert.Convert(val, cty.String)
	if err != nil {
		return "", fmt.Errorf("value must be a string: %w", err)
	}
	if val.IsNull() {
		return "", errors.New("value must not be null")
	}
	return val.AsString(), nil
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
