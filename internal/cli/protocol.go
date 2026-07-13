package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tedilabs/tfvault/internal/backend"
	"github.com/tedilabs/tfvault/internal/hostenc"
)

// runProtocol handles the three Terraform credentials helper verbs.
func runProtocol(verb string, args []string, configPath, profile string, stdin io.Reader, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		// store must fully consume stdin before failing, whatever the error.
		if verb == "store" {
			_, _ = io.Copy(io.Discard, stdin)
		}
		fmt.Fprintf(stderr, "tfvault: %s: %v\n", verb, err)
		return 1
	}

	if len(args) != 1 {
		return fail(fmt.Errorf("expected exactly one hostname argument, got %d", len(args)))
	}
	hostname, err := hostenc.Normalize(args[0])
	if err != nil {
		return fail(err)
	}
	b, err := resolveBackend(configPath, profile, stderr)
	if err != nil {
		return fail(err)
	}

	switch verb {
	case "get":
		return runGet(b, hostname, stdout, stderr)
	case "store":
		return runStore(b, hostname, stdin, stderr)
	case "forget":
		return runForget(b, hostname, stderr)
	}
	panic("unreachable")
}

func runGet(b backend.Backend, hostname string, stdout, stderr io.Writer) int {
	token, found, err := b.Get(hostname)
	if err != nil {
		// Fail closed: an ambiguous backend error must not become an empty
		// {} success, which would let Terraform proceed unauthenticated.
		fmt.Fprintf(stderr, "tfvault: get %s: %v\n", hostname, err)
		return 1
	}
	if !found {
		fmt.Fprintln(stdout, "{}")
		return 0
	}
	out, err := json.Marshal(struct {
		Token string `json:"token"`
	}{token})
	if err != nil {
		fmt.Fprintf(stderr, "tfvault: get %s: %v\n", hostname, err)
		return 1
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

func runStore(b backend.Backend, hostname string, stdin io.Reader, stderr io.Writer) int {
	// The protocol requires the helper to fully consume stdin before
	// failing, so read everything up front.
	raw, err := io.ReadAll(stdin)
	fail := func(err error) int {
		fmt.Fprintf(stderr, "tfvault: store %s: %v\n", hostname, err)
		return 1
	}
	if err != nil {
		return fail(fmt.Errorf("reading credentials from stdin: %w", err))
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fail(fmt.Errorf("parsing credentials JSON: %w", err))
	}
	// Reject any property other than "token" outright: silently dropping
	// unknown properties would corrupt credentials written by future
	// Terraform versions.
	for k := range obj {
		if k != "token" {
			return fail(fmt.Errorf("credentials object contains unsupported property %q", k))
		}
	}
	rawToken, ok := obj["token"]
	if !ok {
		return fail(errors.New(`credentials object is missing the "token" property`))
	}
	var token string
	if err := json.Unmarshal(rawToken, &token); err != nil {
		return fail(errors.New(`the "token" property must be a string`))
	}

	if err := b.Store(hostname, token); err != nil {
		if errors.Is(err, backend.ErrReadOnly) {
			return fail(fmt.Errorf("the %q backend is read-only; set the corresponding environment variable instead", b.Name()))
		}
		return fail(err)
	}
	return 0
}

func runForget(b backend.Backend, hostname string, stderr io.Writer) int {
	if err := b.Forget(hostname); err != nil {
		if errors.Is(err, backend.ErrReadOnly) {
			fmt.Fprintf(stderr, "tfvault: forget %s: the %q backend is read-only\n", hostname, b.Name())
			return 1
		}
		fmt.Fprintf(stderr, "tfvault: forget %s: %v\n", hostname, err)
		return 1
	}
	return 0
}
