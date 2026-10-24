# iOS アーキテクチャ(Morph / Server-Driven UI)

> 正典は `docs/00_decisions.md`。本書は iOS 側の実装設計。DSL スキーマは統一版 v1 `docs/dsl/README.md`(旧 iOS 案: `docs/ios/dsl-proposal.md`)。**注**: 本書 §2〜§5 の `{{式}}`/`state`/`Interpolator` の記述は v1 で廃止され、`screen.entity` のドラフト + `field` 束縛(`FormDraft`)・`initialScreen` + push/sheet/fullScreen・`app.tabs[]`(TabView)に置き換わった。画面内配置は `area`(6 列 × 可視高さ÷12 行のグリッド)で `GridScreenLayout` が絶対配置する。
> スケルトン実装: `ios/Morph/`(`xcodegen generate` → `xcodebuild -scheme Morph build` 済み)。

## 0. 追加決定の反映(2026-10-24 昼)
| 項目 | 決定 | iOS 側の帰結 |
|---|---|---|
| ユーザーデータ保存先 | **バックエンド(Firestore / appconfig の汎用エンティティ API)** | iOS に永続層を持たない。`entity.*` アクションは全て HTTP。オフライン時のみ最後のレスポンスをメモリ/ディスクにキャッシュ(P2) |
| 2回目以降の改変 | **JSON Patch(RFC 6902)はサーバー側で適用** | iOS はパッチを扱わない。`POST /v1/morph` の応答 or ポーリングで**最新 DSL 全体**を受け取り差し替えるだけ |
| P0 ネイティブ | **カメラ + ローカル通知** | `camera` コンポーネント / `native: notification.*` を実装済み。位置/センサー/共有は P1(ブリッジに stub) |
| デモ3本 | 家計簿 / トレーニング記録 / 散歩メモ | サンプル DSL は `docs/dsl/examples/*.json`(4 本)をそのままバンドル |
| 認証 | Sign in with Apple → 詰まったら匿名認証 | `TokenProvider` 抽象の裏で切替。UI/ランタイムは影響なし |
| STT | WhisperKit 確定・**導入済み** | `SpeechTranscriber` 抽象。実機 `WhisperKitRecognizer`(model `small`)/ シミュレータ `MockTranscriber`(§6) |
| iOS CI | 5分制限なし | WhisperKit/Firebase 解決込みのビルドでも可 |

## 1. 全体構成

```
┌──────────── SwiftUI (View 層) ────────────┐
│ RootView ── ScreenView ── DSLView(再帰) ── ComponentViews │
│ VoiceInputView(音声)                                    │
└──────────────┬───────────────────────────────────────────┘
               │ @Environment(AppRuntime.self)
┌──────────────▼──── Runtime 層 (@Observable, @MainActor) ─┐
│ AppRuntime: document / state(JSONValue) / paths / sheet   │
│            run(Action) / resolve("{{..}}") / binding(path) │
│ Interpolator (式展開)                                      │
└──────────────┬───────────────────────────────────────────┘
               │ protocol
┌──────────────▼──── Services 層 ─────────────────────────┐
│ AppConfigClient(HTTP / InMemory)  AIClient(HTTP / Mock)  │
│ DocumentSync(Polling / Noop)      TokenProvider(Firebase)│
│ NativeBridge(UIKit)               SpeechTranscriber(WhisperKit / Mock) │
└──────────────────────────────────────────────────────────┘
┌──────────── Model 層 (純 Swift, Decodable, Sendable) ─────┐
│ AppDocument / Screen / Node / Component / Action / JSONValue │
└──────────────────────────────────────────────────────────┘
```

| 層 | ディレクトリ | 責務 | 依存 |
|---|---|---|---|
| Model | `DSL/Model` | DSL の Codable 表現。UI/通信に非依存。ユニットテスト対象 | Foundation |
| Runtime | `DSL/Runtime` | 単一の `AppRuntime`(Observation)。state・ナビ・アクション実行・式展開 | Model, Services(protocol) |
| Render | `DSL/Render` | Node → View。状態を持たない薄い View | Runtime |
| Features | `Features/Voice` | DSL 外のネイティブ画面(音声入力) | Runtime |
| Services | `Services/{Live,Mock,Native}` | 外部 I/O。protocol で差し替え | Model |
| App | `App` | エントリ・DI(`BuildConfig.makeServices()`) | 全部 |

