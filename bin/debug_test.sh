#!/bin/bash

# Run from project root
cd "$(dirname "$0")/.." || exit 1

TEST_DB_MODE=file KEEP_NAMESPACES=true bun test --test-name-pattern "$1"