# Security Policy

tfvault handles Terraform API tokens, so security reports get priority
attention.

## Reporting a vulnerability

Please do **not** open a public issue for security problems.

Use GitHub's private vulnerability reporting instead: go to the
repository's **Security** tab and click **Report a vulnerability**
([direct link](https://github.com/tedilabs/tfvault/security/advisories/new)).
You will get an initial response within a week; fixes for confirmed
issues are released as fast as severity warrants and credited to the
reporter unless you prefer otherwise.

Include what you can of: affected version (`tfvault version`), backend
in use, platform, reproduction steps, and impact.

## Supported versions

Only the latest release receives security fixes. There is no backport
policy; upgrading is always the fix path.

## Scope

Reports especially welcome:

- Token exposure through argv, environment of child processes, logs, or
  auxiliary command output (the project's core guarantees — see
  ["Security notes" in the README](README.md#security-notes))
- Injection through hostnames or configuration values (path traversal,
  argv or environment-variable injection)
- Protocol behavior that masks a broken backend as "no credentials"
  (fail-open where fail-closed is promised)
- Supply-chain issues in the release pipeline or install script

Known limitations documented in the README's Security notes (for
example, macOS keychain items being readable by other same-user
processes via the `security` CLI, tracked in
[#14](https://github.com/tedilabs/tfvault/issues/14)) are accepted
risks rather than vulnerabilities, but reports that worsen them are
still in scope.
