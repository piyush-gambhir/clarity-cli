---
name: clarity
description: Query Microsoft Clarity analytics and sampled session recordings using the clarity CLI, including exports, recording filters, and saved project profiles.
---

# Clarity

Use the installed `clarity` binary; check subcommand `--help` for flags. The full reference is [commands](../docs/commands.md), with auth and filter examples in [README](../README.md).

- Prefer `-o json --no-input` for machine consumption. Diagnostics are on stderr.
- Select the intended project with `--profile`. A set `CLARITY_API_TOKEN` overrides that profile's saved token; `auth status` shows the effective source without contacting Microsoft.
- `export` is for aggregate reports over the previous 1–3 days. It allows 10 requests/project/day, three dimensions, and 1,000 rows without pagination. Plan dimensions before calling; do not verify auth repeatedly or retry 429s in a loop.
- `analytics query` accepts a focused natural-language question with explicit dates and `--timezone` (default UTC). It is interpreted by Microsoft's backend. Preserve uncertainty when responses are empty or unexpectedly zero.
- `recordings list` returns a sample, at most 250 sessions. Do not represent that sample as all sessions or infer population rates from it. Use `recordings filters -o json` for supported fields; use `--filters-file` for advanced filters.
- `docs search` makes an authenticated request for Clarity documentation snippets.
- All remote commands read data. `auth login/logout/use` change local credentials/configuration, and `update` changes the executable. Stay within the user's requested project and action.
- Setup requires a user-provided project token. Use hidden login input, stdin, or environment secrets; do not invent credentials or place them in reports.
