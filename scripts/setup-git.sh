#!/usr/bin/env bash
# clone ごとに 1 回だけ実行する: .xcstrings のマージドライバを有効にする。
# rerere は使わない: 解き方のキャッシュは worktree 間で共有され、別のブランチの解き方が
# 自動で当たって中身が消えかけたことがある(2026-11-08)。明示的に無効にしておく。
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
git config merge.xcstrings.name "xcstrings key-level 3-way merge"
git config merge.xcstrings.driver "python3 ios/scripts/merge-xcstrings.py %O %A %B"
git config rerere.enabled false
echo "merge.xcstrings を有効にし、rerere を無効にしました"
