#!/usr/bin/env bash
set -euo pipefail
# ponytail: build both binaries, package into tarball.

VERSION="${1:-$(git describe --tags --always 2>/dev/null || echo 'dev')}"
OUTDIR="jobhunter-$VERSION"

echo "Building jobhunter $VERSION..."

# Build Go bridge
echo "→ Go scraper bridge..."
cd scraper
go build -o "../$OUTDIR/scraper" .
cd ..

# Build Rust binary
echo "→ Rust binary..."
cargo build --release
cp "target/release/jobhunter" "$OUTDIR/jobhunter"

# Copy config example
cp config.example.toml "$OUTDIR/config.example.toml"

# Create tarball
echo "→ Packaging..."
tar czf "jobhunter-$VERSION-x86_64-linux.tar.gz" "$OUTDIR"
rm -r "$OUTDIR"

echo "✅ jobhunter-$VERSION-x86_64-linux.tar.gz"
