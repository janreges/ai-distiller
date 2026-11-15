# Cross-Compilation Guide for AI Distiller

## Overview

AI Distiller requires CGO for full language support (tree-sitter parsers). This guide explains how to set up cross-compilation toolchains for all supported platforms.

## Supported Platforms

- Linux AMD64 (native on most CI)
- Linux ARM64 (requires gcc-aarch64-linux-gnu)
- macOS AMD64 (requires osxcross)
- macOS ARM64 (requires osxcross)
- Windows AMD64 (requires mingw-w64)

## Prerequisites

### Ubuntu/Debian

```bash
# Basic build tools
sudo apt-get update
sudo apt-get install -y build-essential git

# Cross-compilation toolchains
sudo apt-get install -y gcc-aarch64-linux-gnu gcc-mingw-w64-x86-64

# For osxcross (macOS cross-compilation)
sudo apt-get install -y clang cmake libssl-dev liblzma-dev libxml2-dev
```

## Linux ARM64 Cross-Compilation

```bash
# Install toolchain
sudo apt-get install gcc-aarch64-linux-gnu

# Build will automatically use aarch64-linux-gnu-gcc
```

## Windows Cross-Compilation

```bash
# Install mingw-w64
sudo apt-get install gcc-mingw-w64-x86-64

# Build will automatically use x86_64-w64-mingw32-gcc
```

## macOS Cross-Compilation (osxcross)

### Step 1: Install osxcross

```bash
# Clone osxcross (already done in tools/osxcross)
cd tools
git clone https://github.com/tpoechtrager/osxcross

# Install dependencies
sudo apt-get install -y clang cmake libssl-dev liblzma-dev libxml2-dev
```

### Step 2: Obtain macOS SDK

**Legal Note**: Ensure you have read and understood the [Xcode license terms](https://www.apple.com/legal/sla/docs/xcode.pdf).

#### Option A: From macOS Machine
1. Download Xcode Command Line Tools
2. Run: `./tools/osxcross/tools/gen_sdk_package_tools.sh`
3. Copy resulting SDK to Linux machine

#### Option B: From Linux
1. Download Command Line Tools .dmg from Apple Developer
2. Run: `./tools/osxcross/tools/gen_sdk_package_tools_dmg.sh <file.dmg>`

#### Option C: Pre-packaged SDKs
Search for community-maintained "MacOSX-SDKs" repositories (use at your own discretion)

### Step 3: Build osxcross

```bash
# Place SDK in tarballs directory
mv MacOSX*.tar.* tools/osxcross/tarballs/

# Build osxcross
cd tools/osxcross
./build.sh

# Add to PATH
export PATH="$PWD/target/bin:$PATH"
```

### Step 4: Verify Installation

```bash
# Check for compilers
which x86_64-apple-darwin21.4-clang
which aarch64-apple-darwin21.4-clang
```

## Building AI Distiller for All Platforms

Once all toolchains are installed:

```bash
# Build releases for all platforms
./scripts/build-releases.sh
```

## Troubleshooting

### Linux ARM64 Build Fails
- Ensure `gcc-aarch64-linux-gnu` is installed
- Check that `aarch64-linux-gnu-gcc` is in PATH

### Windows Build Fails
- Ensure `gcc-mingw-w64-x86-64` is installed
- Check that `x86_64-w64-mingw32-gcc` is in PATH

### macOS Build Skipped
- Ensure osxcross is built and in PATH
- Check for `x86_64-apple-darwin*-clang` in PATH
- Verify SDK is properly installed in osxcross

### CGO Errors
- All builds require CGO_ENABLED=1
- Ensure appropriate C compiler is available for target platform
- Check that tree-sitter WASM files are present

## GitHub Actions Note

Due to the complexity of setting up osxcross in CI, macOS builds are typically done on actual macOS runners or manually. Linux and Windows cross-compilation work well in standard Ubuntu runners.