**状態管理**: `@Observable final class AppRuntime` 1個を `.environment()` で配る。DSL の動的 state は `JSONValue`(object)1本で持ち、`binding(path)` で SwiftUI `Binding` に変換する。View 側に `@State` を置くのは一時 UI(録音フェーズ、PhotosPicker 選択)のみ。
**モジュール分割**: ハッカソン中は単一ターゲット。`DSL/Model` は依存ゼロなので、必要なら SwiftPM ローカルパッケージ `MorphDSL` に切り出せる(テストも同梱)。

## 2. DSL レンダリング

| 要素 | 実装 | ポイント |
|---|---|---|
| ルート | `AppDocument: Decodable` | `screens: [id: Screen]`、`navigation`、初期 `state` |
| ノード | `struct Node { id, visible, padding, component }` | 共通メタ + 種別 |
| 種別 | `indirect enum Component` | `type` 文字列で分岐する手書き `init(from:)` |
| 未知 type | `.unknown(type:)` | 描画は DEBUG で黄色ラベル、RELEASE で `EmptyView` |
| 既知 type だが props 不正 | `.invalid(type:reason:)` | 同上。画面全体は落とさない |
| 配列中の壊れた要素 | `@Lossy var children: [Node]` | 要素単位で skip |
| 未知 enum 値 | `Lenient.decode(..., fallback:)` | `style: "neon"` → `.primary` |
| 再帰 | `DSLView(node:ctx:)` → `switch component` → 子に `DSLView` | `ForEach(children)`(`Node: Identifiable`) |

```swift
extension Component: Decodable {
    init(from decoder: Decoder) throws {
        let type = try decoder.container(keyedBy: Keys.self).decode(String.self, forKey: .type)
        do {
            switch type {
            case "text":   self = .text(try TextProps(from: decoder))
            case "vstack", "hstack", "zstack": self = .stack(try StackProps(from: decoder)) // axis は type から
            // ...
            default:       self = .unknown(type: type)
            }
        } catch { self = .invalid(type: type, reason: "\(error)") }
    }
}
```

描画スコープ `RenderContext { params, item, index }` を再帰で持ち回り、`list` の行だけ `item/index` を上書きする。

## 3. ナビゲーション

| DSL | SwiftUI | 状態 |
|---|---|---|
| `navigation.type = "tab"` + `tabs[]` | `TabView(selection:)` 内に **タブ毎の** `NavigationStack(path:)` | `selectedTab`, `paths[tabId]: [Route]` |
| `navigation.type = "stack"` + `root` | `NavigationStack(path:)` 1本 | `paths["main"]` |
| `navigate` アクション | `paths[currentTab].append(Route)` | `.navigationDestination(for: Route.self) { ScreenView(route:) }` |
| `back` | `removeLast()` | |
| `present(style: sheet/fullScreen)` | `.sheet(item: $rt.sheet)` / `.fullScreenCover` | `Route: Identifiable` |
| `dismiss` | `sheet = nil; fullScreen = nil` | |

`Route { screen: String, params: [String: JSONValue] }` は `Hashable`。params は push 時に `{{item.id}}` 等を**解決済みの値**で保持するので、遷移先は `{{params.id}}` で参照できる。
DSL 差し替え時(`apply`)は paths/sheet を全部リセットする(画面 id が消えている可能性があるため)。存在しない screen id は `ContentUnavailableView` で表示。

## 4. アクションとデータバインディング

### 4.1 バインディング(読み)
`"{{root.path.to.value}}"` テンプレート。root は `state` / `item` / `index` / `params` / `user`(省略時 state)。配列には `.count` と添字。単独式なら型付き `JSONValue`、文中埋め込みなら文字列連結。`visible` / `disabled` は `truthy` 判定。

### 4.2 バインディング(書き)
入力系(`textField`/`toggle`/`picker`/`camera`)は `bind: "state.draft.title"` を持ち、`AppRuntime.binding(path)` が `Binding<JSONValue>` を返す(`state.` 接頭辞は省略可)。

