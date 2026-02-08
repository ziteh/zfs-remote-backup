# 10GB 備份測試指南

**目的**: 在正式使用前進行小規模測試，驗證完整的備份和還原流程

**測試規模**: 約 10GB 資料

**預估時間**:
- 備份: 10-20 分鐘（取決於網路速度）
- 還原: 10-20 分鐘

---

## 前置準備

### 1. 準備測試資料集

```bash
# 創建測試 pool/dataset（如果還沒有）
zfs create testpool/test_data

# 生成 10GB 測試資料
cd /testpool/test_data
for i in {1..10}; do
    dd if=/dev/urandom of=testfile_${i}.bin bs=1M count=1000
done

# 確認大小
du -sh /testpool/test_data
# 應該顯示約 10G

# 創建 level 0 snapshot
./zrb_simple snapshot --pool testpool --dataset test_data --prefix zrb_level0
```

### 2. 生成並驗證 Key Pair

```bash
# 生成 key pair
./zrb_simple genkey

# 輸出類似：
# === Age Key Pair Generated ===
# Public key:  age1xxxxxxxxxx...
# Private key: AGE-SECRET-KEY-1xxxxxxxxxx...

# 保存 private key
echo "AGE-SECRET-KEY-1xxxxxxxxxx..." > /secure/location/private_key.txt
chmod 600 /secure/location/private_key.txt

# 將 public key 添加到 config.yaml
```

### 3. 準備配置檔案

創建 `test_config.yaml`:

```yaml
base_dir: /mnt/backup_test
age_public_key: "age1xxxxxxxxxx..."  # 從上面的 genkey 複製

s3:
  enabled: true
  bucket: your-test-bucket
  region: us-east-1
  prefix: zfs-test-backups
  endpoint: ""  # AWS S3 留空，MinIO 填 http://endpoint:9000
  retry:
    max_attempts: 3
  storage_class:
    backup_data:
      - STANDARD      # Level 0
      - STANDARD      # Level 1
      - STANDARD      # Level 2
      - STANDARD      # Level 3
      - STANDARD      # Level 4
    manifest: STANDARD

tasks:
  - name: test_backup_10gb
    enabled: true
    pool: testpool
    dataset: test_data
```

配置檔案權限：
```bash
chmod 600 test_config.yaml
```

### 4. 測試 Key Pair

```bash
./zrb_simple test-keys \
  --config test_config.yaml \
  --private-key /secure/location/private_key.txt

# 預期輸出：
# Testing age key pair compatibility...
# Public key from config: age1xxxxxxxxxx...
# Private key loaded from: /secure/location/private_key.txt
#
# Encrypting test data with public key...
# Encryption successful
# Decrypting test data with private key...
# Decryption successful
# Content verification successful
```

✅ 如果測試通過，表示 key pair 配對正確

---

## 測試階段

### Test 1: 驗證 AWS 憑證

```bash
# 設置 AWS 憑證（如果使用環境變數）
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key

# 執行備份（會自動驗證憑證）
# 如果憑證無效，會在開始前就失敗
```

### Test 2: 執行 Level 0 備份

```bash
./zrb_simple backup \
  --config test_config.yaml \
  --task test_backup_10gb \
  --level 0

# 觀察輸出：
# - "Verifying AWS credentials and bucket access" - 憑證驗證
# - "AWS credentials verified successfully" - 憑證有效
# - "Latest snapshot found" - 找到快照
# - "Running zfs send and split" - ZFS 匯出開始
# - "ZFS send and split completed successfully" - 匯出完成
# - "Encryption and upload started for part file" - 加密上傳開始
# - "Uploaded to S3" (多次) - 各分片上傳完成
# - "Manifest written" - manifest 建立
# - "Manifest upload completed" - manifest 上傳完成
# - "Backup completed successfully!" - 完成
```

**預期結果**:
- 4 個分片檔案上傳到 S3
- 1 個 manifest 檔案
- 1 個 last_backup_manifest

**檢查點**:
```bash
# 檢查日誌
ls -lh /mnt/backup_test/logs/testpool/test_data/
tail -100 /mnt/backup_test/logs/testpool/test_data/$(date +%Y-%m-%d).log

# 檢查 manifest
cat /mnt/backup_test/run/testpool/test_data/last_backup_manifest.yaml
```

### Test 3: 驗證 S3 備份

```bash
# 使用 list 命令
./zrb_simple list \
  --config test_config.yaml \
  --task test_backup_10gb \
  --level 0 \
  --source s3

# 預期輸出 JSON，包含：
# - task: test_backup_10gb
# - pool: testpool
# - dataset: test_data
# - backups: [...]
#   - level: 0
#   - snapshot: testpool/test_data@zrb_level0_...
#   - parts_count: 4
#   - blake3_hash: ...
```

### Test 4: Dry-Run 還原測試

```bash
./zrb_simple restore \
  --config test_config.yaml \
  --task test_backup_10gb \
  --level 0 \
  --target testpool/restored_test \
  --private-key /secure/location/private_key.txt \
  --source s3 \
  --dry-run

# 預期輸出：
# === DRY RUN MODE ===
# Would restore backup:
#   Task:            test_backup_10gb
#   Pool/Dataset:    testpool/test_data
#   Target:          testpool/restored_test
#   Backup Level:    0
#   Snapshot:        testpool/test_data@zrb_level0_...
#   Parts:           4
#   BLAKE3 Hash:     ...
#   Source:          s3
#
# No changes made.
```

