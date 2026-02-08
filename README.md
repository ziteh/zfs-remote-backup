# ZFS Remote Backup (zrb)

The goal is to back up data stored in [TrueNAS](https://www.truenas.com/)/[OpenZFS](https://openzfs.github.io/openzfs-docs/) to remote object storage services such as AWS S3 using snapshots, enabling off-site backups. Inspired by [someone1/zfsbackup-go](https://github.com/someone1/zfsbackup-go).

Features:

- Specifically designed for S3 Glacier Deep Archive cold storage, optimal storage costs.
- Full, differential, and incremental multi-level backups.
- [Age](https://github.com/FiloSottile/age) encryption.
- [BLAKE3](https://github.com/BLAKE3-team/BLAKE3) integrity verification.

## Usage

```bash
make build
```

```bash
# Full backup (Level 0)
./build/zrb backup --config /path/to/config.yaml --task example_task --level 0
```

## Legacy Implementations

- [archive/rust-experimental/](./archive/rust-experimental/) - Exploratory Rust implementation.
- [archive/python-legacy/](./archive/python-legacy/) - Docker-based Python implementation.

## License

> [!WARNING]
> This software is provided *as is*, without warranty or conditions of any kind.
>
> Please carefully review how this software works and what actions it performs.
> Following the [3-2-1 Backup Rule](https://www.backblaze.com/blog/the-3-2-1-backup-strategy/) can reduce the risk of data loss due to hardware failure or accidents.

Licensed under the Apache 2.0 ([LICENSE](./LICENSE) or <https://opensource.org/license/apache-2-0>).
