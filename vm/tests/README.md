# VM Test Scripts

Test scripts for ZFS Remote Backup (Go implementation) using Multipass VM.

## Prerequisites

- Multipass VM named `zrb-vm` with ZFS support
- VM has testpool with `backup_data` dataset
- Age encryption keys generated

## Test Scripts

### 00_setup_minio.sh
**Purpose**: Set up MinIO (S3-compatible object storage) on the VM

**What it does**:
- Installs MinIO server and client (mc)
- Creates systemd service for MinIO
- Configures MinIO with admin/password123 credentials
- Creates `zrb-test` bucket
- Starts MinIO at http://127.0.0.1:9000

**Run**: `./vm/tests/00_setup_minio.sh`

**Output**:
```
MinIO Details:
  Endpoint:     http://127.0.0.1:9000
  Console:      http://127.0.0.1:9001
  Access Key:   admin
  Secret Key:   password123
  Test Bucket:  zrb-test
```

---

### 00_transfer_zrb_simple.sh
**Purpose**: Transfer compiled binary to VM

---

### 01_prepare_env.sh
**Purpose**: Set up test environment and data

**What it does**:
- Creates test files in `/testpool/backup_data`
- Generates baseline SHA256 checksums
- Sets up MinIO bucket (if available)

**Run**: `./vm/tests/01_prepare_env.sh`

---

### 02_l0_backup.sh
**Purpose**: Execute Level 0 (full) backup

**What it does**:
- Creates config with S3 disabled (local-only)
- Creates ZFS snapshot
- Runs backup command

**Run**: `./vm/tests/02_l0_backup.sh`

---

### 03_l1_to_l4.sh
**Purpose**: Execute Level 1-4 (incremental) backups

---

### 04_restore.sh
**Purpose**: Test restore from local backup

---

### 05_interrupt_recover.sh
**Purpose**: Test resumable upload after interruption

---

### 06_test_list.sh
**Purpose**: Comprehensive testing of `list` command

**Test Coverage**:
1. List all backups (JSON structure)
2. Filter by level 0
3. Filter by level 1
4. Verify metadata fields
5. Verify summary statistics
6. Test S3 source (skipped if not available)
7. Error handling - invalid task
8. Error handling - invalid level

**Run**: `./vm/tests/06_test_list.sh`

**Expected Output**: 7/8 tests pass (1 skipped if S3 not configured)

---

### 07_test_restore.sh
**Purpose**: Comprehensive testing of `restore` command (local source)

**Test Coverage**:
1. Dry-run restore level 0
2. Verify target dataset state
3. Actual restore level 0
4. Verify restored dataset exists
5. Verify data integrity (SHA256)
6. Verify BLAKE3 hash verification
7. Verify SHA256 part verification
8. Error handling - invalid level
9. Error handling - invalid task
10. Error handling - missing private key
11. Cleanup

**Run**: `./vm/tests/07_test_restore.sh`

**Note**: This test script may have issues hanging at Test 2. Manual testing is recommended.

**Manual Test Example**:
```bash
# Dry-run
multipass exec zrb-vm -- sudo /tmp/zrb_simple restore \
  --config /tmp/zrb_simple_config.yaml \
  --task test_backup \
  --level 0 \
  --target testpool/restored_test \
  --private-key /home/ubuntu/age_private_key.txt \
  --source local \
  --dry-run

# Actual restore
multipass exec zrb-vm -- sudo /tmp/zrb_simple restore \
  --config /tmp/zrb_simple_config.yaml \
  --task test_backup \
  --level 0 \
  --target testpool/restored_test \
  --private-key /home/ubuntu/age_private_key.txt \
  --source local

# Verify
multipass exec zrb-vm -- sudo zfs list testpool/restored_test
```

---

### 08_test_s3_restore.sh ⭐ NEW
**Purpose**: Comprehensive testing of `restore` command with S3 source (MinIO)

**Prerequisites**:
- MinIO must be running (use `00_setup_minio.sh`)

**Test Coverage**:
1. Check MinIO availability
2. Create S3-enabled config
3. Backup level 0 to S3 (MinIO)
4. Verify manifests uploaded to S3
5. **Dry-run restore from S3** ⭐
6. Prepare target dataset
7. Actual restore from S3
8. Verify restored dataset
9. Verify data integrity (SHA256)
10. Verify S3 download operations
11. Cleanup

**Run**: `./vm/tests/08_test_s3_restore.sh`

**Test Results** (Last run: 2026-02-07):
```
✓ Test 0: MinIO is running
✓ Test 1: S3-enabled config created
✓ Test 2: Credentials will be passed via environment variables
✓ Test 3: Backup to S3 completed successfully
✓ Test 4: Manifests found in S3
✓ Test 5: Dry-run restore from S3 - SUCCESS!
```

