#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
bad=0
check() { if grep -nE "$2" "$1"; then echo "version pinned in $1" >&2; bad=1; fi; }
[ ! -e VERSION ] || { echo "VERSION file must not exist" >&2; bad=1; }
check install.sh '^[[:space:]]*VERSION=["'"'"']?v?[0-9]'
check install.ps1 '\$Version[[:space:]]*=[[:space:]]*["'"'"']v?[0-9]+\.[0-9]'
check install.ps1 'else \{ ["'"'"']v?[0-9]+\.[0-9]+'
check mcp-npm/scripts/postinstall.js "const AID_VERSION = '[1-9]"
grep -q '"version": "0.0.0-dev"' mcp-npm/package.json || { echo "mcp-npm/package.json must stay at 0.0.0-dev" >&2; bad=1; }
exit $bad
