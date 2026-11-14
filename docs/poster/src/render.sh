#!/bin/zsh
set -e
cd $(cd "$(dirname "$0")" && pwd)
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless=new --disable-gpu --no-pdf-header-footer --virtual-time-budget=4000 --print-to-pdf=$(cd "$(dirname "$0")" && pwd)/out.pdf file://$(cd "$(dirname "$0")" && pwd)/poster.html 2>/dev/null
pdfinfo out.pdf | grep -E "Pages|Page size"
pdftoppm -r 60 -png -f 1 -l 1 out.pdf page   # page-1.png (overview)
pdftoppm -r 150 -png -f 1 -l 1 -x 0 -y 300 -W 3508 -H 2300 out.pdf hero  # hero crop at 150dpi
ls -la page-1.png hero-1.png
