#!/usr/bin/env bash
# Firestore の守り(PITR・削除保護・バックアップのスケジュール・バックアップ)を表示する。
# 読み取りだけ(describe / list)。何も作らない・変えない。docs/infra/firestore-backup-restore.md
#
#   scripts/firestore-backup-status.sh [PROJECT_ID] [DATABASE] [LOCATION]
#   既定: jphacks-kz2601 / (default) / asia-northeast1
#
# 最後に「== 判定」で守りが欠けていないかを見る。欠けていれば NG を出して終了コード 3。
#   PITR が有効・削除保護が有効・バックアップのスケジュールがある・最新の READY のバックアップが BACKUP_MAX_AGE_HOURS(既定 48)以内
# 要るコマンド: gcloud・jq。FIRESTORE_STATUS_NOW(epoch 秒)で今の時刻を差し替えられる(テスト用)
set -euo pipefail

PROJECT_ID="${1:-jphacks-kz2601}"
DATABASE="${2:-(default)}"
LOCATION="${3:-asia-northeast1}"

command -v gcloud >/dev/null || { echo "gcloud が見つかりません" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq が見つかりません" >&2; exit 1; }
MAX_AGE_HOURS="${BACKUP_MAX_AGE_HOURS:-48}"
case "$MAX_AGE_HOURS" in ''|*[!0-9]*) echo "BACKUP_MAX_AGE_HOURS は数で指定する" >&2; exit 2 ;; esac

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

# 判定(読み取りだけ。上の表と同じものを JSON で読み直す)
echo
echo "== 判定"
db_json="$(gcloud firestore databases describe --project="$PROJECT_ID" --database="$DATABASE" --format=json)"
sched_json="$(gcloud firestore backups schedules list --project="$PROJECT_ID" --database="$DATABASE" --format=json)"
backups_json="$(gcloud firestore backups list --project="$PROJECT_ID" --location="$LOCATION" --format=json)"
now="${FIRESTORE_STATUS_NOW:-$(date -u +%s)}"
ng=0
ok() { echo "OK: $*"; }
bad() { echo "NG: $*"; ng=1; }

pitr="$(jq -r '.pointInTimeRecoveryEnablement // "未設定"' <<<"$db_json")"
if [ "$pitr" = POINT_IN_TIME_RECOVERY_ENABLED ]; then ok "PITR が有効"; else bad "PITR が有効でない(${pitr})"; fi
del="$(jq -r '.deleteProtectionState // "未設定"' <<<"$db_json")"
if [ "$del" = DELETE_PROTECTION_ENABLED ]; then ok "削除保護が有効"; else bad "削除保護が有効でない(${del})"; fi
n_sched="$(jq 'length' <<<"$sched_json")"
if [ "$n_sched" -gt 0 ]; then ok "バックアップのスケジュールが ${n_sched} 件"; else bad "バックアップのスケジュールが無い"; fi

# この DB の READY のバックアップのうち一番新しいもの(snapshotTime の小数秒を落として epoch 秒で比べる)。出力は「epoch snapshotTime」
# shellcheck disable=SC2016 # jq code, not shell
latest="$(jq -r --arg db "$DATABASE" '
  map(select((.database // "" | split("/") | last) == $db and .state == "READY")
      | {t: .snapshotTime, e: (.snapshotTime | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601)})
  | max_by(.e) // empty | "\(.e) \(.t)"' <<<"$backups_json")"
if [ -z "$latest" ]; then
  bad "${DATABASE} の READY のバックアップが無い"
else
  age_h=$(((now - ${latest%% *}) / 3600))
  if [ "$age_h" -le "$MAX_AGE_HOURS" ]; then
    ok "最新のバックアップ ${latest#* }(${age_h} 時間前)"
  else
    bad "最新のバックアップが古い ${latest#* }(${age_h} 時間前・基準 ${MAX_AGE_HOURS} 時間以内)"
  fi
fi

if [ "$ng" -ne 0 ]; then
  echo "守りが欠けています。docs/infra/firestore-backup-restore.md を見て直してください。" >&2
  exit 3
fi
