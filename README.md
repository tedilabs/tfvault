![tfvault — Universal Terraform credentials helper](docs/assets/readme-banner-light.png)

# tfvault

A universal [Terraform credentials helper](https://developer.hashicorp.com/terraform/internals/credentials-helpers)
with pluggable secret backends and per-file account isolation.

Terraform asks the helper for a token whenever it talks to a
Terraform-native service — Terraform Cloud/Enterprise, Scalr, Spacelift,
private module/provider registries — keyed by hostname. tfvault answers
from the backend you configure:

| Backend   | Storage                                              | Writable |
|-----------|------------------------------------------------------|----------|
| `keyring` | OS keyring (macOS Keychain, Linux Secret Service)    | yes      |
| `pass`    | [pass](https://www.passwordstore.org/) / [gopass](https://www.gopass.pw/) password store | yes |
| `op`      | [1Password](https://developer.1password.com/docs/cli/) via the `op` CLI | yes |
| `env`     | environment variables (`TF_TOKEN_*` encoding)        | read-only |

Planned: AWS Secrets Manager, SSM Parameter Store, HashiCorp Vault.
Each backend is one Go package + one registration line.

Supported platforms: macOS and Linux, amd64 and arm64.

## Install

Homebrew (macOS and Linux):

```sh
brew install tedilabs/tap/tfvault
```

Or [mise](https://mise.jdx.dev), via either the github or the aqua
backend (both resolve the same release archives; the aqua backend adds
checksum verification through the [aqua registry](https://github.com/aquaproj/aqua-registry)):

```sh
mise use -g github:tedilabs/tfvault
# or
mise use -g aqua:tedilabs/tfvault

tfvault install   # link the helper into ~/.terraform.d/plugins
```

Or the install script:

```sh
curl -fsSL https://raw.githubusercontent.com/tedilabs/tfvault/main/install.sh | sh
```

The script installs the `tfvault` binary into `~/.local/bin` and runs
`tfvault install`, which symlinks it into `~/.terraform.d/plugins/` as
`terraform-credentials-tfvault` — the name Terraform discovers helpers
by. Or manually: download a release archive, verify `checksums.txt`,
put `tfvault` on your PATH and run `tfvault install`. From source:

```sh
go build ./cmd/tfvault
install -m 0755 tfvault ~/.local/bin/
tfvault install
```

## Quick start

Add to `~/.terraformrc`:

```hcl
credentials_helper "tfvault" {
  args = []
}
```

That's it — no config file needed. Tokens go into your OS keyring under
the service name `tfvault`:

```sh
terraform login app.terraform.io    # store a token
terraform logout app.terraform.io   # forget it
```

`tfvault status` shows whether the plugin link and `.terraformrc` are
wired up and which profile and backend requests resolve to.

The helper works for any Terraform-native service hostname, not just
Terraform Cloud.

## Multiple accounts on one machine

The core feature: different `.terraformrc` files can use different
credential sets via profiles. Define profiles in
`~/.config/tfvault/config.yaml`:

```yaml
default_profile: personal

profiles:
  personal:
    backend: keyring
    options:
      service: tfvault-personal
  customer-a:
    backend: keyring
    options:
      service: tfvault-customer-a
  customer-b:
    backend: pass
    options:
      binary: gopass
      prefix: customers/b/terraform
      store_dir: ~/.password-store-customer-b
  ci:
    backend: env
    options:
      prefix: CI_TF_TOKEN_
```

Create one `.terraformrc` per account:

```hcl
# ~/.terraformrc-customer-a
credentials_helper "tfvault" {
  args = ["--profile", "customer-a"]
}
```

Then select it per shell, per direnv, or per invocation:

```sh
export TF_CLI_CONFIG_FILE=~/.terraformrc-customer-a
terraform plan
```

The same hostname (e.g. `app.terraform.io`) resolves to different tokens
in different profiles because each profile points at its own storage
location.

## Configuration reference

Config file lookup order:

1. `--config <path>` (set via `args` in the `credentials_helper` block)
2. `$TFVAULT_CONFIG`
3. `$XDG_CONFIG_HOME/tfvault/config.yaml`, falling back to
   `~/.config/tfvault/config.yaml`

If no config file exists, the implicit `default` profile uses the
`keyring` backend with `service: tfvault`. Requesting any other named
profile without a config file is an error — a named profile implies
isolation you set up on purpose, so tfvault never falls back to shared
storage.

Each entry under `profiles` names exactly one `backend` and passes the
keys under `options` to it:

### `keyring`

```yaml
profiles:
  example:
    backend: keyring
    options:
      service: tfvault # keyring service name (default "tfvault")
```

Entries are stored as (service, hostname). On Linux this requires a
running Secret Service daemon (gnome-keyring, KWallet); on headless
machines use the `pass` backend instead.

### `pass`

```yaml
profiles:
  example:
    backend: pass
    options:
      binary: pass # or "gopass", or an absolute path (default "pass")
      prefix: terraform # entry path: <prefix>/<hostname> (default "terraform")
      store_dir: ~/.password-store # sets PASSWORD_STORE_DIR for per-profile stores (optional)
```

Tokens are exchanged with the child process via stdin/stdout only,
never argv. Both pass and gopass are supported and integration-tested.

### `op` (1Password)

```yaml
profiles:
  example:
    backend: op
    options:
      vault: Work # 1Password vault name (optional; default vault when omitted)
      account: my.1password.com # for multiple 1Password accounts (optional)
      prefix: tfvault/ # item title prefix: <prefix><hostname> (default "tfvault/")
      binary: op # or an absolute path (default "op")
```

Requires the [1Password CLI](https://developer.1password.com/docs/cli/)
(v2) with any of its auth methods: the desktop-app integration,
`OP_SERVICE_ACCOUNT_TOKEN`, or `op signin`. Entries are stored as
"API Credential" items tagged `tfvault`, and tokens are exchanged with
the `op` process via stdin/stdout only — never argv. Different profiles
can point at different vaults or accounts for per-client isolation.

### `env` (read-only)

```yaml
profiles:
  example:
    backend: env
    options:
      prefix: TF_TOKEN_ # default
```

Looks up `<prefix><encoded-hostname>` where `.` becomes `_` and `-`
becomes `__` (Terraform's native `TF_TOKEN_*` encoding), e.g.
`TF_TOKEN_app_terraform_io`. `terraform login` against an env profile
fails with a clear error since the backend cannot write.

Note: Terraform ≥ 1.2 reads `TF_TOKEN_*` variables natively without any
helper. The env backend is useful for the *prefix override* case
(`CUSTOMER_A_TF_TOKEN_*`) where profiles select among variable sets.

## Auxiliary commands

```sh
tfvault install                     # symlink the helper into ~/.terraform.d/plugins
tfvault status                      # plugin link, terraformrc and profile resolution
tfvault profiles                    # list profiles, default marked with *
tfvault --profile customer-b list   # hostnames with stored credentials
tfvault version
```

`install` refuses to overwrite anything at the link path that is not a
symlink (e.g. a binary copied by an old installer); pass `-f`/`--force`
to replace it.

`status` reads the Terraform CLI config (`$TF_CLI_CONFIG_FILE`, else
`~/.terraformrc`) and reports the `credentials_helper` registration,
explicit `credentials` blocks that bypass the helper, and the profile,
backend and stored hostnames the current setup resolves to.

`list` is supported by the `pass`, `op` and `env` backends; OS keyrings
cannot enumerate entries. Token values are never printed by any
auxiliary command.

## Caveats

- **Explicit `credentials` blocks win.** Terraform prefers a
  `credentials "<host>" { token = ... }` block in the CLI config over
  the helper. Remove such blocks for hosts the helper should manage.
- **The helper is asked about every Terraform-native service host.**
  Returning `{}` (no credentials) is normal for public registries like
  `registry.terraform.io`; anonymous access continues to work.
- **gopass** is CLI-compatible with pass and honors
  `PASSWORD_STORE_DIR`; both were verified against real stores.

## Security notes

- Tokens never appear in logs, argv, or child process environments.
- On protocol commands, stdout carries only protocol JSON; all
  diagnostics go to stderr.
- Hostnames are validated before being used as paths, env var names or
  keyring accounts (no path traversal / argv injection).
- Ambiguous backend errors on `get` fail with a nonzero exit instead of
  returning an empty `{}` that would mask a broken setup.
- A world-readable config file produces a warning (it holds no secrets,
  but paths and service names are better kept private).

## Development

```sh
go test ./...                       # unit + protocol compliance tests
go test -tags integration ./...    # real pass/gopass round trips (needs gpg)
go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
```

The Terraform credentials helper protocol is documented at
[developer.hashicorp.com/terraform/internals/credentials-helpers](https://developer.hashicorp.com/terraform/internals/credentials-helpers).

## License

[Apache License 2.0](LICENSE)
