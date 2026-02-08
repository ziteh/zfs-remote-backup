# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Project Overview

ZFS Remote Backup (zrb) is a Go application for backing up ZFS snapshots to remote object storage (AWS S3, MinIO) using full, differential, and incremental backup strategies.

## Commands

### Build

```bash
make build              # Build for current platform
make build-linux        # Build for Linux amd64
make build-all          # Build for all platforms
```

### Run

```bash
./build/zrb genkey
./build/zrb backup --config config.yaml --task taskname --level 0
./build/zrb snapshot --config config.yaml --task taskname
./build/zrb list --config config.yaml --task taskname --source s3
./build/zrb restore --config config.yaml --task taskname --level 0 --target pool/dataset --private-key /path/to/key
```

### Testing

```bash
make test                               # Unit tests
vm/tests/01_prepare_env.sh              # VM setup
vm/tests/02_l0_backup.sh                # Integration test
```

## Architecture

### Directory Structure

```
cmd/zrb/              - Go source files (all package main)
docs/                 - Documentation
vm/                   - VM testing infrastructure
build/                - Build outputs
archive/              - Legacy implementations (Python, Rust)
```

### Pipeline

```
Config → Lock → List Snapshots → ZFS Send (BLAKE3 hash) → Split 3GB chunks
→ Worker Pool (4 concurrent): Encrypt (age) → BLAKE3 → S3 Upload
→ Manifest → Update Last Backup Reference → Cleanup → Unlock
```

### State Files

Located in `{base_dir}/run/{pool}/{dataset}/`:
- `backup_state.yaml` - Resumable state (parts_processed, parts_uploaded)
- `last_backup_manifest.yaml` - Tracks last backup per level
- `zrb.lock` - Concurrency lock with PID

### S3 Structure

```
{bucket}/{prefix}/
├── data/{pool}/{dataset}/{level}/{date}/    # Encrypted backup parts
└── manifests/{pool}/{dataset}/              # Backup manifests
```

## Tech Stack

- **Go 1.23**: age, aws-sdk-go-v2, urfave/cli, blake3
- **Testing**: Multipass VMs, shell scripts

## Code Style

- Go: standard gofmt
- All source files in cmd/zrb/ as package main
