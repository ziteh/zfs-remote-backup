# Graceful Shutdown Guide

**Version**: 0.1.0
**Date**: 2026-02-07

This guide explains how to gracefully stop running backups and ensure proper resource cleanup.

---

## Overview

The ZFS Remote Backup tool implements graceful shutdown handling for:
- **SIGTERM** - Standard termination signal (systemd, kill command)
- **SIGINT** - Interrupt signal (Ctrl+C in terminal)

When a signal is received, the tool will:
1. Stop accepting new work
2. Cancel ongoing operations (ZFS send, encryption, S3 upload)
3. Save current backup state
4. Release ZFS snapshot holds
5. Release file locks
6. Clean up temporary files
7. Exit with appropriate status code

---

## Implementation Details

### Signal Handling

The tool uses Go's `signal.NotifyContext` to handle signals:

```go
// main.go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
```

### Context Propagation

The context is propagated through all critical operations:

1. **ZFS Send** - Already uses context-aware commands
2. **Encryption Workers** - Check context before processing each part
3. **S3 Upload** - AWS SDK respects context cancellation
4. **Worker Pool** - Each worker checks `ctx.Done()` before processing

### Resource Cleanup

Resources are cleaned up using `defer` statements:

```go
// Lock cleanup
defer func() {
    if err := releaseLock(); err != nil {
        slog.Warn("Failed to release lock", "error", err)
    }
}()

// ZFS hold cleanup (in zfs.go)
defer func() {
    releaseCtx, cancelRelease := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancelRelease()
    if err := exec.CommandContext(releaseCtx, "zfs", "release", holdTag, targetSnapshot).Run(); err != nil {
        slog.Warn("Failed to release snapshot hold", "holdTag", holdTag, "error", err)
    }
}()
```

---

## Production Usage

### 1. Manual Interruption

To stop a running backup process:

```bash
# Find the process
ps aux | grep zrb_simple

# Send SIGTERM (graceful shutdown)
kill -TERM <PID>

# Or send SIGINT (Ctrl+C equivalent)
kill -INT <PID>

# Force kill only if graceful shutdown fails (not recommended)
kill -9 <PID>
```

**Example**:
```bash
$ ps aux | grep zrb_simple
root     12345  2.5  1.0  123456  67890 ?  R    14:30   0:15 /usr/local/bin/zrb_simple backup --config /etc/zrb/config.yaml --task prod --level 0

$ kill -TERM 12345
# Wait 5-10 seconds for graceful shutdown
```

### 2. Systemd Integration

Create a systemd service for automated scheduling:

**File**: `/etc/systemd/system/zrb-backup@.service`

```ini
[Unit]
Description=ZFS Remote Backup - %i
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root

# Main command
ExecStart=/usr/local/bin/zrb_simple backup --config /etc/zrb/config.yaml --task %i --level 0

# Graceful shutdown configuration
TimeoutStopSec=300
KillMode=mixed
KillSignal=SIGTERM

# Restart policy (don't auto-restart on interruption)
Restart=no

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=zrb-backup-%i

[Install]
WantedBy=multi-user.target
```

**Usage**:
```bash
# Start backup for task 'prod'
systemctl start zrb-backup@prod.service

# Stop backup gracefully
systemctl stop zrb-backup@prod.service

# Check status
systemctl status zrb-backup@prod.service

# View logs
journalctl -u zrb-backup@prod.service -f
```

### 3. Systemd Timer (Scheduled Backups)

**File**: `/etc/systemd/system/zrb-backup@.timer`

```ini
[Unit]
Description=ZFS Remote Backup Timer - %i
Requires=zrb-backup@%i.service

[Timer]
# Run daily at 2 AM
OnCalendar=daily
OnCalendar=*-*-* 02:00:00

# Run 10 minutes after boot (first backup)
OnBootSec=10min

# Persistence (run missed backups on startup)
Persistent=true

[Install]
WantedBy=timers.target
```

**Usage**:
```bash
# Enable timer for task 'prod'
systemctl enable zrb-backup@prod.timer
systemctl start zrb-backup@prod.timer

# Check timer status
systemctl list-timers | grep zrb-backup

# To stop scheduled backups
systemctl stop zrb-backup@prod.timer
systemctl disable zrb-backup@prod.timer
```

### 4. TrueNAS SCALE Integration

TrueNAS SCALE uses systemd, so you can use the service files above.

#### Installation

1. Copy binary to TrueNAS:
   ```bash
   scp zrb_simple truenas_admin@truenas:/usr/local/bin/
   ssh truenas_admin@truenas chmod +x /usr/local/bin/zrb_simple
   ```

2. Copy service files:
   ```bash
   ssh truenas_admin@truenas
   nano /etc/systemd/system/zrb-backup@.service
   nano /etc/systemd/system/zrb-backup@.timer
   ```

3. Create config file:
   ```bash
   mkdir -p /etc/zrb
   nano /etc/zrb/config.yaml
   ```

4. Enable and start:
   ```bash
   systemctl daemon-reload
   systemctl enable zrb-backup@prod.timer
   systemctl start zrb-backup@prod.timer
   ```

#### Monitoring

```bash
# Check timer status
systemctl list-timers | grep zrb

# View logs
journalctl -u zrb-backup@prod.service -n 100

# Check if backup is running
systemctl is-active zrb-backup@prod.service
```

#### Manual Operations

```bash
# Run backup manually
systemctl start zrb-backup@prod.service

# Stop running backup
systemctl stop zrb-backup@prod.service

# Check backup status
systemctl status zrb-backup@prod.service
```

---

## Resumable Backups

When a backup is interrupted, the tool saves its state to:
```
{base_dir}/run/{pool}/{dataset}/backup_state.yaml
```

