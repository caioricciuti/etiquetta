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

  step "Downloading $BINARY_NAME..."
  TMP_FILE=$(mktemp)

  if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"; then
    rm -f "$TMP_FILE"
    error "Failed to download binary. Check if version $VERSION exists for $PLATFORM"
  fi

  chmod +x "$TMP_FILE"
  info "Downloaded successfully"
}

install_binary() {
  step "Installing to $INSTALL_DIR/etiquetta..."

  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_FILE" "$INSTALL_DIR/etiquetta"
  else
    sudo mv "$TMP_FILE" "$INSTALL_DIR/etiquetta"
  fi

  info "Binary installed: $INSTALL_DIR/etiquetta"
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
