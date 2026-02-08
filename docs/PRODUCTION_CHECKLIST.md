# Production Deployment Checklist

## 生產部署前檢查清單

Last Updated: 2026-02-07
Version: 0.1.0-alpha.1

---

## ✅ 已完成功能 (Ready for Production)

### 核心命令
- [x] **backup** - 完整備份流程（L0-L4）
  - 支援 full/incremental/differential 備份
  - 3GB 分片
  - BLAKE3 完整性驗證
  - Age 加密
  - 並發上傳（4 workers）
  - 可恢復上傳（backup_state.yaml）
- [x] **list** - 列出可用備份
  - JSON 格式輸出
  - 本地/S3 來源
  - Level 過濾
  - Glacier 檢查
- [x] **restore** - Phase 1 還原
  - Dry-run 預覽
  - 本地/S3 來源（立即可存取的 storage class）
  - SHA256 + BLAKE3 驗證
  - 自動解密和 ZFS receive
- [x] **snapshot** - 手動創建快照
- [x] **genkey** - 生成 age 密鑰對

### 安全性
- [x] Age 加密（所有備份資料）
- [x] SHA256 驗證（每個加密分片）
- [x] BLAKE3 驗證（整個快照）
- [x] 檔案鎖機制（防止並發執行）

### 可靠性
- [x] AWS SDK Standard 重試機制（可配置）
- [x] 可恢復上傳（中斷後繼續）
- [x] ZFS snapshot hold/release
- [x] 原子性操作（.tmp 重命名）

### S3 Glacier 優化
- [x] 按 level 配置 storage class
- [x] Manifest 使用 STANDARD_IA（立即可存取）
- [x] 明確拒絕從 GLACIER 讀取（避免意外費用）
- [x] CRC32 自動校驗（AWS SDK）
- [x] Multipart upload（64MB chunks）

---

## ⚠️ 生產部署建議

### 🔴 必須處理 (CRITICAL)

#### 1. 私鑰安全管理
**當前狀態**: 私鑰以明文形式存儲

**建議**:
```bash
# 使用嚴格權限
chmod 600 /path/to/age_private_key.txt
chown root:root /path/to/age_private_key.txt

# 或使用密鑰管理服務
# - AWS Secrets Manager
# - HashiCorp Vault
# - 系統 keyring
```

**待實作**: genkey 命令應該自動設置正確權限

#### 2. 監控和告警
**當前狀態**: 僅有 slog 日誌輸出

**必須添加**:
- 備份失敗告警（email/Slack/PagerDuty）
- 備份成功確認通知
- Storage quota 監控
- 長時間運行告警（可能卡住）

**建議實作**:
```bash
# Cron 包裝腳本
#!/bin/bash
if ! /usr/local/bin/zrb_simple backup --config /etc/zrb/config.yaml --task prod_backup --level 0; then
    # 發送告警
    curl -X POST https://hooks.slack.com/... -d "Backup failed!"
    exit 1
fi
```

#### 3. 備份驗證
**當前狀態**: 只在 restore 時驗證

**建議添加**:
- 定期 restore 測試（每週/每月）
- 自動驗證腳本
- Checksum 記錄和比對

```bash
# 每月驗證腳本
0 0 1 * * /usr/local/bin/verify_backup.sh
```

#### 4. Glacier 恢復流程文檔
**當前狀態**: restore 會拒絕 GLACIER，但沒有詳細流程

**需要文檔化**:
```bash
# 步驟 1: 發起 Glacier restore request
aws s3api restore-object \
  --bucket my-backup-bucket \
  --key zfs-backups/data/pool/dataset/level0/20260125/snapshot.part-000000.age \
  --restore-request '{"Days":7,"GlacierJobParameters":{"Tier":"Bulk"}}'

# 步驟 2: 等待 12-48 小時（DEEP_ARCHIVE）

# 步驟 3: 檢查狀態
aws s3api head-object --bucket my-backup-bucket --key <key>

# 步驟 4: restore 數據
zrb_simple restore --config config.yaml --task prod_backup --level 0 ...
```

#### 5. 災難恢復計劃 (Disaster Recovery Plan)
**必須準備**:
1. **完整的配置備份**（包括 age 私鑰）
2. **恢復順序文檔**（先 L0，再 L1...）
3. **緊急聯絡人**
4. **預估恢復時間** (RTO)
5. **測試紀錄**

