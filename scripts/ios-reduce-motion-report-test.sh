#!/usr/bin/env bash
# scripts/ios-reduce-motion-report.sh を作り物の Swift で確かめる(ネットワーク不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/ios-reduce-motion-report.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }

mkdir -p "$TMP/ios/Features" "$TMP/ios/Tests"
cat >"$TMP/ios/Features/Bad.swift" <<'SW'
struct A: View { var body: some View { Circle().animation(.easeIn.repeatForever(), value: x) } }
SW
cat >"$TMP/ios/Features/BadTimeline.swift" <<'SW'
struct B: View { var body: some View { TimelineView(.animation) { _ in Text("x") } } }
SW
cat >"$TMP/ios/Features/Good.swift" <<'SW'
struct C: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    var body: some View { Circle().animation(reduceMotion ? nil : .easeIn.repeatForever(), value: x) }
}
SW
cat >"$TMP/ios/Features/Plain.swift" <<'SW'
struct D: View { var body: some View { Text("x") } }
SW
cat >"$TMP/ios/Tests/FakeTests.swift" <<'SW'
let s = "repeatForever"
SW

set +e; OUT="$("$SCRIPT" "$TMP/ios" 2>&1)"; code=$?; set -e
[ "$code" = 0 ] || fail "終了コード 0 のはずが $code"
has "Features/Bad.swift"
has "Features/BadTimeline.swift"
hasnt "Good.swift"
hasnt "Plain.swift"
hasnt "FakeTests.swift"
has "見ていないファイル: 2 件"

set +e; OUT="$("$SCRIPT" "$TMP/none" 2>&1)"; code=$?; set -e
[ "$code" = 2 ] || fail "無い場所で終了コード 2 のはずが $code"
echo "ok"
