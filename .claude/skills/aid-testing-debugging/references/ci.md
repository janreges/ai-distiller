# CI

`.github/workflows/ci.yml` runs on a push to `main` and on a pull request into
`main`; a push to any other branch runs nothing. It has four jobs, which run in
parallel:

| Job | Runner | What it runs |
|---|---|---|
| `test` | ubuntu-latest | builds `aid` into the runner's temp directory, then the full suite with `AID_TEST_BINARY` pointing at that binary, so the snapshot and conformance suites test it instead of building their own |
| `build` | ubuntu-latest and macos-14 | builds `aid`, runs `aid --version` and checks that `aid testdata/python/01_basic/source.py --stdout` prints `is_valid_email`; no tests |
| `checks` | ubuntu-latest | `go vet` on the same package list as the suite; `go mod tidy` must leave `go.mod` and `go.sum` unchanged; `scripts/version-drift.sh`; `scripts/tests/install_sh_test.sh`; `scripts/check-cgo.sh`; `scripts/tests/check_skill_anchors_test.sh` and `scripts/check-skill-anchors.sh` |
| `mcp` | ubuntu-latest, Node 20 | `npm ci` and `npm test` in `mcp-npm/` |

All jobs check out with `submodules: recursive` except `mcp`; the Go jobs use the
Go version set in the workflow's `env`.

Before opening a pull request, the local equivalents (from the repository root,
with `CGO_ENABLED=1 CGO_CFLAGS=-w` exported):

```bash
go test -count=1 $(go list -e ./... | grep -vE '/tools/|/test_project/|/cmd/aid-validate')
go vet $(go list -e ./... | grep -vE '/tools/|/test_project/|/cmd/aid-validate')
sh scripts/version-drift.sh
sh scripts/tests/install_sh_test.sh
sh scripts/check-cgo.sh
sh scripts/tests/check_skill_anchors_test.sh
(cd mcp-npm && npm test)
```

For the skill anchor check itself, run the command in the workflow's
`Skill anchors` step. `go mod tidy` is left out above:
it reads every Go package in the working tree, including directories that git
ignores, so in a checkout that has such packages it fails or differs from CI.

## Release workflow

`.github/workflows/release.yml` (on a `v*` tag or a manual run) has its own `test`
job: the same build and full suite as the CI `test` job. The `build` job needs it,
so no release binary is built from a commit whose suite fails. Its `publish-npm`
job runs `npm ci` and `npm test` in `mcp-npm/` again before publishing. The
release workflow does not run the `checks` job's steps.
