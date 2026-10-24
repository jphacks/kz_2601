# 01 ゴールと要件（JPHACKS 2026・2日間）

> 正典は `00_decisions.md`。本書はその上に「何を・どの順で・どこまで」作るかを定める。矛盾したら 00 が勝つ。

## 1. 2日間のゴール

**コアデモ**: 初期画面(音声のみ)で審査員の前で話す → 10〜15秒後にアプリが"そのアプリ"に変わる → その場で触って動く → もう一度話すとアプリが改変される。

| 区分 | ID | ゴール | 成立条件 |
|---|---|---|---|
| **Must** | G1 | 音声→テキスト | 実機でボタン押下→発話→文字起こしが画面に出る(WhisperKit) |
| **Must** | G2 | テキスト→DSL JSON | ai サービスが Claude で DSL JSON を生成しスキーマ検証を通過、appconfig に保存 |
| **Must** | G3 | DSL JSON→画面 | iOS レンダラーが JSON からコンポーネント10種以上を描画し、画面遷移・入力・一覧追加が動く |
| **Must** | G4 | ネイティブ機能1つ | 生成アプリからカメラ or ローカル通知が実際に動く(どちらかは必達、両方が目標) |
| **Must** | G5 | デモ3本が実機で通る | §2 の3シナリオが iPhone 13 実機で連続成功。失敗時はフォールバック(§4)で止まらない |
| **Must** | G6 | 提出物 | デモ動画・README(JPHACKSテンプレ)・public リポに秘密なし |
| Should | G7 | 追加発話で改変 | 既存アプリに「〜も追加して」で差分反映(フル再生成でも可) |
| Should | G8 | 生成中UX | 生成中はスケルトン/進捗表示、文字起こし確認後に送信 |
| Should | G9 | Sign in with Apple | Firebase Auth でユーザー別に DSL を保持 |
| Should | G10 | 位置情報 + 地図 | Map コンポーネントと現在地取得 |
| Could | G11 | 履歴/バージョン戻し | appconfig の version を使って1つ前のアプリに戻す |
| Could | G12 | 他アプリ連携 | Share Sheet / Shortcuts / HealthKit 歩数 |
| Could | G13 | 複数アプリの保持と切替 | 「家計簿に戻して」で切替 |

**最小構成(G1〜G6)の定義**: 音声画面1 + レンダラー + コンポーネント10種 + ai/appconfig の2エンドポイント + カメラ or 通知 + テンプレフォールバック。これ以外は Day1 夜時点で切れる。

## 2. デモシナリオ3本

共通の流れ: ①初期画面(音声)→②発話→③文字起こし表示→④生成中→⑤アプリ化→⑥触る→⑦(Should)追加発話で改変。

| # | アプリ | 発話(例) | 生成される画面 / コンポーネント | ネイティブ機能 | 改変発話(Should) |
|---|---|---|---|---|---|
| A | 家計簿 | 「コンビニで使ったお金を記録する家計簿が欲しい。レシートの写真も残したい」 | ホーム: SummaryCard(今月合計)+List(明細) / 追加画面: NumberField(金額)+TextField(メモ)+Picker(カテゴリ)+DatePicker+CameraCapture+Button(保存) | **カメラ**(写真ライブラリ) | 「カテゴリ別の棒グラフを足して」→ Chart 追加 |
| B | トレーニング記録 | 「腕立てとスクワットの回数を毎日記録したい。夜9時に忘れないように通知して」 | ホーム: Stepper×2(種目ごと回数)+Button(今日の記録を保存)+List(履歴)+Chart(週間) / 設定: Toggle(通知)+DatePicker(時刻) | **ローカル通知** | 「懸垂も追加して」→ Stepper 追加 |
| C | 散歩メモ | 「散歩中に見つけた良い場所を、写真と場所つきでメモしたい」 | ホーム: Map(ピン一覧)+List / 追加画面: CameraCapture+TextField+Button(現在地で保存) / 詳細: Image+Text+Map | **位置情報** + カメラ + MapKit | 「お気に入りマークをつけたい」→ Toggle 追加 |

- A はコンポーネント網羅性、B は通知(シミュレータでも動く=保険)、C は"いかにもネイティブ"感を見せる。
- 審査本番の順序は **B→A→C**(リスク低→高)。C が不安なら A+B の2本で成立させ、C は動画のみ。

## 3. 機能要件一覧

優先度: P0=Must(無いとデモ不成立) / P1=Should / P2=Could。領域: iOS / appconfig / ai / infra / docs。

