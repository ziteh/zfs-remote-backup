FROM ubuntu:latest

WORKDIR /app

RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    python3-venv \
    cron \
    zfsutils-linux && \
    rm -rf /var/lib/apt/lists/*

COPY crontab /etc/cron.d/zfs_snapshot_cron

COPY requirements.txt /app/requirements.txt
COPY app/zfs_snapshot_script.py /usr/local/bin/zfs_snapshot_script.py
COPY app/sha256.py /usr/local/bin/sha256.py
COPY app/test.py /usr/local/bin/test.py

RUN python3 -m venv /app/venv
RUN /app/venv/bin/pip install --no-cache-dir -r /app/requirements.txt

RUN chmod +x /usr/local/bin/zfs_snapshot_script.py && \
    chmod 0644 /etc/cron.d/zfs_snapshot_cron

ENV PATH="/app/venv/bin:${PATH}"

CMD ["cron", "-f"]
