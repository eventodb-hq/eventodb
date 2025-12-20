#!/usr/bin/env bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo ""
echo -e "${CYAN}=== Race Condition Detection ===${NC}"
echo ""

cd golang

# Fix for Xcode 16+ compatibility with Go race detector
# Downgrade SDK warnings to avoid breaking the build
export CGO_CFLAGS="-Wno-error=nullability-completeness -Wno-error=availability"

# Method 1: Try local race detection with CGO
echo -e "${CYAN}Method 1: Attempting local race detection with CGO...${NC}"
go clean -testcache

if CGO_ENABLED=1 go test ./... -race -timeout 120s 2>&1 | tee /tmp/race_local.log; then
    echo -e "${GREEN}✅ No race conditions detected!${NC}"
    exit 0
fi

# Check if failure was due to compiler/CGO issues
if grep -q "visionos\|unknown platform\|clang.*error" /tmp/race_local.log; then
    echo ""
    echo -e "${YELLOW}⚠️  Local CGO/compiler issues detected (Xcode SDK compatibility)${NC}"
    echo -e "${YELLOW}    This is a known issue with macOS Xcode 16+ and VisionOS SDK${NC}"
    echo ""
    
    # Method 2: Try with Docker
    echo -e "${CYAN}Method 2: Attempting race detection with Docker...${NC}"
    
    if ! command -v docker &> /dev/null; then
        echo -e "${YELLOW}Docker not found. Please install Docker to run race detection.${NC}"
        echo ""
        echo -e "${CYAN}Alternative: Run tests in CI/CD where CGO works properly${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}Using Docker to bypass local compiler issues...${NC}"
    
    # Go back to project root for Docker
    cd ..
    
    # Run tests in Docker with proper CGO support
    if docker run --rm \
        -v "$(pwd)/golang:/app" \
        -w /app \
        golang:1.23 \
        bash -c "go clean -testcache && go test ./... -race -timeout 120s"; then
        echo ""
        echo -e "${GREEN}✅ No race conditions detected (via Docker)!${NC}"
        exit 0
    else
        echo ""
        echo -e "${RED}❌ Race conditions or test failures detected!${NC}"
        echo -e "${YELLOW}Review the output above for details.${NC}"
        exit 1
    fi
else
    # Test failures unrelated to CGO
    echo ""
    echo -e "${RED}❌ Race conditions or test failures detected!${NC}"
    echo -e "${YELLOW}Review the output above for details.${NC}"
    exit 1
fi
