#!/usr/bin/env bash
# scripts/firestore-backup-status.sh の判定を偽の gcloud で確かめる(GCP に触らない・jq だけ)。
# 偽の gcloud は --format=json の時に $FAKE_DIR/{describe,schedules,backups}.json を返し、表の時は 1 行の目印だけ出す。
# 正常の組は testdata/firestore-backup-status/。崩した組は jq で一時ディレクトリに作る。
set -euo pipefail
cd "$(dirname "$0")"
D=testdata/firestore-backup-status
fail() { echo "FAIL: $*" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"

cat >"$TMP/bin/gcloud" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"$TMP/calls"
case "\$*" in
  "firestore databases describe"*) kind=describe ;;
  "firestore backups schedules list"*) kind=schedules ;;
  "firestore backups list"*) kind=backups ;;
  "firestore databases list"*) kind=databases ;;
  *) echo "fake gcloud: unexpected \$*" >&2; exit 1 ;;
esac
case "\$*" in
  *--format=json*) cat "\$FAKE_DIR/\$kind.json" ;;
  *) echo "(table \$kind)" ;;
esac
EOF
chmod +x "$TMP/bin/gcloud"

NOW="$(jq -n '"2026-11-13T00:00:00Z" | fromdateiso8601')"
# case_dir <名前> [<ファイル> <jq>]: 正常の組を写し、1 つのファイルだけ jq で崩す
case_dir() {
  local dir="$TMP/$1"
  mkdir -p "$dir"
  cp "$D"/*.json "$dir/"
  if [ $# -ge 3 ]; then
    jq "$3" "$D/$2.json" >"$dir/$2.json"
  fi
  echo "$dir"
}
# expect <期待コード> <ディレクトリ> [args...]: 出力を OUT に入れる
expect() {
  local want="$1" dir="$2" code; shift 2
  set +e
  OUT="$(PATH="$TMP/bin:$PATH" FAKE_DIR="$dir" FIRESTORE_STATUS_NOW="$NOW" ./firestore-backup-status.sh fake-project "$@" 2>&1)"
  code=$?
  set -e
  [ "$code" = "$want" ] || { printf '%s\n' "$OUT" >&2; fail "$(basename "$dir"): 終了コード $want のはずが $code"; }
}
has() { grep -qxF -- "$1" <<<"$OUT" || { printf '%s\n' "$OUT" >&2; fail "出力に「${1}」の行が無い"; }; }
hasnt() { if grep -qF -- "$1" <<<"$OUT"; then printf '%s\n' "$OUT" >&2; fail "出力に「${1}」がある"; fi; }

# 1. 正常: 4 つとも OK・終了コード 0・別の DB と CREATING のバックアップは見ない
expect 0 "$(case_dir ok)"
has "== database: fake-project/(default)"
has "(table describe)"
has "OK: PITR が有効"
has "OK: 削除保護が有効"
has "OK: バックアップのスケジュールが 1 件"
has "OK: 最新のバックアップ 2026-11-12T18:00:00.123456Z(6 時間前)"
hasnt "NG:"
grep -q -- '--project=fake-project --database=(default) --format=json' "$TMP/calls" || fail "PROJECT_ID・DATABASE が渡っていない"
grep -q -- 'backups list --project=fake-project --location=asia-northeast1 --format=json' "$TMP/calls" || fail "既定の LOCATION"

# 2. PITR なし
expect 3 "$(case_dir no-pitr describe '.pointInTimeRecoveryEnablement = "POINT_IN_TIME_RECOVERY_DISABLED"')"
has "NG: PITR が有効でない(POINT_IN_TIME_RECOVERY_DISABLED)"
has "OK: 削除保護が有効"
has "守りが欠けています。docs/infra/firestore-backup-restore.md を見て直してください。"
expect 3 "$(case_dir pitr-missing describe 'del(.pointInTimeRecoveryEnablement)')"
has "NG: PITR が有効でない(未設定)"

# 3. 削除保護なし
expect 3 "$(case_dir no-delete-protection describe '.deleteProtectionState = "DELETE_PROTECTION_DISABLED"')"
has "NG: 削除保護が有効でない(DELETE_PROTECTION_DISABLED)"
has "OK: PITR が有効"

# 4. スケジュールなし
expect 3 "$(case_dir no-schedule schedules '[]')"
has "NG: バックアップのスケジュールが無い"

# 5. 最後のバックアップが古い(既定 48 時間)・BACKUP_MAX_AGE_HOURS で変えられる
expect 3 "$(case_dir stale backups 'map(.snapshotTime = "2026-11-10T00:00:00Z")')"
has "NG: 最新のバックアップが古い 2026-11-10T00:00:00Z(72 時間前・基準 48 時間以内)"
day_old="$(case_dir day-old backups 'map(select(.name | endswith("b-old")))')"
expect 0 "$day_old"
has "OK: 最新のバックアップ 2026-11-11T18:00:00Z(30 時間前)"
BACKUP_MAX_AGE_HOURS=24 expect 3 "$day_old"
has "NG: 最新のバックアップが古い 2026-11-11T18:00:00Z(30 時間前・基準 24 時間以内)"

# 6. この DB の READY のバックアップが無い(別の DB・作成中だけ)
expect 3 "$(case_dir no-backup backups 'map(select(.name | endswith("b-other") or endswith("b-creating")))')"
has "NG: (default) の READY のバックアップが無い"

# 7. BACKUP_MAX_AGE_HOURS が数でなければ gcloud を呼ばずに 2
rm -f "$TMP/calls"
BACKUP_MAX_AGE_HOURS=x expect 2 "$TMP/ok"
[ ! -e "$TMP/calls" ] || fail "BACKUP_MAX_AGE_HOURS=x で gcloud を呼んだ"
echo "ok"
