#!/bin/bash

# Build script for MCP server releases
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Read version
VERSION=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# LDFLAGS
LDFLAGS="-X 'main.serverVersion=$VERSION' \
         -X 'main.gitCommit=$GIT_COMMIT' \
         -X 'main.buildDate=$BUILD_DATE'"

# Build directory
BUILD_DIR="build/mcp-releases"
mkdir -p $BUILD_DIR

echo -e "${GREEN}Building AI Distiller MCP Server v$VERSION${NC}"
echo ""

# Function to build for a platform
build_platform() {
    local GOOS=$1
    local GOARCH=$2
    local EXT=$3
    local CC=$4
    local BINARY_NAME="aid-mcp$EXT"
    local OUTPUT="$BUILD_DIR/aid-mcp_${VERSION}_${GOOS}_${GOARCH}$EXT"
    
    echo -e "${YELLOW}Building MCP for $GOOS/$GOARCH...${NC}"
    
    if [ -z "$CC" ]; then
        # Native build
        cd mcp-npm && CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH go build -tags "cgo" -ldflags "$LDFLAGS" -o "../$OUTPUT" ./cmd/aid-mcp && cd ..
    else
        # Cross-compile with specific compiler
        cd mcp-npm && CC=$CC CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH go build -tags "cgo" -ldflags "$LDFLAGS" -o "../$OUTPUT" ./cmd/aid-mcp && cd ..
    fi
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Built: $BINARY_NAME for $GOOS/$GOARCH${NC}"
        
        # Create tar.gz archive
        cd "$BUILD_DIR"
        tar -czf "aid-mcp_${VERSION}_${GOOS}_${GOARCH}.tar.gz" "aid-mcp_${VERSION}_${GOOS}_${GOARCH}$EXT"
        rm "aid-mcp_${VERSION}_${GOOS}_${GOARCH}$EXT"
        cd - > /dev/null
        
        echo -e "${GREEN}✓ Created: aid-mcp_${VERSION}_${GOOS}_${GOARCH}.tar.gz${NC}"
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
elif command -v x86_64-apple-darwin*-clang &> /dev/null 2>&1; then
    CC=$(command -v x86_64-apple-darwin*-clang | head -1)
    build_platform "darwin" "amd64" "" "$CC"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    build_platform "darwin" "amd64" "" ""
else
    echo -e "${YELLOW}Skipping macOS AMD64 - requires osxcross or macOS host${NC}"
fi

# macOS ARM64 (requires osxcross or Darwin host)
if command -v oa64-clang &> /dev/null; then
    build_platform "darwin" "arm64" "" "oa64-clang"
elif command -v aarch64-apple-darwin*-clang &> /dev/null 2>&1; then
    CC=$(command -v aarch64-apple-darwin*-clang | head -1)
    build_platform "darwin" "arm64" "" "$CC"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    build_platform "darwin" "arm64" "" ""
else
    echo -e "${YELLOW}Skipping macOS ARM64 - requires osxcross or macOS host${NC}"
fi

# Windows AMD64 (requires mingw-w64)
if command -v x86_64-w64-mingw32-gcc &> /dev/null; then
    build_platform "windows" "amd64" ".exe" "x86_64-w64-mingw32-gcc"
else
    echo -e "${YELLOW}Skipping Windows AMD64 - install mingw-w64${NC}"
fi

echo -e "${GREEN}Build complete!${NC}"
echo ""
echo "MCP release archives created in: $BUILD_DIR"
echo ""

# List created archives
if ls $BUILD_DIR/*.tar.gz 2>/dev/null >/dev/null; then
    echo "Created archives:"
    ls -lh $BUILD_DIR/*.tar.gz | awk '{print "  " $9 " (" $5 ")"}'
    echo ""
    
    # Generate checksums
    echo "Generating checksums..."
    cd $BUILD_DIR
    sha256sum *.tar.gz > checksums.txt
    echo -e "${GREEN}✓ Created checksums.txt${NC}"
    cd - > /dev/null
else
    echo -e "${YELLOW}No archives were created. Check build errors above.${NC}"
fi