✅ 確認資訊正確後，進行實際還原

### Test 5: 實際還原

```bash
# 創建還原目標（如果不存在）
zfs create testpool/restored_test

# 執行還原
./zrb_simple restore \
  --config test_config.yaml \
  --task test_backup_10gb \
  --level 0 \
  --target testpool/restored_test \
  --private-key /secure/location/private_key.txt \
  --source s3

# 觀察輸出：
# - "Downloading part from S3" (多次) - 下載分片
# - "Decrypting and verifying part" (多次) - 解密驗證
# - "SHA256 verified" - 分片驗證成功
# - "Merging parts" - 合併分片
# - "Verifying BLAKE3 hash" - 完整性驗證
# - "BLAKE3 verified" - 驗證成功
# - "Executing ZFS receive" - 還原到 ZFS
# - "Restore completed successfully!" - 完成
```

### Test 6: 驗證還原結果

```bash
# 檢查還原的資料集
zfs list testpool/restored_test

# 比較檔案數量和大小
du -sh /testpool/test_data
du -sh /testpool/restored_test

# 驗證檔案內容（抽樣檢查）
diff /testpool/test_data/testfile_1.bin /testpool/restored_test/testfile_1.bin
# 應該沒有輸出（表示相同）

# 或使用 checksum 比較所有檔案
cd /testpool/test_data
find . -type f -exec sha256sum {} \; | sort > /tmp/original_checksums.txt

cd /testpool/restored_test
find . -type f -exec sha256sum {} \; | sort > /tmp/restored_checksums.txt

diff /tmp/original_checksums.txt /tmp/restored_checksums.txt
# 應該沒有輸出（表示完全相同）
```

✅ 如果檔案完全相同，表示備份和還原流程正確！

---

## 清理測試環境

```bash
# 刪除還原的資料集
zfs destroy testpool/restored_test

# 刪除本地備份檔案（如果有）
rm -rf /mnt/backup_test/task/testpool/test_data/*

# 刪除 S3 上的測試備份（可選，或保留作為範例）
# aws s3 rm s3://your-test-bucket/zfs-test-backups/ --recursive
```

---

## 成本估算

### 10GB 測試的 S3 成本（STANDARD）

**儲存成本**:
- 10GB * 1.3 (加密 overhead) = 13GB
- STANDARD: $0.023/GB/月
- 月成本: 13GB * $0.023 = **$0.30/月**
- 測試一天: **$0.01**

**請求成本**:
- PUT: 4 個分片 + 2 個 manifest = 6 requests * $0.005/1000 = **$0.00003**
- GET (restore): 6 requests * $0.0004/1000 = **$0.0000024**

**總成本**: 測試一天約 **$0.01 USD**

💡 測試完成後刪除資料可節省儲存成本

---

## 故障排除

### 備份失敗

**問題**: "Failed to acquire lock"
```bash
# 檢查鎖檔案
cat /mnt/backup_test/run/testpool/test_data/zrb.lock

# 如果是殘留鎖，手動刪除
rm /mnt/backup_test/run/testpool/test_data/zrb.lock
```

**問題**: "AWS credentials verification failed"
```bash
# 檢查憑證
aws s3 ls s3://your-test-bucket/

# 重新設置憑證
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
```

**問題**: "No snapshots found"
```bash
# 確認 snapshot 存在
zfs list -t snapshot | grep testpool/test_data

# 如果沒有，手動創建
./zrb_simple snapshot --pool testpool --dataset test_data --prefix zrb_level0
```

### 還原失敗

**問題**: "decryption failed"
```bash
# 驗證 key pair
./zrb_simple test-keys \
  --config test_config.yaml \
  --private-key /secure/location/private_key.txt
```

**問題**: "BLAKE3 mismatch"
```bash
# 檢查下載的檔案是否完整
# 重新執行 restore（會重新下載）
```

---

## 測試檢查清單

### 準備階段
- [ ] 準備 10GB 測試資料
- [ ] 創建 ZFS snapshot
- [ ] 生成 age key pair
- [ ] 創建並配置 test_config.yaml
- [ ] 測試 key pair 配對
- [ ] 驗證 AWS 憑證

### 備份測試
- [ ] 執行 Level 0 備份
- [ ] 檢查日誌無錯誤
- [ ] 驗證 S3 上的檔案
- [ ] 使用 list 命令確認備份

### 還原測試
- [ ] Dry-run 還原預覽
- [ ] 執行實際還原
- [ ] 驗證檔案完整性
- [ ] 比較 checksum

### 結果
- [ ] 備份成功
- [ ] 還原成功
- [ ] 資料完全一致
- [ ] **✅ 可以導入正式使用**

---

## 下一步：正式部署

如果 10GB 測試全部通過，可參考 `PRODUCTION_CHECKLIST.md` 進行正式部署：

1. **Phase 0: 準備階段**（1-2 天）
   - 完善私鑰安全管理
   - 設置監控和告警
   - 撰寫災難恢復文檔

2. **Phase 1: 試運行**（1-2 週）
   - 小規模 dataset 正式備份
   - 每日監控
   - 至少一次完整 restore 驗證

3. **Phase 2: 擴展部署**（2-4 週）
   - 逐步增加 datasets
   - 監控效能和成本

---

**測試日期**: _______________
**測試人員**: _______________
**測試結果**: [ ] 通過  [ ] 失敗
**備註**: _______________________________________