**Test 5 Output**:
```
=== DRY RUN MODE ===
Would restore backup:
  Task:            s3_test_backup
  Pool/Dataset:    testpool/backup_data
  Target:          testpool/restored_from_s3
  Backup Level:    0
  Snapshot:        testpool/backup_data@zrb_level0_1769346027
  Parts:           1
  BLAKE3 Hash:     975dcd41baf9d749906d9f797c7b360fee976f0d14aa20e3aa91330dd78cdeef
  Source:          s3

No changes made.
```

**Key Features Demonstrated**:
- Downloads manifests from S3
- Shows correct backup metadata
- **Source displayed as "s3"**
- Confirms dry-run with "No changes made"

---

## Test Configurations

### Local-Only Configuration
Used by: `02_l0_backup.sh`, `03_l1_to_l4.sh`, `06_test_list.sh`, `07_test_restore.sh`

```yaml
base_dir: /home/ubuntu/zrb_base
age_public_key: "age1tawkwd7rjxwjmhnyv0df6s5c9pfmk5fnsyu439mr89lrn0f0594q3hjcav"
s3:
  enabled: false
tasks:
  - name: test_backup
    enabled: true
    pool: testpool
    dataset: backup_data
```

### S3-Enabled Configuration (MinIO)
Used by: `08_test_s3_restore.sh`

```yaml
base_dir: /home/ubuntu/zrb_s3_base
age_public_key: "age1tawkwd7rjxwjmhnyv0df6s5c9pfmk5fnsyu439mr89lrn0f0594q3hjcav"
s3:
  enabled: true
  bucket: zrb-test
  region: us-east-1
  prefix: test-backups/
  endpoint: http://127.0.0.1:9000
  storage_class:
    manifest: STANDARD
    backup_data:
      - STANDARD  # Level 0
      - STANDARD  # Level 1
      - STANDARD  # Level 2+
  retry:
    max_attempts: 3
tasks:
  - name: s3_test_backup
    enabled: true
    pool: testpool
    dataset: backup_data
```

**Important**: When using `sudo` with S3, AWS credentials must be passed via environment variables:
```bash
export AWS_ACCESS_KEY_ID=admin
export AWS_SECRET_ACCESS_KEY=password123
sudo -E /tmp/zrb_simple backup --config /tmp/config.yaml ...
```

---

## Common Issues

### Issue: "failed to acquire lock"
**Cause**: Previous backup still running or abnormally terminated

**Solution**:
```bash
# Check lock file
multipass exec zrb-vm -- cat /home/ubuntu/zrb_base/run/testpool/backup_data/zrb.lock

# Remove if no process running
multipass exec zrb-vm -- sudo rm /home/ubuntu/zrb_base/run/testpool/backup_data/zrb.lock
```

### Issue: "failed to refresh cached credentials" (S3)
**Cause**: AWS credentials not accessible when using `sudo`

**Solution**: Use `sudo -E` with environment variables instead of `~/.aws/credentials`:
```bash
export AWS_ACCESS_KEY_ID=admin
export AWS_SECRET_ACCESS_KEY=password123
sudo -E /tmp/zrb_simple backup ...
```

### Issue: MinIO not running
**Cause**: MinIO service not started

**Solution**:
```bash
# Run setup script
./vm/tests/00_setup_minio.sh

# Or manually start
multipass exec zrb-vm -- sudo systemctl start minio
```

### Issue: Test script hangs
**Cause**: Various (dataset operations, locks, etc.)

**Solution**:
- Stop the script (Ctrl+C)
- Check VM state manually
- Run individual test commands manually
- Check for zombie processes or locks

---

## Test Order Recommendation

For new VM setup:
1. `00_setup_minio.sh` (if testing S3 features)
2. `01_prepare_env.sh`
3. `02_l0_backup.sh`
4. `03_l1_to_l4.sh`
5. `06_test_list.sh`
6. `07_test_restore.sh` (or manual restore test)
7. `08_test_s3_restore.sh` (if MinIO available)

---

## Verification

After running tests, verify:

1. **Backups exist**:
   ```bash
   multipass exec zrb-vm -- ls -R /home/ubuntu/zrb_base/task/
   ```

2. **Manifests are valid**:
   ```bash
   multipass exec zrb-vm -- cat /home/ubuntu/zrb_base/run/testpool/backup_data/last_backup_manifest.yaml
   ```

3. **S3 uploads** (if using MinIO):
   ```bash
   multipass exec zrb-vm -- mc ls myminio/zrb-test/test-backups/ --recursive
   ```

4. **Restored data integrity**:
   ```bash
   multipass exec zrb-vm -- diff /tmp/baseline_sha256.txt /tmp/restore_sha256_new.txt
   ```

---

**Last Updated**: 2026-02-07
**Version**: 0.1.0-alpha.1
