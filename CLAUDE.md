# Clarity CLI agent guide

Use `clarity --help` and `docs/commands.md` for current flags. See `clarity/SKILL.md` for operational guidance.

- Implementation: Go/Cobra in `cli-go/`; module `github.com/piyush-gambhir/clarity-cli/cli-go`.
- `make test`, `make vet`, `make build`, and `make docs` run from the repo root.
- Authentication is one Data Export token per Clarity project. No OAuth or account-wide token is implemented.
- Never add a background verification request: the Export API allows only 10 requests/project/day. Tests use fake transports and temporary configs.
- The only background request is the update notice's GitHub release check (`cmd/update.go`, `internal/update`): release builds only, at most once a day (cached in `update-check.json` next to the config, failures too), interactive terminals only. It is skipped when stderr is not a terminal, `CI` is set, `CLARITY_NO_UPDATE_NOTIFIER`/`NO_UPDATE_NOTIFIER` is set, with `--quiet`, and for `update`/`version`/`completion`/`help`. It must not delay output, except that the run that made the day's lookup waits at most 1s for it after the command's output (a cached result never waits). `update --check` and `update` store their result in the same cache. Tests use local servers. Release lookups read the `github.com/.../releases/latest` redirect without following it; never use `api.github.com`, whose unauthenticated limit is 60 requests/hour per IP.
- `clarity update` verifies checksums and replaces the executable on macOS, Linux, and Windows (rename aside to `clarity.exe.old`). Keep it leaving the old binary working on any failure.
- Preserve stdout as command data and stderr as diagnostics. All endpoints are remote reads, including the MCP backend POSTs.
- Keep request bodies aligned with the upstream source linked in README. In particular, recording sort values are numeric and dates appear at both top level and under filters.date.
- Do not turn empty analytics into an error or manufacture missing results. Explicit application errors should fail with a nonzero exit code.
- Document user-visible changes and regenerate command docs when flags change.