### 4.3 アクション
| type | 実行先 | 備考 |
|---|---|---|
| `navigate` / `back` / `present` / `dismiss` | ローカル(ナビ) | |
| `setState` | ローカル(`state`) | `value` 内の文字列も `{{}}` 展開 |
| `entity.list` / `create` / `update` / `delete` | **appconfig(Firestore)** | `list` は `into` で state に格納。ユーザーデータは常にサーバー |
| `native` | `NativeBridge` | `feature` 名で分岐、結果は `into` に格納可 |
| `openURL` | UIApplication | |
| `ai.prompt` | ai サービス | 文字で改変(音声と同じ経路) |
| `sequence` | 直列実行 | 「作成→state クリア→再取得→haptic→dismiss」の定石 |

実行は `AppRuntime.run(_:_:) async`(MainActor)。失敗は `toast` に出すだけで握りつぶさない。**ローカル state vs サーバー**の線引き: 入力中の下書き・UI フラグ・取得済み一覧のキャッシュは `state`、永続レコードは全部 entity API。

## 5. ネイティブ機能ブリッジ

| 機能 | 優先 | 公開形 | 実装 | 権限キー |
|---|---|---|---|---|
| マイク(STT) | P0 | `aiVoice` コンポーネント / FAB | AVAudioRecorder → WhisperKit | `NSMicrophoneUsageDescription` |
| カメラ/写真 | P0 | `camera { bind }` コンポーネント | v0: `PhotosPicker`(シミュレータ可) → 実機: `UIImagePickerController(.camera)` を `UIViewControllerRepresentable` で追加 | `NSCameraUsageDescription`, `NSPhotoLibraryUsageDescription` |
| ローカル通知 | P0 | `native: notification.requestPermission` / `notification.schedule {title, body, inSeconds}` | `UNUserNotificationCenter`(実装済み) | 不要(実行時ダイアログ) |
| 触覚 | P0 | `native: haptic {style}` | `UIImpactFeedbackGenerator`(実装済み) | 不要 |
| URL 起動 | P0 | `openURL` / `native: openURL` | `UIApplication.open`(実装済み) | 不要 |
| 位置 | P1 | `native: location.current → into` / `map` コンポーネント | `CLLocationManager` 1回取得(stub) / MapKit(実装済み) | `NSLocationWhenInUseUsageDescription` |
| モーション/歩数 | P1 | `native: motion.read` | CoreMotion / CMPedometer(stub) | `NSMotionUsageDescription` |
| 共有 | P1 | `native: share {text}` | `UIActivityViewController`(stub) | 不要 |

画像は v0 では **base64 JPEG 文字列として state に置き、entity.create の data に含めて送る**(Firestore ドキュメント 1MB 制限に注意 → 圧縮 0.6、長辺 1024px に縮小を追加予定)。Cloud Storage 経由は P2。
全 usage description は `ios/Morph/Resources/Info.plist` に記入済み。

## 6. 音声入力画面とホットリロード

```
[録音] WhisperKit.AudioProcessor.startRecordingLive  (AVAudioEngine → 16kHz mono Float32 をメモリに蓄積)
  → [文字起こし] WhisperKit.transcribe(audioArray:, DecodingOptions(language: "ja", chunkingStrategy: .vad))  ※オンデバイス・停止後に一括
  → [送信] POST ai /v1/morph { appId, currentVersion, transcript }
       (ai: Claude が JSON Patch 生成 → スキーマ検証 → appconfig に適用 → 最新 DSL を返す)
  → [適用] AppRuntime.apply(doc)  withAnimation(.spring) + RootView.id(version) + transition
```

### WhisperKit 実装要点(`Services/Live/WhisperKitRecognizer.swift`)

