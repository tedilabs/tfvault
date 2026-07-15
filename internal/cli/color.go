package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/tedilabs/tfvault/internal/config"
)

// palette colorizes auxiliary command output with ANSI escapes. It is
// never applied to protocol (get/store/forget) or list output, which
// other programs consume.
type palette struct {
	enabled bool
}

// newPalette decides whether color is enabled, in precedence order:
// the --no-color flag, the NO_COLOR environment variable
// (https://no-color.org), the config file's color setting, and finally
// whether stdout is a terminal.
func newPalette(noColorFlag bool, configPath string, stdout io.Writer) *palette {
	if noColorFlag {
		return &palette{}
	}
	if os.Getenv("NO_COLOR") != "" {
		return &palette{}
	}
	// A config read error is surfaced by the command itself; for the
	// color decision it just means "unset".
	if cfg, err := config.Load(configPath); err == nil && cfg != nil && cfg.Color != nil && !*cfg.Color {
		return &palette{}
	}
	return &palette{enabled: isTerminal(stdout)}
}

// isTerminal reports whether w is a character device (a terminal).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

func (p *palette) wrap(code, s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}

func (p *palette) bold(s string) string   { return p.wrap(ansiBold, s) }
func (p *palette) dim(s string) string    { return p.wrap(ansiDim, s) }
func (p *palette) red(s string) string    { return p.wrap(ansiRed, s) }
func (p *palette) green(s string) string  { return p.wrap(ansiGreen, s) }
func (p *palette) yellow(s string) string { return p.wrap(ansiYellow, s) }
func (p *palette) cyan(s string) string   { return p.wrap(ansiCyan, s) }

// ok/warn/fail render the status prefixes used by the diagnostic
// commands so severity is readable at a glance.
func (p *palette) ok(label string) string   { return p.green(label) }
func (p *palette) warn(label string) string { return p.yellow(label) }
func (p *palette) fail(label string) string { return p.red(label) }

// sectionf prints a bold section header.
func (p *palette) sectionf(w io.Writer, format string, a ...any) {
	fmt.Fprintln(w, p.bold(fmt.Sprintf(format, a...)))
}
