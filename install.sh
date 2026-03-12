#!/bin/sh
# xquare CLI installer
# Usage: curl -fsSL https://raw.githubusercontent.com/team-xquare/xquare-cli/main/install.sh | sh

set -e

REPO="team-xquare/xquare-cli"
BINARY="xquare"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)
    echo "Unsupported OS: $OS"
    echo "Download manually from: https://github.com/${REPO}/releases"
    exit 1
    ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Resolve latest version
if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
fi

if [ -z "$VERSION" ]; then
  echo "Could not determine latest version. Set VERSION env var to override."
  exit 1
fi

# Build download URL
ARCHIVE="${BINARY}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

echo "Installing xquare ${VERSION} (${OS}/${ARCH})..."

# Download and extract
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "${TMP}/${ARCHIVE}"
tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP"

# Install binary
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  # Fallback to ~/.local/bin
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  echo "Note: installed to $INSTALL_DIR (add to PATH if needed)"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"

echo ""
echo "xquare ${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Get started:"
echo "  xquare login"
echo "  xquare project list"
