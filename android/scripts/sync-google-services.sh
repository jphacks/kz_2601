#!/bin/sh
# Firebase の google-services.json をリポ外の secrets フォルダから android/app/src/<flavor>/ へコピーする(#356)。
# iOS の ios/scripts/sync-firebase-plist.sh と同じ置き場・同じ確かめ方。
#   google-services.json      (prod・必須) -> app/src/prod/google-services.json
#   google-services-stg.json  (stg・任意)  -> app/src/stg/google-services.json
# 無い flavor はビルドが警告だけ出して通り、実行時はモックの認証(固定 uid)で動く。
# コピー先は .gitignore 済み。コミットされないことを git check-ignore で確かめてから終わる。
set -eu
SECRETS_DIR="${METAMON_SECRETS_DIR:-$HOME/Desktop/JPHACKS2026/secrets}"
ANDROID_DIR="$(cd "$(dirname "$0")/.." && pwd)"

if [ ! -f "$SECRETS_DIR/google-services.json" ]; then
  echo "sync-google-services: $SECRETS_DIR/google-services.json がありません(Firebase Console の Android アプリからダウンロードして置いてください)" >&2
  exit 1
fi

copy() {
  src="$SECRETS_DIR/$1"
  dst="$ANDROID_DIR/app/src/$2/google-services.json"
  mkdir -p "$(dirname "$dst")"
  cp "$src" "$dst"
  if ! git -C "$ANDROID_DIR" check-ignore -q "$dst"; then
    echo "sync-google-services: $dst が .gitignore されていません。コミットしないでください!" >&2
    rm -f "$dst"
    exit 1
  fi
  echo "sync-google-services: copied -> $dst (git-ignored)"
}

copy google-services.json prod
if [ -f "$SECRETS_DIR/google-services-stg.json" ]; then
  copy google-services-stg.json stg
else
  rm -f "$ANDROID_DIR/app/src/stg/google-services.json"
  echo "sync-google-services: stg の json は無し。stg のビルドはモックの認証で動きます(ビルド時に警告)"
fi
