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

## Dependency maintenance

Prefer current stable releases, pinned to exact module versions and immutable GitHub Action commit SHAs. Check upstream stable releases when scaffolding or updating dependencies; do not assume another repository's pins are current. Exclude prereleases and review breaking major-version migrations explicitly.

Dependabot checks daily and groups minor/patch updates per ecosystem so coupled packages and CodeQL init/analyze updates are tested together. Major updates remain separate. Run `go get -u -t ./...` and `go mod tidy` inside `cli-go`, then run platform CI. Check the GoReleaser binary version explicitly in both CI and release workflows; Dependabot's GitHub Actions updater does not manage that action input. CI builds snapshot archives using the same GoReleaser version as releases.

To release, run `goreleaser check --config cli-go/.goreleaser.yaml` from the repository root, run CI, set `cli-go/VERSION`, and push a signed version tag. A local packaging check is `goreleaser release --snapshot --clean --skip=publish --config cli-go/.goreleaser.yaml`, also from the root. The release workflow builds binaries and checksums. `install.sh` and `clarity update` use that repository's release assets. Local development versions do not trigger any background update requests.
