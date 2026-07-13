#!/usr/bin/env bash
set -euo pipefail

REPO="sambaths/loop"
PREFIX="${HOME}/.local"
VERSION="${VERSION:-}"

usage() {
	cat <<EOF
Usage: $(basename "$0") [PREFIX] [--dir BIN_DIR] [--version VERSION]

Install the loop binary from GitHub releases.

PREFIX defaults to \$HOME/.local, with fallback to \$HOME if
\$HOME/.local/bin is not writable.

Options:
  PREFIX            Install to PREFIX/bin/loop (positional argument)
  --dir BIN_DIR     Install directly to BIN_DIR (overrides PREFIX)
  --version VER     Version tag to install (default: latest)
  -h, --help        Show this help
EOF
	exit 0
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--dir)
			BIN_DIR="$2"
			shift 2
			;;
		--version)
			VERSION="$2"
			shift 2
			;;
		-h|--help) usage ;;
		-*)
			echo "unknown option: $1"
			usage
			;;
		*)
			PREFIX="$1"
			shift
			;;
	esac
done

# Resolve BIN_DIR
if [ -z "${BIN_DIR:-}" ]; then
	if [ -w "${PREFIX}/bin" ] 2>/dev/null; then
		BIN_DIR="${PREFIX}/bin"
	else
		if touch "${PREFIX}/.writable_test" 2>/dev/null; then
			rm -f "${PREFIX}/.writable_test"
			BIN_DIR="${PREFIX}/bin"
		else
			PREFIX="${HOME}"
			BIN_DIR="${HOME}/bin"
		fi
	fi
fi

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
	x86_64) ARCH="amd64" ;;
	aarch64) ARCH="arm64" ;;
esac
case "$OS" in
	linux|darwin) EXT="tar.gz" ;;
	mingw*|msys*|cygwin*) OS="windows"; EXT="zip" ;;
	*) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Resolve latest version if not specified
if [ -z "$VERSION" ]; then
	VERSION=$(curl -sfL "https://api.github.com/repos/$REPO/releases/latest" \
		| grep '"tag_name"' | cut -d'"' -f4)
fi
if [ -z "$VERSION" ]; then
	echo "Error: no release found at github.com/$REPO"
	exit 1
fi
VERSION="${VERSION#v}"

# Download and install
ARCHIVE="loop_${VERSION}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/$REPO/releases/download/v${VERSION}/$ARCHIVE"

mkdir -p "$BIN_DIR"
echo "==> Downloading loop ${VERSION} for ${OS}/${ARCH} ..."
curl -fsL "$URL" -o "/tmp/$ARCHIVE"

if [ "$EXT" = "zip" ]; then
	unzip -o "/tmp/$ARCHIVE" -d "$BIN_DIR" loop.exe
else
	tar -xzf "/tmp/$ARCHIVE" -C "$BIN_DIR" loop
fi

rm "/tmp/$ARCHIVE"
chmod +x "${BIN_DIR}/loop"
echo "==> Installed loop ${VERSION} to ${BIN_DIR}/loop"
echo ""
echo "    Ensure ${BIN_DIR} is in your PATH:"
echo "      export PATH=\"${BIN_DIR}:\$PATH\""
case "${SHELL:-}" in
	*/zsh) echo "      echo 'export PATH=\"${BIN_DIR}:\$PATH\"' >> ~/.zshrc" ;;
	*/bash) echo "      echo 'export PATH=\"${BIN_DIR}:\$PATH\"' >> ~/.bashrc" ;;
esac