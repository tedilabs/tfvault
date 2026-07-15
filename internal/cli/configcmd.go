package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tedilabs/tfvault/internal/config"
)

// fallbackEditor is used by "config edit" when neither the config
// file's editor setting nor $EDITOR is set.
const fallbackEditor = "vi"

// runConfig dispatches the config subcommands.
func runConfig(args []string, configPath, profileFlag string, noColorFlag bool, pal *palette, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "tfvault: config: expected a subcommand: show, edit")
		return 1
	}
	if len(args) > 1 {
		fmt.Fprintf(stderr, "tfvault: config %s: unexpected argument %q\n", args[0], args[1])
		return 1
	}
	switch args[0] {
	case "show":
		return runConfigShow(configPath, profileFlag, noColorFlag, pal, stdout, stderr)
	case "edit":
		return runConfigEdit(configPath, stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "tfvault: config: unknown subcommand %q (expected show or edit)\n", args[0])
		return 1
	}
}

// runConfigShow prints the effective configuration: the config file
// settings with command line flags applied on top, each value annotated
// with where it came from.
func runConfigShow(configPath, profileFlag string, noColorFlag bool, pal *palette, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: %v\n", err)
		return 1
	}

	path := ""
	if cfg != nil {
		path = cfg.Path
		for _, w := range cfg.Warnings {
			fmt.Fprintf(stderr, "tfvault: warning: %s\n", w)
		}
	} else {
		path, _ = config.ResolvePath(configPath)
	}

	pal.sectionf(stdout, "Config file:")
	if cfg == nil {
		fmt.Fprintf(stdout, "  %s %s\n", path, pal.dim("(not found; zero-config defaults)"))
	} else {
		fmt.Fprintf(stdout, "  %s\n", path)
	}

	// Profile, following the same precedence as a helper invocation.
	profile, profileSource := profileFlag, "--profile flag"
	if profile == "" && cfg != nil && cfg.DefaultProfile != "" {
		profile, profileSource = cfg.DefaultProfile, "config default_profile"
	}
	if profile == "" {
		profile, profileSource = "default", "fallback"
	}

	editor, editorSource := resolveEditor(cfg)

	fmt.Fprintln(stdout)
	pal.sectionf(stdout, "Settings:")
	fmt.Fprintf(stdout, "  profile: %s %s\n", pal.bold(profile), pal.dim("(from "+profileSource+")"))
	if cfg != nil {
		if _, ok := cfg.Profiles[profile]; !ok {
			fmt.Fprintf(stdout, "  %s profile %q not found in %s (available: %v)\n", pal.warn("warning:"), profile, cfg.Path, cfg.ProfileNames())
		}
	}
	color, colorSource := effectiveColor(noColorFlag, cfg, pal)
	fmt.Fprintf(stdout, "  color: %s %s\n", color, pal.dim("(from "+colorSource+")"))
	fmt.Fprintf(stdout, "  editor: %s %s\n", editor, pal.dim("(from "+editorSource+")"))

	fmt.Fprintln(stdout)
	pal.sectionf(stdout, "Profiles:")
	if cfg == nil {
		fmt.Fprintf(stdout, "  %s %s  %s\n", pal.green("*"), pal.bold("default"), pal.cyan(zeroConfigBackend))
		return 0
	}
	for _, name := range cfg.ProfileNames() {
		p := cfg.Profiles[name]
		marker, label := " ", name
		if name == profile {
			marker, label = pal.green("*"), pal.bold(name)
		}
		fmt.Fprintf(stdout, "  %s %s  %s%s\n", marker, label, pal.cyan(p.Backend), pal.dim(formatOptions(p.Options)))
	}
	return 0
}

// effectiveColor reports whether color is enabled and which setting
// decided it, mirroring the precedence in newPalette.
func effectiveColor(noColorFlag bool, cfg *config.Config, pal *palette) (string, string) {
	switch {
	case noColorFlag:
		return "disabled", "--no-color flag"
	case os.Getenv("NO_COLOR") != "":
		return "disabled", "NO_COLOR env"
	case cfg != nil && cfg.Color != nil && !*cfg.Color:
		return "disabled", "config color"
	case pal.enabled:
		return "enabled", "terminal detection"
	default:
		return "disabled", "terminal detection"
	}
}

// resolveEditor returns the editor command for "config edit" and where
// it came from: the config file's editor setting, then $EDITOR, then a
// fixed fallback (the same precedence git gives core.editor).
func resolveEditor(cfg *config.Config) (string, string) {
	if cfg != nil && cfg.Editor != "" {
		return cfg.Editor, "config editor"
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e, "$EDITOR"
	}
	return fallbackEditor, "fallback"
}

// runConfigEdit opens the global config file in the user's editor. A
// missing file (and directory) is created first with owner-only
// permissions, matching the loose-permission warning config.Load emits.
func runConfigEdit(configPath string, stdin io.Reader, stdout, stderr io.Writer) int {
	path, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: config edit: %v\n", err)
		return 1
	}

	// A broken config file must still be editable — that is what this
	// command is for — so a load error only disables the config-file
	// editor setting.
	cfg, _ := config.Load(configPath)
	editor, _ := resolveEditor(cfg)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintf(stderr, "tfvault: config edit: %v\n", err)
		return 1
	}
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: config edit: %v\n", err)
		return 1
	}
	f.Close()

	// The editor value may carry arguments ("code -w"), so it runs
	// through the shell; the path is passed as a positional parameter
	// and never interpolated.
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "tfvault-edit", path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "tfvault: config edit: editor %q: %v\n", editor, err)
		return 1
	}

	// Catch mistakes while the editing session is still fresh instead
	// of on the next terraform run.
	if _, err := config.Load(configPath); err != nil {
		fmt.Fprintf(stderr, "tfvault: warning: saved config is invalid: %v\n", err)
		return 1
	}
	return 0
}
