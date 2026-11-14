#!/bin/bash
# ポスターの HTML を A1 の PDF にする(Google Chrome と poppler が要る)
set -euo pipefail
cd "$(dirname "$0")"
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless=new --disable-gpu --no-pdf-header-footer --virtual-time-budget=4000 \
  --print-to-pdf="$PWD/out.pdf" "file://$PWD/poster.html" 2>/dev/null
pdfinfo out.pdf | grep -E "Pages|Page size"
pdftoppm -r 60 -png -f 1 -l 1 out.pdf page   # page-1.png(見本)
