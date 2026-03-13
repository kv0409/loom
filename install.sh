#!/usr/bin/env bash
# Install loom CLI from GitHub Releases.
# Usage: curl -sSL https://raw.githubusercontent.com/kv0409/loom/main/install.sh | bash
set -euo pipefail

REPO="kv0409/loom"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

echo "Detecting platform... ${OS}/${ARCH}"

# Get latest release tag
TAG=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
VERSION="${TAG#v}"
echo "Latest version: ${VERSION}"

ARCHIVE="loom_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${ARCHIVE}..."
curl -sSfL "$URL" -o "${TMP}/${ARCHIVE}"

echo "Installing to ${INSTALL_DIR}/loom..."
tar xzf "${TMP}/${ARCHIVE}" -C "$TMP"

if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/loom" "${INSTALL_DIR}/loom"
else
  sudo mv "${TMP}/loom" "${INSTALL_DIR}/loom"
fi
chmod +x "${INSTALL_DIR}/loom"

echo "loom ${VERSION} installed. Run 'loom --help' to get started."
