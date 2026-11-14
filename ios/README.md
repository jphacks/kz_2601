# ios (Swift / SwiftUI) — Tailor It

xcodegen で `project.yml` から Xcode プロジェクトを生成する(`.xcodeproj` はコミットしない)。

```sh
cd ios
./scripts/sync-firebase-plist.sh                 # 1) Firebase の plist をリポ外からコピー(git-ignored)
METAMON_TEAM_ID=CHP337D75C xcodegen generate     # 2) プロジェクト生成(Team 注入)
open Metamon.xcodeproj
```

## 構成(マルチモジュール)

```
ios/
├── Metamon/                 # アプリ本体(薄い): MetamonApp / AppRuntime / RootView / Features/{Auth,Voice} / BuildConfig(DI の入口)
├── Firebase/                # Firebase の plist の置き場(git-ignored。sync-firebase-plist.sh がコピー)
├── scripts/sync-firebase-plist.sh   # GoogleService-Info*.plist を ~/Desktop/JPHACKS2026/secrets から Firebase/ へ
└── Packages/
    ├── MetamonDSL/          # DSL モデル・GridArea・Record/Aggregator(Foundation のみ・swift test 可)
    ├── MetamonRenderer/     # SwiftUI View。Components/ は 1 種 = 1 ファイル、ComponentRegistry で登録
    └── MetamonServices/     # Protocols/ + Live/ Mock/ Native/ 実装。WhisperKit と FirebaseAuth の依存はここ
```

依存の向き: `App → Renderer → DSL`、`App → Services → DSL`。Renderer は Services を知らない(詳細: `docs/ios/architecture.md` §1)。

## 秘密情報(GoogleService-Info.plist)
- 置き場はリポ外 `~/Desktop/JPHACKS2026/secrets/GoogleService-Info.plist`(別の場所なら `METAMON_SECRETS_DIR=... ./scripts/sync-firebase-plist.sh`)。
- `sync-firebase-plist.sh` が `ios/Firebase/` にコピーし、`git check-ignore` で無視されていることを確認する(public リポ。**絶対にコミットしない**)。
  - `GoogleService-Info.plist` = prod(必須)。`GoogleService-Info-Staging.plist` = stg(secrets にあればコピー)。
  - バンドルへはビルドフェーズ「Select Firebase plist」が構成に合う方を `GoogleService-Info.plist` として入れる(下の「ビルド構成と接続先」)。
- plist が無くてもビルド/起動はできる(起動ログに警告、認証はモックの固定 uid・Authorization ヘッダ無し)。

## 認証(Firebase Auth)
- 起動時にサインイン済みでなければ `SignInView`: **Sign in with Apple**(`SignInWithAppleButton` → nonce/SHA-256 → `OAuthProvider.appleCredential` → Firebase)、下に「ログインせずに使う(匿名)」(`signInAnonymously`)。
- 以降の全 HTTP は `AuthTokenProvider` が Firebase ID トークンを `Authorization: Bearer` に付ける。
- 音声画面の最下部に `uid 先頭6桁` の小さなラベル(タップ → ログアウト)。デバッグ用。
- Sign in with Apple は Apple Developer Program の App ID capability が必要。`xcodebuild -allowProvisioningUpdates` の自動署名で付与される(`Metamon/Metamon.entitlements` は xcodegen が `project.yml` から生成)。

## バックエンド接続(ai / appconfig は別々に設定)
| 設定 | 環境変数(実行時) | Info.plist キー / build setting | 未設定時 |
|---|---|---|---|
| ai(`POST /v1/generate`) | `METAMON_AI_BASE_URL` | `MetamonAIBaseURL` ← `$(METAMON_AI_BASE_URL)` | `MockAIClient`(同梱サンプル DSL) |
| appconfig(DSL/records) | `METAMON_APPCONFIG_BASE_URL` | `MetamonAppConfigBaseURL` ← `$(METAMON_APPCONFIG_BASE_URL)` | `InMemoryAppConfigClient`(ホットリロード無し) |

