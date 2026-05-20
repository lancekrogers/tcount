#!/usr/bin/env bash
# Publish the npm wrapper for an existing tcount GitHub release (backfill or manual).

set -euo pipefail

if [ $# -lt 1 ]; then
    echo "Usage: $0 <version> [dist-tag]"
    echo "Example: $0 0.4.2 latest"
    exit 1
fi

TAG="$1"
DIST_TAG="${2:-latest}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Create a temp copy so we don't mutate the working tree
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Staging npm package for tcount ${TAG} into temp dir..."
rsync -a "$ROOT_DIR/npm/" "$TMP_DIR/"

echo "Publishing @lancekrogers/tcount@${TAG} with tag '${DIST_TAG}'..."

NPM_PACKAGE_DIR="$TMP_DIR" \
NPM_PACKAGE_VERSION="$TAG" \
NPM_DIST_TAG="$DIST_TAG" \
NPM_PUBLISH_INTERACTIVE=never \
  "$ROOT_DIR/scripts/publish_npm_package.sh"
