#!/usr/bin/env sh
set -eu

VERSION="${1:-}"
EXPECTED_SHA256="${2:-}"
INSTALL_DIR="${HAMSTER_INTEGRATIONS_INSTALL_DIR:-/usr/local/bin}"

if [ -z "$VERSION" ] || [ -z "$EXPECTED_SHA256" ]; then
  echo "usage: install.sh <cli-vX.Y.Z> <expected-sha256>" >&2
  exit 2
fi

case "$VERSION" in
  cli-v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "invalid fixed CLI version: $VERSION" >&2; exit 2 ;;
esac

ASSET="hamster-integrations-linux-amd64.tar.gz"
URL="https://github.com/hamster-switch/server-integrations/releases/download/${VERSION}/${ASSET}"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT INT TERM

curl --fail --location --proto '=https' --tlsv1.2 --output "$TEMP_DIR/$ASSET" "$URL"
ACTUAL_SHA256="$(sha256sum "$TEMP_DIR/$ASSET" | awk '{print $1}')"
if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
  echo "SHA-256 mismatch; refusing installation" >&2
  exit 1
fi

tar -xzf "$TEMP_DIR/$ASSET" -C "$TEMP_DIR"
install -d -m 0755 "$INSTALL_DIR"
install -m 0755 "$TEMP_DIR/hamster-integrations-linux-amd64" "$INSTALL_DIR/hamster-integrations"
echo "installed $INSTALL_DIR/hamster-integrations ($VERSION)"
