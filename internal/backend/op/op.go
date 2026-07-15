// Package op stores credentials in 1Password by executing the op CLI
// (version 2). Entries are "API Credential" items titled
// <prefix><hostname> and tagged "tfvault" so they can be listed.
//
// Tokens are always exchanged with the child process over stdin/stdout
// (item JSON is piped to `op item create -`), never argv, so they
// cannot leak through the process table. Authentication is whatever
// the op CLI is configured with: the desktop-app integration,
// OP_SERVICE_ACCOUNT_TOKEN, or `op signin`.
package op

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/tedilabs/tfvault/internal/backend"
)

// DefaultPrefix is the item title prefix under which entries are kept:
// <prefix><hostname>.
const DefaultPrefix = "tfvault/"

// tag marks items managed by tfvault so List can enumerate them.
const tag = "tfvault"

func init() {
	backend.Register("op", New)
}

// Backend implements backend.Backend by executing the 1Password CLI.
type Backend struct {
	binary  string
	prefix  string
	vault   string // --vault when non-empty
	account string // --account when non-empty (for multiple 1Password accounts)
}

// New builds the op backend from profile options.
func New(opts map[string]string) (backend.Backend, error) {
	b := &Backend{binary: "op", prefix: DefaultPrefix}
	for k, v := range opts {
		switch k {
		case "binary":
			if v == "" {
				return nil, errors.New(`op backend: option "binary" must not be empty`)
			}
			b.binary = v
		case "prefix":
			if v == "" {
				return nil, errors.New(`op backend: option "prefix" must not be empty`)
			}
			b.prefix = v
		case "vault":
			b.vault = v
		case "account":
			b.account = v
		default:
			return nil, fmt.Errorf("op backend: unknown option %q (supported: binary, prefix, vault, account)", k)
		}
	}
	return b, nil
}

func (b *Backend) Name() string { return "op" }

// Check verifies the op binary is on PATH. It deliberately does not
// invoke it: any op command may trigger an interactive unlock or
// authorization prompt.
func (b *Backend) Check() error {
	if _, err := exec.LookPath(b.binary); err != nil {
		return fmt.Errorf("op backend: %w", err)
	}
	return nil
}

func (b *Backend) title(hostname string) string {
	return b.prefix + hostname
}

// scope returns the --vault/--account flags shared by all commands.
func (b *Backend) scope() []string {
	var args []string
	if b.vault != "" {
		args = append(args, "--vault", b.vault)
	}
	if b.account != "" {
		args = append(args, "--account", b.account)
	}
	return args
}

// run executes the op binary with the given stdin and returns stdout.
func (b *Backend) run(stdin string, args ...string) (stdout string, stderr string, err error) {
	cmd := exec.Command(b.binary, args...)
	cmd.Env = os.Environ()
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), errBuf.String(), err
}

// isNotFound reports whether op stderr indicates a missing item. op 2.x
// prints `"<name>" isn't an item. Specify the item with its UUID, name,
// or domain.` both when getting and deleting a nonexistent item. The
// integration test (go test -tags integration) checks this against the
// real CLI.
func isNotFound(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "isn't an item")
}

// failure formats a child process error, capping stderr so unexpected
// verbose output cannot flood the caller.
func (b *Backend) failure(op string, err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if len(msg) > 500 {
		msg = msg[:500] + "..."
	}
	if msg != "" {
		return fmt.Errorf("%s %s: %w: %s", b.binary, op, err, msg)
	}
	return fmt.Errorf("%s %s: %w", b.binary, op, err)
}

func (b *Backend) Get(hostname string) (string, bool, error) {
	args := append([]string{"item", "get", b.title(hostname), "--fields", "label=credential", "--reveal"}, b.scope()...)
	stdout, stderr, err := b.run("", args...)
	if err != nil {
		if isNotFound(stderr) {
			return "", false, nil
		}
		// Fail closed: a locked vault or expired session must not look
		// like "no credentials stored".
		return "", false, b.failure("item get", err, stderr)
	}
	token := strings.TrimRight(stdout, "\r\n")
	if token == "" {
		return "", false, fmt.Errorf("item %q exists but its credential field is empty", b.title(hostname))
	}
	return token, true, nil
}

// item is the subset of the op item JSON template used for creation.
type item struct {
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Fields   []field  `json:"fields"`
}

type field struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value"`
}

func (b *Backend) Store(hostname, token string) error {
	// 1Password allows duplicate titles, so remove any existing entry
	// first. Not atomic, but a failed create leaves a clear error
	// rather than a silently stale token.
	if err := b.Forget(hostname); err != nil {
		return err
	}
	payload, err := json.Marshal(item{
		Title:    b.title(hostname),
		Category: "API_CREDENTIAL",
		Tags:     []string{tag},
		Fields: []field{
			{ID: "credential", Type: "CONCEALED", Label: "credential", Value: token},
		},
	})
	if err != nil {
		return err
	}
	// "-" makes op read the item JSON from stdin; the token never
	// appears in argv.
	args := append([]string{"item", "create", "-"}, b.scope()...)
	if _, stderr, err := b.run(string(payload)+"\n", args...); err != nil {
		return b.failure("item create", err, stderr)
	}
	return nil
}

func (b *Backend) Forget(hostname string) error {
	args := append([]string{"item", "delete", b.title(hostname)}, b.scope()...)
	if _, stderr, err := b.run("", args...); err != nil {
		if isNotFound(stderr) {
			return nil
		}
		return b.failure("item delete", err, stderr)
	}
	return nil
}

// List enumerates hostnames from items tagged "tfvault" whose title
// carries the configured prefix.
func (b *Backend) List() ([]string, error) {
	args := append([]string{"item", "list", "--tags", tag, "--format", "json"}, b.scope()...)
	stdout, stderr, err := b.run("", args...)
	if err != nil {
		return nil, b.failure("item list", err, stderr)
	}
	var items []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		return nil, fmt.Errorf("parsing op item list output: %w", err)
	}
	var hosts []string
	for _, it := range items {
		if strings.HasPrefix(it.Title, b.prefix) {
			hosts = append(hosts, strings.TrimPrefix(it.Title, b.prefix))
		}
	}
	sort.Strings(hosts)
	return hosts, nil
}
