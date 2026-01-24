#!/bin/bash

go build -o build/zrb_simple

# For TrueNAS CE (Debian based)
GOOS=linux GOARCH=amd64 go build -o build/zrb_simple_linux_amd64

GOOS=linux GOARCH=arm64 go build -o build/zrb_simple_linux_arm64

# VM Setup
multipass transfer build/zrb_simple_linux_arm64 zfs-minio:/tmp/
multipass exec zfs-minio -- sudo chmod +x /tmp/zrb_simple_linux_arm64
multipass exec zfs-minio -- sudo /tmp/zrb_simple_linux_arm64 -h
