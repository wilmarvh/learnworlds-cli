# AGENTS.md

Guidance for AI coding agents working **on** this repository. To *use* the CLI as an agent, read
[skills/learnworlds/SKILL.md](skills/learnworlds/SKILL.md) instead.

## What this is

`lw` is a Go CLI over the LearnWorlds public API v2 (https://www.learnworlds.dev). It uses one
dependency for commands ([cobra](https://github.com/spf13/cobra)) and one for hidden password
input (`golang.org/x/term`). Everything else is the standard library.

## Layout

```
cmd/lw/main.go            entrypoint
internal/cli/
  endpoints.go            the endpoint table: one row per API operation → one command
  commands.go             root command, global flags, turns table rows into cobra commands,
                          query/body building, summaries, breadcrumbs
  client.go               config + credentials files, OAuth token minting/caching,
                          HTTP with retry/backoff, pagination
  output.go               Error type + exit codes, JSON envelope, terminal tables
  admin.go                hand-written commands: auth, profile, doctor, api, commands, skill
  cli_test.go             end-to-end test against a fake LearnWorlds server
skills/learnworlds/SKILL.md   agent skill, embedded into the binary (skills/embed.go)
```

## Commands

```bash
go build -o bin/lw ./cmd/lw
go test ./...
go vet ./...
gofmt -l .        # must print nothing
```

Run all four before you finish. CI runs the same.

## Adding or changing an endpoint

1. Add or edit a row in `endpoints` in `internal/cli/endpoints.go`.
   - `Path` uses `{param}` placeholders, filled in order from positional args.
   - Extra positional args go into the JSON body via `ArgBody`.
   - `Query`: `"name"`, `"name:bool"`, `"name:time"` (accepts dates, sent as unix seconds).
     The flag name is the kebab-case form of the param.
   - `Body`: typed flags (`str`, `num`, `int`, `bool`, `list`). A non-empty `Def` is always sent.
   - `Paged` adds `--page/--all`, `Limit` adds `--limit` (`items_per_page`).
   - `Cols` picks table columns (dotted paths allowed). `Next` adds breadcrumbs, and `<arg>`
     placeholders get the user's actual args.
2. If the command needs logic the table can't express, add a small hook in `commands.go`, as
   `users tag` does. Don't fork the generic path.
3. Update `skills/learnworlds/SKILL.md` if agents should know about it.
4. `go test ./...`. The test asserts that every table row shows up in `lw commands`.

Source of truth for the API is the OpenAPI spec behind learnworlds.dev (Stoplight project
`learnworlds/api`, file `reference/API.yaml`).

## Conventions

- Commands are `lw <resource> <action>`. Resource groups never run on their own; they only print help.
- Return `*Error` with a stable `Code` from `RunE`. Never print errors and `os.Exit` yourself
  (except `doctor`, which exits 1 after reporting failing checks).
- All success output goes through `render(...)` / `ok(...)` so `--json` and `--quiet` keep working.
- Only GET requests are retried after network errors. Writes are retried on 429 only, because a
  429 means the request was not processed. Don't widen this: a retried POST can enroll someone twice.
- Match the surrounding style: small functions, few comments, stdlib first.

## Security rules (public repo)

- Never commit real school URLs, client IDs, secrets, tokens, or learner data (names, emails).
  Use `example.com`, `yourschool.learnworlds.com`, `jane@example.com`, `intro-course`.
- Secrets are read from stdin or the TTY, never from flags (flags leak via shell history and `ps`).
- Never log `Authorization` headers or the client secret, even with `--verbose`.
- `credentials.json` must stay mode `0600` and the config directory `0700`.
- Test fixtures use obviously fake values only.
