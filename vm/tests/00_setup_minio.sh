#!/bin/bash
set -e

VM="zrb-vm"

echo "=== Setting up MinIO on VM ==="

# Check if MinIO is already installed
echo ""
echo "Step 1: Check MinIO installation"
echo "----------------------------------------"
if multipass exec "$VM" -- test -f /usr/local/bin/minio; then
    echo "✓ MinIO binary already installed"
else
    echo "Installing MinIO binary..."
    multipass exec "$VM" -- bash -lc "
        wget -q https://dl.min.io/server/minio/release/linux-arm64/minio -O /tmp/minio
        sudo install /tmp/minio /usr/local/bin/
        sudo chmod +x /usr/local/bin/minio
        rm /tmp/minio
    "
    echo "✓ MinIO binary installed"
fi

# Check if MinIO client (mc) is installed
echo ""
echo "Step 2: Check MinIO client (mc) installation"
echo "----------------------------------------"
if multipass exec "$VM" -- test -f /usr/local/bin/mc; then
    echo "✓ MinIO client (mc) already installed"
else
    echo "Installing MinIO client..."
    multipass exec "$VM" -- bash -lc "
        wget -q https://dl.min.io/client/mc/release/linux-arm64/mc -O /tmp/mc
        sudo install /tmp/mc /usr/local/bin/
        sudo chmod +x /usr/local/bin/mc
        rm /tmp/mc
    "
    echo "✓ MinIO client installed"
fi

# Create MinIO data directory
echo ""
echo "Step 3: Create MinIO data directory"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "
    sudo mkdir -p /home/ubuntu/minio-data
    sudo chown -R ubuntu:ubuntu /home/ubuntu/minio-data
"
echo "✓ MinIO data directory created"

# Create systemd service for MinIO
echo ""
echo "Step 4: Create MinIO systemd service"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "
sudo tee /etc/systemd/system/minio.service > /dev/null <<'EOF'
[Unit]
Description=MinIO Object Storage
After=network.target

[Service]
Type=simple
User=ubuntu
Group=ubuntu
Environment=\"MINIO_ROOT_USER=admin\"
Environment=\"MINIO_ROOT_PASSWORD=password123\"
ExecStart=/usr/local/bin/minio server /home/ubuntu/minio-data --console-address :9001
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
"
echo "✓ MinIO systemd service created"

# Start MinIO service
echo ""
echo "Step 5: Start MinIO service"
echo "----------------------------------------"
multipass exec "$VM" -- sudo systemctl daemon-reload
multipass exec "$VM" -- sudo systemctl enable minio
multipass exec "$VM" -- sudo systemctl restart minio

# Wait for MinIO to start
echo "Waiting for MinIO to start..."
sleep 5

if multipass exec "$VM" -- bash -lc "curl -s http://127.0.0.1:9000/minio/health/live >/dev/null 2>&1"; then
    echo "✓ MinIO service started successfully"
else
    echo "✗ MinIO service failed to start"
    echo "Checking service status..."
    multipass exec "$VM" -- sudo systemctl status minio --no-pager
    exit 1
fi

# Configure mc alias
echo ""
echo "Step 6: Configure MinIO client alias"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "
    mc alias set myminio http://127.0.0.1:9000 admin password123 >/dev/null 2>&1 || true
"
echo "✓ MinIO client configured"

# Create test bucket
echo ""
echo "Step 7: Create test bucket"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "
    mc mb myminio/zrb-test >/dev/null 2>&1 || echo 'Bucket already exists'
"
echo "✓ Test bucket 'zrb-test' ready"

# Verify setup
echo ""
echo "Step 8: Verify MinIO setup"
echo "----------------------------------------"
BUCKET_LIST=$(multipass exec "$VM" -- mc ls myminio/ 2>&1)
if echo "$BUCKET_LIST" | grep -q "zrb-test"; then
    echo "✓ MinIO setup verified"
    echo "$BUCKET_LIST"
else
    echo "✗ MinIO setup verification failed"
    exit 1
fi

echo ""
echo "=== MinIO setup completed ==="
echo ""
echo "MinIO Details:"
echo "  Endpoint:     http://127.0.0.1:9000"
echo "  Console:      http://127.0.0.1:9001"
echo "  Access Key:   admin"
echo "  Secret Key:   password123"
echo "  Test Bucket:  zrb-test"
echo ""
echo "To access MinIO from host machine:"
echo "  multipass info zrb-vm  # Get VM IP"
echo "  # Then use http://<VM_IP>:9000"
