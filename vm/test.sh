#!/bin/bash
set -e

ZFS_POOL="testpool"
ZFS_DATASET="testpool/backup_data"
ZFS_MOUNT="/testpool/backup_data"
MINIO_ALIAS="myminio"
MINIO_BUCKET="zfs-backups"
MC_BIN="/usr/local/bin/mc"

echo "Checking ZFS pool status..."
zpool status $ZFS_POOL
echo ""

echo "Checking ZFS dataset..."
zfs list $ZFS_DATASET
echo ""

# Create test files
echo "Creating test files in ZFS dataset..."
mkdir -p $ZFS_MOUNT/test-dir
echo "Test file 1 - $(date)" > $ZFS_MOUNT/test-file-1.txt
echo "Test file 2 - $(date)" > $ZFS_MOUNT/test-file-2.txt
dd if=/dev/urandom of=$ZFS_MOUNT/test-binary.bin bs=1M count=10 2>/dev/null
echo "Files created in $ZFS_MOUNT"
ls -lh $ZFS_MOUNT/
echo ""

# Take ZFS snapshot
echo "Taking ZFS snapshot..."
SNAPSHOT_NAME="$ZFS_DATASET@backup-$(date +%s)"
zfs snapshot $SNAPSHOT_NAME
echo "Snapshot created: $SNAPSHOT_NAME"
echo ""

# Check MinIO connectivity
echo "Testing MinIO connectivity..."
$MC_BIN ls $MINIO_ALIAS --insecure
echo ""

# Upload files to MinIO
echo "Uploading files to MinIO..."
$MC_BIN cp $ZFS_MOUNT/test-file-1.txt $MINIO_ALIAS/$MINIO_BUCKET/ --insecure
$MC_BIN cp $ZFS_MOUNT/test-file-2.txt $MINIO_ALIAS/$MINIO_BUCKET/ --insecure
$MC_BIN cp $ZFS_MOUNT/test-binary.bin $MINIO_ALIAS/$MINIO_BUCKET/ --insecure
echo "Files uploaded"
echo ""

# List uploaded files
echo "Listing uploaded files in MinIO..."
$MC_BIN ls --recursive $MINIO_ALIAS/$MINIO_BUCKET/ --insecure
echo ""

# Verify file integrity
echo "Verifying uploaded files..."
TEMP_DIR=$(mktemp -d)
$MC_BIN cp $MINIO_ALIAS/$MINIO_BUCKET/test-file-1.txt $TEMP_DIR/ --insecure
if diff -q $ZFS_MOUNT/test-file-1.txt $TEMP_DIR/test-file-1.txt > /dev/null; then
  echo "File integrity verified"
else
  echo "File integrity check failed"
fi
rm -rf $TEMP_DIR
echo ""

# Show ZFS dataset info
echo "ZFS dataset information..."
zfs get used,available,compression $ZFS_DATASET
echo ""
