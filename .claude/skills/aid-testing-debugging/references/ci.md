# CI

`.github/workflows/ci.yml` runs on a push to `main` and on a pull request into
`main`; a push to any other branch runs nothing. It has four jobs, which run in
parallel:

| Job | Runner | What it runs |
|---|---|---|
| `test` | ubuntu-latest | builds `aid` into the runner's temp directory, then the full suite with `AID_TEST_BINARY` pointing at that binary, so the snapshot and conformance suites test it instead of building their own |
| `build` | ubuntu-latest and macos-14 | builds `aid`, runs `aid --version` and checks that `aid testdata/python/01_basic/source.py --stdout` prints `is_valid_email`; no tests |
| `checks` | ubuntu-latest | `gofmt -s -l` over the tracked `cmd/` and `internal/` Go files except `testdata` and `internal/parser/grammars` (any output fails); `go vet` on the same package list as the suite; `golangci-lint` v2.12.2 via `golangci-lint-action` with `only-new-issues: true` (config in `.golangci.yml`, runs with `CGO_ENABLED=1 CGO_CFLAGS=-w`); `go mod tidy` must leave `go.mod` and `go.sum` unchanged; `scripts/version-drift.sh`; `scripts/tests/install_sh_test.sh`; `scripts/check-cgo.sh`; `scripts/tests/check_skill_anchors_test.sh` and `scripts/check-skill-anchors.sh`; `scripts/tests/readme_links_test.sh` |
| `mcp` | ubuntu-latest, Node 24 | `npm ci` and `npm test` in `mcp-npm/` |

All jobs check out with `submodules: recursive` except `mcp`; the Go jobs use the
Go version set in the workflow's `env`.

Before opening a pull request, the local equivalents (from the repository root,
with `CGO_ENABLED=1 CGO_CFLAGS=-w` exported):

```bash
make fmt-check
go test -count=1 $(go list -e ./... | grep -vE '/tools/|/test_project/|/cmd/aid-validate')
go vet $(go list -e ./... | grep -vE '/tools/|/test_project/|/cmd/aid-validate')
make lint    # needs golangci-lint on PATH (make setup installs v2.12.2)
sh scripts/version-drift.sh
sh scripts/tests/install_sh_test.sh
sh scripts/check-cgo.sh
sh scripts/tests/check_skill_anchors_test.sh && sh scripts/check-skill-anchors.sh
sh scripts/tests/readme_links_test.sh
(cd mcp-npm && npm test)
```

`make setup` and `make dev-init` install the same golangci-lint version as CI
with `go install`. v2.12.2 is the newest v2 release whose `go.mod` asks for no
more than Go 1.25, the version in this project's `go.mod`; v2.13.0 and later
need Go 1.26, so `go install` would fail with `GOTOOLCHAIN=local` on Go 1.25.
Keep the three pins (`.github/workflows/ci.yml`, `Makefile` `setup` and
`dev-init`) equal.

`go mod tidy` is left out above: it reads every Go package in the working
tree, including directories that git ignores, so in a checkout that has such
packages it fails or differs from CI.

## Release workflow

`.github/workflows/release.yml` (on a `v*` tag or a manual run) has its own `test`
job: the same build and full suite as the CI `test` job. The `build` job needs it,
so no release binary is built from a commit whose suite fails. Its `publish-npm`
job runs `npm ci` and `npm test` in `mcp-npm/` again before publishing. The
release workflow does not run the `checks` job's steps.
