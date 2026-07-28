#!/bin/bash
set -e

IMAGE_NAME="${IMAGE_NAME:-ceph:16.2.14-fix4}"

echo "=== Ceph 16.2.14-fix4: Build & Package ==="
echo "Branch: $(git branch --show-current)"
echo "HEAD:   $(git log --oneline -1)"
echo "Image:  $IMAGE_NAME"
echo ""

# Step 1: Build
echo ">>> Step 1/2: Compiling (incremental if build/ exists)..."
./build-complete.sh

# Step 2: Package
echo ""
echo ">>> Step 2/2: Packaging production image..."
IMAGE_NAME="$IMAGE_NAME" ./package-production.sh

echo ""
echo "=== All done ==="
echo ""
docker run --rm --entrypoint="" "$IMAGE_NAME" ceph --version
