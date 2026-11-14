#!/usr/bin/env bash
# 絵だけのボタン(Button の label が Image(systemName:) だけ)で、読み上げの名前が無い所の候補を一覧に出す(#3083)。
# 読むだけ。見落としの候補なので誤検知はある。落とさない(終了コードは 0。引数の誤りだけ 2)。
# 使い方: scripts/ios-icon-button-label-report.sh [<探す場所>...]   既定は ios/
# 見方: Button の行から次の Button の手前まで(最大 12 行)に Image(systemName が有り、
#   accessibilityLabel・iconButtonLabel・Label(・Text( が無ければ「ラベル無し」。
# 出力: 「<パス>:<行>」を 1 行ずつ + 件数。Label( だけの所は数だけ別に出す。
set -euo pipefail
cd "$(dirname "$0")/.."
[ "$#" -gt 0 ] || set -- ios
for d in "$@"; do [ -e "$d" ] || { echo "ない場所: $d" >&2; exit 2; }; done
python3 -I - "$@" <<'PY'
import os, re, sys

BTN = re.compile(r'\bButton\s*[({]')
OK = re.compile(r'accessibilityLabel|iconButtonLabel|Text\(')
files = []
for root in sys.argv[1:]:
    if os.path.isfile(root):
        files.append(root)
        continue
    for dp, dn, fn in os.walk(root):
        dn[:] = sorted(d for d in dn if d not in ('.build', 'build', 'DerivedData', '.git', 'Pods'))
        files += [os.path.join(dp, f) for f in sorted(fn) if f.endswith('.swift')]

missing, with_label = [], 0
for path in sorted(files):
    with open(path, encoding='utf-8', errors='replace') as fh:
        lines = fh.read().splitlines()
    for i, line in enumerate(lines):
        if not BTN.search(line):
            continue
        end = min(i + 12, len(lines))
        for j in range(i + 1, end):
            if BTN.search(lines[j]):
                end = j
                break
        win = '\n'.join(lines[i:end])
        if 'Image(systemName' not in win:
            continue
        if OK.search(win):
            continue
        if 'Label(' in win:
            with_label += 1
            continue
        missing.append('%s:%d' % (path, i + 1))

for m in missing:
    print(m)
print('ラベル無しの候補: %d 件(Label( で足りている所: %d 件)' % (len(missing), with_label))
PY