---

### 🟡 強烈建議 (HIGH PRIORITY)

#### 6. 配置檔案保護
```bash
# 建議權限
chmod 600 /etc/zrb/config.yaml
chown root:root /etc/zrb/config.yaml

# 不應包含在備份中
echo "*.yaml" >> /path/to/.gitignore
```

#### 7. 日誌管理
**當前**: 日誌寫入 `{base_dir}/logs/{pool}/{dataset}/YYYY-MM-DD.log`

**建議**:
- Log rotation（logrotate）
- 中央化日誌（rsyslog/journald）
- 保留策略（30-90 天）
- JSON 格式日誌（便於解析）

```bash
# /etc/logrotate.d/zrb
/mnt/p1/ds1/bk/logs/*/* {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
}
```

#### 8. Cron 排程設置
```bash
# /etc/cron.d/zrb-backup

# 每天 2:00 AM - Level 0 (週日)
0 2 * * 0 root /usr/local/bin/zrb_simple backup --config /etc/zrb/config.yaml --task prod --level 0 >> /var/log/zrb/cron.log 2>&1

# 每天 2:00 AM - Level 1 (週一到週六)
0 2 * * 1-6 root /usr/local/bin/zrb_simple backup --config /etc/zrb/config.yaml --task prod --level 1 >> /var/log/zrb/cron.log 2>&1

# Level 2-4 依需求排程
```

#### 9. S3 Lifecycle Policy
**目的**: 清理 incomplete multipart uploads

```json
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
```

#### 10. 容量規劃
**需要追蹤**:
- 每日備份大小
- 增量變化率
- S3 storage class 分佈
- 月度成本

**工具**:
```bash
# 監控腳本
#!/bin/bash
BUCKET="my-backup-bucket"
PREFIX="zfs-backups/data/"

aws s3 ls s3://$BUCKET/$PREFIX --recursive --summarize | \
  grep "Total Size" | \
  awk '{print $3}' | \
  numfmt --to=iec-i --suffix=B
```

---

### 🟢 建議改進 (NICE TO HAVE)

#### 11. 並發下載優化
**當前**: restore 時順序下載和解密
**建議**: 並發下載多個 parts（類似 backup 的 worker pool）

#### 12. 進度顯示
**當前**: 只有日誌輸出
**建議**: 進度條（backup/restore 時）

#### 13. 配置驗證
**建議添加**:
```bash
zrb_simple validate-config --config config.yaml
```

檢查：
- Storage class 配置合理性
- S3 連接性
- 權限設置
- Age 密鑰有效性

#### 14. Cleanup 命令
**用途**: 清理舊備份

```bash
zrb_simple cleanup \
  --config config.yaml \
  --task prod \
  --keep-last 7 \
  --keep-weekly 4 \
  --keep-monthly 12
```

#### 15. 健康檢查端點
**用途**: 監控系統整合

```bash
zrb_simple healthcheck --config config.yaml
# 返回 JSON: last_backup, disk_space, s3_connectivity
```

---

## 📋 部署步驟

### 1. 編譯
```bash
cd simple_backup
GOOS=linux GOARCH=amd64 go build -o zrb_simple
```

### 2. 安裝
```bash
# TrueNAS Scale 或其他 Linux
sudo cp zrb_simple /usr/local/bin/
sudo chmod +x /usr/local/bin/zrb_simple
```

### 3. 配置
```bash
# 創建配置目錄
sudo mkdir -p /etc/zrb
sudo chmod 700 /etc/zrb

# 生成密鑰
zrb_simple genkey > /tmp/keys.txt
# 手動分離 public/private key 並安全存儲

# 創建配置檔案
sudo vi /etc/zrb/config.yaml
sudo chmod 600 /etc/zrb/config.yaml
```

### 4. 測試
```bash
# Dry-run
sudo zrb_simple backup --config /etc/zrb/config.yaml --task test --level 0 --dry-run

# 實際備份（小規模測試）
sudo zrb_simple backup --config /etc/zrb/config.yaml --task test --level 0

# 驗證
sudo zrb_simple list --config /etc/zrb/config.yaml --task test

# 恢復測試
sudo zrb_simple restore --config /etc/zrb/config.yaml --task test --level 0 \
  --target testpool/restore_test --private-key /etc/zrb/private.key --dry-run
```

