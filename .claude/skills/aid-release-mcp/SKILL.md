---
name: aid-release-mcp
description: >-
  How AI Distiller is versioned, built, released and distributed: the
  tag-driven release workflow, cgo cross-compilation, the archive naming shared
  by release.yml, postinstall.js and the installers, npm Trusted Publishing,
  and the MCP server in mcp-npm/ with its tool-to-flag mapping. Use before and
  after cutting a release, when touching anything in .github/workflows,
  mcp-npm/, install.sh, install.ps1 or build scripts, when a user reports an
  install or MCP problem, when exposing a CLI flag through MCP, or when version
  numbers disagree. Also for "vydej verzi", "nejde nainstalovat", "MCP
  nefunguje", "npm balíček".
---

# Releasing and distributing AI Distiller

One `aid` binary reaches users three ways: the archives on a GitHub Release,
the installers `install.sh` and `install.ps1` (they download those archives),
and the npm package `@janreges/ai-distiller-mcp` (an MCP server whose
postinstall downloads the same archives). `.github/workflows/release.yml`
builds and publishes all of it from one git tag. Run commands from the
repository root.

Tagging, pushing a tag, creating a release and publishing to npm are the
owner's steps. This skill describes them; an agent runs the read-only checks
and hands those commands over.

References, loaded when needed:

- `references/release-runbook.md`: numbered steps, what each job leaves
  behind, safe re-runs, re-running a release for an existing tag, the manual
  fallback with `scripts/build-releases.sh`.
- `references/version-map.md`: every place a version appears, and its writer.
- `references/mcp-tools.md`: every MCP tool, its parameters, the `aid`
  arguments it produces; calling a tool by hand.
- `references/installers.md`: both installers, compared with postinstall.
- `.claude/skills/aid-release-mcp/scripts/verify-release.sh` (section 2).

## 1. The tag is the version

No file in the repository holds a release version. The git tag `vX.Y.Z` is
the only input, and the build and the publish derive everything from it, so
the binary, the archives, the npm package and the installers cannot disagree.
The repository holds placeholders: `internal/version/version.go#Version` is
`dev`; `mcp-npm/package.json` and `mcp-npm/scripts/postinstall.js#AID_VERSION`
are `0.0.0-dev`; the installers name no version and, unless given one, ask
GitHub for the latest release.

`scripts/version-drift.sh` (CI `checks` job) fails when a `VERSION` file
exists, when `install.sh` or `install.ps1` assigns a numeric version, when
`AID_VERSION` starts with a digit 1-9, or when the package version is not
`0.0.0-dev`.

Where the version (the tag without its `v`) is stamped: the `build` job sets
`-ldflags -X .../internal/version.Version=<version>` (plus `Commit` and
`Date`), so `aid --version` prints `aid version <version>`, and names the
archives (section 3); the `publish-npm` job, in its own checkout, runs
`npm version <version>` (writes `mcp-npm/package.json` and the lockfile) and
a `sed` on `AID_VERSION`. Nothing is committed back: a release needs no
commit, and "bump the version" is never a file edit. Every place with its
writer: `references/version-map.md`.

## 2. Release procedure

Exact commands and failure handling: `references/release-runbook.md`.

Before the tag (read-only checks an agent can run):

1. The commit is on `main` and its CI run is green:
   `gh run list --workflow ci.yml --commit "$(git rev-parse origin/main)"`.
2. `(cd mcp-npm && npm ci && npm test)` passes (in `publish-npm` a failure
   stops the release after the GitHub Release exists).
3. `sh scripts/version-drift.sh` exits 0.
4. The version is new: `git ls-remote --tags origin vX.Y.Z` prints nothing and
   `npm view @janreges/ai-distiller-mcp@X.Y.Z version` fails with E404.

The tag, run by the owner on the commit CI checked (`v1.4.0` and
`v1.4.0-rc.3` are annotated tags like this one):

```bash
git tag -a v1.4.1 -m "Release v1.4.1" origin/main
git push origin v1.4.1
```

