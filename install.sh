#!/bin/sh
#
# Universal installer for the 'aid' CLI.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/janreges/ai-distiller/main/install.sh | bash
#   curl -sSL https://raw.githubusercontent.com/janreges/ai-distiller/main/install.sh | bash -s -- --sudo
#   curl -sSL https://raw.githubusercontent.com/janreges/ai-distiller/main/install.sh | bash -s -- v1.0.0
#   curl -sSL https://raw.githubusercontent.com/janreges/ai-distiller/main/install.sh | bash -s -- --sudo v1.0.0
#
# Options:
#   --sudo    Install to /usr/local/bin instead of ~/.aid/bin (requires sudo)
#
# This script is designed to be idempotent and safe to run multiple times.

set -e # Exit immediately if a command exits with a non-zero status
set -u # Treat unset variables as an error when substituting
set -o pipefail # Pipeline fails on first command failure

# --- Parse arguments ---
USE_SUDO=false
VERSION=""

for arg in "$@"; do
    case "$arg" in
        --sudo)
            USE_SUDO=true
            ;;
        v[0-9]* | [0-9]*)
            VERSION="${arg#v}"  # Remove 'v' prefix if present
            ;;
        *)
            echo "aid-installer: Error: unknown argument '$arg'. Usage: install.sh [--sudo] [v]MAJOR.MINOR.PATCH" >&2
            exit 1
            ;;
    esac
done

# --- Configuration ---
REPO="janreges/ai-distiller"
if [ "$USE_SUDO" = true ]; then
    INSTALL_DIR="/usr/local/bin"
    AID_INSTALL_ROOT="/usr/local"
else
    AID_INSTALL_ROOT="${AID_INSTALL_ROOT:-"$HOME/.aid"}"
    INSTALL_DIR="$AID_INSTALL_ROOT/bin"
fi

# --- Helper Functions ---

# Logging function that prints to stderr
say() {
    echo "aid-installer: $1" >&2
}

# Check for command presence
ensure_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        say "Error: command '$1' is not installed. Please install it and try again."
        exit 1
    fi
}

# --- Main Installation Logic ---

