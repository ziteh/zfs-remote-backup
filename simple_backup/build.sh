#!/bin/bash
# Build script for Linux amd64

GOOS=linux GOARCH=amd64 go build -o zrb_simple
