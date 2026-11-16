# Version map

Every place a version appears when `aid` is built, published or run, and who
writes it. The only input is the git tag `vX.Y.Z`; `<version>` below is the tag
without its `v`. The repository itself holds placeholders only, and
`scripts/version-drift.sh` keeps it that way (SKILL.md section 1).

## In the binary

| Where | In the repository | Written by | Read by |
|---|---|---|---|
| `internal/version/version.go#Version` | `dev` | `-ldflags -X` in the `build` job of `.github/workflows/release.yml` (`<version>`); `Makefile` and `scripts/build-releases.sh` (`git describe --tags --abbrev=0` without the `v`, else `dev`); `Dockerfile` (`ARG VERSION`, default `dev`; the `docker` job passes `<version>`) | `cmd/aid/main.go#main` copies it to `internal/cli/root.go#Version` |
| `internal/version/version.go#Commit` | `none` | the same builds: the `build` job writes the first 7 characters of the commit SHA, `Makefile` and `scripts/build-releases.sh` `git rev-parse --short HEAD`, the `docker` job the full SHA | no code reads it |
| `internal/version/version.go#Date` | `unknown` | the `build` job, `Makefile`, `scripts/build-releases.sh`: the UTC build time; the `Dockerfile` does not set it | `--version` prints `built: YYYY-MM-DD` when it parses |
| `aid --version` | | | prints `aid version <version>`, the date line, the project URL (`internal/cli/root.go#initFlags`, the PreRun) |
| help header | | | `internal/cli/help.go#getVersionInfo`: `v<version>`, plus `built <date>` |

A binary from `go build` without ldflags says `aid version dev`. A `make build`
at a commit after a tag says that tag's version: `git describe` takes the
nearest tag reachable from `HEAD`.

## On GitHub

| Where | Written by |
|---|---|
| the tag `v<version>` | the owner, by hand (`references/release-runbook.md`, step 6) |
| archive names `aid-<os>-<arch>-v<version>.<ext>` | the `build` job (SKILL.md section 3) |
| `checksums.txt` lines, which contain the archive names | the `release` job |
| release title `AI Distiller v<version>`, prerelease flag (tag contains `-`), "latest" flag (the opposite) | the `release` job |
| Docker Hub tags `v<version>` and `latest` | the `docker` job, only with the Docker Hub secrets |

## On npm

| Where | In the repository | Written by | Read by |
|---|---|---|---|
| `mcp-npm/package.json` `version` | `0.0.0-dev` | `npm version <version> --no-git-tag-version --allow-same-version` in the `publish-npm` job, in its checkout only | npm (the published version); the server reports it as its MCP server version and as `server_version` of `get_capabilities`; postinstall prints it as `MCP Version:` |
| `mcp-npm/package-lock.json` `version` (two places) | `0.0.0-dev` | the same `npm version` | npm |
| `mcp-npm/scripts/postinstall.js#AID_VERSION` | `'0.0.0-dev'` | `sed` in the `publish-npm` job; the job then fails unless both stamps hold the version | postinstall: which release it downloads from; an existing binary whose `--version` matches skips the download; `0.0.0-dev` downloads nothing |
| dist-tag `latest` or `next` | | `npm publish` in the `publish-npm` job: `--tag next` when the tag contains `-`, else npm's default `latest` | `npx @janreges/ai-distiller-mcp` without a version, `@next` |

The workflow gives the package version and `AID_VERSION` the same value, so a
package version always installs the binary of the release with the same
number. A publish by hand must do the same (`references/release-runbook.md`, manual
fallback, step 6).

## In the installers

| Where | Written by |
|---|---|
| `install.sh` | no version in the file. Takes `vX.Y.Z` or `X.Y.Z` as an argument, else follows the redirect of `https://github.com/janreges/ai-distiller/releases/latest` |
| `install.ps1` | no version in the file. Takes `$env:AID_VERSION`, else `tag_name` of the GitHub API's latest release; writes the version it installed as `DisplayVersion` of its uninstall registry key |

Details: `references/installers.md`.

## Not release versions

- `get_capabilities` reports `protocol_version`: the MCP protocol version of
  the server's initialize answer, recorded by
  `mcp-npm/mcp-server-sdk.ts#AidDistillerServer.connectTransport` (the
  SDK picks the client's version when it supports it, else its latest), not
  the package version.
- `mcp-npm/package.json` `engines.node` (`>=20`, the minimum of `@hono/node-server`,
  which the MCP SDK depends on; CI tests Node 20, 22 and 24) and the dependency ranges.
