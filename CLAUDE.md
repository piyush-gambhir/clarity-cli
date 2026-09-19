# Clarity CLI agent guide

Use `clarity --help` and `docs/commands.md` for current flags. See `clarity/SKILL.md` for operational guidance.

- Implementation: Go/Cobra in `cli-go/`; module `github.com/piyush-gambhir/clarity-cli/cli-go`.
- `make test`, `make vet`, `make build`, and `make docs` run from the repo root.
- Authentication is one Data Export token per Clarity project. No OAuth or account-wide token is implemented.
- Never add a background verification request: the Export API allows only 10 requests/project/day. Tests use fake transports and temporary configs.
- Preserve stdout as command data and stderr as diagnostics. All endpoints are remote reads, including the MCP backend POSTs.
- Keep request bodies aligned with the upstream source linked in README. In particular, recording sort values are numeric and dates appear at both top level and under filters.date.
- Do not turn empty analytics into an error or manufacture missing results. Explicit application errors should fail with a nonzero exit code.
- Document user-visible changes and regenerate command docs when flags change.
