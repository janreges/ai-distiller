# Release runbook

The normal path is steps 1-9: checks, the tag, the workflow, checks again.
Tagging, pushing, creating a release and publishing to npm are the owner's
steps; an agent runs the read-only checks and hands the rest over. Examples
use `v1.4.1`; replace it.

## Before the tag

1. The commit to release is on `main` on GitHub, and CI is green for exactly
   that commit:

   ```bash
   git fetch origin
   gh run list --workflow ci.yml --commit "$(git rev-parse origin/main)"
   ```

   The newest line must say `completed` and `success`. The release workflow runs
   the Go suite and the package tests again, but not the CI `checks` job
   (version drift, installer test, cgo check, skill anchors); see
   `.claude/skills/aid-testing-debugging/references/ci.md`.
2. The package tests pass locally: `(cd mcp-npm && npm ci && npm test)`.
3. `sh scripts/version-drift.sh` exits 0 and prints nothing.
4. The version is new. Each of these must come back empty or not found:

   ```bash
   git ls-remote --tags origin v1.4.1
   gh release view v1.4.1 --repo janreges/ai-distiller
   npm view @janreges/ai-distiller-mcp@1.4.1 version
   ```

5. Release or prerelease: a tag with a hyphen (`v1.4.1-rc.1`) becomes a
   GitHub prerelease, is not marked latest, and goes to npm under the `next`
   tag (SKILL.md section 2).

## The tag

6. The owner tags the commit CI checked and pushes the tag:

   ```bash
   git tag -a v1.4.1 -m "Release v1.4.1" origin/main
   git push origin v1.4.1
   ```

   The pushed tag is the normal trigger. `workflow_dispatch` with a `tag`
   input re-runs the workflow for an existing tag (step 8).

## The workflow

7. Follow the run:

   ```bash
   gh run list --workflow release.yml --limit 1
   gh run watch <run-id> --exit-status
   ```

   | Job | Needs | Does | Leaves outside the run |
   |---|---|---|---|
   | `test` | | builds `aid`, runs the full suite against it | nothing |
   | `build` (5 targets) | `test` | cgo build per target, packs the archive, uploads it as a run artifact | nothing |
   | `release` | `test`, `build` | writes `checksums.txt`, creates the GitHub Release "AI Distiller v1.4.1" with generated notes, the five archives and `checksums.txt` | the GitHub Release |
   | `publish-npm` | `build`, `release` | checks the release is downloadable and lists linux-amd64 in `checksums.txt`; `npm ci`, `npm test`; stamps the version; builds; publishes | the npm version |
   | `docker` | `test`, `build` | builds and pushes an image only when the `DOCKER_USERNAME` secret is set (in the `v1.4.0` run login and push were skipped) | Docker Hub tags `v1.4.1` and `latest` |
   | `notify` | `release`, `publish-npm`, runs always | prints both results, fails unless both succeeded | nothing |

## When a job fails

`gh run rerun <run-id> --failed` re-runs the failed jobs and the jobs that
depend on them, in the same run: same tag, same commit, same workflow file.
Before re-running, check what already exists outside the run.

| Failed job | Already published | What to do |
|---|---|---|
| `test` or a `build` target | nothing | A transient failure (runner, network, apt): re-run failed jobs. A failure that needs a code change: the tag points at the old commit, so the owner tags the fixed commit with a new version; or, since nothing was published, deletes the tag (`git push origin :refs/tags/v1.4.1`, `git tag -d v1.4.1`) and tags the fixed commit again. |
| `release` | maybe a partial release: `gh release view v1.4.1 --json assets` | Re-run failed jobs. `softprops/action-gh-release@v2` reuses an existing release for the tag and replaces assets of the same name (`overwrite_files` defaults to true). |
| `publish-npm` | the GitHub Release; npm: check `npm view @janreges/ai-distiller-mcp@1.4.1 version` | E404: nothing reached npm; re-run failed jobs. The version is shown: the publish went through; do not re-run `publish-npm`, npm refuses to publish a version twice. A real test failure in "Install and test" fails again on re-run; it needs a code change and so a new version, and this version stays a GitHub Release without an npm package. |
| `docker` | independent of the rest; `notify` does not wait for it | Re-run if the image matters. |
| `notify` | nothing of its own | It only reports; fix the job it names. |

## Re-running a release for an existing tag

