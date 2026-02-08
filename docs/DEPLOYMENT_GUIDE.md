# ZFS Remote Backup - 部署指南

## 快速開始（TrueNAS Scale）

### 1. 編譯
```bash
# 在開發機器上
cd simple_backup
GOOS=linux GOARCH=amd64 go build -o zrb_simple
```

### 2. 傳輸到 TrueNAS
```bash
scp zrb_simple admin@truenas-ip:/mnt/pool/apps/
ssh admin@truenas-ip
sudo mv /mnt/pool/apps/zrb_simple /usr/local/bin/
sudo chmod +x /usr/local/bin/zrb_simple
```

### 3. 生成加密密鑰
```bash
zrb_simple genkey

# 輸出範例:
# === Age Key Pair Generated ===
# Public key:  age1ed9kad8ddqpjnt7k50tmxu8ngxte5ghmwsse7ckt6kdtvc7afpkqlj5ddk
# Private key: AGE-SECRET-KEY-1XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
#
# !! Keep your private key secure !!
```

**重要**:
- 公鑰放入配置檔案
- 私鑰安全存儲：`/root/.age/private.key`（chmod 600）
- 備份私鑰到安全位置（密碼管理器、保險箱等）

### 4. 創建配置檔案
```bash
sudo mkdir -p /etc/zrb
sudo vi /etc/zrb/config.yaml
```

**配置範例** (TrueNAS Scale → AWS S3):
```yaml
base_dir: /mnt/pool/backups/zrb
age_public_key: age1ed9kad8ddqpjnt7k50tmxu8ngxte5ghmwsse7ckt6kdtvc7afpkqlj5ddk

s3:
  enabled: true
  bucket: my-truenas-backups
  region: us-east-1
  prefix: zfs-backups/
  endpoint: ""  # 空白表示使用 AWS S3

  storage_class:
    # Manifest 必須立即可存取
    manifest: STANDARD_IA

    # 每個 level 的 storage class
    backup_data:
      - DEEP_ARCHIVE   # Level 0: 完整備份，冷儲存
      - GLACIER        # Level 1: 每日增量
      - GLACIER_IR     # Level 2+: 頻繁增量

  retry:
    max_attempts: 8  # AWS 重試次數

tasks:
  - name: production_data
    description: 生產資料每日備份
    pool: tank
    dataset: production
    enabled: true

  - name: user_files
    description: 使用者檔案每日備份
    pool: tank
    dataset: users
    enabled: true
```

**設置權限**:
```bash
sudo chmod 600 /etc/zrb/config.yaml
sudo chown root:root /etc/zrb/config.yaml
```

### 5. 配置 AWS 憑證
```bash
# 方法 1: 環境變數（推薦用於測試）
export AWS_ACCESS_KEY_ID="AKIAXXXXXXXXXXXXXXXX"
export AWS_SECRET_ACCESS_KEY="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

# 方法 2: AWS CLI 配置（推薦用於生產）
aws configure
# 或直接編輯
sudo mkdir -p /root/.aws
sudo vi /root/.aws/credentials
```

`/root/.aws/credentials`:
```ini
[default]
aws_access_key_id = AKIAXXXXXXXXXXXXXXXX
aws_secret_access_key = XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
```

### 6. 創建初始快照
```bash
# 為每個要備份的 dataset 創建 Level 0 快照
sudo zfs snapshot tank/production@zrb_level0_$(date +%s)
sudo zfs snapshot tank/users@zrb_level0_$(date +%s)
```

### 7. 執行首次備份（測試）
```bash
# Level 0 (完整備份)
sudo zrb_simple backup \
  --config /etc/zrb/config.yaml \
  --task production_data \
  --level 0

# 檢查結果
sudo zrb_simple list \
  --config /etc/zrb/config.yaml \
  --task production_data \
  --source local
```

### 8. 設置自動備份 (Cron)
```bash
sudo vi /etc/cron.d/zrb-backup
```