The pushed tag (`v*`) starts the workflow; a manual run (`workflow_dispatch`
with a `tag` input) re-runs it for an existing tag (runbook step 8).
**Prereleases**: a tag with a hyphen (`v1.4.1-rc.1`) makes the `release` job
mark the GitHub Release as a prerelease and not as latest, and `publish-npm`
adds `--tag next`. Users get it with `npx @janreges/ai-distiller-mcp@next` or
`install.sh v1.4.1-rc.1`; the installers' default and the npm `latest` tag
stay on the last full release.

After the tag:

1. Follow the run: `gh run list --workflow release.yml --limit 1`, then
   `gh run watch <run-id> --exit-status`. `notify` fails unless both
   `release` and `publish-npm` succeeded.
2. `sh .claude/skills/aid-release-mcp/scripts/verify-release.sh v1.4.1`: one
   `ok`/`FAIL` line per check (the five archives and `checksums.txt`, a
   `checksums.txt` line per archive with GitHub's hash for it, the npm
   version); exit 0 only when all pass. `v1.4.0` gives 13 `ok` lines; a tag
   without a release gives `FAIL release not found` and exit 1.
3. A clean `npx` install from an empty npm cache (runbook step 9): exit 0,
   and the downloaded binary prints the new version.

## 3. The naming contract

Every archive is `aid-<os>-<arch>-v<version>.<ext>` on the release with tag
`v<version>`, at
`https://github.com/janreges/ai-distiller/releases/download/v<version>/<name>`:

| Target | Archive | Binary at the archive root |
|---|---|---|
| linux amd64, linux arm64 | `aid-linux-<arch>-v<version>.tar.gz` | `aid` |
| darwin amd64, darwin arm64 | `aid-darwin-<arch>-v<version>.tar.gz` | `aid` |
| windows amd64 | `aid-windows-amd64-v<version>.zip` | `aid.exe` |

Next to them, `checksums.txt` holds `sha256sum` output (`<hash>  <name>`) for
every archive.

Producers: the `build` and `release` jobs of `.github/workflows/release.yml`;
by hand, `scripts/build-releases.sh`. Consumers:

- `mcp-npm/scripts/postinstall.js#assetName` and
  `mcp-npm/scripts/postinstall.js#platformInfo` (name and extension),
  `mcp-npm/scripts/postinstall.js#checksumFor` (the `checksums.txt` line);
- `install.sh` and `install.ps1` (`references/installers.md`);
- the `publish-npm` step "Verify the release binary is downloadable" (fetches
  the linux-amd64 archive and greps `checksums.txt` for it);
- `.claude/skills/aid-release-mcp/scripts/verify-release.sh`, and the tests
  `mcp-npm/test/postinstall.test.js` and `scripts/tests/install_sh_test.sh`.

A name changed in one place breaks installs: the consumer asks for a file
that does not exist and gets HTTP 404, so a change touches all of the above
in one commit. The documented install commands fetch the installers from
`main`, and they install any version, so a new scheme must still find the
archives of older releases.

## 4. cgo

