# Tailor It の PV(ブラウザで再生)

発表の時に流す映像を、1 枚の HTML で作ったもの。ビルドは要らない。

## 開く

- `index.html` をブラウザで開く(ダブルクリックでよい)。
- 文字の形をそろえたい時(Google Fonts を読むので、ネットにつながっている方がよい)はローカルのサーバーで開く:

  ```sh
  cd web/pv
  python3 -m http.server 8000
  # http://localhost:8000/
  ```

画面は 16:9 で、ウィンドウの大きさに合わせて縮む(上下か左右に帯が出る)。約 58 秒で 1 周し、くり返す。

## 操作

| 操作 | 動き |
|---|---|
| クリック / スペース | 一時停止・再生 |
| ← / → | 前の場面 / 次の場面 |
| 下の点 | その場面へ |
| 右下のボタン / F | 全画面 |

操作のバーは、再生中は 2 秒で隠れる。マウスを動かすと出る。
OS で「視差効果を減らす」(prefers-reduced-motion)を ON にしていると、動きを止めた静止画で、ゆっくり進む。

## 場面

| # | 秒 | 中身 |
|---|---|---|
| 1 | 6.5 | 欲しいのは「アプリ」ではなく「自分の道具」 |
| 2 | 6.5 | マイクはひとつ。「家計簿がほしい」 |
| 3 | 14 | 初期画面が家計簿に変わる(6 列の方眼に部品を置く)。「合計をもっと大きく」で合計だけが大きくなる |
| 4 | 9 | ほかのアプリ: トレーニング記録・散歩メモ・割り勘 |
| 5 | 7 | スマホの機能: カメラ・通知・地図・カレンダー・共有 |
| 6 | 7.5 | 家族や友だちと同じアプリを使う |
| 7 | 8 | 締め: アイコン・Tailor It・「話しかけるだけで、あなたのアプリに。」 |

場面の切り替えでは、赤い糸が画面を縫って横切る。

## URL の末尾(ハッシュ)で変わる表示

| URL | 表示 |
|---|---|
| `index.html` | PV をくり返し再生 |
| `index.html#splash` | 起動のロゴだけ(約 2.5 秒: 縫い目 → アイコン → 文字)。最後の形で止まる |
| `index.html#end` | 締めのカードだけ(操作のバーなし) |
| `index.html#s3` | 3 番目の場面から再生(`#s1`〜`#s7`) |
| `index.html?rec` | 操作のバーを一切出さない(録画用) |

`#splash` の CSS と HTML は `index.html` の中の `SPLASH` の印の間にまとめてあり、他の部分に頼らない。アプリの起動画面に移す時はここだけを見ればよい。

## mp4 に録画する

Playwright(chromium)で webm を録って、ffmpeg で mp4 にする。

```python
# rec.py   python3 rec.py
import asyncio
from pathlib import Path
from playwright.async_api import async_playwright

URL = Path('web/pv/index.html').resolve().as_uri() + '?rec'

async def main():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        ctx = await b.new_context(viewport={'width': 1920, 'height': 1080},
                                  record_video_dir='/tmp/pv-rec',
                                  record_video_size={'width': 1920, 'height': 1080})
        page = await ctx.new_page()
        await page.goto(URL)
        await page.wait_for_timeout(59_000)   # 1 周 = 58.5 秒
        await ctx.close()                     # ここで webm が書き出される
        await b.close()

asyncio.run(main())
```

```sh
ffmpeg -i /tmp/pv-rec/*.webm -c:v libx264 -pix_fmt yuv420p -crf 18 -r 30 tailor-it-pv.mp4
```

頭の白い一瞬が気になる時は `-ss 0.5` を足す。

## ファイル

- `index.html` … 全部(CSS・JS は中に書いてある)
- `assets/` … アイコン(512px)と鶴の絵(`ios/Shared/Brand.xcassets` から縮めて色数を減らしたもの)
