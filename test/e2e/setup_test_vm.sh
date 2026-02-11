#!/bin/bash

# Resource settings for VM
CPU="2"
MEM="4G"
DISK="24G"

# Configuration
VM_NAME="zrb-test-vm"
TMP_DIR="./tmp/"
VM_USER="ubuntu"
AGE_VERSION="v1.3.1"

MINIO_URL="https://dl.min.io/server/minio/release/linux-arm64/minio"
MC_URL="https://dl.min.io/client/mc/release/linux-arm64/mc"
AGE_URL="https://github.com/FiloSottile/age/releases/download/${AGE_VERSION}/age-${AGE_VERSION}-linux-arm64.tar.gz"

MINIO_STORAGE_DIR="/home/$VM_USER/minio_data"

echo "--- Prepare ---"
# Prepare temp directory
mkdir -p "$TMP_DIR"
if [ ! -f "$TMP_DIR/.gitignore" ]; then
    touch "$TMP_DIR/.gitignore"
    echo "*" > "$TMP_DIR/.gitignore"
fi

# Download Minio
if [ ! -f "$TMP_DIR/minio" ]; then
    echo "Downloading Minio..."
    curl -L "$MINIO_URL" -o "$TMP_DIR/minio"
    # chmod +x "$TMP_DIR/minio"
fi

# Download MC
if [ ! -f "$TMP_DIR/mc" ]; then
    echo "Downloading Minio Client..."
    curl -L "$MC_URL" -o "$TMP_DIR/mc"
    # chmod +x "$TMP_DIR/mc"
fi

# Download and extract Age
if [ ! -f "$TMP_DIR/age" ]; then
    echo "Downloading and extracting Age..."
    curl -L "$AGE_URL" -o "$TMP_DIR/age.tar.gz"
    tar -xzf "$TMP_DIR/age.tar.gz" -C "$TMP_DIR" --strip-components 1
    rm "$TMP_DIR/age.tar.gz"
    # chmod +x "$TMP_DIR/age" "$TMP_DIR/age-keygen"
fi

echo "--- Launching Multipass VM: $VM_NAME ---"
# Check if VM already exists
if multipass info "$VM_NAME" >/dev/null 2>&1; then
    echo "VM $VM_NAME already exists. Deleting and purging..."
    multipass delete "$VM_NAME" --purge
fi
multipass launch --name "$VM_NAME" --cpus "$CPU" --memory "$MEM" --disk "$DISK" 24.04

# Copy binaries to VM
echo "--- Copy binaries to VM ---"
multipass exec "$VM_NAME" -- mkdir -p /home/$VM_USER/tmp
multipass transfer "$TMP_DIR/"* "$VM_NAME:/home/$VM_USER/tmp/"

# Setup env
echo "--- Setup environment inside VM ---"
multipass exec "$VM_NAME" -- sudo apt-get update
multipass exec "$VM_NAME" -- sudo apt-get install -y zfsutils-linux

multipass exec "$VM_NAME" -- bash -c "sudo mv /home/$VM_USER/tmp/* /usr/local/bin/"
multipass exec "$VM_NAME" -- bash -c "sudo chown root:root /usr/local/bin/*"
multipass exec "$VM_NAME" -- bash -c "sudo chmod 755 /usr/local/bin/* "

# Setup MinIO systemd service
echo "--- Setting up MinIO systemd service ---"
multipass exec "$VM_NAME" -- bash -c "sudo mkdir -p $MINIO_STORAGE_DIR"
multipass exec "$VM_NAME" -- bash -c "sudo chown $VM_USER:$VM_USER $MINIO_STORAGE_DIR"
multipass exec "$VM_NAME" -- bash -c "sudo bash -c 'cat <<EOT > /etc/systemd/system/minio.service
[Unit]
Description=MinIO
After=network.target

[Service]
Type=simple
User=$VM_USER
Group=$VM_USER
Environment=\"MINIO_ROOT_USER=admin\"
Environment=\"MINIO_ROOT_PASSWORD=password\"
ExecStart=/usr/local/bin/minio server $MINIO_STORAGE_DIR --address \"127.0.0.1:9000\" --console-address \":9001\"
Restart=on-failure
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOT'"

multipass exec "$VM_NAME" -- bash -c "sudo systemctl daemon-reload"
multipass exec "$VM_NAME" -- bash -c "sudo systemctl enable minio"
multipass exec "$VM_NAME" -- bash -c "sudo systemctl start minio"

# wait until MinIO is ready
multipass exec "$VM_NAME" -- bash -c '
  until curl -s http://127.0.0.1:9000/minio/health/live > /dev/null; do
    sleep 1
  done
'

multipass exec "$VM_NAME" -- /usr/local/bin/mc alias set myminio http://127.0.0.1:9000 admin password
multipass exec "$VM_NAME" -- /usr/local/bin/mc mb myminio/mybucket

multipass stop "$VM_NAME"
multipass snapshot "$VM_NAME" --name "initial"
multipass start "$VM_NAME"

echo "--- Complete ---"
echo "Access the VM using: multipass shell $VM_NAME"
