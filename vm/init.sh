#!/bin/bash

STORAGE_PATH="/home/ubuntu/minio_storage"
MINIO_BIN="/usr/local/bin/minio"
MC_BIN="/usr/local/bin/mc"

# Install OpenZFS
sudo apt-get update
sudo apt-get install -y zfsutils-linux

# Setup ZFS
if ! zpool list testpool > /dev/null 2>&1; then
    truncate -s 2G /home/ubuntu/zfs_disk.img
    sudo zpool create testpool /home/ubuntu/zfs_disk.img
    sudo zfs create testpool/backup_data
    sudo chown -R ubuntu:ubuntu /testpool/backup_data
fi

# Install shasum (via coreutils)
echo "Installing additional tools..."
sudo apt-get install -y coreutils wget

# Install age
echo "Installing age..."
wget -q https://github.com/FiloSottile/age/releases/download/v1.1.1/age-v1.1.1-linux-amd64.tar.gz -O age.tar.gz
if [ -f age.tar.gz ]; then
    tar -xzf age.tar.gz
    if [ -d age ]; then
        sudo mv age/age /usr/local/bin/
        sudo mv age/age-keygen /usr/local/bin/
        rm -rf age.tar.gz age/
        echo "age installed successfully."
    else
        echo "Failed to extract age."
        exit 1
    fi
else
    echo "Failed to download age."
    exit 1
fi

# Install b3sum (BLAKE3)
echo "Installing b3sum..."
wget -q https://github.com/BLAKE3-team/BLAKE3/releases/download/1.8.3/b3sum_linux_x64_bin -O b3sum
if [ -f b3sum ]; then
    chmod +x b3sum
    sudo mv b3sum /usr/local/bin/
    echo "b3sum installed successfully."
else
    echo "Failed to download b3sum."
    exit 1
fi

echo "Additional tools installed successfully."

# Install MinIO
echo "Downloading MinIO Server and Client..."
ARCH=$(uname -m)
if [ "$ARCH" = "aarch64" ]; then
    # Server
    wget -q https://dl.min.io/server/minio/release/linux-arm64/minio -O minio
    # Client
    wget -q https://dl.min.io/client/mc/release/linux-arm64/mc -O mc
else
    # Server
    wget -q https://dl.min.io/server/minio/release/linux-amd64/minio -O minio
    # Client
    wget -q https://dl.min.io/client/mc/release/linux-amd64/mc -O mc
fi
chmod +x minio mc
sudo mv minio $MINIO_BIN
sudo mv mc $MC_BIN
export MC_HOST_myminio=http://admin:password123@127.0.0.1:9000

# Create MinIO systemd service
mkdir -p $STORAGE_PATH
sudo bash -c "cat <<EOT > /etc/systemd/system/minio.service
[Unit]
Description=MinIO
After=network.target

[Service]
Type=simple
User=ubuntu
Group=ubuntu
Environment=\"MINIO_ROOT_USER=admin\"
Environment=\"MINIO_ROOT_PASSWORD=password123\"
ExecStart=$MINIO_BIN server $STORAGE_PATH --address \":9000\" --console-address \":9001\"
Restart=always
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOT"

# Start service
sudo systemctl daemon-reload
sudo systemctl enable minio
sudo systemctl start minio

echo "MinIO service started successfully."
