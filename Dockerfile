FROM ubuntu:latest

RUN apt-get update && apt-get install -y \
    cron \
    zfsutils-linux \
    bash &&
    rm -rf /var/lib/apt/lists/*

COPY zfs_snapshot_script.sh /usr/local/bin/zfs_snapshot_script.sh

RUN chmod +x /usr/local/bin/zfs_snapshot_script.sh

COPY crontab /etc/cron.d/zfs-snapshot-cron

RUN chmod 0644 /etc/cron.d/zfs-snapshot-cron

CMD ["cron", "-f"]
