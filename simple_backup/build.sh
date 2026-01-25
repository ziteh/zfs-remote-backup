#!/bin/bash

VM="zrb-vm"

go build -o build/zrb_simple

# For TrueNAS CE (Debian based)
GOOS=linux GOARCH=amd64 go build -o build/zrb_simple_linux_amd64

GOOS=linux GOARCH=arm64 go build -o build/zrb_simple_linux_arm64

# VM Setup
multipass transfer build/zrb_simple_linux_arm64 "$VM:/tmp/zrb_simple"
# multipass transfer zrb_simple_config.yaml "$VM:/tmp/"
multipass exec "$VM" -- sudo chmod +x /tmp/zrb_simple
multipass exec "$VM" -- sudo /tmp/zrb_simple -h
