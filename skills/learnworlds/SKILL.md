---
name: learnworlds
description: Manage a LearnWorlds school with the `lw` CLI — users, enrollments, courses, progress, payments, promotions/coupons, affiliates, certificates, seats, user groups, roles, community and event logs. Use for ANY LearnWorlds question or action (e.g. "enroll Jane in the intro course", "who bought X this month", "reset Sam's progress").
---

# LearnWorlds CLI (`lw`)

`lw` wraps the full LearnWorlds API v2. Commands are `lw <resource> <action> [args] [flags]`.

## Rules for agents

- Pipe or pass `--json`: every command returns `{"ok", "data", "summary", "meta", "breadcrumbs"}`. `--quiet` returns only `data`.
- Errors return `{"ok": false, "error", "code", "retryable", "hint"}` with a non-zero exit. Codes: `usage`(2), `auth`/`forbidden`/`config`(3), `not_found`(4), `rate_limited`(5), `validation`, `api_error`, `network`(1).
- Follow `breadcrumbs` for the next command. Lists are paged: use `--page N` or `--all`.
- Users can be referenced by **id or email** everywhere (`<user>`).
- Course ids are slugs (e.g. `intro-course`). Resolve names with `lw courses list --all --json` first; never guess an id.
- Use `--dry-run` to preview any write. Confirm with the human before deletes, unenrollments, suspensions, progress resets or certificate revokes.
- Rate limit is 30 requests / 10 s; the CLI retries 429s itself. Prefer `--all` over looping pages yourself.
- `lw commands --json` lists every command, its flags and the API endpoint behind it.

## Common tasks

```bash
lw doctor                                                     # verify setup
lw users show jane@example.com                                # user by email
lw users list --tags vip --status paying --all                # filter users
lw users list -Q cf_city=Athens                               # custom field filter
lw courses list --all                                         # all courses (ids + titles)
lw users enroll jane@example.com intro-course                 # free course enrollment
lw users enroll jane@example.com pro-bundle --type bundle --price 0 --justification "comp" --notify
lw users unenroll jane@example.com intro-course
lw users courses jane@example.com                             # their enrollments
lw users course-progress jane@example.com intro-course
lw users complete-course jane@example.com intro-course        # whole course
lw users reset-course jane@example.com intro-course --units unit1,unit2
lw users create --email new@example.com --username "New Person" --tags trainer
lw users tag jane@example.com --add vip,2026-cohort
lw users suspend jane@example.com
lw courses users intro-course --all                           # course roster
lw payments list --created-after 2026-09-01 --all             # payments since a date
lw logs list --activity enrollment --user-id jane@example.com
lw promotions add-coupon <promotion> --code SPRING --quantity 50 --expires 2026-12-31
lw certificates list --user-id <user-id>
```

## Resources

courses, bundles, plans, subscriptions, installments, events, users, payments, leads, promotions,
affiliates, certificates, logs, assessments, forms, async, seats, groups, roles,
community (collections | spaces | posts). Run `lw <resource> --help` for actions.

## Request bodies

Typed flags cover common fields. For anything else:
- `-f key=value` string field, `-F key=value` JSON-typed field (`-F products='[{"id":"x","type":"course"}]'`)
- `--data '{...}'`, `--data @body.json`, or `--data -` (stdin)
- Raw escape hatch: `lw api /v2/<path> -X METHOD ...`

## Setup

```bash
op read "op://Vault/LearnWorlds/secret" | lw auth login --school myschool.learnworlds.com --client-id <id> -P myschool
```
Env overrides: `LW_PROFILE`, `LW_SCHOOL_URL`, `LW_CLIENT_ID`, `LW_CLIENT_SECRET`, `LW_ACCESS_TOKEN`.
