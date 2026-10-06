# lw: LearnWorlds CLI

`lw` is a command-line interface for the [LearnWorlds API](https://www.learnworlds.dev). It manages
users, enrollments, courses, progress, payments, coupons, affiliates and more from your terminal,
your scripts or an AI agent.

Its design follows [basecamp-cli](https://github.com/basecamp/basecamp-cli): resource/action
commands, a JSON envelope with breadcrumbs, profiles, `doctor`, and a bundled agent skill.

> Unofficial project, not affiliated with or endorsed by LearnWorlds.

- **Full API v2 coverage:** every public endpoint is a command (`lw commands` lists them)
- **Readable and scriptable:** tables in a terminal, JSON when piped
- **Multiple schools:** named profiles, token caching and refresh
- **Polite:** paginates for you (`--all`) and retries the 30 req/10 s rate limit
- **Agent-ready:** stable error codes, `--dry-run`, `lw commands --json`, Claude skill

## Install

Requires Go 1.26+.

```bash
go install github.com/wilmarvh/learnworlds-cli/cmd/lw@latest
```

Or build from source:

```bash
git clone https://github.com/wilmarvh/learnworlds-cli && cd learnworlds-cli
go build -o ~/bin/lw ./cmd/lw
```

## Setup

The API is available on LearnWorlds **Learning Center** plans and above. In your school go to
**Settings → Developers → API** and create a client. Note the client ID and secret.

```bash
lw auth login --school yourschool.learnworlds.com --client-id <client-id>
# prompts for the client secret (input hidden)

lw doctor        # checks config, token and API access
```

`lw` reads the secret from stdin when you pipe it, so you can use a password manager:

```bash
op read "op://Private/LearnWorlds/secret" | lw auth login --school yourschool.learnworlds.com --client-id <id>
```

### Multiple schools

```bash
lw auth login -P staging --school staging.example.com --client-id <id>
lw -P staging courses list
lw profile list
lw profile use staging
```

### Configuration

| Location | Contents |
|---|---|
| `~/.config/learnworlds/config.json` | profiles: school URL, client ID, default profile |
| `~/.config/learnworlds/credentials.json` | client secret and cached access token (mode `0600`) |

`$XDG_CONFIG_HOME` and `$LW_CONFIG_DIR` move this directory. Environment variables override the stored profile:

| Variable | |
|---|---|
| `LW_PROFILE` | profile to use |
| `LW_SCHOOL_URL` | school URL |
| `LW_CLIENT_ID` | API client ID |
| `LW_CLIENT_SECRET` | API client secret (not persisted) |
| `LW_ACCESS_TOKEN` | use this token as-is, skip minting |

## Usage

```bash
lw courses list --all
lw users show jane@example.com                       # users accept id or email
lw users list --tags vip --status paying --all
lw users list -Q cf_city=Athens                      # custom user field filter
lw users enroll jane@example.com intro-course        # free course enrollment
lw users enroll jane@example.com pro-bundle --type bundle --notify
lw users unenroll jane@example.com intro-course
lw users course-progress jane@example.com intro-course
lw users tag jane@example.com --add vip,cohort-2026
lw courses users intro-course --all -q | jq -r '.[].email'
lw payments list --created-after 2026-01-01 --all
lw promotions add-coupon <promotion-id> --code SPRING --quantity 50 --expires 2026-12-31
lw api /v2/users/jane@example.com                    # raw request to any endpoint
```

Run `lw --help`, `lw <resource> --help`, or `lw commands` to explore.

### Resources

`courses` · `bundles` · `plans` · `subscriptions` · `installments` · `events` · `users` ·
`payments` · `leads` · `promotions` · `affiliates` · `certificates` · `logs` · `assessments` ·
`forms` · `async` · `seats` · `groups` · `roles` · `community collections|spaces|posts`

### Output

```bash
lw users list            # table in a terminal, JSON envelope when piped
lw users list --json     # JSON envelope
lw users list --quiet    # bare data
```

```json
{
  "ok": true,
  "data": [ ... ],
  "summary": "50 users (page 1/3, 142 total)",
  "meta": { "page": 1, "totalPages": 3, "totalItems": 142, "itemsPerPage": 50 },
  "breadcrumbs": [
    { "action": "show", "cmd": "lw users show <user>" },
    { "action": "next page", "cmd": "lw users list --page 2" }
  ]
}
```

Errors use the same envelope with `ok: false`:

```json
{ "ok": false, "error": "Course not found (HTTP 404)", "code": "not_found", "retryable": false }
```

| Exit | Code |
|---|---|
| 0 | success |
| 1 | `api_error`, `network`, `validation` |
| 2 | `usage` |
| 3 | `auth`, `forbidden`, `config` |
| 4 | `not_found` |
| 5 | `rate_limited` (after retries) |

### Writes

Typed flags cover the common fields (`lw users enroll --help`). For anything else:

```bash
-f key=value          # string field
-F key=value          # JSON-typed field: numbers, true/false, arrays, objects, @file
--data '{"a":1}'      # whole body; also --data @body.json or --data - (stdin)
--dry-run             # show the request without sending it
```

## AI agents

```bash
lw skill install       # installs SKILL.md to ~/.claude/skills/learnworlds
lw skill               # print it (point other agents at skills/learnworlds/SKILL.md)
lw commands --json     # machine-readable catalog of commands, flags and endpoints
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Agents working on this repo: see [AGENTS.md](AGENTS.md).
Security issues: see [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
