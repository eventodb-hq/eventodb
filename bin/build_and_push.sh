#!/bin/bash

set -e  # Exit on error

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "==> Building production image..."
BUILD_LOG=$(mktemp)
s3dock --log-level 3 build eventodb --platform linux/amd64 --dockerfile Dockerfile --context . 2>&1 | tee "$BUILD_LOG"

# Extract the image name from the build output
# Looking for pattern: Successfully built <image_name>:<tag>
IMAGE_TAG=$(grep -o 'Successfully built [^ ]*' "$BUILD_LOG" | awk '{print $3}')
rm -f "$BUILD_LOG"

if [ -z "$IMAGE_TAG" ]; then
    echo "Error: Could not extract image tag from build output"
    exit 1
fi

echo ""
echo "==> Extracted image tag: $IMAGE_TAG"
echo ""

echo "==> Pushing image to s3dock..."
s3dock push "$IMAGE_TAG"

echo ""
echo "==> Promoting image to production..."
s3dock promote "$IMAGE_TAG" production

echo ""
echo "==> Done! Image $IMAGE_TAG has been built, pushed, and promoted to production."
