#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}→ Building TypeScript...${NC}"
npm run build

echo -e "${YELLOW}→ Running tests...${NC}"
npm test

echo -e "${GREEN}✓ All tests passed!${NC}"
