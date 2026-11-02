#!/usr/bin/env bash
# clone ごとに 1 回だけ実行する: .xcstrings のマージドライバと rerere を有効にする。
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
git config merge.xcstrings.name "xcstrings key-level 3-way merge"
git config merge.xcstrings.driver "python3 ios/scripts/merge-xcstrings.py %O %A %B"
git config rerere.enabled true
echo "merge.xcstrings と rerere を有効にしました"
