# Security

## Reporting a vulnerability

Please **don't** open a public issue. Use GitHub's
[private vulnerability reporting](https://github.com/wilmarvh/learnworlds-cli/security/advisories/new)
instead. You'll get a response within a few days.

## How `lw` handles credentials

- The client secret is read from stdin or a hidden prompt. It is never accepted as a flag.
- Secrets and cached access tokens are stored in `~/.config/learnworlds/credentials.json` with
  mode `0600`, inside a `0700` directory. They are not stored in the OS keychain.
- `LW_CLIENT_SECRET` / `LW_ACCESS_TOKEN` from the environment are used but never written to disk.
- `--verbose` logs method, URL and status only, never headers or bodies.
- `lw auth token` prints the access token on purpose, for scripts. Treat its output as a secret.
- `lw auth logout` deletes the stored secret and token for a profile. To fully revoke access,
  delete or rotate the API client in your school under **Settings → Developers → API**.

## Scope

An API client has admin-level access to your school's data. Give the CLI its own API client so you
can revoke it independently. Prefer a test school when you experiment.
