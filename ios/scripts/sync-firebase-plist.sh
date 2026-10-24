#!/bin/sh
# Firebase の GoogleService-Info.plist をリポ外の secrets フォルダからアプリの Resources へコピーする(xcodegen generate の前に実行)。
# コピー先は .gitignore 済み。コミットされないことを git check-ignore で確認してから終了する。
set -eu
SECRETS_DIR="${METAMON_SECRETS_DIR:-$HOME/Desktop/JPHACKS2026/secrets}"
SRC="$SECRETS_DIR/GoogleService-Info.plist"
IOS_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DST="$IOS_DIR/Metamon/Resources/GoogleService-Info.plist"

if [ ! -f "$SRC" ]; then
  echo "sync-firebase-plist: $SRC がありません(Firebase Console からダウンロードして置いてください)" >&2
  exit 1
fi
cp "$SRC" "$DST"
if ! git -C "$IOS_DIR" check-ignore -q "$DST"; then
  echo "sync-firebase-plist: $DST が .gitignore されていません。コミットしないでください!" >&2
  rm -f "$DST"
  exit 1
fi
echo "sync-firebase-plist: copied -> $DST (git-ignored)"
