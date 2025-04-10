# ZFS Remote Backup

The goal is to back up data stored in [TrueNAS](https://www.truenas.com/)/[OpenZFS](https://openzfs.github.io/openzfs-docs/) to remote object storage services such as AWS S3 using snapshots, enabling off-site backups.

By leveraging the properties of OpenZFS snapshots, the system performs *full*, *differential*, and *incremental* backups to strike a balance between storage efficiency and data reliability.

## Usage

```bash
docker build -t zfs-remote-backup .
docker run -d --name zfs-remote-backup zfs-remote-backup
```

```bash
uv pip freeze > requirements.txt
```

## License

> [!WARNING]
> This software is provided *as is*, without warranty or conditions of any kind.
>
> Please carefully review how this software works and what actions it performs.  
> Following the *3-2-1 backup rule* can reduce the risk of data loss due to hardware failure or accidents.

Licensed under the Apache 2.0 ([`LICENSE`](./LICENSE) or <https://opensource.org/license/apache-2-0>).
