#!/bin/bash
cd kids-real-ui
lsof -i :3555 | awk 'NR>1 {print $2}' | xargs kill -9
bun run dev