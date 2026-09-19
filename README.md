# Microsoft Clarity CLI

A Go command-line interface for Microsoft Clarity: export dashboard metrics, query analytics, find session recordings, and search documentation.

Designed for people and coding agents. Named project profiles, table/JSON/YAML output, shell completion, and a single cross-platform binary. Independent project; not affiliated with Microsoft.

[![CI](https://github.com/piyush-gambhir/clarity-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/piyush-gambhir/clarity-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/piyush-gambhir/clarity-cli)](https://github.com/piyush-gambhir/clarity-cli/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Install a release

```bash
curl -fsSL https://raw.githubusercontent.com/piyush-gambhir/clarity-cli/main/install.sh | sh
```

Installs to `~/.local/bin` by default. Override with `INSTALL_DIR` or pin a release with `VERSION`:

```bash
curl -fsSL https://raw.githubusercontent.com/piyush-gambhir/clarity-cli/main/install.sh | VERSION=v0.1.0 sh
```

Prebuilt macOS/Linux/Windows binaries and checksums are on the [releases page](https://github.com/piyush-gambhir/clarity-cli/releases).

## Install from source

Requires Go 1.26+; this project selects Go 1.27.1, matching the other CLIs in the suite.

```bash
git clone https://github.com/piyush-gambhir/clarity-cli.git
cd clarity-cli/cli-go
make build                 # bin/clarity
make install               # $(go env GOPATH)/bin/clarity
# Or choose the destination:
make install INSTALL_DIR="$HOME/.local/bin"
```

From a checkout, you can also use the standalone release installer:

```bash
sh install.sh
# Optional: VERSION=v0.1.0 INSTALL_DIR="$HOME/.local/bin" sh install.sh
```

The installer supports macOS/Linux amd64 and arm64 and verifies SHA-256 checksums. Windows release archives contain `clarity.exe` (amd64/arm64). On Windows, replace the executable manually to update.

## Quick start

An admin generates a token in **Clarity → project → Settings → Data Export → Generate new API token**. Each project needs its own token; there is no documented account-wide API login.

```bash
# Hidden token prompt; saves a local profile, no API request
clarity auth login --profile my-website

# Inspect local configuration without spending API quota
clarity auth status

# Aggregate metrics from the documented REST API
clarity export --days 3 --dimension Device --dimension Country/Region -o json

# Natural-language query through Microsoft's MCP backend
clarity analytics query 'Top pages by rage clicks in the last 3 days' \
  --timezone Asia/Kolkata -o json

# Sample session recordings, not a complete recording export
clarity recordings list --days 7 --device Mobile --rage-clicks --count 25 -o json

clarity docs search 'How do I track custom events?' -o yaml
```

No MCP server, Node.js process, or LLM API key is required. The CLI calls the HTTPS endpoints used by Microsoft's official MCP server directly. Natural-language interpretation happens at Microsoft's backend.

## Commands

| Command | Purpose |
| --- | --- |
| `auth login`, `login` | Save a token from a hidden prompt, environment, or stdin |
| `auth status`, `status` | Inspect credential source; optional `--verify` checks remote access |
| `auth list`, `auth use NAME`, `auth logout` | List, select, and remove local profiles |
| `config show`, `config list-profiles`, `config use-profile NAME` | Configuration aliases consistent with the other CLIs |
| `export` | Aggregate dashboard data for 1–3 days |
| `export dimensions` | List supported dimensions offline |
| `analytics query QUESTION` | Ask a time-scoped analytics question |
| `recordings list` (`sessions list`) | Sample recordings by dates, device, URLs, behavior, and more |
| `recordings filters` | Inspect the JSON filter schema offline |
| `docs search QUESTION` | Search Clarity's documentation |
| `completion bash\|zsh\|fish\|powershell` | Generate shell completion |
| `version` | Version, commit, and build date |
| `update [--check]` | Check/install a published release |

The complete [command and flag reference](docs/commands.md) is generated from the command tree with `make docs`.

## Authentication and profiles

```bash
clarity auth login --profile production
clarity auth login --profile staging --token-stdin < /path/to/token-file
clarity auth list
clarity auth use production
clarity export --profile staging -o json
clarity auth logout --profile staging
```

Credentials resolve in order: `--token` → `CLARITY_API_TOKEN` → selected saved profile. Profile selection is `--profile` → `CLARITY_PROFILE` → saved current profile. **An environment token overrides a selected profile**; unset it when switching saved project tokens.

Config lives at `~/.config/clarity-cli/config.yaml`, respecting `XDG_CONFIG_HOME`. `CLARITY_CONFIG` overrides the full path. Local writes are locked and atomic. Unix credentials are stored in an owner-readable/writable file (0600); they are plaintext, not encrypted. Windows uses the user's filesystem ACLs. Tokens never appear in `auth status`, profile lists, or verbose request logs.

`auth login --verify` verifies before saving. `auth status --verify` checks the resolved token. Each verification consumes one Export API request; ordinary login/status are local only. Login replaces the selected profile and makes it current. Logout removes only the local saved token; revoke or rotate it in Clarity as needed. There is no automatic token refresh.

For CI, set `CLARITY_API_TOKEN` through your secret manager and use `--no-input`; saving a profile is optional. The CLI has no background update or authentication calls.

| Environment variable | Meaning |
| --- | --- |
| `CLARITY_API_TOKEN` | Project token; overrides saved profile credentials |
| `CLARITY_PROFILE` | Default profile selection |
| `CLARITY_CONFIG` | Full config file path |
| `XDG_CONFIG_HOME` | Base config directory |
| `CLARITY_NO_INPUT` | Disable prompts (`1` or `true`) |
| `CLARITY_QUIET` | Suppress informational stderr messages |
| `CLARITY_VERBOSE` | Log method/URL/status, not headers or request bodies |
| `CLARITY_READ_ONLY` | Block local credential modifications and self-update |

## Output and automation

`-o table` is the default; `-o json` and `-o yaml` preserve the returned data, including large numeric identifiers. Table output presents export metrics in separate tables and nests variable backend objects as JSON cells. Use JSON for detailed recordings/analytics responses. No response pagination or hidden aggregation is performed.

Data goes to stdout; diagnostics/errors go to stderr. JSON/YAML modes also format errors structurally. Exit status is 0 for success and 1 for failure. `--timeout` defaults to 30s. Interrupts cancel API requests. All remote operations are read-only, even when the HTTP method is POST. `--read-only` additionally blocks local profile changes and binary updates; `update --check` remains available.

## Advanced recording filters

```json
{
  "deviceType": ["Mobile"],
  "visitedUrls": [{"url": "/checkout", "operator": "contains"}],
  "javascriptErrors": [""],
  "sessionDuration": {"min": 1, "max": null},
  "scrollDepth": {"min": 50, "max": 100}
}
```

```bash
clarity recordings list --filters-file filters.json \
  --start 2026-09-01 --end 2026-09-08 --count 50 \
  --sort SessionDuration_DESC -o json
clarity recordings filters -o json
```

Dates accept RFC3339 or `YYYY-MM-DD` (midnight UTC, including the end date). The backend receives both top-level start/end and matching `filters.date`. Without explicit dates, the interval ends now and starts two days earlier. `--days` can change this interval but cannot combine with an explicit start or a file-provided date. Explicit flags override matching file filters. Use `--rage-clicks=false` to explicitly request false. Range filters need both `min` and `max`; either may be null. Supported ranges are nonnegative; percentages must be 0–100. Device values use `PC`, not `Desktop`.

The maximum sample size is 250. Date lookbacks do not extend Clarity's retention or guarantee data availability. These commands return metadata/links/timelines exposed by the backend, not downloadable recording videos or heatmap images.

## API coverage and limits

| Surface | Endpoint | Notes |
| --- | --- | --- |
| Export | `GET https://www.clarity.ms/export-data/api/v1/project-live-insights` | 1–3 days; 10 requests/project/day; max 3 dimensions; 1,000 rows; no pagination |
| Analytics | `POST https://clarity.microsoft.com/mcp/dashboard/query` | Natural-language query plus IANA timezone |
| Recordings | `POST https://clarity.microsoft.com/mcp/recordings/sample` | Dates, filters, numeric sort, count (1–250) |
| Documentation | `POST https://clarity.microsoft.com/mcp/documentation/query` | A focused documentation question |

The MCP-backed endpoints are taken from Microsoft's open-source server and have no separately versioned public REST contract. Their quotas are not assumed to match Export API quotas. The CLI makes no automatic retries, reports 429/Retry-After, and rejects explicit backend error responses even if HTTP returned 200. A successful empty/zero response is preserved; verify unexpected results against the dashboard.

Project/team administration and browser tracking APIs are outside this read-only CLI's scope. Browser APIs must run in your instrumented website, not a terminal.

Upstream references checked 2026-09-20:

- [Data Export API](https://learn.microsoft.com/en-us/clarity/setup-and-installation/clarity-data-export-api)
- [MCP endpoints](https://github.com/microsoft/clarity-mcp-server/blob/main/src/constants.ts)
- [MCP request bodies](https://github.com/microsoft/clarity-mcp-server/blob/main/src/tools.ts)
- [MCP filters and sort values](https://github.com/microsoft/clarity-mcp-server/blob/main/src/types.ts)

## Development and releases

```bash
make build
make test       # race-enabled tests; mocked HTTP, no Clarity token required
make vet
make docs       # refresh docs/commands.md from actual flags
```

Layout follows the CLI suite: Go implementation in `cli-go/`, user docs in `docs/`, agent skill in `clarity/`, CI/release workflows in `.github/`. Release tags `v*` trigger GoReleaser to produce macOS/Linux/Windows binaries and checksums. Run CI before tagging. No repository, release, remote data, or deployment is created by building locally.

See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), [CLAUDE.md](CLAUDE.md), and [the agent skill](clarity/SKILL.md). MIT licensed.
