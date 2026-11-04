#!/bin/sh
# Firebase の plist をリポ外の secrets フォルダから ios/Firebase/ へコピーする(xcodegen generate の前に実行・#292)。
#   GoogleService-Info.plist         (prod・必須) : Release / Review が使う
#   GoogleService-Info-Staging.plist (stg・任意)  : Debug / Staging が使う(無ければ prod の plist に戻る)
# バンドルへは Metamon ターゲットのビルドフェーズ(scripts/select-firebase-plist.sh)が構成に合う方を入れる。
# コピー先は .gitignore 済み。コミットされないことを git check-ignore で確認してから終了する。
set -eu
SECRETS_DIR="${METAMON_SECRETS_DIR:-$HOME/Desktop/JPHACKS2026/secrets}"
IOS_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DST_DIR="$IOS_DIR/Firebase"
mkdir -p "$DST_DIR"

if [ ! -f "$SECRETS_DIR/GoogleService-Info.plist" ]; then
  echo "sync-firebase-plist: $SECRETS_DIR/GoogleService-Info.plist がありません(Firebase Console からダウンロードして置いてください)" >&2
  exit 1
fi

copy() {
  dst="$DST_DIR/$1"
  cp "$SECRETS_DIR/$1" "$dst"
  if ! git -C "$IOS_DIR" check-ignore -q "$dst"; then
    echo "sync-firebase-plist: $dst が .gitignore されていません。コミットしないでください!" >&2
    rm -f "$dst"
    exit 1
  fi
  echo "sync-firebase-plist: copied -> $dst (git-ignored)"
}

copy GoogleService-Info.plist
if [ -f "$SECRETS_DIR/GoogleService-Info-Staging.plist" ]; then
  copy GoogleService-Info-Staging.plist
else
  rm -f "$DST_DIR/GoogleService-Info-Staging.plist"
  echo "sync-firebase-plist: stg の plist は無し。Debug / Staging も prod の Firebase で認証します(ビルド時に警告)"
fi

# #292 より前の置き場(Metamon/Resources)に残ったコピーを消す。今はビルドフェーズがバンドルへ入れる
rm -f "$IOS_DIR/Metamon/Resources/GoogleService-Info.plist"