### 5. 設置 Cron
```bash
sudo vi /etc/cron.d/zrb-backup
# 添加排程（見上方範例）
```

### 6. 監控設置
- 配置告警（Slack/Email）
- 設置日誌輪替
- 建立儀表板（Grafana）

---

## 🚨 已知限制

### Phase 1 限制
1. **不支援 Glacier 自動恢復** - 需手動使用 AWS CLI
2. **單一 level restore** - 不會自動還原整個 backup chain
3. **順序下載** - restore 時不並發下載
4. **無進度顯示** - 只有日誌輸出

### 設計限制
1. **3GB 固定分片大小** - 不可配置
2. **4 個固定 workers** - 不可配置
3. **ZFS 特定** - 只能用於 ZFS 系統

---

## 📊 成本估算（AWS S3 Glacier）

### 假設
- Pool: 1TB
- Level 0 (weekly): 1TB → DEEP_ARCHIVE
- Level 1 (daily): 10GB/day → GLACIER
- Manifest: 10KB → STANDARD_IA

### 月度成本估算
| 項目 | 容量 | Storage Class | 單價 | 月費 |
|------|------|---------------|------|------|
| L0 (4 weeks) | 4TB | DEEP_ARCHIVE | $0.00099/GB | $4.06 |
| L1 (28 days) | 280GB | GLACIER | $0.004/GB | $1.12 |
| Manifest | 1MB | STANDARD_IA | $0.0125/GB | ~$0 |
| **總計** | | | | **$5.18** |

### 恢復成本（緊急情況）
- DEEP_ARCHIVE Expedited: $0.03/GB + $0.10/request
- 1TB 恢復: ~$30 + retrieval time (12-48h)

---

## ✅ 生產就緒度評估

| 分類 | 狀態 | 評分 | 備註 |
|------|------|------|------|
| **核心功能** | ✅ Ready | 9/10 | 缺少 cleanup 命令 |
| **安全性** | ⚠️ Needs Work | 7/10 | 私鑰管理需改善 |
| **可靠性** | ✅ Ready | 9/10 | 已有重試和恢復機制 |
| **監控** | ❌ Missing | 3/10 | 需要添加告警 |
| **文檔** | ⚠️ Basic | 6/10 | 需要更多操作文檔 |
| **測試** | ✅ Tested | 8/10 | 已通過基本測試 |
| **整體** | ⚠️ **Alpha** | **7/10** | **可小規模試用，需完善監控和告警** |

---

## 🎯 建議的部署路徑

### Phase 0: 準備階段 (1-2 天)
- [ ] 完善私鑰安全管理
- [ ] 設置監控和告警
- [ ] 撰寫災難恢復文檔
- [ ] 配置 S3 lifecycle policy

### Phase 1: 試運行 (1-2 週)
- [ ] 小規模 dataset 測試
- [ ] 每日備份監控
- [ ] 至少一次完整 restore 測試
- [ ] 驗證成本符合預期

### Phase 2: 擴展部署 (2-4 週)
- [ ] 逐步增加 datasets
- [ ] 監控效能和成本
- [ ] 建立運維流程
- [ ] 團隊培訓

### Phase 3: 生產穩定 (ongoing)
- [ ] 定期 restore 演練（每月）
- [ ] 成本優化
- [ ] 功能增強（cleanup, health check）
- [ ] 考慮 Phase 2 restore (Glacier 自動恢復)

---

## 📞 緊急聯絡

**備份失敗**:
1. 檢查日誌: `{base_dir}/logs/{pool}/{dataset}/`
2. 檢查鎖: `{base_dir}/run/{pool}/{dataset}/zrb.lock`
3. 檢查狀態: `{base_dir}/run/{pool}/{dataset}/backup_state.yaml`

**恢復失敗**:
1. 確認 storage class 可存取
2. 確認私鑰正確
3. 確認 manifest 存在
4. 手動驗證 BLAKE3: `blake3sum merged_file`

**Glacier 恢復**:
1. 參考上方 Glacier 恢復流程
2. 預期等待時間: 12-48 小時（DEEP_ARCHIVE Bulk）
3. 加急選項: Expedited (~$30/TB, 1-5 分鐘)

---

**Last Review**: 2026-02-07
**Next Review**: Before production deployment
**Owner**: Infrastructure Team
