#!/usr/bin/env bash
# Firestore の守り(PITR・削除保護・バックアップのスケジュール・バックアップ)を表示する。
# 読み取りだけ(describe / list)。何も作らない・変えない。docs/infra/firestore-backup-restore.md
#
#   scripts/firestore-backup-status.sh [PROJECT_ID] [DATABASE] [LOCATION]
#   既定: jphacks-kz2601 / (default) / asia-northeast1
set -euo pipefail

PROJECT_ID="${1:-jphacks-kz2601}"
DATABASE="${2:-(default)}"
LOCATION="${3:-asia-northeast1}"

command -v gcloud >/dev/null || { echo "gcloud が見つかりません" >&2; exit 1; }

echo "== database: ${PROJECT_ID}/${DATABASE}"
gcloud firestore databases describe --project="$PROJECT_ID" --database="$DATABASE" \
  --format='table[box](
    pointInTimeRecoveryEnablement:label=PITR,
    deleteProtectionState:label=DELETE_PROTECTION,
    versionRetentionPeriod:label=RETENTION,
    earliestVersionTime:label=EARLIEST_VERSION)'

echo
echo "== backup schedules"
gcloud firestore backups schedules list --project="$PROJECT_ID" --database="$DATABASE" \
  --format='yaml(name.basename(),retention,dailyRecurrence,weeklyRecurrence)'

echo
echo "== backups (${LOCATION}, newest first)"
gcloud firestore backups list --project="$PROJECT_ID" --location="$LOCATION" \
  --sort-by='~snapshotTime' \
  --format='table(name.basename():label=BACKUP_ID,database.basename():label=DATABASE,snapshotTime,expireTime,state)'

echo
echo "== databases in project (restore 用の DB が残っていないか)"
gcloud firestore databases list --project="$PROJECT_ID" \
  --format='table(name.basename():label=DATABASE,locationId,deleteProtectionState)'