To resume:
```bash
# Simply run the same backup command again
zrb_simple backup --config config.yaml --task prod --level 0
```

The tool will automatically detect the saved state and resume:
- Skip already processed parts
- Skip already uploaded parts
- Continue from where it left off

**Example output**:
```
INFO Found existing backup state, resuming
INFO Skipping already processed part part=snapshot.part-aaaaaa
INFO Skipping already uploaded part part=snapshot.part-aaaaab
INFO Processing part part=snapshot.part-aaaaac
```

---

## Cron Integration (Alternative to Systemd)

If you prefer using cron:

**File**: `/etc/cron.d/zrb-backup`

```bash
# Daily backup at 2 AM
0 2 * * * root /usr/local/bin/zrb_simple backup --config /etc/zrb/config.yaml --task prod --level 0 >> /var/log/zrb-backup.log 2>&1
```

**Stopping cron-based backup**:
```bash
# Find the process
ps aux | grep zrb_simple

# Stop it gracefully
kill -TERM <PID>
```

---

## Timeout Configuration

### Systemd TimeoutStopSec

Controls how long systemd waits for graceful shutdown:

```ini
[Service]
TimeoutStopSec=300  # Wait 5 minutes before force kill
```

Recommended values:
- **Small backups (<10GB)**: 60-120 seconds
- **Medium backups (10-100GB)**: 300-600 seconds
- **Large backups (>100GB)**: 900-1800 seconds

### S3 Upload Timeouts

S3 uploads have their own timeout handling:
- AWS SDK automatically retries failed uploads
- Context cancellation stops uploads immediately
- Partially uploaded multipart uploads remain in S3 (can be cleaned up with lifecycle policies)

---

## Troubleshooting

### Process Won't Stop

If the process doesn't stop after SIGTERM:

1. **Wait longer** - Large S3 uploads may take time to cancel
2. **Check logs**:
   ```bash
   journalctl -u zrb-backup@prod.service -n 50
   ```
3. **Force kill** (last resort):
   ```bash
   kill -9 <PID>
   ```
   Note: This may leave:
   - Lock files (will be cleaned on next run)
   - ZFS holds (need manual cleanup)
   - Temporary files

### Lock Not Released

If lock file remains after interruption:

```bash
# Check lock file
cat /mnt/pool/zrb/run/{pool}/{dataset}/zrb.lock

# Verify no process is running
ps aux | grep zrb_simple

# Remove stale lock manually
rm /mnt/pool/zrb/run/{pool}/{dataset}/zrb.lock
```

### ZFS Hold Not Released

If ZFS hold remains:

```bash
# List holds
zfs holds pool/dataset@snapshot

# Release manually
zfs release <tag> pool/dataset@snapshot
```

### Resume Not Working

If resume doesn't work:

1. **Check state file**:
   ```bash
   cat /mnt/pool/zrb/run/{pool}/{dataset}/backup_state.yaml
   ```

2. **Remove state to start fresh**:
   ```bash
   rm /mnt/pool/zrb/run/{pool}/{dataset}/backup_state.yaml
   ```

---

## Best Practices

1. **Use systemd** for production deployments (better than cron)
2. **Set appropriate timeouts** based on backup size
3. **Monitor logs** regularly
4. **Test interruption** before production use
5. **Configure S3 lifecycle policies** to clean up incomplete multipart uploads
6. **Don't force kill** unless absolutely necessary
7. **Verify resumable backups** work in your environment

---

## Exit Codes

- **0** - Success
- **1** - Error
- **130** - Interrupted by SIGINT (Ctrl+C)
- **143** - Interrupted by SIGTERM (kill command)

---

## Security Considerations

- **Root access required** for ZFS operations
- **Age private key** must be accessible to the running user
- **AWS credentials** must be available (environment or ~/.aws/credentials)
- **Lock files** prevent concurrent backups of same dataset
- **Signal handlers** do not expose sensitive information

---

## Example: Complete TrueNAS Setup

```bash
# 1. Install binary
scp zrb_simple truenas_admin@truenas:/usr/local/bin/
ssh truenas_admin@truenas chmod +x /usr/local/bin/zrb_simple

# 2. Create directories
ssh truenas_admin@truenas mkdir -p /etc/zrb /var/log/zrb

# 3. Create config
ssh truenas_admin@truenas
cat > /etc/zrb/config.yaml <<EOF
base_dir: /mnt/tank/zrb
age_public_key: age1...
s3:
  enabled: true
  bucket: my-backup-bucket
  region: us-east-1
  prefix: zfs-backups/
  storage_class:
    manifest: STANDARD_IA
    backup_data:
      - DEEP_ARCHIVE
      - GLACIER
      - GLACIER_IR
tasks:
  - name: prod
    pool: tank
    dataset: important
    enabled: true
EOF

# 4. Test manually
zrb_simple backup --config /etc/zrb/config.yaml --task prod --level 0

# 5. Setup systemd service
cat > /etc/systemd/system/zrb-backup@.service <<EOF
[Unit]
Description=ZFS Remote Backup - %i
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/zrb_simple backup --config /etc/zrb/config.yaml --task %i --level 0
TimeoutStopSec=300
KillSignal=SIGTERM
StandardOutput=journal
EOF

# 6. Setup systemd timer
cat > /etc/systemd/system/zrb-backup@.timer <<EOF
[Unit]
Description=ZFS Remote Backup Timer - %i

[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true

[Install]
WantedBy=timers.target
EOF

# 7. Enable and start
systemctl daemon-reload
systemctl enable zrb-backup@prod.timer
systemctl start zrb-backup@prod.timer

# 8. Verify
systemctl list-timers | grep zrb
systemctl status zrb-backup@prod.timer
```

---

**Last Updated**: 2026-02-07
**Version**: 0.1.0-alpha.1
