# ZFS Remote Backup (zrb)

High-performance Go tool for backing up ZFS snapshots to remote object storage (AWS S3, MinIO).

## Features

- Full, differential, and incremental ZFS backups
- Age encryption for data at rest
- Resumable multipart uploads
- BLAKE3 integrity verification
- Graceful shutdown handling
- AWS S3 storage classes (STANDARD, GLACIER, DEEP_ARCHIVE)
- Cross-platform (Linux, macOS)

## Quick Start

### Build
```bash
make build
```

### Generate Keys
```bash
./build/zrb genkey
```

### Configure
```bash
cp config.example.yaml /etc/zrb/config.yaml
# Edit with your settings
```

### Backup
```bash
# Full backup
./build/zrb backup --config /etc/zrb/config.yaml --task mytask --level 0

# Incremental
./build/zrb backup --config /etc/zrb/config.yaml --task mytask --level 1
```

### Restore
```bash
./build/zrb restore \
  --config /etc/zrb/config.yaml \
  --task mytask \
  --level 0 \
  --target tank/restore \
  --private-key /path/to/key \
  --source s3
```

## Documentation

- [Deployment Guide](docs/DEPLOYMENT_GUIDE.md)
- [Production Checklist](docs/PRODUCTION_CHECKLIST.md)
- [Graceful Shutdown](docs/GRACEFUL_SHUTDOWN.md)
- [Testing Guide](docs/TEST_GUIDE_10GB.md)

## Architecture

```
Config → Lock → ZFS Send (BLAKE3) → Split 3GB chunks
→ Worker Pool (4): Encrypt (age) → BLAKE3 → S3 Upload
→ Manifest → Update Reference → Cleanup → Unlock
```

## Legacy Implementations

- [archive/python-legacy/](archive/python-legacy/) - Docker-based Python implementation
- [archive/rust-experimental/](archive/rust-experimental/) - Exploratory Rust implementation

## License

> [!WARNING]
> This software is provided *as is*, without warranty or conditions of any kind.
>
> Please carefully review how this software works and what actions it performs.
> Following the *3-2-1 backup rule* can reduce the risk of data loss due to hardware failure or accidents.

Licensed under the Apache 2.0 ([`LICENSE`](./LICENSE) or <https://opensource.org/license/apache-2-0>).
