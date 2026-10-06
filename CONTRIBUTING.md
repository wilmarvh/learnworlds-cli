# Contributing

Issues and pull requests are welcome.

## Development setup

You need Go 1.26+.

```bash
git clone https://github.com/wilmarvh/learnworlds-cli && cd learnworlds-cli
go build -o bin/lw ./cmd/lw
./bin/lw --help
```

Before opening a PR:

```bash
gofmt -l .      # no output
go vet ./...
go test ./...
```

The tests run against a fake LearnWorlds server, so they need no credentials and make no network calls.

## Testing against a real school

Use a **test or staging school**, not production. Most commands change real learner data.

```bash
./bin/lw auth login -P dev --school yourschool.learnworlds.com --client-id <id>
./bin/lw -P dev doctor
./bin/lw -P dev users enroll test@example.com some-course --dry-run   # inspect first
```

## Pull requests

- Keep PRs focused. One feature or fix per PR.
- New API endpoints are usually one row in `internal/cli/endpoints.go`. See [AGENTS.md](AGENTS.md)
  for the fields.
- Add or extend a test in `internal/cli/cli_test.go` for new behavior (pagination, body building,
  error mapping and so on).
- Update `README.md` and `skills/learnworlds/SKILL.md` when user-facing behavior changes.
- **Don't include real data**: no school URLs, client IDs, tokens, or learner names/emails in code,
  tests, issues, or screenshots. Redact `lw --verbose` output before you paste it.

## Commit messages

Use short imperative subjects: `Add seats bulk import`, `Fix pagination when totalPages is missing`.

## Reporting bugs

Include `lw --version`, the command you ran (redacted), the `--json` error output, and what you
expected. Run `lw doctor` first.
