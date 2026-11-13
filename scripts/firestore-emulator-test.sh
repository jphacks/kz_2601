#!/usr/bin/env bash
# scripts/firestore-emulator.sh の「黙ってスキップさせない」判定(judge)を作り物の go test -json で確かめる
# (エミュレータ・gcloud・Go 不要・jq だけ)。testdata/firestore-emulator/*.json は go test -json の形の JSON lines。
set -euo pipefail
cd "$(dirname "$0")"
D=testdata/firestore-emulator
fail() { echo "FAIL: $*" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# expect <期待コード> <入力> [go test の終了コード]: 出力を OUT に入れる
expect() {
  local want="$1" code; shift
  set +e; OUT="$(env -u GITHUB_STEP_SUMMARY ./firestore-emulator.sh judge "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || { printf '%s\n' "$OUT" >&2; fail "judge $*: 終了コード $want のはずが $code"; }
}
has() { grep -qxF -- "$1" <<<"$OUT" || { printf '%s\n' "$OUT" >&2; fail "出力に「${1}」の行が無い"; }; }

# 1. 全部通る: TestFirestore* と */firestore を数える(関係ないテストのスキップは気にしない)
expect 0 "$D/pass.json"
has "Firestore emulator tests passed: 2"
# summary にも書く
GITHUB_STEP_SUMMARY="$TMP/summary.md" ./firestore-emulator.sh judge "$D/pass.json" >/dev/null
[ "$(cat "$TMP/summary.md")" = "Firestore emulator tests passed: 2" ] || fail "summary: $(cat "$TMP/summary.md")"

# 2. エミュレータが無くてスキップ(理由に FIRESTORE_EMULATOR_HOST not set)
expect 1 "$D/skip-emulator.json"
has "::error::emulator tests were skipped"

# 3. Firestore 系がほかの理由でスキップ: スキップした名前を全部出す
expect 1 "$D/skip-firestore.json"
has "::error::skipped Firestore tests:"
has "TestFirestoreIndex"
has "TestStoreContract/firestore"

# 4. 失敗: go test の終了コードを返す(通った数は出す)
expect 1 "$D/fail.json" 1
has "Firestore emulator tests passed: 1"
expect 2 "$D/fail.json" 2

# 5. Firestore 系が 1 件も無い(どれも通っていない)
grep -v -e 'Firestore' -e '/firestore' "$D/pass.json" >"$TMP/none.json"
expect 1 "$TMP/none.json"
has "Firestore emulator tests passed: 0"
has "::error::no Firestore emulator test ran"

# 6. 入力が無ければ使い方で 2
expect 2
echo "ok"