| 項目 | 内容 |
|---|---|
| パッケージ | `https://github.com/argmaxinc/argmax-oss-swift` **1.1.1 固定**(`exactVersion`)。WhisperKit は v1.0.0 で Argmax OSS SDK に統合され、旧 URL `argmaxinc/WhisperKit` は同リポジトリへリダイレクト。product は `WhisperKit` のみリンク(`ArgmaxCore` は `@_exported`) |
| モデル | `BuildConfig.whisperModel`(環境変数 `MORPH_WHISPER_MODEL` / Info.plist `MorphWhisperModel`、既定 **`small`**)。短縮名は `openai_whisper-small` に解決し、Hugging Face `argmaxinc/whisperkit-coreml` から取得 |
| 配置 | `Application Support/WhisperKit/models/argmaxinc/whisperkit-coreml/openai_whisper-small/`(約 500MB)。`MelSpectrogram/AudioEncoder/TextDecoder.mlmodelc` + `config.json` が揃っていればオフラインでも Hub に問い合わせずロード |
| DL と状態 | `prepare()` を `AppRuntime.bootstrap()` が起動直後に呼ぶ。`status`: `notDownloaded → downloading(downloadProgress 0〜1) → loading(CoreML ロード + ANE 特化 prewarm) → ready → recognizing`。失敗は `.failed(message)` で UI に「再試行」ボタン |
| 権限 | `AudioProcessor.requestRecordPermission()`(`NSMicrophoneUsageDescription`、記入済み)。`NSSpeechRecognitionUsageDescription` は Apple Speech フォールバック用に残置(WhisperKit 自体は不要) |
| 録音 | `startRecordingLive(callback: nil)` → 停止で `audioSamples` を取り出し `purgeAudioSamples`。0.5 秒未満は無音扱いで送らない(幻聴対策) |
| 制約 | シミュレータでは `#if targetEnvironment(simulator)` で `MockTranscriber` + 文字入力のみ(マイク非表示)。ストリーミング(途中結果)は未対応=停止後に一括。`small` のメモリは推論時 ~1GB 弱、初回 prewarm は 30〜90 秒 |

| フェーズ | UI |
|---|---|
| idle | 大きなマイク、"タップして話す"(モデル未ロードなら "準備中…") |
| listening | 赤ボタン + パルスリング |
| transcribing | waveform アイコン |
| thinking | sparkles + 全画面 `.ultraThinMaterial` オーバーレイ "アプリを生成中…" |
| 適用 | 旧ツリー fade out → 新ツリー scale(0.94→1)+fade in |

生成後も右下の **マイク FAB**(ランタイム側、DSL 外)から同じシートを開いて再改変できる。初期 DSL は `aiVoice` 1画面のみ。

**ホットリロード方式(提案)**
| 案 | 仕組み | 評価 |
|---|---|---|
| A. ポーリング(v0・実装済み) | 2秒毎 `GET /v1/apps/{id}`、`version` が進んだら apply。`If-None-Match`/`ETag` 対応は backend 次第 | 実装5分・確実。Cloud Run の無料枠でも問題ない量 |
| B. SSE | `GET /v1/apps/{id}/events` を `URLSession.bytes(for:)` で読む | Go 側は `http.Flusher` で簡単。Cloud Run はタイムアウト(既定5分)で切れるので再接続必須 |
| C. Firestore onSnapshot | iOS に FirebaseFirestore SDK を追加し `apps/{id}` を listen | **最も「生きてる」感が出る**。backend 不要。SDK が重い(+10MB) |

推奨: **Day1 は A**、Day2 に余裕があれば **C**(Firestore が正典なので一番筋が良い)。B は中間なので採らない。

## 7. 認証

| 項目 | 方針 |
|---|---|
| 第一候補 | Firebase Auth + Sign in with Apple(`ASAuthorizationAppleIDProvider` → `OAuthProvider.credential(providerID: .apple, idToken:, rawNonce:)`) |
| 代替 | `Auth.auth().signInAnonymously()`。UID が端末固有 ID として機能する(再インストールで変わる点だけ注意) |
| 切替条件 | Apple Developer Program(有料)未加入 / Capability 設定で30分詰まったら即匿名へ |
| トークン付与 | `FirebaseTokenProvider: TokenProvider { Auth.auth().currentUser?.getIDToken() }` → `HTTP.send` が `Authorization: Bearer` を付ける(実装済み・現状は `NoAuthTokenProvider`) |
| 起動シーケンス | `bootstrap()` 冒頭で `currentUser == nil` なら サインイン → 以降の全リクエストにトークン |
| Go 側 | `firebase.google.com/go/v4/auth` の `VerifyIDToken`。`uid` を entity レコードの owner に使う |

