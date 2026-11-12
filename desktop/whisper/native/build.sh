#!/usr/bin/env bash
# whisper.cpp + JNI の口を、今の機械(Linux x64・macOS)向けに作る(#1751)。
# 出来た物は out/<os>-<arch>/ に置く(Compose の appResourcesRootDir の形。配る形に同梱され、:desktop:run でも読む)。git には入れない。
# 要る物: CMake 3.22 以上・C/C++ コンパイラ・JDK(JAVA_HOME か PATH の java)・取得のための通信(初回だけ)。
# 使い方: desktop/whisper/native/build.sh
set -euo pipefail
cd "$(dirname "$0")"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=macos ;;
  *) echo "Windows は build.ps1 を使う" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=x64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "知らない CPU: $(uname -m)" >&2; exit 1 ;;
esac
target="$os-$arch"

if [ -z "${JAVA_HOME:-}" ]; then
  if [ "$os" = macos ] && /usr/libexec/java_home >/dev/null 2>&1; then
    JAVA_HOME="$(/usr/libexec/java_home)"
  elif command -v java >/dev/null; then
    JAVA_HOME="$(java -XshowSettings:properties -version 2>&1 | sed -n 's/^ *java.home = //p')"
  fi
  export JAVA_HOME
fi
echo "JAVA_HOME=${JAVA_HOME:-?}"

cmake -S . -B "build/$target" -DCMAKE_BUILD_TYPE=Release
cmake --build "build/$target" --config Release --target metamon_whisper -j "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"

lib=libmetamon_whisper.so
[ "$os" = macos ] && lib=libmetamon_whisper.dylib
mkdir -p "out/$target"
cp "build/$target/$lib" "out/$target/$lib"
echo "out/$target/$lib"
