#!/bin/bash

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Get the project root (one level up from bin/)
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT/backend"
go build .

lsof -i :3333 | grep LISTEN| awk 'NR>0 {print $2}' | xargs kill -9
###### postgres config 
# export DB_DRIVER=postgres
# export DB_HOST=localhost
# export DB_PORT=5466
# export DB_USER=postgres
# export DB_PASSWORD=postgres
# export DB_NAME=learning_app


###### tracks config 
export TRACKS_ENABLED=true
export TRACKS_S3_BUCKET=moo-staging
export TRACKS_S3_PREFIX=track_releases
export TRACKS_S3_REGION=eu-central-1
export TRACKS_S3_ENDPOINT=https://s3.eu-central-1.wasabisys.com
export TRACKS_ENVIRONMENT=staging
export TRACKS_POLL_INTERVAL=1m
export TRACKS_DOWNLOAD_DIR="$PROJECT_ROOT/tmp/data/tracks"

### this comes from .envrc or other local files!
# export TRACKS_S3_ACCESS_KEY=xxxx
# export TRACKS_S3_SECRET_KEY=xxxx

./replicache-backend -test-mode --data-dir "$PROJECT_ROOT/tmp/data"
# ./replicache-backend  --data-dir "$PROJECT_ROOT/tmp/data"
