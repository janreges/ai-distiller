#!/bin/bash

# Script to build and publish AI Distiller MCP npm package
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building and publishing AI Distiller MCP package${NC}"
echo ""

# Check if we're in the right directory
if [ ! -f "package.json" ]; then
    echo -e "${RED}Error: package.json not found. Run this script from mcp-npm directory${NC}"
    exit 1
fi

# Refuse to publish the placeholder version: its postinstall installs no binary
if [ "$(node -p "require('./package.json').version")" = "0.0.0-dev" ]; then
    echo -e "${RED}Error: package.json is at 0.0.0-dev (placeholder). Set the real release version before publishing by hand; the release workflow normally does this from the tag.${NC}"
    exit 1
fi

# Check if logged in to npm
if ! npm whoami &> /dev/null; then
    echo -e "${RED}Error: Not logged in to npm. Run 'npm login' first${NC}"
    exit 1
fi

# Clean previous builds
echo -e "${YELLOW}Cleaning previous builds...${NC}"
rm -rf dist/

# Install dependencies
echo -e "${YELLOW}Installing dependencies...${NC}"
npm install

# Build TypeScript
echo -e "${YELLOW}Building TypeScript...${NC}"
npm run build

if [ ! -f "dist/mcp-server-sdk.js" ]; then
    echo -e "${RED}Error: TypeScript build failed${NC}"
    exit 1
fi

# Create a test package
echo -e "${YELLOW}Creating test package...${NC}"
npm pack --dry-run

# Show what will be published
echo ""
echo -e "${GREEN}Package contents:${NC}"
npm pack --dry-run 2>&1 | grep -E "^npm notice [0-9]+B" | sort -k3 -h

echo ""
echo -e "${YELLOW}Ready to publish version $(node -p "require('./package.json').version")${NC}"
echo -e "This will publish to: ${GREEN}@janreges/ai-distiller-mcp${NC}"
echo ""
read -p "Continue with publish? (y/N) " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}Publishing to npm...${NC}"
    npm publish --access public
    
    if [ $? -eq 0 ]; then
        echo ""
        echo -e "${GREEN}✓ Successfully published!${NC}"
        echo ""
        echo "Users can now install with:"
        echo -e "  ${GREEN}npm install -g @janreges/ai-distiller-mcp${NC}"
        echo ""
        echo "Or use with npx:"
        echo -e "  ${GREEN}npx @janreges/ai-distiller-mcp${NC}"
    else
        echo -e "${RED}✗ Publish failed${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}Publish cancelled${NC}"
fi