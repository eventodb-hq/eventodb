#!/bin/bash

# Disable CGO for consistent builds across platforms
export CGO_ENABLED=0

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Install dependencies if node_modules doesn't exist
if [ ! -d "node_modules" ]; then
    echo -e "${YELLOW}→ Installing dependencies...${NC}"
    npm install
fi

echo -e "${YELLOW}→ Building TypeScript...${NC}"
npm run build

echo -e "${YELLOW}→ Running tests...${NC}"
npm test

echo -e "${GREEN}✓ All tests passed!${NC}"
