#!/bin/bash

SNAPSHOT_POOL="gen_pool_1"
OUTPUT_FILE="/mnt/gen_pool_1/temporarily/zfs_snapshots.txt"

if [[ "$1" == "--manual" ]]; then
    echo "Manually triggered: Listing snapshots..."
    zfs list -H -t snapshot -o name,used,avail,refer ${SNAPSHOT_POOL} > ${OUTPUT_FILE}
    echo "Snapshot list saved to ${OUTPUT_FILE}"
else
    echo "Scheduled task: Listing snapshots..."
    zfs list -H -t snapshot -o name,used,avail,refer ${SNAPSHOT_POOL} > ${OUTPUT_FILE}
    echo "Snapshot list saved to ${OUTPUT_FILE}"
fi
