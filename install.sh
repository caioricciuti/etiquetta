#!/usr/bin/env bash
set -euo pipefail

# ============================================
# Etiquetta Installer
# ============================================

VERSION="${ETIQUETTA_VERSION:-latest}"
INSTALL_DIR="${ETIQUETTA_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${ETIQUETTA_DATA_DIR:-/var/lib/etiquetta}"
WITH_SYSTEMD="${ETIQUETTA_SYSTEMD:-false}"
GITHUB_REPO="caioricciuti/etiquetta"
TMP_FILE=""
CHECKSUM_FILE=""
INSTALL_TMP=""
PREVIOUS_TMP=""

cleanup() {
  if [ -n "$TMP_FILE" ]; then
    rm -f "$TMP_FILE"
  fi
  if [ -n "$CHECKSUM_FILE" ]; then
    rm -f "$CHECKSUM_FILE"
  fi
  if [ -n "$INSTALL_TMP" ] && [ -e "$INSTALL_TMP" ]; then
    if [ -w "$INSTALL_TMP" ]; then
      rm -f "$INSTALL_TMP"
    else
      sudo rm -f "$INSTALL_TMP"
    fi
  fi
  if [ -n "$PREVIOUS_TMP" ] && [ -e "$PREVIOUS_TMP" ]; then
    if [ -w "$PREVIOUS_TMP" ]; then
      rm -f "$PREVIOUS_TMP"
    else
      sudo rm -f "$PREVIOUS_TMP"
    fi
  fi
}
trap cleanup EXIT

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${GREEN}✓${NC} $1"; }
warn()  { echo -e "${YELLOW}⚠${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; exit 1; }
step()  { echo -e "${BLUE}→${NC} $1"; }

# Parse arguments
for arg in "$@"; do
  case $arg in
    --with-systemd) WITH_SYSTEMD=true ;;
    --version=*) VERSION="${arg#*=}" ;;
    --help)
      echo "Etiquetta Installer"
      echo ""
      echo "Usage: install.sh [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --with-systemd    Install and enable systemd service (Linux)"
      echo "  --version=TAG     Install specific version (default: latest)"
      echo ""
      echo "Environment variables:"
      echo "  ETIQUETTA_INSTALL_DIR  Binary location (default: /usr/local/bin)"
      echo "  ETIQUETTA_DATA_DIR     Data directory (default: /var/lib/etiquetta)"
      exit 0
      ;;
  esac
done

detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) error "Unsupported architecture: $ARCH" ;;
  esac

  case "$OS" in
    linux|darwin) ;;
    *) error "Unsupported OS: $OS" ;;
  esac

  PLATFORM="${OS}-${ARCH}"
  info "Detected platform: $PLATFORM"
}