| ID | 要件 | 優先 | 領域 | 依存 |
|---|---|---|---|---|
| F01 | DSL スキーマ v0(App/Screen/Component/Action/State)を JSON Schema + Go型 + Swift型で定義 | P0 | ai+appconfig+iOS | — |
| F02 | 初期状態 DSL(AI会話画面のみ)を seed として保持 | P0 | appconfig | F01 |
| F03 | 音声入力画面: 録音・波形/状態表示・停止で文字起こし(WhisperKit) | P0 | iOS | — |
| F04 | 文字起こし結果の表示・編集・送信(Whisper失敗時はテキスト入力に切替) | P0 | iOS | F03 |
| F05 | `POST /v1/generate` 発話テキスト→Claude 構造化出力→DSL JSON | P0 | ai | F01 |
| F06 | 生成 JSON のスキーマ検証 + 未知コンポーネントの除去/置換 | P0 | ai | F01,F05 |
| F07 | `GET/PUT /v1/apps/{id}`: DSL の取得・保存(Firestore・version 採番) | P0 | appconfig | F01 |
| F08 | ai が検証済み DSL を appconfig に保存し、レスポンスに同梱して返す | P0 | ai | F05,F06,F07 |
| F09 | iOS レンダラー: DSL→SwiftUI 再帰描画、Screen 遷移(NavigationStack) | P0 | iOS | F01 |
| F10 | コンポーネント目録(§3.1)の P0 分を実装 | P0 | iOS | F09 |
| F11 | Action 実行: navigate / saveItem / deleteItem / openCamera / scheduleNotification | P0 | iOS | F09 |
| F12 | 生成アプリのユーザーデータ(明細・記録)をローカル永続化(SwiftData or UserDefaults) | P0 | iOS | F11 |
| F13 | 生成完了後に iOS がアプリ全体を差し替えて再描画(アニメーション付き) | P0 | iOS | F08,F09 |
| F14 | カメラ撮影→Image 表示→ローカル保存 | P0 | iOS | F10,F11 |
| F15 | ローカル通知のスケジュール(時刻指定・権限要求) | P0 | iOS | F11 |
| F16 | LLM 失敗/検証失敗時のフォールバック(リトライ1回→キーワードでテンプレ DSL) | P0 | ai | F06 |
| F17 | GCP 構築手順: Cloud Run×2 / Firestore / Secret Manager / 環境変数 | P0 | infra | — |
| F18 | CI: Go lint+test、iOS build+unit test(5分以内) | P0 | infra | — |
| F19 | デモ用 seed: 3シナリオの"正解" DSL をテンプレとして同梱(フォールバック兼回帰テスト) | P0 | ai+docs | F01 |
| F20 | 追加発話で既存 DSL を改変(現 DSL を context に渡しフル再生成 or JSON Patch) | P1 | ai+iOS | F08,F13 |
| F21 | 生成中スケルトン/進捗(STT中/生成中/反映中の3段階) | P1 | iOS | F13 |
| F22 | Firebase Auth(Sign in with Apple)+Go 側 ID トークン検証、ユーザー別 app | P1 | iOS+appconfig+ai | F07 |
| F23 | Map コンポーネント + 現在地取得(CoreLocation) | P1 | iOS | F10 |
| F24 | Chart(棒/折れ線)コンポーネント(Swift Charts) | P1 | iOS | F10 |
| F25 | 生成の構造化ログ(prompt/所要時間/検証結果)と Firestore 保存 | P1 | ai | F05 |
| F26 | 1つ前の version に戻す UI + API | P2 | iOS+appconfig | F07 |
| F27 | 複数アプリ保持と音声切替 | P2 | iOS+ai | F20 |
| F28 | Share Sheet / Shortcuts / HealthKit 連携アクション | P2 | iOS | F11 |

### 3.1 コンポーネント目録(候補 15種・うち P0 13種)

| コンポーネント | 役割 | 優先 | 使用シナリオ |
|---|---|---|---|
| Screen | 画面コンテナ(title, children, ナビ先) | P0 | 全部 |
| VStack / Section | 縦並び・グルーピング | P0 | 全部 |
| Text | 見出し・本文(style: title/body/caption) | P0 | 全部 |
| TextField | 文字入力(bind: state key) | P0 | A,C |
| NumberField | 数値入力(通貨/整数) | P0 | A |
| Button | アクション実行(action 参照) | P0 | 全部 |
| List | state 配列の一覧(行テンプレ、スワイプ削除) | P0 | 全部 |
| SummaryCard | 集計カード(sum/count/avg) | P0 | A,B |
| Stepper | カウンター(+/-) | P0 | B |
| Toggle | ON/OFF | P0 | B,C |
| DatePicker | 日付・時刻 | P0 | A,B |
| Picker | 選択肢(カテゴリ) | P0 | A |
| CameraCapture / Image | 撮影と表示 | P0 | A,C |
| Chart | 棒/折れ線 | P1 | A,B |
| Map | ピン表示・現在地 | P1 | C |

- P0 は 13種(Chart/Map を除く)。Claude に渡すのは「この目録と JSON 例」だけ。目録外は F06 で落とす。
- Action は `navigate` / `saveItem` / `deleteItem` / `openCamera` / `scheduleNotification` / `requestLocation`(P1) の6種に固定。