```sh
# ai をローカル起動(別 worktree で feature/ai-generate を checkout した backend/ で)
set -a; source ~/Desktop/JPHACKS2026/.env; set +a; PORT=8081 go run ./cmd/ai

# シミュレータ: localhost でよい(起動して使うビルドに CODE_SIGNING_ALLOWED=NO を付けない。
# 付けると application-identifier entitlement が無くなり Firebase Auth のキーチェーンが -34018 で失敗 → サインイン画面に英語のエラーが出る。test だけなら付けてよい)
xcodebuild -scheme Metamon -destination 'platform=iOS Simulator,name=iPhone 17' build METAMON_AI_BASE_URL=http://127.0.0.1:8081

# 実機: Mac の LAN IP を渡す(ATS は NSAllowsLocalNetworking のみ許可。IP 直指定の http はこれで通る)
IP=$(ipconfig getifaddr en0)
xcodebuild -scheme Metamon -destination 'id=150ACFA5-8AF6-569A-8EBD-47283A3D1E0B' -allowProvisioningUpdates DEVELOPMENT_TEAM=CHP337D75C build METAMON_AI_BASE_URL=http://$IP:8081
APP=$(xcodebuild -scheme Metamon -destination 'id=150ACFA5-8AF6-569A-8EBD-47283A3D1E0B' -showBuildSettings 2>/dev/null | awk '/ TARGET_BUILD_DIR =/{d=$3} / WRAPPER_NAME =/{w=$3} END{print d"/"w}')
xcrun devicectl device install app --device 150ACFA5-8AF6-569A-8EBD-47283A3D1E0B "$APP"
xcrun devicectl device process launch --device 150ACFA5-8AF6-569A-8EBD-47283A3D1E0B jp.kz2601.metamon
```
- appId は端末ごとに固定(初回起動で UUID を生成し `UserDefaults` に保存)。ai のインメモリ store は再起動で消えるが、未知の appId は初期状態として扱われるので再生成すれば続きから使える(version ズレは 409 → 現行基準で 1 回リトライ)。

## ビルド構成と接続先(#292)