UI は専用ログイン画面を作らない(匿名なら無言、Apple なら初回に `SignInWithAppleButton` をシートで出す)。

## 8. DSL スキーマ
→ 統一版 v1: `docs/dsl/README.md`(決定表)+ `docs/dsl/dsl.schema.json`。旧 iOS 案は `docs/ios/dsl-proposal.md`。

## 9. デモ3本の DSL 表現可能性
| デモ | 画面 | 使う部品 | 不足 |
|---|---|---|---|
| 家計簿 | 一覧/追加/詳細/設定 | list, card, textField(number), picker, camera, notification | 合計金額の集計(`{{sum}}` 無し)→ 一覧取得時に backend が `summary` を返すか、`entity.list` の `query.aggregate` で対応 |
| トレーニング記録 | 種目一覧/記録/履歴 | list, picker, toggle, progress, notification(リマインド) | 日付入力(`datePicker` 未定義 → v0 は textField、v0.1 で追加候補) |
| 散歩メモ | 地図/メモ一覧/写真 | map, camera, textField(multiline), list | `location.current`(P1 stub)→ 実機で CoreLocation 実装が必要 |

## 10. リスクと対処
| リスク | 影響 | 対処 |
|---|---|---|
| WhisperKit モデル DL(`small` ~500MB)初回に数分 | デモで待たされる | 起動直後に `prepare()` を非同期実行、DL 中は進捗バー表示(実装済み)。**会場 Wi-Fi 前提にせず前日に実機へ DL 済みにする**(一度 DL すればオフラインでロード)。文字入力フォールバック(実装済み)を常に残す |
| WhisperKit はシミュレータで実用不可(CoreML/ANE) | 開発中の検証 | シミュレータは `MockTranscriber` + 文字入力のみ(実装済み)。音声は実機のみ |
| Firebase + WhisperKit の SwiftPM 解決が遅い | CI/初回ビルド | CI 5分制限なしの決定済み。DerivedData キャッシュ(`actions/cache`) |
| Swift 6 コンパイラ + strict concurrency | 警告/エラー増 | **Swift 5 言語モード + `targeted`** に固定(project.yml)。Model は全て `Sendable`、UI 寄りは `@MainActor`。Swift 6 モード移行はハッカソン後 |
| AI が生成する DSL の揺れ(未知 type/typo) | クラッシュ | 未知は無視・不正は skip の設計(§2)。サーバー側スキーマ検証と二重防御 |
| `RootView.id(version)` で全ツリー再生成 | 入力中の内容が消える | 仕様として許容(改変=アプリ再生成)。version が同じなら再生成しない |
| Firestore 1MB/doc | 画像 base64 | 圧縮+縮小。超えるなら Cloud Storage |
| Sign in with Apple の Capability/Team 設定 | 実機動作が止まる | 匿名認証へ即切替(§7)。`DEVELOPMENT_TEAM` は `MORPH_TEAM_ID` 環境変数で注入 |
| Widget(ホーム画面ウィジェット)の可変性 | 期待値ズレ | **スコープ外**。WidgetKit は静的タイムラインで DSL 動的描画と相性が悪い。やるなら「最新レコード1件を表示」の固定 Widget のみ |
| 実機 iPhone 13 のみ・カメラはシミュレータ不可 | 検証の遅れ | カメラ v0 を PhotosPicker にしたので UI 動線はシミュレータで確認可能 |

## 11. ビルド/実行
```sh
cd ios
xcodegen generate                              # .xcodeproj 生成(コミットしない)
xcodebuild -scheme Morph -destination 'generic/platform=iOS Simulator' build CODE_SIGNING_ALLOWED=NO
xcodebuild -scheme Morph -destination 'platform=iOS Simulator,name=iPhone 17 Pro' test CODE_SIGNING_ALLOWED=NO
# 実サーバー接続: Scheme の環境変数に MORPH_API_BASE_URL / MORPH_AI_BASE_URL を設定(未設定なら全モック)
```