main() {
    # 1. Detect if we're on Windows with Git Bash/MSYS/Cygwin and delegate to PowerShell
    if uname | grep -qiE 'mingw|msys|cygwin'; then
        say "Detected Windows environment. Delegating to PowerShell installer..."
        ps_prefix=""
        if [ -n "$VERSION" ]; then
            ps_prefix="\$env:AID_VERSION='${VERSION}'; "
        fi
        ps_command="${ps_prefix}iwr https://raw.githubusercontent.com/${REPO}/main/install.ps1 -useb | iex"
        if [ "${AID_INSTALLER_DRY_RUN:-}" = 1 ]; then
            echo "powershell_command=$ps_command"
            exit 0
        fi
        powershell.exe -NoProfile -ExecutionPolicy Bypass \
            -Command "$ps_command"
        exit $?
    fi

    # 2. Check for required dependencies
    ensure_command "curl"
    ensure_command "tar"

    # 3. Detect OS and Architecture
    os_type=$(uname -s)
    arch_type=$(uname -m)

    case "$os_type" in
        Linux)
            os="linux"
            ;;
        Darwin)
            os="darwin"
            ;;
        *)
            say "Error: Unsupported OS '$os_type'. Only Linux and macOS are supported."
            exit 1
            ;;
    esac

    case "$arch_type" in
        x86_64 | amd64)
            arch="amd64"
            ;;
        aarch64 | arm64)
            arch="arm64"
            ;;
        *)
            say "Error: Unsupported architecture '$arch_type'. Only amd64 (x86_64) and arm64 (aarch64) are supported."
            exit 1
            ;;
    esac

    # 4. Resolve the version (when not given) and construct download URLs
    if [ -z "$VERSION" ]; then
        latest_url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${AID_LATEST_URL:-https://github.com/$REPO/releases/latest}") || latest_url=""
        VERSION=$(printf '%s' "$latest_url" | sed -n 's#.*/tag/v\{0,1\}\([0-9][^/]*\)$#\1#p')
        [ -n "$VERSION" ] || { say "Error: could not determine the latest version. Pass one explicitly, for example: bash -s -- v1.4.0"; exit 1; }
    fi
    download_url="https://github.com/${REPO}/releases/download/v${VERSION}/aid-${os}-${arch}-v${VERSION}.tar.gz"
    checksum_url="https://github.com/${REPO}/releases/download/v${VERSION}/checksums.txt"
    archive_name="aid-${os}-${arch}-v${VERSION}.tar.gz"

    if [ "${AID_INSTALLER_DRY_RUN:-}" = 1 ]; then
        echo "download_url=$download_url"
        exit 0
    fi

    # 5. Create secure temporary directory
    tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t aid-install.XXXXXX)
    trap 'rm -rf "$tmp_dir"' EXIT

    # 6. Download archive
    say "Downloading aid v${VERSION} for ${os}/${arch}..."
    if ! curl --fail --silent --location --show-error --output "$tmp_dir/$archive_name" "$download_url"; then
        say "Error: Failed to download from $download_url"
        exit 1
    fi

    # 7. Download and verify checksum
    say "Verifying checksum..."
    if curl --fail --silent --location --output "$tmp_dir/checksums.txt" "$checksum_url" 2>/dev/null; then
        # Determine checksum command
        if command -v sha256sum >/dev/null 2>&1; then
            checksum_cmd="sha256sum"
        elif command -v shasum >/dev/null 2>&1; then
            checksum_cmd="shasum -a 256"
        else
            say "Warning: 'sha256sum' or 'shasum' not found. Skipping checksum verification."
            checksum_cmd=""
        fi

        if [ -n "$checksum_cmd" ]; then
            if ! (cd "$tmp_dir" && grep "$archive_name" checksums.txt | $checksum_cmd -c -) >/dev/null 2>&1; then
                say "Error: Checksum verification failed."
                exit 1
            fi
            say "Checksum verified successfully."
        fi
    else
        say "Warning: Could not download checksums.txt. Skipping verification."
    fi

    # 8. Extract archive
    say "Extracting archive..."
    cd "$tmp_dir"
    tar -xzf "$archive_name"

    # 9. Install binary
    say "Installing 'aid' to $INSTALL_DIR..."
    
    # Handle sudo installation if needed
    if [ "$USE_SUDO" = true ]; then
        if ! [ -w "$INSTALL_DIR" ]; then
            say "Root privileges required for installation to $INSTALL_DIR"
            sudo mkdir -p "$INSTALL_DIR"
            sudo install -m 755 "$tmp_dir/aid" "$INSTALL_DIR/aid"
        else
            mkdir -p "$INSTALL_DIR"
            install -m 755 "$tmp_dir/aid" "$INSTALL_DIR/aid"
        fi
    else
        mkdir -p "$INSTALL_DIR"
        install -m 755 "$tmp_dir/aid" "$INSTALL_DIR/aid"
    fi

    # 10. Success message with PATH guidance
    say "Installation successful!"
    say ""
    say "The 'aid' command was installed to: $INSTALL_DIR/aid"
    say ""

    # Check if INSTALL_DIR is in PATH
    case ":$PATH:" in
        *":$INSTALL_DIR:"*)
            say "✓ The installation directory is already in your PATH."
            say "  You can start using 'aid' right away!"
            ;;
        *)
            if [ "$USE_SUDO" = true ]; then
                # /usr/local/bin is usually in PATH, but let's make sure
                say "✓ Installed to system directory $INSTALL_DIR"
                say "  This directory should already be in your PATH."
            else
                say "⚠ The installation directory is not in your PATH."
                say ""
                say "To use 'aid', add this line to your shell configuration file"
                say "(~/.bashrc, ~/.zshrc, ~/.profile, etc.):"
                say ""
                say "  export PATH=\"$INSTALL_DIR:\$PATH\""
                say ""
                say "Then restart your shell or run:"
                say "  source ~/.bashrc  # or your shell's config file"
            fi
            ;;
    esac
    say ""
    say "Verify the installation by running: aid --version"
}

# Execute
main "$@"