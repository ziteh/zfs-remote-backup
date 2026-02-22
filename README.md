# ZFS Remote Backup (zrb)

The goal is to back up data stored in [TrueNAS](https://www.truenas.com/)/[OpenZFS](https://openzfs.github.io/openzfs-docs/) to remote object storage services such as AWS S3 using snapshots, enabling off-site backups. Inspired by [someone1/zfsbackup-go](https://github.com/someone1/zfsbackup-go).

Features:

- Specifically designed for S3 Glacier Deep Archive cold storage, optimal storage costs.
- Full, differential, and incremental multi-level backups.
- [Age](https://github.com/FiloSottile/age) encryption.
- [BLAKE3](https://github.com/BLAKE3-team/BLAKE3) integrity verification.

## Usage

Build:

```bash
make build
```

```bash
./build/zrb --help
```

### Prepare

Generate key:

```bash
zrb genkey
```

```
Public key:  age1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
Public key saved to:  zrb_public.key
Private key saved to: zrb_private.key

IMPORTANT: Keep the private key secure and do not share it with anyone.
If you lose the private key, your backups cannot be restored.
```

Create `config.yaml`:

```yaml
# config.yaml
base_dir: /var/lib/zrb/
age_public_key: age1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx  # Enter the public key above
s3:
  enabled: true
  bucket: my-backup-bucket
  region: us-east-1
  prefix: zfs-backups/
  endpoint: "" # Leave empty for AWS S3, or specify custom endpoint for S3-compatible services
  storage_class:
    manifest: STANDARD
    backup_data:
      - DEEP_ARCHIVE # Level 0 (full backup)
      - DEEP_ARCHIVE # Level 1
      - DEEP_ARCHIVE # Level 2
      - GLACIER      # Level 3
  retry:
    max_attempts: 8
tasks:
  - name: example_task
    description: Example backup task
    pool: pool
    dataset: temp
    enabled: true
```

Validate configuration and connectivity:

```bash
zrb check --config config.yaml
```

`zrb` does not automatically create ZFS snapshots. You must create ZFS snapshots using another method (such as TrueNAS's Periodic Snapshot Tasks, or `zrb snapshot`). Note that only snapshots with the `zrb_level<N>` prefix in the name will be used by `zrb` (e.g., `zrb_level0_2026-01-01_00-00` used for level 0 backup task).

### Backup

Level 0 (Full backup):

```bash
zrb backup --config config.yaml --task example_task --level 0
```

Level 1 (based on level 0):

```bash
zrb backup --config config.yaml --task example_task --level 1
```

### List

List available backups

```bash
# List all backup on S3
zrb list --config config.yaml --task example_task --source s3

# Only level 1
zrb list --config config.yaml --task example_task --source s3 --level 1
```

## Legacy Implementations

- [archive/rust-experimental/](https://github.com/ziteh/zfs-remote-backup/tree/feat/rust/archive/rust-experimental) - Exploratory Rust implementation (`feat/rust` branch).

## License

> [!WARNING]
> This software is provided *as is*, without warranty or conditions of any kind.
>
> Please carefully review how this software works and what actions it performs.
> Following the [3-2-1 Backup Rule](https://www.backblaze.com/blog/the-3-2-1-backup-strategy/) can reduce the risk of data loss due to hardware failure or accidents.

Licensed under the Apache 2.0 ([LICENSE](./LICENSE) or <https://opensource.org/license/apache-2-0>).
