#!/usr/bin/env bash
# scripts/ios-fixed-font-report.sh を作り物の Swift で確かめる(Xcode 不要)。
set -euo pipefail
cd "$(dirname "$0")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/ios/App/Views" "$TMP/ios/.build/checkouts"

cat >"$TMP/ios/App/Views/Card.swift" <<'EOF'
import SwiftUI

struct Card: View {
    @ScaledMetric(relativeTo: .body) private var iconSize: CGFloat = 24
    let diameter: CGFloat

    var body: some View {
        VStack {
            Text("固定").font(.system(size: 17, weight: .bold))
            Text("式").font(.system(size: diameter * 0.4))
            Image(systemName: "star").font(.system(size: 18))
            Image(systemName: "moon")
                .resizable()
                .font(.system(size: 20, weight: .semibold))
            Text("追従").font(.system(size: iconSize))
            Text("self").font(.system(size: self.iconSize * 2))
            Text("相対").font(.custom("X", size: 12, relativeTo: .body)).font(.system(size: 12, relativeTo: .body))
            // Text("コメント").font(.system(size: 9))
            Text("ふつう").font(.body)
            Text("二つ").font(.system(size: 9)).overlay(Text("b").font(.system(size: 8)))
        }
        Text("頭")
            .padding()
            .font(.system(size: 11))
    }
}
EOF

cat >"$TMP/ios/App/Clean.swift" <<'EOF'
import SwiftUI
struct Clean: View { var body: some View { Text("a").font(.headline) } }
EOF

# 依存のチェックアウトは見ない
cat >"$TMP/ios/.build/checkouts/Dep.swift" <<'EOF'
let x = Text("x").font(.system(size: 10))
EOF

OUT="$(./ios-fixed-font-report.sh "$TMP/ios")"
fail() { printf '%s\n' "$OUT" >&2; echo "FAIL: $1" >&2; exit 1; }
has() { grep -qxF -- "$1" <<<"$OUT" || fail "出力に「$1」の行が無い"; }

# 文字: 17・式・二つ(1 行)・頭の後ろの修飾 = 4 / 絵: star・moon の並び = 2
has "文字	絵	ファイル"
has "4	2	ios/App/Views/Card.swift"
has "合計: 文字 4 行・絵 2 行(1 ファイル)"
grep -q "Clean.swift" <<<"$OUT" && fail "固定の無いファイルは出さない"
grep -q "Dep.swift" <<<"$OUT" && fail ".build の下は見ない"

# 何も無いディレクトリでも落ちない
mkdir -p "$TMP/empty"
OUT="$(./ios-fixed-font-report.sh "$TMP/empty")"
has "合計: 文字 0 行・絵 0 行(0 ファイル)"

# 無いディレクトリは 2
set +e
./ios-fixed-font-report.sh "$TMP/none" 2>/dev/null
code=$?
set -e
[ "$code" = 2 ] || { echo "FAIL: 無いディレクトリは終了コード 2 のはずが $code" >&2; exit 1; }

echo "ok: ios-fixed-font-report"
