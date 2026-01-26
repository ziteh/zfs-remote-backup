# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ZFS Remote Backup (zrb) backs up ZFS snapshots to remote object storage (AWS S3, MinIO) using full, differential, and incremental backup strategies. The project has two implementations:

- **Go implementation** (`simple_backup/`) - Primary/modern, actively developed on `feat/simple_backup` branch
- **Python implementation** (`app/`) - Legacy

## Commands

### Go (simple_backup/)

```bash
# Build
cd simple_backup && go build -o build/zrb_simple
./build.sh                              # Cross-compile + transfer to VM

# Run
./zrb_simple genkey                     # Generate age encryption key pair
./zrb_simple backup --config config.yaml --task taskname --level 0
./zrb_simple snapshot --config config.yaml --task taskname
```

### Python (app/)

```bash
uv sync --all-extras --dev              # Install dependencies
uv run pytest tests                     # Run all tests
uv run pytest tests/test_integration_task.py  # Run specific test
uv run ruff check .                     # Lint
uv run ruff format .                    # Format
```

### VM Testing

```bash
vm/tests/01_prepare_env.sh              # Setup test data
vm/tests/02_l0_backup.sh                # Level 0 (full) backup
vm/tests/03_l1_to_l4.sh                 # Incremental backups
vm/tests/04_restore.sh                  # Restore test
vm/tests/05_interrupt_recover.sh        # Resumable upload test
```

## Architecture

### Go Backup Pipeline

```
Config → Lock → List Snapshots → ZFS Send (BLAKE3 hash) → Split 3GB chunks
→ Worker Pool (4 concurrent): Encrypt (age) → SHA256 → S3 Upload
→ Manifest → Update Last Backup Reference → Cleanup → Unlock
```

Key files in `simple_backup/`:
- `main.go` - CLI entry point, backup orchestration
- `zfs.go` - ZFS send with TeeReader for BLAKE3, split, hold/release
- `remote.go` - S3 backend with resumable multipart uploads
- `config.go` - Config, BackupManifest, BackupState structs
- `crypto.go` - Age encryption, SHA256
- `lock.go` - File-based PID locking per pool/dataset

### Python Pipeline (app/)

8-stage pipeline managed by `backup_manager.py`:
`snapshot_export → snapshot_test → split → compress → encrypt → upload → verify → clear → done`

### State Files

Located in `{base_dir}/run/{pool}/{dataset}/`:
- `backup_state.yaml` - Resumable state (parts_processed, parts_uploaded)
- `last_backup_manifest.yaml` - Tracks last backup per level for incremental chains
- `zrb.lock` - Concurrency lock with PID

### S3 Structure

```
{bucket}/{prefix}/
├── data/{pool}/{dataset}/{level}/{date}/    # Encrypted backup parts
└── manifests/{pool}/{dataset}/              # Backup manifests
```

## Tech Stack

- **Go 1.23**: age, aws-sdk-go-v2, urfave/cli, blake3
- **Python ≥3.12**: boto3, pyrage, msgpack, loguru, zstd
- **Package Manager**: UV (Python)
- **Testing**: pytest, Multipass VMs

## Code Style

- Python: ruff (line-length 100, 4-space indent, double quotes)
- Go: standard gofmt
