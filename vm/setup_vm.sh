#!/bin/bash
set -e

VM_NAME="zrb-vm"
SETUP_SCRIPT="init.sh"
TEST_SCRIPT="test.sh"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'
log_info() {
  echo -e "${BLUE}[INFO]${NC} $1"
}
log_success() {
  echo -e "${GREEN}[SUCCESS]${NC} $1"
}
log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

# Check if multipass is installed
if ! command -v multipass &> /dev/null; then
  log_error "Multipass is not installed."
  exit 1
fi

# Download required binaries
DOWNLOAD_DIR="./downloads"
mkdir -p "$DOWNLOAD_DIR"

# gitignore
touch "$DOWNLOAD_DIR/.gitignore"
echo "*" >> "$DOWNLOAD_DIR/.gitignore"


log_info "Downloading required binaries..."

# Age
if [ ! -f "$DOWNLOAD_DIR/age.tar.gz" ]; then
  log_info "Downloading age..."
  curl -L -o "$DOWNLOAD_DIR/age.tar.gz" https://github.com/FiloSottile/age/releases/download/v1.3.1/age-v1.3.1-linux-arm64.tar.gz
else
  log_info "age already downloaded."
fi

# MinIO Server (arm64)
if [ ! -f "$DOWNLOAD_DIR/minio" ]; then
  log_info "Downloading MinIO Server..."
  curl -L -o "$DOWNLOAD_DIR/minio" https://dl.min.io/server/minio/release/linux-arm64/minio
else
  log_info "MinIO Server already downloaded."
fi
if [ ! -f "$DOWNLOAD_DIR/mc" ]; then
  log_info "Downloading MinIO Client..."
  curl -L -o "$DOWNLOAD_DIR/mc" https://dl.min.io/client/mc/release/linux-arm64/mc
else
  log_info "MinIO Client already downloaded."
fi

log_success "Downloads completed."

# Create and setup VM
log_info "Creating Multipass VM..."

# Check if VM already exists
if multipass list | grep -q "$VM_NAME"; then
  log_info "VM '$VM_NAME' already exists"
  read -p "Do you want to delete and recreate it? (y/N): " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    log_info "Stopping and deleting existing VM..."
    multipass stop "$VM_NAME" 2>/dev/null || true
    multipass delete "$VM_NAME"
    multipass purge
  else
    log_info "Using existing VM"
  fi
fi

# Start VM
if ! multipass list | grep -q "$VM_NAME"; then
  multipass launch -n "$VM_NAME" -c 4 -m 8G -d 30G 22.04
else
  multipass start "$VM_NAME" 2>/dev/null || true
fi

VM_IP=$(multipass info "$VM_NAME" | grep "IPv4" | awk '{print $2}')
log_info "VM is running at $VM_IP"
echo ""

# Setup VM
log_info "Setup script..."
if [ ! -f "$SETUP_SCRIPT" ]; then
  log_error "Setup script not found: $SETUP_SCRIPT"
  exit 1
fi
multipass transfer "$SETUP_SCRIPT" "$VM_NAME:/tmp/"
# Transfer downloaded binaries
log_info "Transferring downloaded binaries..."
for file in "$DOWNLOAD_DIR"/*; do
  if [ -f "$file" ]; then
    multipass transfer "$file" "$VM_NAME:/tmp/"
  fi
done
log_success "Binaries transferred."

# Create age private key on the VM for tests
log_info "Installing test Age private key on VM"
multipass exec "$VM_NAME" -- bash -lc 'cat > /home/ubuntu/age_private_key.txt <<"KEY"
AGE-SECRET-KEY-1Q0DFM9WHSEV0K7MPJA02LFQV6CE28JAZ6EQTVWG9KRLSMWS74TFSA7SNWJ
KEY
'
multipass exec "$VM_NAME" -- sudo bash "/tmp/$SETUP_SCRIPT"
log_success "Setup script completed"
echo ""

log_info "Test script..."
if [ ! -f "$TEST_SCRIPT" ]; then
  log_error "script not found: $TEST_SCRIPT"
  exit 1
fi
multipass transfer "$TEST_SCRIPT" "$VM_NAME:/tmp/"
multipass exec "$VM_NAME" -- sudo bash "/tmp/$TEST_SCRIPT"
log_success "Test script completed"
echo ""

log_success "Done"
