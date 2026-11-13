#!/usr/bin/env bash
# scripts/ios-icon-button-label-report.sh を作り物の Swift で確かめる(ネットワーク不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/ios-icon-button-label-report.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }

mkdir -p "$TMP/ios"
cat >"$TMP/ios/Bad.swift" <<'SW'
struct V: View {
    var body: some View {
        Button { go() } label: { Image(systemName: "xmark") }
    }
}
SW
cat >"$TMP/ios/Good.swift" <<'SW'
struct V: View {
    var body: some View {
        Button { go() } label: { Image(systemName: "xmark") }
            .accessibilityLabel("閉じる")
        Button {
            go()
        } label: {
            Image(systemName: "plus")
        }
        .iconButtonLabel("足す")
        Button { go() } label: { Image(systemName: "star"); Text("星") }
    }
}
SW
cat >"$TMP/ios/WithLabel.swift" <<'SW'
struct V: View {
    var body: some View {
        Button { go() } label: { Label("設定", systemImage: "gear"); Image(systemName: "gear") }
    }
}
SW
cat >"$TMP/ios/NoImage.swift" <<'SW'
struct V: View {
    var body: some View { Button("OK") { go() } }
}
SW

set +e; OUT="$("$SCRIPT" "$TMP/ios" 2>&1)"; code=$?; set -e
[ "$code" = 0 ] || fail "終了コード 0 のはずが $code"
has "Bad.swift:3"
hasnt "Good.swift"
hasnt "NoImage.swift"
hasnt "WithLabel.swift:"
has "ラベル無しの候補: 1 件(Label( で足りている所: 1 件)"

set +e; OUT="$("$SCRIPT" "$TMP/none" 2>&1)"; code=$?; set -e
[ "$code" = 2 ] || fail "無い場所で終了コード 2 のはずが $code"
echo "ok"
