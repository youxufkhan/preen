#!/bin/sh
# Preen installation script
# https://github.com/youxufkhan/preen
#
# Usage:
#   curl -fsSL https://youxufkhan.github.io/preen/install.sh | sh
#   curl -fsSL https://youxufkhan.github.io/preen/install.sh | sh -s -- --cli

set -e

REPO="youxufkhan/preen"
FALLBACK_TAG="v0.1.1"

# ANSI colors
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
DIM='\033[0;90m'
NC='\033[0m'

say() {
    printf "${CYAN}==>${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}==>${NC} %s\n" "$1"
}

warn() {
    printf "${RED}Warning:${NC} %s\n" "$1"
}

die() {
    printf "${RED}Error:${NC} %s\n" "$1" >&2
    exit 1
}

# Check OS and architecture
OS="$(uname -s)"
ARCH="$(uname -m)"

if [ "$OS" != "Linux" ]; then
    die "Preen is only available for Linux. Detected OS: $OS"
fi

case "$ARCH" in
    x86_64|amd64)
        ;;
    *)
        die "Preen pre-built binaries are currently x86_64 only. Detected: $ARCH. You can build from source with 'make'."
        ;;
esac

# Parse arguments
PREFER_CLI="false"
for arg in "$@"; do
    case "$arg" in
        --cli)
            PREFER_CLI="true"
            ;;
    esac
done

# Check for download tool
if command -v curl >/dev/null 2>&1; then
    FETCH="curl -fsSL"
elif command -v wget >/dev/null 2>&1; then
    FETCH="wget -qO-"
else
    die "Either curl or wget is required to install Preen."
fi

# Detect latest version
say "Checking latest Preen release..."
TAG=""
if command -v curl >/dev/null 2>&1; then
    TAG=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)
fi
if [ -z "$TAG" ]; then
    TAG="$FALLBACK_TAG"
fi
VERSION="${TAG#v}"
say "Installing Preen ${TAG}..."

# Detect sudo / root
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
    fi
fi

# Detect distribution
IS_DEBIAN_BASED="false"
if [ "$PREFER_CLI" != "true" ] && command -v apt-get >/dev/null 2>&1 && [ -f /etc/debian_version ]; then
    IS_DEBIAN_BASED="true"
fi

TMP_DIR="$(mktemp -d -t preen-install.XXXXXX)"
cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if [ "$IS_DEBIAN_BASED" = "true" ]; then
    say "Debian/Ubuntu detected. Installing desktop package (.deb)..."
    DEB_URL="https://github.com/${REPO}/releases/download/${TAG}/preen_${VERSION}_amd64.deb"
    DEB_FILE="${TMP_DIR}/preen_${VERSION}_amd64.deb"

    if command -v curl >/dev/null 2>&1; then
        curl -fSL "$DEB_URL" -o "$DEB_FILE"
    else
        wget -q "$DEB_URL" -O "$DEB_FILE"
    fi

    if [ -n "$SUDO" ]; then
        $SUDO apt-get install -y "$DEB_FILE"
    else
        apt-get install -y "$DEB_FILE"
    fi
else
    say "Installing standalone Preen CLI binary..."
    BIN_URL="https://github.com/${REPO}/releases/download/${TAG}/preen-linux-amd64"
    BIN_FILE="${TMP_DIR}/preen"

    if command -v curl >/dev/null 2>&1; then
        curl -fSL "$BIN_URL" -o "$BIN_FILE"
    else
        wget -q "$BIN_URL" -O "$BIN_FILE"
    fi
    chmod +x "$BIN_FILE"

    # Determine destination directory
    TARGET_DIR="/usr/local/bin"
    if [ -n "$SUDO" ]; then
        $SUDO install -Dm755 "$BIN_FILE" "${TARGET_DIR}/preen"
    elif [ "$(id -u)" -eq 0 ]; then
        install -Dm755 "$BIN_FILE" "${TARGET_DIR}/preen"
    else
        TARGET_DIR="${HOME}/.local/bin"
        mkdir -p "$TARGET_DIR"
        install -Dm755 "$BIN_FILE" "${TARGET_DIR}/preen"
        case ":$PATH:" in
            *:"$TARGET_DIR":*) ;;
            *)
                echo ""
                warn "${TARGET_DIR} is not in your PATH."
                echo "Add it with: export PATH=\"\$HOME/.local/bin:\$PATH\""
                ;;
        esac
    fi
fi

echo ""
success "Preen ${TAG} installed successfully!"
echo ""
echo "Quick start:"
echo "  preen clean --dry-run      # Preview reclaimable system space"
echo "  preen status --watch       # Real-time telemetry dashboard"
echo "  preen uninstall --list     # View installed applications"
echo "  preen --help               # View all commands and flags"
if [ "$IS_DEBIAN_BASED" = "true" ]; then
    echo "  preen-gui                  # Launch the KDE Kirigami desktop app"
fi
echo ""
