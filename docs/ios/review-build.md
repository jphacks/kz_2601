# App Store 審査用の版(Review 構成・APP_STORE_REVIEW)

#245 で追加。背景: App Store Review Guidelines 2.5.19(2026-10-16 新設)は、隠した機能・眠らせた機能を禁止している。そのため、リモート設定でオフにしておき、審査の後でオンにする方法は使えない。審査で危ない機能は**バイナリから外して**出す。

## 目次

1. [版の種類](#版の種類)
2. [審査用の版で外すもの](#審査用の版で外すもの)
3. [仕組み](#仕組み)
4. [ビルドの手順](#ビルドの手順)
5. [外れたことの確かめ方](#外れたことの確かめ方)
6. [AI 側の制限](#ai-側の制限)
7. [部品・機能を足す / 戻すとき](#部品機能を足す--戻すとき)

## 版の種類

| 版 | scheme / 構成 | 用途 | 機能 |
|---|---|---|---|
| 通常 | `Metamon` / Debug・Release | 開発・デモ・TestFlight | 全部(#245 の前と同じ) |
| 審査用 | `Metamon-Review` / Review | App Store に出す版 | 下の表の機能を外す |

## 審査用の版で外すもの

| 機能 | 外すもの | 理由(ガイドライン) |
|---|---|---|
| 単体アプリとしてのリリース申請 | `ReleaseRequestSheet` / `ReleaseUI` / `HTTPReleaseRequestClient` / カードメニューの項目 | 2.5.2・4.2.6 |
| 「探す」の公開カタログ(公開・インストール) | `DiscoverSheet` / `DiscoverEntry` / `SharingClient` の publish・install | DPLA 3.3.1(B)(c)・1.2 |
| 誰でも開ける共有リンク(アプリ側の入口) | `ShareAppSheet` / `SharingHooks`(URL を受けて取り込む処理)/ `HTTPSharingClient`、Info.plist の `CFBundleURLTypes`(`tailorit://`) | 1.2(通報・ブロックが無い) |
| HealthKit(`healthStat` / `Field.health`) | 部品と登録 / `HealthData` / `AppRuntime+Health`、`NSHealth*` / `NSMotionUsageDescription`、entitlement `com.apple.developer.healthkit*` | 4.7.2・5.1.3 |
| バックグラウンドでの位置の記録(`locationTracker`) | 部品と登録 / `LocationTracking`、`UIBackgroundModes` の `location` | 4.7.2・5.1.5・2.5.4 |
| 近くの端末との共有(`nearbyShare`・MultipeerConnectivity) | 部品と登録 / `Features/Nearby` 全部、`NSLocalNetworkUsageDescription` / `NSBonjourServices` | 4.7.2 |
| DSL から使うカメラの撮影(`photoField` / `videoField`) | `CameraPicker` / `VideoCameraPicker`、撮影の選択肢 | 4.7.2・2.5.14 |
| 外した部品を使う同梱の例 | `health` / `walk_route` / `attendance_nearby`(`DSLExamples.reviewExcluded`) | 上と同じ |

**残すもの**
- `photoField` / `videoField` は、**写真ライブラリから選ぶ**機能だけ残す。PhotosPicker は別プロセスで動き、権限の確認も要らない。
  - `photoField.analyze`(#418・写真の文字・金額・分類・骨格の読み取り)は残す。選んだ写真を端末内の Vision / Foundation Models で読むだけで、カメラも権限も使わない。カメラの映像を見ながらの読み取り(回数カウントなど)を足す時は、撮影と同じく審査用の版から外す。
- 現在地を 1 回だけ取る `locationButton` と、地図の `mapView`(使用中の位置だけ)は残す。
- QR を読む `qrScanner` は残す(カメラの説明文はこれに合わせて差し替える)。
- 裏で定期的に同期する `fetch` は残す。
- 最初の版のスコープは、調査メモ(リポジトリ外の `調査_ライセンスとApp Store審査.md`)の「最初の版で削る機能」の表に合わせている。

## 仕組み

- **コード**
  - 外す機能のコードは `#if !APP_STORE_REVIEW` で囲む。ファイル全体を囲むか、呼び出し箇所だけを囲む。
  - 部品のレジストリ(`ComponentRegistry.standard`)にも登録しない。
  - それでも DSL に外した部品が来た場合は、未知の type としてプレースホルダで描く。
- **ローカル Swift Package への条件の渡し方**
  - MetamonDSL / Renderer / Services は、project の build setting を継承しない。
  - そのため `scripts/build-review.sh` が `SWIFT_ACTIVE_COMPILATION_CONDITIONS=$(inherited) APP_STORE_REVIEW` をコマンドラインで全ターゲットに渡す。
  - 渡し忘れたときは、アプリ本体が参照する `MetamonRenderer.ReviewBuildPackageMarker` が見つからず、**コンパイルエラーで止まる**。たとえば Xcode の GUI だけで Review をビルドした場合。
- **Info.plist**
  - 元のファイルは 1 つ(通常の版と共有)。
  - Review 構成だけ、最後のビルドフェーズ `scripts/strip-review-plist.sh` が、出来上がった Info.plist からキーを消し、説明文を差し替える。署名の前に走る。
  - Debug / Release では何もしない。判定は `METAMON_APP_STORE_REVIEW=YES` で行う。
- **entitlement**
  - Review 構成は `Metamon/Metamon-Review.entitlements` を使う。HealthKit を除いたもの。
  - 通常の版は今まで通り `Metamon/Metamon.entitlements` を使う。
- **AI への依頼**
  - `MetamonDSL.ClientCapabilities` が、審査用の版のときだけ `allowedComponents` と `disabledFeatures` を返す。`HTTPAIClient` はこれを generate / chat の body に載せる。
  - 通常の版は nil なので、何も送らない。

## ビルドの手順

前提: `xcodegen`、Xcode 26 系、自動署名できる Team(有料の Developer Program)。

```sh
cd ios
# 1) シミュレータ向け(署名なし)で通るか
./scripts/build-review.sh build -derivedDataPath ../dd

# 2) 実機向けアーカイブ(App Store Connect に出す前段)
./scripts/build-review.sh archive -allowProvisioningUpdates \
  DEVELOPMENT_TEAM=<TEAM_ID> CODE_SIGN_STYLE=Automatic
#   → ios/build/TailorIt-Review.xcarchive(ARCHIVE_PATH で変更可)
```

`build-review.sh` は `xcodegen generate` を先に実行する。そのあと `xcodebuild -scheme Metamon-Review -configuration Review` を呼ぶ。

アーカイブの後(この文書の範囲外。手作業で行う):
1. Xcode Organizer を開くか、`xcodebuild -exportArchive` で App Store Connect 向けに書き出す。
2. App Store Connect で次を確認する。
   - プライバシーの表示(栄養ラベル)が、外した機能を含まないこと。健康・位置の常時・ローカルネットワークを書かない。
   - 審査メモ(調査メモの下書き)が、このバイナリと一致していること。
3. 審査の期間中は、backend(ai / appconfig)を動かしておく。

## 外れたことの確かめ方

```sh
APP=<DerivedData>/Build/Products/Review-iphoneos/Metamon.app
plutil -p "$APP/Info.plist" | grep -E 'NSHealth|NSMotion|NSLocalNetwork|Bonjour|CFBundleURLTypes|UIBackgroundModes' -A2
#   → UIBackgroundModes が [fetch] だけで、他は出ないこと
codesign -d --entitlements :- "$APP"
#   → com.apple.developer.healthkit が無いこと
otool -L "$APP/Metamon" | grep -E 'HealthKit|MultipeerConnectivity|CoreMotion'
#   → 何も出ないこと
strings "$APP/Metamon" | grep -E 'HKHealthStore|MCSession|CMPedometer|UIImagePickerController|release-requests|public-apps'
#   → 何も出ないこと
ls "$APP" | grep -E '^(health|walk_route|attendance_nearby)\.json$'
#   → 何も出ないこと
```

テスト:
- `swift test --package-path Packages/MetamonDSL --filter ReviewBuildTests -Xswiftc -DAPP_STORE_REVIEW` を実行する。
- これで、審査用の条件での allowlist と、主要な例(家計簿・習慣・日記・TODO・割り勘ほか)が外した部品を使わないことを確かめる。
- 通常の版のテストは、今まで通り `xcodebuild -scheme Metamon test`。

## AI 側の制限

契約は `docs/backend/openapi-ai.yaml` の `AllowedComponents` / `DisabledFeatures`。backend は `backend/internal/ai/restrict.go`。

- **リクエスト**: `POST /v1/generate` と `POST /v1/chat` に、省略可能な 2 つを足す。
  - `allowedComponents`: 使ってよい部品の type の一覧。
  - `disabledFeatures`: `fieldHealth` / `cameraCapture`。
- **省略したとき**(通常の版・旧 iOS): 今まで通り全部使える。プロンプトもスキーマも、#245 の前とバイト単位で同じ。
- **指定したとき**
  - 部品カタログ・チャットの「できること」の索引・生成スキーマから、許可外の部品を外す。
  - fieldHealth が無効なら `health` 属性を、cameraCapture が無効なら source の camera / both も外す。
  - 生成後の検証で、許可外の部品は再生成に回す。最後の試行でも残っていれば、その部品を取り除く。`health` 属性は消し、source は library に直す。
  - edit で、今の DSL に既に許可外の部品がある場合は取り除く(DSL 全体で保存)。何をしたかは `meta.restricted` に入る。

## 部品・機能を足す / 戻すとき

- **新しい部品がネイティブ API を使う場合**(カメラ・位置・健康・近距離通信など)
  1. 部品のファイルと `ComponentRegistry` の登録を `#if !APP_STORE_REVIEW` で囲む。
  2. `ClientCapabilities.reviewExcludedComponents` に足す。
  3. 権限の説明が要るなら、`strip-review-plist.sh` に消す処理を足す。
- **審査用の版に戻す場合**(例: v1.1 で「探す」を戻す)
  - `#if` を外す。
  - **通報・ブロック・フィルタなど、審査で求められるものを先に入れてから**、新しい版として審査に出す。審査の後でフラグを切り替えるだけ、はしない(2.5.19)。