Release binaries are built with `CGO_ENABLED=1` and `-tags cgo`: the
tree-sitter grammars are C code, and without cgo `aid` does not compile
(`CLAUDE.md` section 2; what fails:
`.claude/skills/aid-language-parsers/references/tree-sitter-cgo.md`). So each
target needs a C compiler for that target (darwin builds also set
`MACOSX_DEPLOYMENT_TARGET=14.0`, else they require the runner's macOS):

| Target | Runner | C compiler (`CC`) |
|---|---|---|
| linux amd64 | ubuntu-latest | the runner's default |
| linux arm64 | ubuntu-latest | `aarch64-linux-gnu-gcc` (apt `gcc-aarch64-linux-gnu`) |
| windows amd64 | ubuntu-latest | `x86_64-w64-mingw32-gcc` (apt `gcc-mingw-w64-x86-64`) |
| darwin arm64 | macos-15 | the runner's default, native |
| darwin amd64 | macos-15 | `clang -arch x86_64`, cross-compiled on Apple Silicon |

There is no windows arm64 build: the matrix has no entry for it, and
postinstall reports `windows-arm64` as unsupported. A new target needs a
matrix entry with its C compiler, `platformInfo` in postinstall and its
test, the installers, the archive list in verify-release.sh and the tables
in this skill.

Never add a build without cgo. `scripts/check-cgo.sh` (CI `checks` job) fails
on a tracked `CGO_ENABLED=0` (the files it scans: `CLAUDE.md` section 2).

## 5. The MCP package

`mcp-npm/` is the npm package. It ships the compiled server, the wrapper and
the postinstall, not the binary. Start-up chain:

1. `npm install` or `npx` runs the postinstall: it maps
   `os.platform()`/`os.arch()` to a target, downloads that archive and
   `checksums.txt` from the release of `AID_VERSION`, verifies the sha256 (no
   file, no line or a mismatch fails the install), extracts into the
   package's bin directory and runs `aid --version`; a binary there with the
   same version skips the download. At `0.0.0-dev` (a checkout) it prints
   "development checkout: build aid and copy it to mcp-npm/bin/".
2. The package's only bin, `aid-mcp`, is `mcp-npm/mcp-server-wrapper.js`: it
   starts the compiled server with the same `node`, stdio inherited, and
   forwards SIGINT and SIGTERM.
3. `mcp-npm/mcp-server-sdk.ts#startServer` calls `requireBinary`, which exits 1
   with repair hints when the bin directory has no `aid` (`aid.exe` on
   Windows); the server never looks at `PATH`. It registers the tools, serves
   MCP on stdio, and exits 0 when stdin closes.
4. A tool call confines its path parameter to `AID_ROOT` (table below), then
   `mcp-npm/mcp-server-sdk.ts#AidDistillerServer.executeAidCommand` queues
   aid's argument list on a `mcp-npm/run-aid.ts#SerialQueue`: one `aid` at a
   time, and a failing run does not block the next. `mcp-npm/run-aid.ts#runAid`
   spawns the binary without a shell, stdin ignored, in `AID_ROOT`. The result
   is aid's stderr (if any), a blank line, its stdout; AI-action tools put the
   path of aid's file in front (`mcp-npm/tool-text.ts#aiActionText`). A
   non-zero exit is a tool error carrying aid's stderr.

The server's environment:

| Variable | Default | Effect |
|---|---|---|
| `AID_ROOT` | the server's working directory | working directory of every `aid` run; base for `list_files` and for the output paths the AI-action tools report. Every path parameter (`file_path`, `directory_path`, `target_path`, `path` of `list_files`) must lie inside it with symlinks resolved, else the call fails before `aid` runs with an error that names `AID_ROOT` and how to widen it; no opt-out (`mcp-npm/confine.ts#confinePath`; `aid` gets the real path relative to the real `AID_ROOT`) |
| `AID_MCP_TIMEOUT_MS` | `120000` | wall-clock limit of one run, counted from its start, not while it waits in the queue; on expiry the child is killed and the tool fails with a message naming this variable |
| `AID_MCP_MAX_OUTPUT_BYTES` | `1000000` | cap on one run's stdout; at the cap the child is killed and the result comes back with an "[Output incomplete: ...]" notice first and last (`mcp-npm/tool-text.ts#truncationNotice`) |
| `AID_MCP_DEBUG=true` (or `DEBUG=true`/`1`) | off | logs the binary path, each command line and the limits to stderr |

A limit that is not a positive integer falls back to its default
(`mcp-npm/run-aid.ts#limitsFromEnv`).

The 16 tools are registered in `mcp-npm/mcp-server-sdk.ts` by the
`AidDistillerServer` methods `registerCoreTools`, `registerSpecializedTools`,
`registerAIActionTools`, `registerLegacyTools` and `registerMetaTools`;
`list_files` and `get_capabilities` do not run `aid`. Table:
`references/mcp-tools.md`. The package tests (`(cd mcp-npm && npm test)`)
never run a tool against the real binary
(`.claude/skills/aid-testing-debugging/SKILL.md`): try a changed tool by hand
(`references/mcp-tools.md`, last section).

## 6. A new CLI flag and MCP

The CLI side is the checklist "Adding a flag" in
`.claude/skills/aid-architecture/references/cli-flags.md`. For MCP, first
decide whether an agent needs the flag per call. The server fixes where
output goes (`distill_*` always pass `--stdout --show-ai-agent-instructions`;
AI-action tools let `aid` write its file and report the path), so a flag
that only changes where output goes stays out.

If it belongs:

1. Pick the tools. The three `distill_*` tools and `aid_analyze` map filter
   parameters to flags one to one; the other analysis tools take few
   parameters and set their own defaults (`aid_hunt_bugs` passes
   `--private=1 --protected=1 --internal=1` unless `include_private` is
   false). Per tool: `references/mcp-tools.md`.
2. Add the parameter to the input schema: in
   `mcp-npm/mcp-server-sdk.ts#TOOL_SCHEMAS` for `aid_hunt_bugs`,
   `aid_suggest_refactoring`, `aid_generate_diagram`, `aid_analyze_security`,
   `aid_generate_docs` and `aid_analyze`; inline in the `registerTool` call
   for the others. Snake_case, `.optional()`, a `.describe()` that states the
   default, in a `z.strictObject({...})`: a key the schema lacks fails the call
   with a `-32602` error naming it (`mcp-npm/test/tool-schemas.test.js`).
3. In the handler, push the flag only when the parameter differs from the
   CLI default, like the existing ones: `--x=1` when a default-off boolean is
   true, `--x=0` when a default-on one is explicitly false, `--x=<value>` for
   a string. Each is one argv element and no shell runs, so nothing needs
   quoting.
4. Update the lists: `mcp-npm/README.md` (section "All Registered Tools",
   parameters per tool), `references/mcp-tools.md`, and for a new tool also
   the root `README.md` (section "Available MCP Tools") and the `tools` list
   of `get_capabilities`.
5. `(cd mcp-npm && npm test)`, then call the tool by hand with
   `AID_MCP_DEBUG=true` and read the logged command line.

It ships with the next npm version, whose postinstall fetches the `aid` of
the same release: the parameter and a binary that knows the flag arrive
together.

## 7. Order matters

`publish-npm` needs `build` and `release`. The GitHub Release with every
archive and `checksums.txt` must exist before npm publishes: once the package
is on npm, installs run postinstall, which downloads from that release, so a
package published first fails every install (HTTP 404 or no checksum). The
job checks this first ("Verify the release binary is downloadable"); a
publish by hand keeps the order (runbook, manual fallback).

npm publishes through Trusted Publishing: the job has
`permissions: id-token: write`, upgrades npm (OIDC publishing needs npm
11.5.1 or later) and runs `npm publish --access public --provenance` with no
npm token. The trusted publisher on npmjs.com names this repository and the
workflow file name release.yml (the workflow's comment says so; the npmjs
setting cannot be read from here), so renaming or moving the workflow file
stops npm publishing until that setting changes. `1.4.0` and `1.4.0-rc.3`
carry a provenance attestation (`npm view ... dist.attestations`), earlier
versions none.

## 8. Install and MCP problems

Before reading code, find out what the user ran: the package version
(`MCP Version:` in the postinstall log, or `npm ls @janreges/ai-distiller-mcp`)
and the platform, `node -p "process.platform + ' ' + process.arch"`. A
published postinstall can differ from the repository's; read it from
`npm pack @janreges/ai-distiller-mcp@<version>` in a temp directory.
Installers: `references/installers.md`.

- **The install fails.** Postinstall prints the URL and the reason.
  Unsupported platform: `platformInfo` throws before any download. HTTP 404:
  the release or the archive is missing; run verify-release.sh for that
  version. Checksum: `checksums.txt` is missing (a 404 naming it), lacks the
  archive's line, or the hash differs.
- **Windows on ARM.** No `aid-windows-arm64-*` archive exists (section 4).
  The repository's postinstall stops with "No prebuilt aid binary for
  windows-arm64. Supported: linux and macOS on x64 and arm64, Windows on
  x64. ..."; the published 1.4.0 has an older one that requests
  `aid-windows-arm64-v1.4.0.zip` and fails with "Download failed with status
  code: 404". `install.ps1` fails on the same download. Options: build from
  source; an x64 Node reports `x64` (`os.arch()` is Node's build
  architecture) and gets the windows-amd64 archive, untested on ARM here.
- **The server exits at start** with "AI Distiller binary not found": the
  package was installed without its postinstall (for example
  `--ignore-scripts`) or the binary was removed. Reinstall.
- **A tool fails or returns too little.** Check `AID_ROOT` (a missing one
  gives "Could not start aid: the working directory ... does not exist", a
  path outside it "... is outside AID_ROOT ..."), then the timeout and the
  output cap (section 5). `AID_MCP_DEBUG=true` logs each command line.
