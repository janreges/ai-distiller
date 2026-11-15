#!/bin/bash

# Build script for creating releases - ALL builds require CGO for full language support
# This requires appropriate cross-compilation toolchains to be installed

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Read version
VERSION=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
[ -n "$VERSION" ] || VERSION=dev
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# LDFLAGS
LDFLAGS="-X 'github.com/janreges/ai-distiller/internal/version.Version=$VERSION' \
         -X 'github.com/janreges/ai-distiller/internal/version.Commit=$GIT_COMMIT' \
         -X 'github.com/janreges/ai-distiller/internal/version.Date=$BUILD_DATE'"

# Build directory
BUILD_DIR="build/releases"
mkdir -p $BUILD_DIR

echo -e "${GREEN}Building AI Distiller v$VERSION (full language support)${NC}"
echo "This requires proper toolchains for cross-compilation!"
echo ""

# Auto-detect and add osxcross to PATH if available
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
OSXCROSS_PATH="$PROJECT_ROOT/tools/osxcross/target/bin"

if [ -d "$OSXCROSS_PATH" ]; then
    echo -e "${GREEN}Found osxcross at: $OSXCROSS_PATH${NC}"
    export PATH="$PATH:$OSXCROSS_PATH"
    echo -e "${GREEN}Added osxcross to PATH${NC}"
    echo ""
fi

# Function to build for a platform
build_platform() {
    local GOOS=$1
    local GOARCH=$2
    local EXT=$3
    local CC=$4
    local BINARY_NAME="aid$EXT"
    local TEMP_OUTPUT="$BUILD_DIR/temp-$GOOS-$GOARCH/aid$EXT"
    
    echo -e "${YELLOW}Building for $GOOS/$GOARCH...${NC}"
    
    # Create temp directory for this platform
    mkdir -p "$BUILD_DIR/temp-$GOOS-$GOARCH"
    
    if [ -z "$CC" ]; then
        # Native build
        CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH go build -tags "cgo" -ldflags "$LDFLAGS" -o "$TEMP_OUTPUT" ./cmd/aid
    else
        # Cross-compile with specific compiler
        CC=$CC CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH go build -tags "cgo" -ldflags "$LDFLAGS" -o "$TEMP_OUTPUT" ./cmd/aid
    fi
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Built: $BINARY_NAME for $GOOS/$GOARCH${NC}"
        
        # Create archive
        cd "$BUILD_DIR/temp-$GOOS-$GOARCH"
        if [ "$GOOS" = "windows" ]; then
            # Create ZIP for Windows
            zip -q "../aid-$GOOS-$GOARCH-v$VERSION.zip" "$BINARY_NAME"
            echo -e "${GREEN}✓ Created: aid-$GOOS-$GOARCH-v$VERSION.zip${NC}"
        else
            # Create tar.gz for Unix systems
            tar -czf "../aid-$GOOS-$GOARCH-v$VERSION.tar.gz" "$BINARY_NAME"
            echo -e "${GREEN}✓ Created: aid-$GOOS-$GOARCH-v$VERSION.tar.gz${NC}"
        fi
        cd - > /dev/null
        
        # Clean up temp directory
        rm -rf "$BUILD_DIR/temp-$GOOS-$GOARCH"
    else
        echo -e "${RED}✗ Failed to build $GOOS/$GOARCH${NC}"
    fi
    echo ""
}

# Linux AMD64 (native on most CI systems)
build_platform "linux" "amd64" "" ""

# Linux ARM64 (requires aarch64-linux-gnu-gcc)
if command -v aarch64-linux-gnu-gcc &> /dev/null; then
    build_platform "linux" "arm64" "" "aarch64-linux-gnu-gcc"
else
    echo -e "${YELLOW}Skipping Linux ARM64 - install gcc-aarch64-linux-gnu${NC}"
fi

# macOS AMD64 (requires osxcross or Darwin host)
if command -v o64-clang &> /dev/null; then
    build_platform "darwin" "amd64" "" "o64-clang"
elif command -v x86_64-apple-darwin23-clang &> /dev/null; then
    # Common osxcross naming
    build_platform "darwin" "amd64" "" "x86_64-apple-darwin23-clang"
elif ls $OSXCROSS_PATH/x86_64-apple-darwin*-clang &> /dev/null 2>&1; then
    # Try to find osxcross compiler by pattern
    CC=$(ls $OSXCROSS_PATH/x86_64-apple-darwin*-clang | head -1)
    build_platform "darwin" "amd64" "" "$CC"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    build_platform "darwin" "amd64" "" ""
else
    echo -e "${YELLOW}Skipping macOS AMD64 - requires osxcross or macOS host${NC}"
    echo -e "${YELLOW}To enable: Install osxcross from tools/osxcross${NC}"
fi

# macOS ARM64 (requires osxcross or Darwin host)
if command -v oa64-clang &> /dev/null; then
    build_platform "darwin" "arm64" "" "oa64-clang"
elif command -v aarch64-apple-darwin23-clang &> /dev/null; then
    # Common osxcross naming
    build_platform "darwin" "arm64" "" "aarch64-apple-darwin23-clang"
elif ls $OSXCROSS_PATH/aarch64-apple-darwin*-clang &> /dev/null 2>&1; then
    # Try to find osxcross compiler by pattern
    CC=$(ls $OSXCROSS_PATH/aarch64-apple-darwin*-clang | head -1)
    build_platform "darwin" "arm64" "" "$CC"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    build_platform "darwin" "arm64" "" ""
else
    echo -e "${YELLOW}Skipping macOS ARM64 - requires osxcross or macOS host${NC}"
    echo -e "${YELLOW}To enable: Install osxcross from tools/osxcross${NC}"
fi

# Windows AMD64 (requires mingw-w64)
if command -v x86_64-w64-mingw32-gcc &> /dev/null; then
    build_platform "windows" "amd64" ".exe" "x86_64-w64-mingw32-gcc"
else
    echo -e "${YELLOW}Skipping Windows AMD64 - install mingw-w64${NC}"
fi

# Windows ARM64 (requires appropriate mingw toolchain)
if command -v aarch64-w64-mingw32-gcc &> /dev/null; then
    build_platform "windows" "arm64" ".exe" "aarch64-w64-mingw32-gcc"
else
    echo -e "${YELLOW}Skipping Windows ARM64 - requires ARM64 mingw toolchain${NC}"
fi

echo -e "${GREEN}Build complete!${NC}"
echo ""
echo "Release archives created in: $BUILD_DIR"
echo ""

# List created archives
if ls $BUILD_DIR/*.tar.gz $BUILD_DIR/*.zip 2>/dev/null >/dev/null; then
    echo "Created archives:"
    ls -lh $BUILD_DIR/*.tar.gz $BUILD_DIR/*.zip 2>/dev/null | awk '{print "  " $9 " (" $5 ")"}'
    echo ""
    
    # Generate checksums
    echo "Generating checksums..."
    cd $BUILD_DIR
    sha256sum *.tar.gz *.zip 2>/dev/null > checksums.txt
    echo -e "${GREEN}✓ Created checksums.txt${NC}"
    cd - > /dev/null
else
    echo -e "${YELLOW}No archives were created. Check build errors above.${NC}"
fi

echo ""
echo "To install required toolchains on Ubuntu/Debian:"
echo "  sudo apt-get install gcc-aarch64-linux-gnu gcc-mingw-w64-x86-64"
echo ""
echo "For macOS cross-compilation, see: https://github.com/tpoechtrager/osxcross"