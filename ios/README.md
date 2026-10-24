# ios (Swift / SwiftUI) — メタモン

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
├── scripts/sync-firebase-plist.sh   # GoogleService-Info.plist を ~/Desktop/JPHACKS2026/secrets から Metamon/Resources/ へ
└── Packages/
    ├── MetamonDSL/          # DSL モデル・GridArea・Record/Aggregator(Foundation のみ・swift test 可)
    ├── MetamonRenderer/     # SwiftUI View。Components/ は 1 種 = 1 ファイル、ComponentRegistry で登録
    └── MetamonServices/     # Protocols/ + Live/ Mock/ Native/ 実装。WhisperKit と FirebaseAuth の依存はここ
```

依存の向き: `App → Renderer → DSL`、`App → Services → DSL`。Renderer は Services を知らない(詳細: `docs/ios/architecture.md` §1)。

## 秘密情報(GoogleService-Info.plist)
- 置き場はリポ外 `~/Desktop/JPHACKS2026/secrets/GoogleService-Info.plist`(別の場所なら `METAMON_SECRETS_DIR=... ./scripts/sync-firebase-plist.sh`)。
- `sync-firebase-plist.sh` が `Metamon/Resources/` にコピーし、`git check-ignore` で無視されていることを確認する(public リポ。**絶対にコミットしない**)。
- plist が無くてもビルド/起動はできる(起動ログに警告、認証はモックの固定 uid・Authorization ヘッダ無し)。
- xcodegen はファイルの有無で Resources を決めるので、**コピーしてから `xcodegen generate`**。

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

# シミュレータ: localhost でよい
xcodebuild -scheme Metamon -destination 'platform=iOS Simulator,name=iPhone 17' build CODE_SIGNING_ALLOWED=NO METAMON_AI_BASE_URL=http://127.0.0.1:8081

# 実機: Mac の LAN IP を渡す(ATS は NSAllowsLocalNetworking のみ許可。IP 直指定の http はこれで通る)
IP=$(ipconfig getifaddr en0)
xcodebuild -scheme Metamon -destination 'id=150ACFA5-8AF6-569A-8EBD-47283A3D1E0B' -allowProvisioningUpdates DEVELOPMENT_TEAM=CHP337D75C build METAMON_AI_BASE_URL=http://$IP:8081
APP=$(xcodebuild -scheme Metamon -destination 'id=150ACFA5-8AF6-569A-8EBD-47283A3D1E0B' -showBuildSettings 2>/dev/null | awk '/ TARGET_BUILD_DIR =/{d=$3} / WRAPPER_NAME =/{w=$3} END{print d"/"w}')
xcrun devicectl device install app --device 150ACFA5-8AF6-569A-8EBD-47283A3D1E0B "$APP"
xcrun devicectl device process launch --device 150ACFA5-8AF6-569A-8EBD-47283A3D1E0B jp.kz2601.metamon
```
- appId は端末ごとに固定(初回起動で UUID を生成し `UserDefaults` に保存)。ai のインメモリ store は再起動で消えるが、未知の appId は初期状態として扱われるので再生成すれば続きから使える(version ズレは 409 → 現行基準で 1 回リトライ)。

## テスト

```sh
xcodebuild -scheme Metamon -destination 'platform=iOS Simulator,name=iPhone 17' test CODE_SIGNING_ALLOWED=NO  # 3 パッケージ一括
(cd Packages/MetamonDSL && swift test)   # DSL のみ、macOS で数秒
```

## コンポーネントを足す(3 行)
1. `Packages/MetamonRenderer/Sources/MetamonRenderer/Components/Leaves/FooLeaf.swift` に `struct FooLeaf: View`
2. `Packages/MetamonDSL/.../Model/Component.swift` の `ComponentType` に `case foo`(`docs/dsl/dsl.schema.json` にも)
3. `ComponentRegistry.standard` に `r.register(.foo) { c, ctx in FooLeaf(c: c, ctx: ctx) }`

`MetamonRendererTests` が schema と登録の一致を検証するので、抜けると赤になる。

## 名称
- 表示名「メタモン」は `Metamon/Resources/Info.plist` の `CFBundleDisplayName` の 1 箇所(提出前に正式名を決める。商標要確認)。
- 内部識別子 `Metamon` / Bundle ID `jp.kz2601.metamon` は Firebase 登録値なので変更しない。