**Cron 配置範例**:
```bash
# ZFS Remote Backup Schedule
# m h  dom mon dow user  command

# 每週日 2:00 AM - Level 0 完整備份
0 2 * * 0 root /usr/local/bin/zrb_snapshot_and_backup.sh production_data 0 >> /var/log/zrb/cron.log 2>&1
0 2 * * 0 root /usr/local/bin/zrb_snapshot_and_backup.sh user_files 0 >> /var/log/zrb/cron.log 2>&1

# 每日 2:00 AM (週一到週六) - Level 1 增量備份
0 2 * * 1-6 root /usr/local/bin/zrb_snapshot_and_backup.sh production_data 1 >> /var/log/zrb/cron.log 2>&1
0 2 * * 1-6 root /usr/local/bin/zrb_snapshot_and_backup.sh user_files 1 >> /var/log/zrb/cron.log 2>&1
```

**包裝腳本** (`/usr/local/bin/zrb_snapshot_and_backup.sh`):
```bash
#!/bin/bash
set -e

TASK=$1
LEVEL=$2
CONFIG="/etc/zrb/config.yaml"

# 從配置中讀取 pool 和 dataset
POOL=$(grep -A3 "name: $TASK" $CONFIG | grep "pool:" | awk '{print $2}')
DATASET=$(grep -A3 "name: $TASK" $CONFIG | grep "dataset:" | awk '{print $2}')

# 創建快照
TIMESTAMP=$(date +%s)
SNAPSHOT="${POOL}/${DATASET}@zrb_level${LEVEL}_${TIMESTAMP}"
zfs snapshot "$SNAPSHOT"

# 執行備份
/usr/local/bin/zrb_simple backup --config "$CONFIG" --task "$TASK" --level "$LEVEL"

# 檢查結果
if [ $? -eq 0 ]; then
    echo "$(date): Backup successful - $TASK level $LEVEL"
else
    echo "$(date): Backup FAILED - $TASK level $LEVEL" >&2
    # 發送告警（可選）
    # curl -X POST https://hooks.slack.com/... -d "{\"text\":\"Backup failed: $TASK\"}"
    exit 1
fi
```

```bash
sudo chmod +x /usr/local/bin/zrb_snapshot_and_backup.sh
```

### 9. 設置日誌輪替
```bash
sudo vi /etc/logrotate.d/zrb
```

```
/mnt/pool/backups/zrb/logs/*/* {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0640 root root
}

/var/log/zrb/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0640 root root
}
```

### 10. 配置 S3 Lifecycle (清理 incomplete uploads)
```bash
# 創建 lifecycle.json
cat > /tmp/lifecycle.json <<'EOF'
{
  "Rules": [
    {
      "Id": "CleanupIncompleteUploads",
      "Status": "Enabled",
      "Filter": {
        "Prefix": "zfs-backups/"
      },
      "AbortIncompleteMultipartUpload": {
        "DaysAfterInitiation": 7
      }
    }
  ]
}
EOF

# 應用到 S3 bucket
aws s3api put-bucket-lifecycle-configuration \
  --bucket my-truenas-backups \
  --lifecycle-configuration file:///tmp/lifecycle.json
```

---

## 災難恢復演練

### 情境：需要還原 production_data

#### 1. 列出可用備份
```bash
sudo zrb_simple list \
  --config /etc/zrb/config.yaml \
  --task production_data \
  --source s3
```

#### 2. Dry-run 預覽
```bash
sudo zrb_simple restore \
  --config /etc/zrb/config.yaml \
  --task production_data \
  --level 0 \
  --target tank/restore_test \
  --private-key /root/.age/private.key \
  --source s3 \
  --dry-run
```

#### 3. 實際還原
```bash
# 確保目標 dataset 不存在
sudo zfs destroy tank/restore_test 2>/dev/null || true

# 執行還原
sudo zrb_simple restore \
  --config /etc/zrb/config.yaml \
  --task production_data \
  --level 0 \
  --target tank/restore_test \
  --private-key /root/.age/private.key \
  --source s3

# 驗證
sudo zfs list tank/restore_test
ls -la /tank/restore_test
```

#### 4. 如果備份在 GLACIER/DEEP_ARCHIVE

**注意**: 必須先發起 restore request！