## 4. 非機能要件

| 項目 | 目標 | 備考 |
|---|---|---|
| 生成レイテンシ | 発話終了→画面切替 **15秒以内**(STT≤3s / Claude≤10s / 反映≤2s)。20秒で警告表示、30秒でフォールバック | 構造化出力 + 目録を短く + sonnet 既定。計測ログは F25 |
| STT 精度 | 日本語デモ文3本が実機で安定。失敗時はテキスト入力へ即切替(F04) | WhisperKit モデルは base/small を実機で比較、会場ノイズを想定し外付けマイク不要で成立させる |
| オフライン | 最後に受け取った DSL と ローカルデータでアプリは動き続ける。生成だけ不可(バナー表示) | DSL は iOS 側にキャッシュ。会場 Wi-Fi 不安定でもデモ継続 |
| フォールバック | Claude 失敗/スキーマ不正→リトライ1回→キーワード一致でテンプレ DSL(F19)。テンプレも無ければ「聞き取れませんでした」で音声画面に戻る | デモ3本は必ずテンプレで救える。ルールベースを恥じず"安定稼働"として説明 |
| セキュリティ | public リポ: Claude/OpenAI キーは **Secret Manager → Cloud Run 環境変数のみ**。iOS はキーを持たない。GoogleService-Info.plist は gitignore + secret scanning | Firestore はクライアント直アクセス禁止(ルールで deny)、Go 経由のみ。Sign in 未実装時は固定デバイスIDで代替 |
| 可用性(デモ) | Cloud Run min-instances=1 で冷起動回避(デモ当日のみ)。ローカル Go を Mac で動かす退路を用意 | — |
| コスト | 無料枠 + Claude 数千円以内。生成1回≒入力3k/出力2k トークン想定 | — |
| 観測性 | 生成毎に所要時間・検証結果・使用モデルをログ(Cloud Logging) | デモ後の「独自技術」説明に数値を使う |

## 5. 審査観点(README テンプレ)との対応

| README 項目 | 書くこと | 効く要件 |
|---|---|---|
| 背景 | 「アプリを探す/作る」のコストをゼロにし、"話せばアプリが生える"体験 | G1-G3 |
| 製品説明 | 音声→Claude→DSL JSON→SwiftUI レンダリングの Server-Driven UI。初期状態は会話画面のみ | F01-F13 |
| 特長1 | 話した10数秒後に、ネイティブ機能(カメラ/通知/位置)つきの動くアプリになる | G4, F14, F15, F23 |
| 特長2 | 追加で話すとアプリが育つ(差分改変・version 管理) | G7, F20, F26 |
| 特長3 | LLM が落ちても止まらない(スキーマ検証 + ルールベースフォールバック)。オフラインでも使い続けられる | F06, F16, F19 |
| 解決できること | 非エンジニアの"自分専用の小さなアプリ"を即席で | §2 シナリオ |
| 今後の展望 | コンポーネント拡張、他アプリ連携、アプリ共有・複数保持 | G11-G13 |
| こだわり | 15秒レイテンシ目標、生成中UX、目録を絞って品質を出す設計 | §4, F21 |
| 独自技術 | DSL スキーマ設計、構造化出力+検証パイプライン、SwiftUI 再帰レンダラー | F01, F05-F06, F09 |
| 活用技術 | Claude API / WhisperKit / Firestore / Cloud Run / Firebase Auth / Swift Charts / MapKit / UserNotifications | 00 スタック表 |

## 6. ユーザーへの質問リスト(判断待ち)

| # | 論点 | 選択肢 | 補佐の推奨 |
|---|---|---|---|
| Q1 | 生成アプリの**ユーザーデータ**(明細・記録)の置き場 | a) iOS ローカル(SwiftData) / b) Firestore を appconfig 経由 / c) DSL JSON 内に埋め込む | **a**。2日間で同期を作らない。オフライン要件にも合う |
| Q2 | **改変方式**(Should G7) | a) 現 DSL を渡してフル再生成 / b) JSON Patch 差分 | **a**。Day1 はフル再生成のみ、余裕が出たら b |
| Q3 | **P0 のネイティブ機能**をどれに絞るか | a) カメラ(実機必須・映える) / b) ローカル通知(シミュレータ可・堅い) / c) 両方 P0 | **c**。ただし Day1 夜に遅れていればカメラを P1 に落とす |
| Q4 | **デモ3本**の確定 | a) 家計簿/トレーニング/散歩メモ(本書案) / b) 別候補に差替(読書記録・カメラメモ・水分記録など) | **a**。差替は Day1 昼の DSL 確定前に |
| Q5 | **Sign in with Apple** の優先度 | a) P1(本書案・固定デバイスIDで代替) / b) P0 に上げる | **a**。Apple Developer の Capability 設定で詰まるリスクを本筋から外す |
