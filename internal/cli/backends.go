package cli

// Blank imports register the built-in backends. Adding a new backend to
// the binary is one line here plus its package.
import (
	_ "github.com/tedilabs/tfvault/internal/backend/env"
	_ "github.com/tedilabs/tfvault/internal/backend/keyring"
	_ "github.com/tedilabs/tfvault/internal/backend/op"
	_ "github.com/tedilabs/tfvault/internal/backend/pass"
)