| 構成 | 使う scheme | 用途 | ai / appconfig | Firebase plist | 表示名 |
|---|---|---|---|---|---|
| Debug | `Metamon` | 開発・CI・UI テスト | stg | stg(無ければ prod に戻して警告) | Tailor It β |
| Staging | `Metamon-Staging` | 端末に入れる検証版 | stg | stg(無ければ prod に戻して警告) | Tailor It β |
| Release | `Metamon`(Archive) | TestFlight / App Store | prod | prod | Tailor It |
| Review | `Metamon-Review` | App Store 審査用(#245) | prod | prod | Tailor It |

- 切り替えは scheme か `-configuration` で選ぶだけ(例: `xcodebuild -scheme Metamon-Staging -configuration Staging ...`)。
- stg の URL は今の `jphacks-kz2601` の stg Cloud Run。stg 用の GCP / Firebase プロジェクトができたら `project.yml` の URL と secrets の `GoogleService-Info-Staging.plist` を差し替える(#288)。
- Bundle ID は既定で全構成 `jp.kz2601.metamon`(同じ端末では上書きになる)。stg 版と prod 版を並べて入れる手順は [docs/ios/architecture.md「ビルド構成と接続先」](../docs/ios/architecture.md#ビルド構成と接続先292)。

## App Store 審査用の版(#245)

審査で危ない機能(HealthKit・背面の位置の記録・近くの端末との共有・カメラの撮影・探す・共有リンク・リリース申請)をバイナリから外した版は `./scripts/build-review.sh build|archive` で作る(scheme `Metamon-Review` / 構成 Review)。詳細は [docs/ios/review-build.md](../docs/ios/review-build.md)。

## サードパーティのライセンス(#306)

設定の「ライセンス」に出す一覧は `Metamon/Resources/ThirdPartyLicenses.json`。手で書かず、依存(`Packages/MetamonServices/Package.resolved`)や `LICENSE` を変えたら作り直す:

```sh
python3 scripts/third-party-licenses.py --source-packages <DerivedData>/SourcePackages   # 生成(Xcode で一度ビルドしてチェックアウトを作ってから)
python3 scripts/third-party-licenses.py --check                                           # 検査(scripts-ci の licenses-check と同じ)
```

ライセンスの種類が判定できない・本文が無い依存があると止まる(推測で埋めない)。

## テスト

```sh
xcodebuild -scheme Metamon -destination 'platform=iOS Simulator,name=iPhone 17' test CODE_SIGNING_ALLOWED=NO  # 3 パッケージ一括
(cd Packages/MetamonDSL && swift test)   # DSL のみ、macOS で数秒(英語の文言のテストだけ落ちる。下の xcodebuild 版なら通る)
scripts/mac-package-test.sh MetamonDSL   # パッケージ 1 つを macOS の xcodebuild test で(mac-ci と同じ。Renderer / Services も可)
```

`swift test` は文字列カタログ(.xcstrings)を訳さないので `EnglishCopyTests` が落ちる。macOS のテストを `xcodebuild` で回す時、
repo か DerivedData が `~/Desktop`・`~/Documents`・`~/Downloads` の下にあると、テストの実行役が読めずに落ちる(macOS の保護)。
その時は repo と `MAC_DERIVED_DATA` を保護の外に置く。

## Mac のアプリ(MetamonMac・#929)

3 つのパッケージを iOS と共有し、殻は `MetamonMac/`、iOS と共有するランタイムの部品は `Shared/`。ターゲットは `project.yml` の `MetamonMac`(アプリ)・`MetamonMacWidget`(ウィジェット)・`MetamonMacTests`・`MetamonMacUITests`。macOS 14 以上・Apple シリコンと Intel の両方。

### ビルド・起動

```sh
cd ios
./scripts/sync-firebase-plist.sh                 # 任意。無ければモックで動く(下の「Firebase の plist が無い時」)
METAMON_TEAM_ID=<TEAM_ID> xcodegen generate      # iOS と同じプロジェクトに Mac のターゲットも入る

# 署名なしで通るか(mac-ci の「MetamonMac をビルド」と同じ)
xcodebuild build -project Metamon.xcodeproj -scheme MetamonMac -configuration Debug \
  -destination 'platform=macOS' -derivedDataPath DerivedData-mac CODE_SIGNING_ALLOWED=NO

# 起動して使う(自動署名。アプリ名は Tailor It.app)
xcodebuild build -project Metamon.xcodeproj -scheme MetamonMac -configuration Debug \
  -destination 'platform=macOS' -derivedDataPath DerivedData-mac -allowProvisioningUpdates
open "DerivedData-mac/Build/Products/Debug/Tailor It.app"
```

- Xcode からなら scheme `MetamonMac` を選んで Run(⌘R)。
- Firebase のログインは Keychain の entitlement を使うので、つないで使う時は署名した版で起動する(iOS のシミュレータと同じ理由)。署名なしはビルドとテストだけに使う。
- `PRODUCT_NAME` は `Tailor It`、Swift のモジュール名は `Metamon` のまま(`@testable import Metamon`)。Bundle ID は iOS と同じ `jp.kz2601.metamon`。

### 構成(scheme)

iOS の [ビルド構成と接続先](#ビルド構成と接続先292) と同じ 4 つ。違いは scheme の名前だけ。

| 構成 | scheme | ai / appconfig | メモ |
|---|---|---|---|
| Debug | `MetamonMac` | stg | 開発・CI。開発用メニュー(`METAMON_STG`)が入る。App Check はデバッグ用のトークン |
| Staging | `MetamonMac-Staging` | stg | 手元に入れる検証版。開発用メニューが入る |
| Release | `MetamonMac`(Archive) | prod | 配布 |
| Review | `MetamonMac-Review` | prod | 審査用。共有のリンク・プッシュ・近くの人に送るの受ける側の通信を外した `MetamonMac/MetamonMac-Review.entitlements`。作り方は [docs/ios/review-build.md](../docs/ios/review-build.md) |

Staging / Release / Review の App Check は DeviceCheck(Mac は App Attest が使えない・`MacServices.swift` の `MacFirebase`)。

### Firebase の plist が無い時

`MacServices.make` は、Firebase が初期化できて ai の URL がある時だけ本物につなぐ(`isLive = true`)。どちらかが無い・環境変数 `METAMON_MOCK=1` の時はモック。

| | 本物(`isLive`) | モック |
|---|---|---|
| ログイン | 未ログインなら起動時に匿名でログイン(`MacAccount.start`)。Apple でのサインインは設定の「アカウント」から | 固定のユーザー。サインインの入口は出ない(`MacAccount.isLive` が false) |
| AI | ai サービス | `MockAIClient`(同梱の DSL の例を返す) |
| 作ったアプリの記録 | appconfig の URL があればサーバー | 端末だけ(`MacRecordVault`) |

### ウィジェット(MetamonMacWidget)

本体に埋め込まれる(`Contents/PlugIns/`)ので、本体をビルドすれば一緒に入る。確かめ方:

1. 署名した版の本体を起動し、作ったアプリで記録を 1 つ足す(本体が App Group に `WidgetSnapshot` を書く)。
2. デスクトップを右クリック →「ウィジェットを編集」で「Tailor It」を置く(小・中・大)。
3. 押すと `tailorit://widget?app=<appId>` で本体のそのアプリが開く。

同梱の例はウィジェットに書かない(ウインドウを閉じると記録が消えるため)。仕組みは [docs/mac/system-integration.md「ウィジェット」](../docs/mac/system-integration.md#ウィジェット)。

### テスト

```sh
# 単体テスト(mac-ci の「テスト MetamonMacTests」と同じ)
xcodebuild test -project Metamon.xcodeproj -scheme MetamonMac -configuration Debug \
  -destination 'platform=macOS' -derivedDataPath DerivedData-mac CODE_SIGNING_ALLOWED=NO

# キーボード操作の UI テスト(scheme では skipped。手元で 1 つだけ回す)
xcodebuild test -scheme MetamonMac -destination 'platform=macOS' -only-testing:MetamonMacUITests
```

- UI テストは初回に「システム設定 > プライバシーとセキュリティ > アクセシビリティ / オートメーション」で Xcode(かターミナル)の許可が要る(`MetamonMacUITests/KeyboardShellUITests.swift` の頭)。
- 見た目の比較(`MacSnapshotTests`)の回し方は [docs/mac/system-integration.md「テスト」](../docs/mac/system-integration.md#テストui-テスト見た目の比較)。
- repo か DerivedData が `~/Desktop` などの下にあるとテストが読めずに落ちる。上の [テスト](#テスト) の段落を見る。

## コンポーネントを足す(3 行)
1. `Packages/MetamonRenderer/Sources/MetamonRenderer/Components/Leaves/FooLeaf.swift` に `struct FooLeaf: View`
2. `Packages/MetamonDSL/.../Model/Component.swift` の `ComponentType` に `case foo`(`docs/dsl/dsl.schema.json` にも)
3. `ComponentRegistry.standard` に `r.register(.foo) { c, ctx in FooLeaf(c: c, ctx: ctx) }`

`MetamonRendererTests` が schema と登録の一致を検証するので、抜けると赤になる。

## 名称
- 表示名「Tailor It」(正式名・#62)は `Metamon/Resources/Info.plist` と `MetamonWidget/Info.plist` の `CFBundleDisplayName`。UI 文言も「Tailor It」で統一(「変身」は比喩として残す)。
- 内部識別子 `Metamon` / Bundle ID `jp.kz2601.metamon` は Firebase 登録値なので変更しない。
