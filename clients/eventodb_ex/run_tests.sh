#!/bin/bash
#
# Run Elixir SDK tests against EventoDB server
#
# Expects EVENTODB_URL environment variable to be set
# Example: EVENTODB_URL=http://localhost:6789 ./run_tests.sh
#


# Disable CGO for consistent builds across platforms
export CGO_ENABLED=0

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [ -z "$EVENTODB_URL" ]; then
    echo -e "${RED}Error: EVENTODB_URL environment variable not set${NC}"
    exit 1
fi

echo -e "${YELLOW}=== Elixir SDK Tests ===${NC}"
echo -e "${YELLOW}Server: $EVENTODB_URL${NC}"

# Check if mix is installed
if ! command -v mix &> /dev/null; then
    echo -e "${RED}Error: mix (Elixir) not found. Please install Elixir.${NC}"
    exit 1
fi

# Get dependencies if needed
if [ ! -d "deps" ]; then
    echo -e "${YELLOW}Installing dependencies...${NC}"
    mix deps.get
fi

# Compile if needed
if [ ! -d "_build" ]; then
    echo -e "${YELLOW}Compiling...${NC}"
    mix compile
fi

# Wait for server to be ready
echo -e "${YELLOW}Waiting for server...${NC}"
for i in {1..30}; do
    if curl -s "$EVENTODB_URL/health" > /dev/null 2>&1; then
        echo -e "${GREEN}Server ready!${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}Server not responding at $EVENTODB_URL${NC}"
        exit 1
    fi
    sleep 0.1
done

# Run tests
echo -e "${YELLOW}Running tests...${NC}"
export EVENTODB_URL
export EVENTODB_ADMIN_TOKEN
mix test --trace

TEST_EXIT_CODE=$?

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✓ Elixir SDK tests passed!${NC}"
else
    echo -e "${RED}✗ Elixir SDK tests failed!${NC}"
fi

exit $TEST_EXIT_CODE
