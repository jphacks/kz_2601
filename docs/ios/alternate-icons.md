# 選べるアプリアイコン(#232)

設定 → 「アイコン」で、ホーム画面のアイコンを選べる。iOS の代替アイコン(`UIApplication.setAlternateIconName`)を使う。

- 全員が使える(Pro は不要)。審査用の版(Review)にも出す。
- 選んだアイコンは iOS が覚える。アプリ側では保存しない。次の起動でもそのまま。
- 変えると iOS が「アイコンを変更しました」の確認を出す。これは避けられない。

## 今の候補

今の 3 つの代替アイコンは、いつものアイコンの色違い。このまま出す(仮の正式版)。

- ダーク: 背景を黒に近い色にした版。
- ライト: 背景を白に近い色にし、縫い目の線を紺にした版。
- モノクロ: 全体を白黒にした版。

新しい絵ができたら、下の「絵の差し替え方」と同じ手順で差し替える。

| 画面の名前(ja / en) | appiconset | 差し替えるファイル |
|---|---|---|
| いつもの / Default | `AppIcon.appiconset` | `AppIcon-1024.png`(今のアイコン・変えない) |
| ダーク / Dark | `AppIcon-Dark.appiconset` | `AppIcon-Dark-1024.png` |
| ライト / Light | `AppIcon-Light.appiconset` | `AppIcon-Light-1024.png` |
| モノクロ / Mono | `AppIcon-Mono.appiconset` | `AppIcon-Mono-1024.png` |

場所はどれも `ios/Metamon/Resources/Assets.xcassets/`。それぞれに設定画面用のプレビュー `AppIconPreview<-名前>.imageset`(180×180)がある。手で描かず、下の手順 3 のスクリプトで作る。

## 絵の差し替え方

1. 新しい絵を用意する。
   - PNG・**1024×1024 px**・**透明(アルファ)なし**・角丸なし(iOS が角を丸める)。
   - 色空間は sRGB か Display P3。
2. 上の表のファイルを、**同じファイル名で上書き**する。`Contents.json` は触らない。
3. 設定画面のプレビュー(`AppIconPreview*.imageset`・180×180)を作り直す。appiconset は `UIImage(named:)` で読めないので、プレビューは別に持っている。
   ```sh
   ios/scripts/app-icon-previews.sh
   ```
4. 確かめる。
   ```sh
   sips -g pixelWidth -g pixelHeight -g hasAlpha ios/Metamon/Resources/Assets.xcassets/AppIcon-*.appiconset/*.png
   ```
   `1024` / `1024` / `hasAlpha: no` になっていればよい。アルファがあると App Store への提出で弾かれる。
   アルファを外すには、たとえば `sips -s format jpeg x.png --out /tmp/x.jpg && sips -s format png /tmp/x.jpg --out x.png`。
5. 画面の名前を変えるなら `AppIconOption.title`(`ios/Metamon/Features/Settings/AppIconPicker.swift`)と `Localizable.xcstrings`(ja / en)を直す。

## 候補を増やす・減らす

3 か所を揃える。1 つでもずれると、選んでもアイコンが変わらない(`AppIconPickerTests` が検出する)。

1. `Assets.xcassets` に `AppIcon-<名前>.appiconset` を足す(中身は上と同じ形の `Contents.json` と `AppIcon-<名前>-1024.png`)。`ios/scripts/app-icon-previews.sh` でプレビュー `AppIconPreview-<名前>.imageset` も作る。減らす時はプレビューも消す。
2. `ios/project.yml` の Metamon ターゲットの `ASSETCATALOG_COMPILER_ALTERNATE_APPICON_NAMES` に `AppIcon-<名前>` を足す(空白区切り)。
3. `AppIconOption.all` に足し、`title` と `Localizable.xcstrings` に名前を足す。

そのあと `cd ios && xcodegen generate`。actool が Info.plist に `CFBundleAlternateIcons` を書く(Info.plist は手で書かない)。

## 仕組み

- `ASSETCATALOG_COMPILER_ALTERNATE_APPICON_NAMES` に並べた appiconset を、actool が代替アイコンとしてバンドルに入れる。
- 画面のプレビューは `AppIconPreview*.imageset` を `UIImage(named:)` で読む(appiconset はこの方法で読めない)。
- `AppIconPickerModel` は `AlternateIconSetting`(`UIApplication` が満たす)を通して読み書きする。テストでは偽物に差し替える。
- 変えられなかった時(端末が対応していない・iOS がエラーを返した)はアラートを出し、選択は元のまま。
