#!/bin/bash
set -e

# Ensure VERSION file exists
if [ ! -f "VERSION" ]; then
    echo "Error: VERSION file is required for building"
    echo "Create a VERSION file with the version number (e.g., echo '1.0.4' > VERSION)"
    exit 1
fi

# Validate VERSION file content
VERSION=$(cat VERSION | tr -d '\n\r' | tr -d ' ')
if [ -z "$VERSION" ]; then
    echo "Error: VERSION file is empty"
    exit 1
fi

# Get build information
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S_UTC')
BUILD_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo "Building skoob-agent with build info:"
echo "  Version: v$VERSION"
echo "  Time: $BUILD_TIME"
echo "  Hash: $BUILD_HASH"

# Build with ldflags to inject build information
go build -ldflags "-X main.version=$VERSION -X main.buildTime=$BUILD_TIME -X main.buildHash=$BUILD_HASH" -o skoob-agent .

echo "Build complete: skoob-agent"