```bash
# 1. 找出需要恢復的物件
aws s3 ls s3://my-truenas-backups/zfs-backups/data/tank/production/level0/20260207/ --recursive

# 2. 對每個 part 發起 restore（可能有多個）
aws s3api restore-object \
  --bucket my-truenas-backups \
  --key zfs-backups/data/tank/production/level0/20260207/snapshot.part-000000.age \
  --restore-request '{"Days":7,"GlacierJobParameters":{"Tier":"Bulk"}}'

# Tier 選項:
# - Bulk: 12-48 小時, $0.0025/GB
# - Standard: 3-5 小時, $0.01/GB
# - Expedited: 1-5 分鐘, $0.03/GB (DEEP_ARCHIVE 不支援)

# 3. 檢查恢復狀態
aws s3api head-object \
  --bucket my-truenas-backups \
  --key zfs-backups/data/tank/production/level0/20260207/snapshot.part-000000.age

# 4. 等待 restore-expiry-date，然後執行 zrb_simple restore
```

---

## 監控和維護

### 檢查備份狀態
```bash
# 查看最近的備份
sudo zrb_simple list \
  --config /etc/zrb/config.yaml \
  --task production_data \
  --source local | jq '.backups[0]'

# 查看所有備份摘要
sudo zrb_simple list \
  --config /etc/zrb/config.yaml \
  --task production_data \
  --source local | jq '.summary'
```

### 查看日誌
```bash
# 最近的備份日誌
tail -f /mnt/pool/backups/zrb/logs/tank/production/$(date +%Y-%m-%d).log

# Cron 執行日誌
tail -f /var/log/zrb/cron.log
```

### 檢查磁碟用量
```bash
# 本地備份空間
du -sh /mnt/pool/backups/zrb/task/

# S3 用量
aws s3 ls s3://my-truenas-backups/zfs-backups/data/ --recursive --summarize | grep "Total Size"
```

### 測試告警（建議每月）
```bash
# 模擬備份失敗
sudo zrb_simple backup --config /etc/zrb/invalid.yaml --task test --level 0 || \
  echo "Backup failed - alert should be triggered"
```

---

## 成本優化建議

### 1. 選擇合適的 Storage Class

| Backup Level | 頻率 | 建議 Storage Class | 原因 |
|-------------|------|-------------------|------|
| Level 0 | 週 | DEEP_ARCHIVE | 最便宜，很少恢復 |
| Level 1 | 日 | GLACIER | 平衡成本和恢復時間 |
| Level 2+ | 多次/日 | GLACIER_IR | 較快恢復 |
| Manifest | - | STANDARD_IA | 必須立即可存取 |

### 2. 定期清理舊備份
```bash
# TODO: 實作 cleanup 命令
# zrb_simple cleanup --task production_data --keep-days 90
```

### 3. 監控每月成本
```bash
# 使用 AWS Cost Explorer 或
aws ce get-cost-and-usage \
  --time-period Start=2026-02-01,End=2026-02-28 \
  --granularity MONTHLY \
  --metrics BlendedCost \
  --filter file://filter.json
```

---

## 故障排除

### 問題：備份失敗 "failed to acquire lock"
**原因**: 上一個備份還在運行或異常終止
**解決**:
```bash
# 檢查鎖檔案
cat /mnt/pool/backups/zrb/run/tank/production/zrb.lock

# 確認沒有進程在運行後，刪除鎖
sudo rm /mnt/pool/backups/zrb/run/tank/production/zrb.lock
```

### 問題：restore 失敗 "BLAKE3 mismatch"
**原因**: 數據損壞或傳輸錯誤
**解決**:
```bash
# 1. 重試（可能是網路問題）
# 2. 嘗試其他 level 的備份
# 3. 檢查 S3 數據完整性
aws s3api head-object --bucket ... --key ... | jq '.ETag'
```

### 問題：S3 upload 很慢
**可能原因**:
- 網路頻寬限制
- 同時上傳過多
- AWS 節流

**建議**:
- 調整備份時間（避開業務高峰）
- 考慮 AWS Direct Connect
- 分散不同 task 的執行時間

---

## 安全檢查清單

- [ ] Age 私鑰已安全存儲（chmod 600）
- [ ] 配置檔案權限正確（chmod 600）
- [ ] AWS 憑證已安全存儲
- [ ] S3 bucket 有適當的 IAM policy
- [ ] 私鑰有離線備份
- [ ] 定期測試恢復流程
- [ ] 日誌不包含敏感資訊
- [ ] Cron 腳本不暴露憑證

---

**版本**: 0.1.0-alpha.1
**最後更新**: 2026-02-07
