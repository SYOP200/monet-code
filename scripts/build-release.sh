#!/usr/bin/env bash
#
# Build the assets that install.sh downloads from a GitHub release:
#   monet-tui-{darwin,linux}-{amd64,arm64}
#   monet-cli.tar.gz   self-contained CLI (package + vendored dependencies)
#   checksums.txt
#
# Usage: scripts/build-release.sh [version]
#
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-$(sed -n 's/^const Version = "\(.*\)"/\1/p' internal/ui/app.go)}"
[ -n "$VERSION" ] || { echo "could not read the version" >&2; exit 1; }

DIST="dist"
PYTHON="${PYTHON:-python3}"

echo "◆ building Monet Code $VERSION"
rm -rf "$DIST" build/cli
mkdir -p "$DIST"

# Keep the CLI catalog in sync with the Go catalog.
cp internal/catalog/models.json monet_cli/models.json

for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do
	os="${target%/*}"
	arch="${target#*/}"
	out="$DIST/monet-tui-$os-$arch"
	echo "  → $out"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
		-trimpath -ldflags "-s -w" -o "$out" ./cmd/monet-tui
done

echo "  → $DIST/monet-cli.tar.gz"
mkdir -p build/cli
if [ -x .venv/bin/pip ]; then
	.venv/bin/pip install --quiet --no-compile --target build/cli .
else
	"$PYTHON" -m pip install --quiet --no-compile --target build/cli .
fi
tar -czf "$DIST/monet-cli.tar.gz" -C build/cli .
[ -f build/cli/monet_cli/__main__.py ] || { echo "the CLI bundle is missing monet_cli/__main__.py" >&2; exit 1; }

(
	cd "$DIST"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum monet-tui-* monet-cli.tar.gz >checksums.txt
	else
		shasum -a 256 monet-tui-* monet-cli.tar.gz >checksums.txt
	fi
)

echo "  ✓ $DIST/checksums.txt"
cat "$DIST/checksums.txt"
