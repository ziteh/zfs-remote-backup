#!/bin/bash

# For TrueNAS CE (Debian based)
GOOS=linux GOARCH=amd64 go build -o build/zrb_simple_linux_amd64

GOOS=linux GOARCH=arm64 go build -o build/zrb_simple_linux_arm64
