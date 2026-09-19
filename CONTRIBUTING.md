# Contributing

Use Go 1.26+ (toolchain Go 1.27.1). From the repository root:

```bash
make test
make vet
make build
make docs
```

Run `gofmt -w .` within `cli-go/` after Go changes. HTTP tests must use fake transports or local test servers and must not contact Clarity. Configuration tests use temporary directories. Never commit project tokens or captured customer recordings.

Add endpoint contracts in `internal/client`, command composition in `cmd`, and output logic in `internal/output`. Keep local input validation ahead of credential resolution/network requests. The published Export contract and the upstream MCP implementation are distinct compatibility surfaces; avoid assuming they share quotas or date limits.

Submit changes through a pull request. Commits must be signed. The default branch requires linear history, resolved review threads, and passing Go CI and CodeQL checks; squash or rebase merges are supported.

To release, validate `cli-go/.goreleaser.yaml` with GoReleaser v2, run CI, set `cli-go/VERSION`, and push a signed version tag. The release workflow builds binaries and checksums. `install.sh` and `clarity update` use that repository's release assets. Local development versions do not trigger any background update requests.
