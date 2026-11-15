#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
fail() { echo "FAIL: $1" >&2; exit 1; }
url() { AID_INSTALLER_DRY_RUN=1 bash install.sh "$@" | sed -n 's/^download_url=//p'; }

case "$(url 1.4.0)"  in *"/download/v1.4.0/aid-"*"-v1.4.0.tar.gz") ;; *) fail "bare version";; esac
case "$(url v1.4.0)" in *"/download/v1.4.0/aid-"*"-v1.4.0.tar.gz") ;; *) fail "v-prefixed version";; esac
case "$(url)"        in *"/download/v"[0-9]*"/aid-"*) ;; *) fail "latest";; esac
# resolve-fails: an unreachable lookup URL must abort with a message, not build a URL
if out=$(AID_LATEST_URL=http://127.0.0.1:9/none AID_INSTALLER_DRY_RUN=1 bash install.sh 2>&1); then fail "resolve-fails: expected non-zero exit"; fi
echo "$out" | grep -q "could not determine the latest version" || fail "resolve-fails: message"
# bad argument: must be rejected, not turned into a version
if out=$(AID_INSTALLER_DRY_RUN=1 bash install.sh --help 2>&1); then fail "bad-arg: expected non-zero exit"; fi
echo "$out" | grep -q "unknown argument" || fail "bad-arg: message"
# Windows (Git Bash) branch: dry run must not delegate to PowerShell
fakebin=$(mktemp -d); trap 'rm -rf "$fakebin"' EXIT
printf '#!/bin/sh\necho MINGW64_NT\n' > "$fakebin/uname"; printf '#!/bin/sh\necho POWERSHELL_RAN >&2; exit 9\n' > "$fakebin/powershell.exe"
chmod +x "$fakebin/uname" "$fakebin/powershell.exe"
out=$(PATH="$fakebin:$PATH" AID_INSTALLER_DRY_RUN=1 bash install.sh v1.4.0 2>&1) || fail "windows dry run: non-zero exit"
case "$out" in *POWERSHELL_RAN*) fail "windows dry run: powershell was executed";; esac
echo "$out" | grep -q "AID_VERSION='1.4.0'" || fail "windows dry run: command not printed"
echo "ok"