get_latest_version() {
  if [ "$VERSION" = "latest" ]; then
    step "Fetching latest version..."
    VERSION=$(curl -sL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    [ -z "$VERSION" ] && error "Failed to fetch latest version"
  fi
  info "Installing version: $VERSION"
}

download_binary() {
  BINARY_NAME="etiquetta-${PLATFORM}"
  DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${BINARY_NAME}"
  CHECKSUM_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/checksums.txt"

  step "Downloading $BINARY_NAME..."
  TMP_FILE=$(mktemp)
  CHECKSUM_FILE=$(mktemp)

  if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"; then
    error "Failed to download binary. Check if version $VERSION exists for $PLATFORM"
  fi

  if ! curl -fsSL "$CHECKSUM_URL" -o "$CHECKSUM_FILE"; then
    error "Release $VERSION has no checksums.txt; refusing an unverified installation"
  fi

  EXPECTED_CHECKSUM=$(awk -v name="$BINARY_NAME" '$2 == name || $2 == ("*" name) { print $1; exit }' "$CHECKSUM_FILE")
  [ -z "$EXPECTED_CHECKSUM" ] && error "No checksum published for $BINARY_NAME"

  if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL_CHECKSUM=$(sha256sum "$TMP_FILE" | awk '{ print $1 }')
  elif command -v shasum >/dev/null 2>&1; then
    ACTUAL_CHECKSUM=$(shasum -a 256 "$TMP_FILE" | awk '{ print $1 }')
  else
    error "A SHA-256 tool (sha256sum or shasum) is required"
  fi

  if [ "$ACTUAL_CHECKSUM" != "$EXPECTED_CHECKSUM" ]; then
    error "Checksum verification failed for $BINARY_NAME"
  fi

  chmod +x "$TMP_FILE"
  info "Downloaded and verified successfully"
}

install_binary() {
  step "Installing to $INSTALL_DIR/etiquetta..."

  if [ -w "$INSTALL_DIR" ]; then
    INSTALL_TMP=$(mktemp "$INSTALL_DIR/.etiquetta-install.XXXXXX")
    cp "$TMP_FILE" "$INSTALL_TMP"
    chmod 0755 "$INSTALL_TMP"

    if [ -f "$INSTALL_DIR/etiquetta" ]; then
      PREVIOUS_TMP=$(mktemp "$INSTALL_DIR/.etiquetta-previous.XXXXXX")
      cp -p "$INSTALL_DIR/etiquetta" "$PREVIOUS_TMP"
      mv -f "$PREVIOUS_TMP" "$INSTALL_DIR/etiquetta.previous"
      PREVIOUS_TMP=""
    fi

    mv -f "$INSTALL_TMP" "$INSTALL_DIR/etiquetta"
  else
    INSTALL_TMP=$(sudo mktemp "$INSTALL_DIR/.etiquetta-install.XXXXXX")
    sudo cp "$TMP_FILE" "$INSTALL_TMP"
    sudo chmod 0755 "$INSTALL_TMP"

    if sudo test -f "$INSTALL_DIR/etiquetta"; then
      PREVIOUS_TMP=$(sudo mktemp "$INSTALL_DIR/.etiquetta-previous.XXXXXX")
      sudo cp -p "$INSTALL_DIR/etiquetta" "$PREVIOUS_TMP"
      sudo mv -f "$PREVIOUS_TMP" "$INSTALL_DIR/etiquetta.previous"
      PREVIOUS_TMP=""
    fi

    sudo mv -f "$INSTALL_TMP" "$INSTALL_DIR/etiquetta"
  fi

  INSTALL_TMP=""
  rm -f "$TMP_FILE"
  TMP_FILE=""

  info "Binary installed: $INSTALL_DIR/etiquetta"
  if [ -f "$INSTALL_DIR/etiquetta.previous" ]; then
    info "Previous binary preserved: $INSTALL_DIR/etiquetta.previous"
  fi
}

setup_systemd() {
  [ "$WITH_SYSTEMD" != "true" ] && return
  [ "$OS" != "linux" ] && { warn "Systemd only available on Linux"; return; }

  step "Setting up systemd service..."

  # Create etiquetta user if not exists
  if ! id -u etiquetta &>/dev/null; then
    sudo useradd --system --no-create-home --shell /usr/sbin/nologin etiquetta
    info "Created system user: etiquetta"
  fi

  # Create data directory
  sudo mkdir -p "$DATA_DIR"
  sudo chown etiquetta:etiquetta "$DATA_DIR"
  sudo chmod 0700 "$DATA_DIR"
  info "Created data directory: $DATA_DIR"

  # Create systemd service
  sudo tee /etc/systemd/system/etiquetta.service > /dev/null << EOF
[Unit]
Description=Etiquetta Analytics
After=network.target

[Service]
Type=simple
User=etiquetta
Group=etiquetta
WorkingDirectory=$DATA_DIR
ExecStart=$INSTALL_DIR/etiquetta serve
Restart=always
RestartSec=5
TimeoutStopSec=120
UMask=0077

# Environment (read by binary via env var fallbacks)
Environment=ETIQUETTA_DATA_DIR=$DATA_DIR

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
EOF

  sudo systemctl daemon-reload
  sudo systemctl enable etiquetta
  info "Systemd service installed and enabled"
}

print_success() {
  echo ""
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${GREEN}  Etiquetta installed successfully!${NC}"
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo "Next steps:"
  echo ""

  if [ "$WITH_SYSTEMD" = "true" ] && [ "$OS" = "linux" ]; then
    echo "  # Run setup wizard"
    echo "  sudo -u etiquetta $INSTALL_DIR/etiquetta init --data=$DATA_DIR"
    echo ""
    echo "  # Start the service"
    echo "  sudo systemctl start etiquetta"
    echo ""
    echo "  # View logs"
    echo "  sudo journalctl -u etiquetta -f"
  else
    echo "  # Run setup wizard"
    echo "  etiquetta init"
    echo ""
    echo "  # Start the server"
    echo "  etiquetta serve"
  fi

  echo ""
  echo "Dashboard will be available at: http://localhost:3456"
  echo ""
}

main() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  Etiquetta Installer"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""

  detect_platform
  get_latest_version
  download_binary
  install_binary
  setup_systemd
  print_success
}

main "$@"
