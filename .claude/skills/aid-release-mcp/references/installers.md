# Installers

`install.sh` (Linux, macOS, and Git Bash/MSYS/Cygwin on Windows) and
`install.ps1` (Windows PowerShell) download one archive from a GitHub Release
and install the `aid` binary. They hold no version (SKILL.md section 1) and
use the archive names of SKILL.md section 3. The documented commands fetch
them from `main`:

```bash
curl -sSL https://raw.githubusercontent.com/janreges/ai-distiller/main/install.sh | bash
curl -sSL https://raw.githubusercontent.com/janreges/ai-distiller/main/install.sh | bash -s -- --sudo v1.4.0
```

```powershell
iwr https://raw.githubusercontent.com/janreges/ai-distiller/main/install.ps1 -useb | iex
$env:AID_VERSION="1.4.0"; iwr https://raw.githubusercontent.com/janreges/ai-distiller/main/install.ps1 -useb | iex
```

Never run `install.sh` here without `AID_INSTALLER_DRY_RUN=1`: it would
install into your home directory. `install.ps1` has no dry run.

## install.sh

- **Arguments**: `--sudo`, and a version as `vX.Y.Z` or `X.Y.Z` (the `v` is
  dropped). Anything else stops with `unknown argument` and exit 1.
- **Version**: the argument; without one, it follows the redirect of
  `https://github.com/janreges/ai-distiller/releases/latest` (overridable with
  `AID_LATEST_URL`, which the test uses) and takes the version from the final
  `/tag/v...` URL. GitHub's latest release is never a prerelease. If that
  fails it stops with "could not determine the latest version" and exit 1.
- **Windows**: when `uname` reports MINGW, MSYS or Cygwin, it hands over to
  PowerShell, running `install.ps1` from `main` with `$env:AID_VERSION` set
  when a version was given; `--sudo` is not passed on.
- **Platform**: needs `curl` and `tar`. `uname -s` Linux or Darwin; `uname -m`
  `x86_64`/`amd64` becomes `amd64`, `aarch64`/`arm64` becomes `arm64`;
  anything else stops with an error. It downloads
  `aid-<os>-<arch>-v<version>.tar.gz`.
- **Checksum**: it downloads `checksums.txt` of the same release and checks
  the archive's line with `sha256sum -c` (or `shasum -a 256`). A missing line
  or a wrong hash stops the install ("Checksum verification failed"). If
  `checksums.txt` cannot be downloaded, or neither tool exists, it warns and
  installs without the check.
- **Location**: `$AID_INSTALL_ROOT/bin/aid`, default `~/.aid/bin/aid`; with
  `--sudo`, `/usr/local/bin/aid` (it calls `sudo` only when that directory is
  not writable). It prints how to add the directory to `PATH` and changes no
  shell file. The download is unpacked in a temporary directory that is
  removed on exit.
- **Dry run**: with `AID_INSTALLER_DRY_RUN=1` it resolves the version (a
  network request when none is given), prints `download_url=<url>` and exits
  0 before downloading anything. On the Windows branch it prints
  `powershell_command=<command>` instead and does not start PowerShell.

```bash
AID_INSTALLER_DRY_RUN=1 bash install.sh v1.4.0 </dev/null
# download_url=https://github.com/janreges/ai-distiller/releases/download/v1.4.0/aid-linux-amd64-v1.4.0.tar.gz
```

`scripts/tests/install_sh_test.sh` (CI `checks` job) runs only dry runs: a
bare and a `v`-prefixed version, the latest version, a failing lookup, an
unknown argument, and the Windows branch with stubbed `uname` and
`powershell.exe`.

## install.ps1

- **Version**: `$env:AID_VERSION` (a leading `v` is trimmed); without it,
  `tag_name` of `https://api.github.com/repos/janreges/ai-distiller/releases/latest`.
  If that request fails it stops: "could not determine the latest version;
  set $env:AID_VERSION".
- **Platform**: 32-bit Windows stops with an error. The architecture comes
  from `PROCESSOR_ARCHITEW6432`, else `PROCESSOR_ARCHITECTURE`: a value
  containing `ARM` becomes `arm64`, anything else `amd64`. It downloads
  `aid-windows-<arch>-v<version>.zip`; no `arm64` archive is published, so on
  Windows ARM the download fails and the script stops with
  "Installation failed: ...".
- **Checksum**: it downloads `checksums.txt` and compares `Get-FileHash`
  SHA256 with the line that names the archive. A wrong hash stops the
  install. A missing line only warns. Any error in that block (including a
  failed download) warns "Could not download checksums.txt. Skipping
  verification." and the install goes on.
- **Location**: `$env:AID_INSTALL_ROOT\bin\aid.exe`, default
  `%USERPROFILE%\.aid\bin\aid.exe`. It stops running `aid` processes before
  copying, appends the directory to the user `PATH` (not when the new value
  would exceed 2000 characters; then it warns), and writes an uninstall entry
  under `HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Aid` with
  the installed version as `DisplayVersion`.
- **Tests**: none run it; CI has no Windows job. Only the delegation from
  `install.sh` is tested.

## Postinstall compared

The npm package's `mcp-npm/scripts/postinstall.js` downloads the same
archives (SKILL.md section 5) but is stricter:

| Case | postinstall | `install.sh` | `install.ps1` |
|---|---|---|---|
| version | `AID_VERSION`, stamped at publish | argument, else latest | `$env:AID_VERSION`, else latest |
| `checksums.txt` cannot be downloaded | install fails | warns, installs | warns, installs |
| no line for the archive | install fails | install fails | warns, installs |
| wrong hash | install fails | install fails | install fails |
| windows arm64 | "No prebuilt aid binary for windows-arm64 ..." before any download | under Git Bash, MSYS or Cygwin: delegates to `install.ps1` | download fails (no such archive) |
| other platforms | "No prebuilt aid binary for `<os>-<arch>` ..." | error naming the OS or the architecture | 32-bit: error |
| where | the package's bin directory | `~/.aid/bin` or `/usr/local/bin` | `%USERPROFILE%\.aid\bin` |
