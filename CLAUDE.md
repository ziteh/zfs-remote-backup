# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Project Overview

ZFS Remote Backup (zrb) is a Go application for backing up ZFS snapshots to remote object storage (AWS S3, MinIO) using full, differential, and incremental backup strategies.

## Commands

### Build

```bash
make build              # Production build: Linux amd64 → build/zrb
make build-dev          # Dev/VM build: Linux native arch → build/zrb_dev
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
make test               # Unit tests only
make test-unit          # Unit tests only (./internal/...)
make test-e2e-vm        # E2E tests on Multipass VM (./test/e2e/)
make test-all           # Unit + E2E tests
make test-coverage      # Coverage report
```

## Architecture

### Directory Structure

```
cmd/zrb/main.go         - CLI entry point only
internal/
├── config/             - Configuration types and loading
├── logging/            - Multi-handler logger
├── lock/               - File-based concurrency lock
├── crypto/             - Age encryption, BLAKE3 hashing
├── zfs/                - ZFS send/split, snapshots
├── remote/             - S3 backend interface and implementation
├── manifest/           - Backup manifest types and I/O
├── util/               - Path builders, setup helpers
├── backup/             - Backup command logic
├── restore/            - Restore command logic
├── list/               - List command logic
└── keys/               - Key generation and testing
test/e2e/               - End-to-end tests
vm/                     - VM testing infrastructure
docs/                   - Documentation
build/                  - Build outputs
archive/                - Legacy implementations (Python, Rust)
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

Specifically designed to utilize **AWS S3 Glacier Deep Archive** for data storage. Due to its inherent characteristics, to avoid incurring substantial costs, data cannot be accessed immediately after upload. Additionally, API operation fees are quite high, necessitating the minimization of API calls.

```
{bucket}/{prefix}/
├── data/{pool}/{dataset}/{level}/{date}/    # Encrypted backup parts
└── manifests/{pool}/{dataset}/              # Backup manifests
```

## Tech Stack

- **Go 1.24**: age, aws-sdk-go-v2, urfave/cli, blake3
- **Testing**: testify, Multipass VMs

## Code Style

- Go: standard gofmt
- Business logic in `internal/` packages, CLI entry in `cmd/zrb/main.go`
- Unit tests co-located with code: `internal/<pkg>/<pkg>_test.go`

## Code Development Principles

- This project adheres to **Semantic Versioning** and **Conventional Commits**.
- Prefer **Fail Fast** over Defensive Programming.
- Prefer **explicit** over implicit defaults or fallbacks.
- No need to provide too many documents (.md), or even any at all.
- No need to provide too many comments.
- This project has not yet released an official version (>=1.0.0); modifications do not require backward compatibility.
