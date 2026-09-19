# Clarity CLI command reference

Generated from the command tree. Run `make docs` to refresh.

Global flags apply to every command.

```text
      --no-input           Disable interactive prompts
  -o, --output string      Output format: table, json, yaml (default "table")
      --profile string     Named project profile (or CLARITY_PROFILE)
  -q, --quiet              Suppress informational stderr output
      --read-only          Also block local credential changes and self-update
      --timeout duration   HTTP request timeout (default 30s)
      --token string       API token override (prefer CLARITY_API_TOKEN or auth login)
  -v, --verbose            Log request method, URL, and status to stderr (no tokens/bodies)
```

## clarity

Microsoft Clarity analytics and session recordings from your terminal

Read Microsoft Clarity data using project API tokens.
Export dashboard metrics, query analytics, find session recordings, and search documentation.
All remote operations are read-only. Profiles store one token per project.

```text
clarity
```

## clarity analytics

Query analytics through the endpoints used by Microsoft's MCP server

```text
clarity analytics
```

## clarity analytics query

Ask a focused analytics question with an explicit time range

```text
clarity analytics query QUESTION [flags]
```

```bash
  clarity analytics query 'Top pages by rage clicks in the last 3 days' --timezone Asia/Kolkata -o json
```

```text
      --timezone string   IANA timezone used to interpret the query (default "UTC")
```

## clarity auth

Manage project tokens and named profiles

```text
clarity auth
```

## clarity auth list

List saved project profiles, without tokens

```text
clarity auth list
```

## clarity auth login

Save a project token (hidden prompt, environment, or stdin)

Save a token generated in Clarity → Settings → Data Export.
This saves locally without making an API request unless --verify is set.
--verify consumes one Export API request; that API allows 10 per project per day.
An existing profile's token is replaced, and the profile becomes current.

```text
clarity auth login [flags]
```

```bash
  clarity auth login --profile website
  clarity auth login --profile website --token-stdin < token.txt
  CLARITY_API_TOKEN=... clarity auth login --no-input --profile website
```

```text
      --token-stdin   Read token from stdin
      --verify        Verify remotely before saving (uses one export request)
```

## clarity auth logout

Remove a saved token locally; does not revoke it at Microsoft

```text
clarity auth logout
```

## clarity auth status

Show credential source without revealing the token

```text
clarity auth status [flags]
```

```text
      --verify   Check remote access (uses one export request)
```

## clarity auth use

Set the default project profile

```text
clarity auth use NAME
```

## clarity completion

Generate shell completion script

```text
clarity completion [bash|zsh|fish|powershell]
```

## clarity config

Inspect configuration and select profiles

```text
clarity config
```

## clarity config list-profiles

List saved project profiles, without tokens

```text
clarity config list-profiles
```

## clarity config show

Show credential source without revealing the token

```text
clarity config show [flags]
```

```text
      --verify   Check remote access (uses one export request)
```

## clarity config use-profile

Set the default project profile

```text
clarity config use-profile NAME
```

## clarity docs

Search Microsoft Clarity documentation

```text
clarity docs
```

## clarity docs search

Retrieve documentation snippets (requires a project token)

```text
clarity docs search QUESTION
```

## clarity export

Export dashboard metrics for the previous 1–3 days

Download aggregate dashboard data from the documented Export API.
Limits: 10 requests/project/day; previous 1–3 days; up to 3 dimensions;
1,000 rows without pagination. Times are UTC. No automatic retries.

```text
clarity export [flags]
```

```bash
  clarity export --days 3 --dimension Device --dimension Country/Region -o json
```

```text
      --days int            Lookback window in days (1, 2, or 3) (default 1)
      --dimension strings   Group by dimension; repeat up to 3 times (see export dimensions)
```

## clarity export dimensions

List supported export dimensions (offline)

```text
clarity export dimensions
```

## clarity login

Save a project token (hidden prompt, environment, or stdin)

Save a token generated in Clarity → Settings → Data Export.
This saves locally without making an API request unless --verify is set.
--verify consumes one Export API request; that API allows 10 per project per day.
An existing profile's token is replaced, and the profile becomes current.

```text
clarity login [flags]
```

```bash
  clarity auth login --profile website
  clarity auth login --profile website --token-stdin < token.txt
  CLARITY_API_TOKEN=... clarity auth login --no-input --profile website
```

```text
      --token-stdin   Read token from stdin
      --verify        Verify remotely before saving (uses one export request)
```

## clarity recordings

Find sampled session recordings and interaction timelines

```text
clarity recordings
```

## clarity recordings filters

List supported JSON filter fields and enum values (offline)

```text
clarity recordings filters
```

## clarity recordings list

List up to 250 sampled recordings

List sampled recordings using Microsoft's MCP backend. This is not a full recording export.
Defaults to the previous 2 days. Dates accept RFC3339 or YYYY-MM-DD (midnight UTC).
--filters-file accepts a JSON filters object; explicit flags override matching fields.
Use recordings filters for the accepted schema. Range objects require min and max (number or null).

```text
clarity recordings list [flags]
```

```bash
  clarity recordings list --days 7 --device Mobile --rage-clicks -o json
  clarity recordings list --start 2026-09-01 --end 2026-09-08 --count 25
  clarity recordings list --filters-file filters.json -o yaml
```

```text
      --browser stringArray            Filter by browser (repeat flag for multiple values)
      --campaign stringArray           Filter by campaign (repeat flag for multiple values)
      --channel stringArray            Filter by channel (repeat flag for multiple values)
      --city stringArray               Filter by city (repeat flag for multiple values)
      --count int                      Number of sampled recordings (1–250) (default 100)
      --country stringArray            Filter by country (repeat flag for multiple values)
      --days int                       Lookback days; used when start is not specified (default 2)
      --dead-clicks                    Filter sessions by dead-clicks; explicit =false is supported
      --device stringArray             Filter by device (repeat flag for multiple values)
      --end string                     End date/time sent to Clarity (default: now)
      --event stringArray              Filter by event (repeat flag for multiple values)
      --excessive-scroll               Filter sessions by excessive-scroll; explicit =false is supported
      --filters-file string            JSON filters file, or - for stdin
      --javascript-error stringArray   Filter by javascript-error (repeat flag for multiple values)
      --medium stringArray             Filter by medium (repeat flag for multiple values)
      --os stringArray                 Filter by os (repeat flag for multiple values)
      --quick-backs                    Filter sessions by quick-backs; explicit =false is supported
      --rage-clicks                    Filter sessions by rage-clicks; explicit =false is supported
      --sort string                    Sort: SessionStart_DESC, SessionStart_ASC, SessionDuration_ASC, SessionDuration_DESC, SessionClickCount_ASC, SessionClickCount_DESC, PageCount_ASC, PageCount_DESC (default "SessionStart_DESC")
      --source stringArray             Filter by source (repeat flag for multiple values)
      --start string                   Start date/time, inclusive interval boundary sent to Clarity
      --state stringArray              Filter by state (repeat flag for multiple values)
      --url string                     Match a substring of a visited URL
```

## clarity status

Show credential source without revealing the token

```text
clarity status [flags]
```

```text
      --verify   Check remote access (uses one export request)
```

## clarity update

Install the latest GitHub release after SHA-256 verification

```text
clarity update [flags]
```

```text
      --check   Only check the latest published release
```

## clarity version

Print build information

```text
clarity version
```