8. When the whole workflow must run again for a tag (for example an archive
   on the release is broken or was deleted), dispatch it by hand:

   ```bash
   gh workflow run release.yml --ref v1.4.1 -f tag=v1.4.1
   ```

   - `--ref` selects the workflow file and the commit the jobs check out.
     Without it GitHub uses the default branch: the archives would be built
     from `main` and still be named after the tag.
   - It is a new run. Every archive is rebuilt (the build date is stamped
     into the binary, so the files differ from the first run's), and the
     `release` job replaces the archives and `checksums.txt` on the existing
     release together, so postinstall's check still matches.
   - If the version is already on npm, `publish-npm` fails at the publish step
     and `notify` fails with it; that is expected when the run only repairs
     the GitHub Release.

## After the release

9. Check the result from the outside:

   ```bash
   sh .claude/skills/aid-release-mcp/scripts/verify-release.sh v1.4.1
   tmp=$(mktemp -d)
   (cd "$tmp" && npx -y --cache "$tmp/npm-cache" @janreges/ai-distiller-mcp@1.4.1 </dev/null; echo "npx exit=$?")
   find "$tmp/npm-cache/_npx" -path '*/ai-distiller-mcp/bin/aid' -exec {} --version \;
   rm -rf "$tmp"
   AID_INSTALLER_DRY_RUN=1 bash install.sh </dev/null
   ```

   - verify-release.sh: every line `ok`, exit 0.
   - The `npx` line installs into an empty cache, so postinstall downloads
     and verifies the archive; the server starts and exits 0 because stdin
     is closed. Expect `npx exit=0`.
   - `find` runs the downloaded binary: `aid version 1.4.1`.
   - The installer dry run resolves "latest" and prints the download URL; for
     a full release it names `v1.4.1`, for a prerelease the previous release.

## Manual fallback

Only when the workflow cannot produce the release (for example GitHub
Actions is unavailable). The tag is still pushed first and is still the
version. A release made this way has no npm provenance attestation, and its
binaries are not stripped (`scripts/build-releases.sh` does not pass `-s -w`).

1. Check out the tag: `git switch --detach v1.4.1`. `scripts/build-releases.sh`
   takes the version from `git describe --tags --abbrev=0`, the nearest tag
   reachable from `HEAD`, not from the tag you mean: run at an untagged
   commit after `v1.4.0`, it names the archives `...-v1.4.0...` and stamps
   `1.4.0` into binaries built from that later commit.
2. Build:

   ```bash
   CGO_CFLAGS=-w bash scripts/build-releases.sh
   ```

   It writes the archives under the correct names, plus a `checksums.txt`
   over the archives it built, into build/releases (ignored by git). Each
   target needs its C compiler, as in the workflow:
   - linux amd64: the host's default compiler (it sets no `CC`, so the host
     must be linux amd64);
   - linux arm64: `aarch64-linux-gnu-gcc`; windows amd64:
     `x86_64-w64-mingw32-gcc`. On Ubuntu the script's closing hint installs
     both: `sudo apt-get install gcc-aarch64-linux-gnu gcc-mingw-w64-x86-64`;
   - darwin amd64 and arm64: osxcross compilers (`o64-clang`/`oa64-clang`,
     `x86_64-apple-darwin23-clang`/`aarch64-apple-darwin23-clang` on `PATH`,
     or the ones in tools/osxcross/target/bin), or a macOS host, where the
     script builds both with the default compiler.

   A target without its compiler is skipped with a `Skipping ...` line and
   gets no archive. On a Linux host without osxcross at most three archives
   come out. With `aarch64-w64-mingw32-gcc` on `PATH` the script also builds
   `aid-windows-arm64-v1.4.1.zip`; leave that one out, nothing downloads it.
3. Put exactly the five archives into one directory (the darwin ones from a
   Mac if needed) and write `checksums.txt` over them the way the workflow
   does, then count:

   ```bash
   sha256sum aid-*.tar.gz aid-*.zip > checksums.txt
   cat checksums.txt     # five lines
   ```

   On macOS, `shasum -a 256` writes the same format.
4. The owner creates the release with all five archives and `checksums.txt`
   (add `--prerelease` for a tag with a hyphen):

   ```bash
   gh release create v1.4.1 --verify-tag --title "AI Distiller v1.4.1" --generate-notes \
     aid-linux-amd64-v1.4.1.tar.gz aid-linux-arm64-v1.4.1.tar.gz \
     aid-darwin-amd64-v1.4.1.tar.gz aid-darwin-arm64-v1.4.1.tar.gz \
     aid-windows-amd64-v1.4.1.zip checksums.txt
   ```

   If a failed run already created the release, replace its assets instead:
   `gh release upload v1.4.1 <files> --clobber`.
5. `sh .claude/skills/aid-release-mcp/scripts/verify-release.sh v1.4.1`: every
   line except the npm one must say `ok` before npm. Publishing to npm first
   breaks every install (SKILL.md section 7).
6. The owner publishes to npm from `mcp-npm/`, stamping the two files the
   workflow stamps, then restores them:

   ```bash
   cd mcp-npm
   npm version 1.4.1 --no-git-tag-version --allow-same-version
   sed -i "s/const AID_VERSION = '[^']*'/const AID_VERSION = '1.4.1'/" scripts/postinstall.js
   npm ci && npm test
   ./publish.sh
   git checkout -- package.json package-lock.json scripts/postinstall.js
   ```

   - `npm version` writes `mcp-npm/package.json` and
     `mcp-npm/package-lock.json`. The `sed` is the workflow's (GNU sed; on
     macOS `sed -i ''`).
   - `npm ci` now runs the stamped postinstall, which downloads this
     release's archive and checks it against `checksums.txt`: a first test
     of the release.
   - `mcp-npm/publish.sh` refuses `0.0.0-dev`, needs an npm login with
     publish rights (`npm whoami`), runs `npm install` and the build, shows
     `npm pack --dry-run`, asks y/N, then runs `npm publish --access public`.
     It passes no `--tag`: for a prerelease run
     `npm run build && npm publish --access public --tag next` instead.
   - The `git checkout` matters: `scripts/version-drift.sh` fails CI on a
     stamped `mcp-npm/package.json` or `AID_VERSION`.
7. Run step 9 of the normal path.
