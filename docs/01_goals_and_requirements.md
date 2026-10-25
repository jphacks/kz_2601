# 01 ゴールと要件（JPHACKS 2026・2日間）v2

> 正典は `00_decisions.md`(決定)と `docs/dsl/README.md`(DSL v1・グリッド仕様)。本書はその上に「何を・どの順で・どこまで」作るかを定める。矛盾したら 00 → dsl/README → 本書 の順で勝つ。
> **最終更新: 2026-10-24(Day1 午後)**

### 変更履歴
| 版 | 日時 | 変更 |
|---|---|---|
| v1 | 10/24 午前 | 初版(ゴール G1〜G13・デモ3本・F01〜F28・Q1〜Q5) |
| v2 | 10/24 午後 | Q1〜Q5 の決定を反映(ユーザーデータ=Firestore / 改変=JSON Patch / カメラ+通知の両方 P0 / ログインあり)。**グリッド座標配置を中核要件に追加**(G3・F29〜F33)。タブナビ・versions restore・WhisperKit `small`・ホットリロード・アプリ名メタモン(F34〜F41)。デモ3本の表を `docs/dsl/examples/*.json` と一致させた。旧記述(ローカル保存・フル再生成・Morph・再帰レンダラー)を削除 |
| v3 | 10/24 夜 | 正式名 **Tailor It** に決定(#62/#69)。F40 を更新・Q6 を決着。G18 ウィジェットは実機確認済み(#39) |

## 1. 2日間のゴール

**コアデモ**: 初期画面(音声のみ)で審査員の前で話す → 10〜15秒後にアプリが"そのアプリ"に変わる(**AI がタブ構成と画面レイアウトまで決める**) → その場で触ると記録がバックエンドに保存される → もう一度話すと差分でアプリが育つ。

| 区分 | ID | ゴール | 成立条件 |
|---|---|---|---|
| **Must** | G1 | 音声→テキスト | 実機でボタン押下→発話→文字起こしが画面に出る(WhisperKit `small`・オンデバイス・事前DL) |
| **Must** | G2 | テキスト→DSL JSON | ai が `claude-sonnet-5-5` 構造化出力で DSL v1 を生成、スキーマ+意味検証(id 参照・**area 重なり**)を通過して appconfig に保存 |
| **Must** | G3 | DSL JSON→画面(**グリッド配置込み**) | iOS が DSL v1 を描画: **タブ(2〜5)**・**`area:"A1:C3"` 付きコンポーネントを 6 列×N 行のグリッドに配置**・可変長はセル内スクロール・`area` 省略は上から積む。入力→保存→一覧が動く |
| **Must** | G4 | ユーザーデータはバックエンド | 生成アプリで記録した明細が appconfig `records`(Firestore)に保存され、アプリ再起動・別端末ログインでも残る |
| **Must** | G5 | ログイン | Firebase Auth で uid を得て全 API に ID トークンを付ける。**Sign in with Apple 優先、詰まれば匿名認証**(どちらでも成立) |
| **Must** | G6 | ネイティブ機能 2 つ | 生成アプリから **カメラ(`photoField`)** と **ローカル通知(`app.notifications` / `scheduleNotification`)** の両方が実機で動く |
| **Must** | G7 | デモ3本が実機で通る | §2 の A/B/C が iPhone 13 実機で連続成功。失敗時はフォールバック(§4)で止まらない |
| **Must** | G8 | 提出物 | デモ動画・README(JPHACKS テンプレ)・public リポに秘密なし |
| Should | G9 | 追加発話で改変 | ai が **JSON Patch(RFC 6902)** を生成→appconfig が適用・検証・version+1→iOS はフル DSL を受け直す |
| Should | G10 | **Claude が破綻なく `area` を出す** | デモ3本の発話で重なりエラー 0 回(再生成 1 回以内)。失敗時は `area` を落として G3 の「上から積む」で成立させる |
| Should | G11 | 「元に戻して」 | ai が意図を検出→ `versions/{n}/restore` で 1 つ前に戻る |
| Should | G12 | 生成中 UX | STT 中/生成中/反映中の 3 段階表示、文字起こし確認後に送信 |
| Should | G13 | ホットリロード | Day1 はポーリングで version 変化を検知、Day2 に Firestore リスナーへ |
| Could | G14 | 位置情報 + 地図 | Field `location` と `map` Leaf(DSL は P1 枠を予約済み) |
| Could | G15 | 他アプリ連携 | Share Sheet / Shortcuts / HealthKit |
| Could | G16 | 複数アプリの保持と切替 | 現状は **1 ユーザー 1 アプリ**前提 |
| Could | G18 | ホーム画面ウィジェット(**実装済み**・Issue #39) | 生成アプリの `stat` 先頭 3 個を本体が集計し App Group 経由で WidgetKit に表示(small=1 個/medium=3 個)。制約: 更新は本体起動中のみ・DSL の動的描画はしない(`docs/ios/architecture.md` §12) |
| Could | **G17** | **生成前の対話(数ラリーで要件を固めてから作る)** | **backend 実装済み(10/24・Issue #74)**: `POST /v1/chat` が 1 問+選択肢で聞き返し、揃えば `ready` の `transcript` を既存 create に渡す。最大 3 ラリー・「もう作って」で即 ready・会話状態は ai 内 30 分(appconfig 永続化は後段)。eval 6 本全通過・p50 2.4s(`docs/backend/ai-eval/2026-10-24-chat.md`)。**iOS 側は #75** |

**グリッド配置を必達に含める判断**: 「AI が画面をレイアウトする」ことが審査での差別化点なので、**「`area` 付き DSL を描画できる」(G3・iOS 側)は必達**にする。実装は `GeometryReader` 1 個で有界(Screen 直下 1 階層のみ)であり、デモ3本の seed(F19)は手書きで正しい `area` を持つため、Claude の出力品質に依存せずデモで見せられる。一方 **「Claude が破綻なく `area` を出す」(G10)はストレッチ**に置く。LLM の座標出力は揺れるため、失敗時は backend が重なり/範囲外をエラー列挙→再生成 1 回→それでもダメなら `area` を全部落として「上から積む」決定的フォールバック(F31)で必達を守る。

**最小構成(G1〜G8)**: 音声画面 + タブ付きグリッドレンダラー + Container 3 種/Leaf 14 種(chart 除く) + `/generate` + apps/records API + Firebase Auth + カメラ + 通知 + テンプレフォールバック。これ以外は Day1 夜時点で切れる。Day1 夜に遅れていたらカメラを P1 に落とす判断をユーザーに仰ぐ(§6 Q7)。

## 2. デモシナリオ3本(= `docs/dsl/examples/{kakeibo,training,walk}.json`)

共通の流れ: ①初期画面(`voiceInput` 全画面 `A1:F12`)→②発話→③文字起こし表示→④生成中→⑤アプリ化(タブ・グリッド)→⑥触る(記録が Firestore へ)→⑦(Should)追加発話で JSON Patch 改変。seed JSON と本表は常に一致させる(ずれたら JSON が正)。

| # | アプリ | 発話(例) | タブ | 画面 / コンポーネント(`area`) | ネイティブ | 改変発話(Should・JSON Patch) |
|---|---|---|---|---|---|---|
| A | 家計簿 | 「コンビニで使ったお金を記録する家計簿が欲しい。レシートの写真も残したい」 | ホーム / 集計 | **home**: `hstack`(stat 今月の支出 sum・件数 count) `A1:F2` / `section` 最近の支出(list・セル内スクロール) `A3:F10` / `button` 支出を追加 `D11:F12`。**expense_form**(sheet): `section` 内容(numberField 金額・picker カテゴリ segmented・textField メモ・datePicker 日付) `A1:F7` / `photoField` レシートを撮る `A8:F9` / `button` 削除 `A11:F12`。**stats**: `chart` 円(カテゴリ別) `A1:F6` / `chart` 棒(月別) `A7:F12`(P1。未実装時はプレースホルダ) | **カメラ** | 「追加ボタンを左端まで広げて」→ `replace /screens/0/components/1/area` = `A11:F12`(**AI がレイアウトを変える**を見せる) |
| B | トレーニング記録 | 「腕立てとスクワットの回数を毎日記録したい。毎朝8時に忘れないように通知して」 | 今日 / 履歴 | **home**: `hstack`(stat 回数 sum・回数 count・重量) `A1:F2` / `chart` 週ごと `A3:F7` / `chart` 種目別 `A8:F10` / `button` 今日の記録を保存 `A11:F12`。**workout_form**: `section`(picker 種目・numberField 回数・numberField 重量・toggle 完了・datePicker) `A1:F8` / `button` 削除 `A11:F12`。**history**: `list` 全履歴 `A1:F12` | **ローカル通知**(`app.notifications`: daily 08:00) | 「懸垂も追加して」→ `add /entities/0/fields/0/options/-` |
| C | 散歩メモ | 「散歩中に見つけた良い場所を、写真つきでひとことメモしたい。夜に見返したい」 | なし(単一スタック) | **home**: `stat` 今日のメモ `A1:C2` / `stat` ぜんぶ `D1:F2`(**2 枚並び**) / `list` `A3:F12`、toolbar「撮る」。**memo_form**: `image` プレビュー `A1:F5` / `photoField` 写真を撮る `A6:F7` / `textField` ひとこと `A8:F9` / `button` 今夜18時に思い出す(`scheduleNotification`) `A11:F12` | カメラ + 通知(単発)。Field `place:location` は P1(表示なし) | 「お気に入りマークをつけたい」→ `add` で boolean Field + `toggle` |

- A はタブ+グリッド+カメラ+集計の網羅、B は宣言的通知と chart、C はグリッドの横分割と単発通知。3 本とも P0 部品のみで成立し、chart(P1)が無くても崩れない。
- 審査本番の順序は **A→B→C**(A が最も完成度を見せる。C の位置情報は P1 に落ちたためリスクは低い)。不安なら A+B の 2 本で成立させ、C は動画のみ。

## 3. 機能要件一覧

優先度: P0=Must / P1=Should / P2=Could。領域: iOS / appconfig / ai / infra / docs。**v2 で変更・追加した行は太字**。

| ID | 要件 | 優先 | 領域 | 依存 |
|---|---|---|---|---|
| F01 | **DSL v1**(`docs/dsl/dsl.schema.json` 正典・非再帰 2 段 Screen>Container>Leaf)を Go 型(embed・CI で byte 一致)+ Swift 型で共有 | P0 | ai+appconfig+iOS | — |
| F02 | 初期状態 DSL(`examples/initial.json`: `voiceInput` 1 つ・`voiceEntry:fullscreen`)を seed として保持 | P0 | appconfig | F01 |
| F03 | 音声入力画面: 録音・波形/状態表示・停止で文字起こし(WhisperKit) | P0 | iOS | F36 |
| F04 | 文字起こし結果の表示・編集・送信(Whisper 失敗時はテキスト入力に切替) | P0 | iOS | F03 |
| F05 | `POST /generate` 発話テキスト→Claude(`claude-sonnet-5-5` 既定)構造化出力→DSL v1 | P0 | ai | F01 |
| F06 | 生成 JSON のスキーマ検証 + **意味検証**(id 参照整合・enum・`area` 範囲)。未知 type は除去 | P0 | ai | F01,F05,F32 |
| F07 | `GET/PUT /apps/{appId}/dsl`・`/versions`: DSL の取得・保存(Firestore・version 単調増加・封筒 `DSLEnvelope`) | P0 | appconfig | F01 |
| F08 | ai が検証済み DSL を appconfig に保存し、フル DSL 封筒をレスポンスで返す | P0 | ai | F05,F06,F07 |
| F09 | iOS レンダラー: **非再帰 2 段**の DSL→SwiftUI 描画。タブごとの `NavigationStack`、sheet/fullScreen | P0 | iOS | F01 |
| F10 | コンポーネント目録(§3.1)の P0 分(Container 3 + Leaf 14)を実装 | P0 | iOS | F09 |
| F11 | Action 9 種: navigate / back / saveRecord / deleteRecord / openVoice / openUrl / resetForm / scheduleNotification / cancelNotification | P0 | iOS | F09 |
| F12 | **生成アプリのユーザーデータ(明細・記録)は appconfig `records` API(Firestore)へ保存。iOS は表示用キャッシュのみ**(旧: ローカル永続化) | P0 | iOS | F11,F37 |
| F13 | 生成完了後に iOS がアプリ全体を差し替えて再描画(アニメーション付き) | P0 | iOS | F08,F09 |
| F14 | カメラ: `photoField`(camera/library)→ ≤800px・q0.5 に縮小→ `data:image/jpeg;base64` を Field 値として保存→ `image` で表示 | P0 | iOS | F10,F12 |
| F15 | ローカル通知: `app.notifications[]` を DSL ロード時に同期(識別子 `dsl.<id>`・once/daily/weekly)+ `scheduleNotification`/`cancelNotification`・権限要求 | P0 | iOS | F11 |
| F16 | LLM 失敗/検証失敗時のフォールバック(エラー列挙を渡して再生成 1 回→キーワードでテンプレ DSL) | P0 | ai | F06,F19 |
| F17 | GCP 構築手順: Cloud Run×2 / Firestore / Firebase Auth / Secret Manager / 環境変数 | P0 | infra | — |
| F18 | CI: Go lint+test(**5 分以内必須**)、iOS build+unit test(時間制限なし) | P0 | infra | — |
| F19 | デモ用 seed: `docs/dsl/examples/{kakeibo,training,walk}.json`(手書きの正しい `area` 付き。フォールバック兼回帰テスト。iOS バンドル+Go テストで両側デコード) | P0 | ai+iOS+docs | F01 |
| F20 | **追加発話で既存 DSL を改変: ai が `PatchDocument{patch,summary}`(RFC 6902)を構造化出力→appconfig が適用・検証・version+1→iOS はフル DSL を受け直す**(旧: フル再生成) | P1 | ai+appconfig+iOS | F08,F13 |
| F21 | 生成中スケルトン/進捗(STT 中/生成中/反映中の 3 段階) | P1 | iOS | F13 |
| F22 | **Firebase Auth(Sign in with Apple 優先・詰まれば匿名認証)+ Go 側 ID トークン検証。全 API は uid スコープ** | **P0** | iOS+appconfig+ai | F07 |
| F23 | Map Leaf + 現在地取得(Field `location`)。DSL は enum に 1 要素足すだけで互換 | P1 | iOS+docs | F10 |
| F24 | Chart Leaf(bar/line/pie・groupBy・bucket、Swift Charts) | P1 | iOS | F10,F38 |
| F25 | 生成の構造化ログ(prompt/所要時間/検証結果/再生成回数)を Cloud Logging + Firestore に保存 | P1 | ai | F05 |
| F26 | **`POST /apps/{appId}/versions/{version}/restore` で 1 つ前に戻す(API + 履歴 UI)** | **P1** | iOS+appconfig | F07 |
| F27 | 複数アプリ保持と音声切替 | P2 | iOS+ai | F20,F41 |
| F28 | Share Sheet / Shortcuts / HealthKit 連携アクション | P2 | iOS | F11 |
| **F29** | **グリッド描画 `GridScreenLayout`: 横 A〜F 6 列(可視幅÷6)× 縦 1〜N 行(1 行=可視高さ÷12)。Screen 直下の `area:"左上:右下"` を絶対配置、13 行目以降は画面全体が縦スクロール(上限 48 行)** | **P0** | iOS | F09 |
| **F30** | **セル内スクロール: 可変長(list / section / vstack / hstack / 長い text)は器の内側でスクロール(`scrollBounceBehavior(.basedOnSize)`)、固定長(入力・ボタン・stat・chart・image)は `clipped`** | **P0** | iOS | F29 |
| **F31** | **`area` 省略フォールバック: `area` で使われた最終行の次から全幅で上から積む(決定的)。`area` が 1 つも無ければ 1 行目から** | **P0** | iOS | F29 |
| **F32** | **backend `Validate()` の `area` 検証: 兄弟同士のセル共有(重なり)・範囲外(G 列/0 行/右下<左上/48 行超)・Container 子の `area` をエラー文言で全件列挙(F16 の再生成入力)** | **P0** | appconfig+ai | F01 |
| **F33** | **Claude プロンプトにグリッド指示 + examples を few-shot で渡し、`area` 付き DSL を安定出力(G10)** | **P1** | ai | F05,F32 |
| **F34** | **タブナビ `app.tabs[]`(2〜5・各タブ独立スタック)。タブ無しは単一スタック** | **P0** | iOS | F09 |
| **F35** | **ai が「元に戻して」の意図を検出し F26 restore を呼ぶ(Patch 生成しない)** | **P1** | ai | F26 |
| **F36** | **WhisperKit `small` モデルを事前 DL(初回起動時 or ビルド同梱)。デモ当日は DL しない。ロード済み/未DL の表示** | **P0** | iOS | — |
| **F37** | **`GET/POST/PATCH/DELETE /apps/{appId}/entities/{entityId}/records[/{recordId}]`(Firestore・uid スコープ・`RecordData` は §3.3 のフィールド型)** | **P0** | appconfig | F07,F22 |
| **F38** | **iOS クライアント集計 `stat`(count/sum/avg/min/max・`thisMonth`/`today` は端末ローカル時刻)** | **P0** | iOS | F12 |
| **F39** | **ホットリロード: Day1 は `GET /apps/{appId}/dsl` ポーリングで version 比較、Day2 に Firestore リスナー(iOS 直読は DSL ドキュメントのみ読取許可)** | **P1** | iOS+infra | F07 |
| **F40** | **アプリ名「Tailor It」(正式名・#62/#69。表示名・UI 文言・README・台本を統一。内部識別子 `Metamon`・Bundle ID `jp.kz2601.metamon` は維持)** | **P1** | iOS | 済 |
| **F41** | **1 ユーザー 1 アプリ: uid→appId を固定で解決(`POST /apps` は初回のみ)。一覧 UI は作らない** | **P0** | appconfig+iOS | F22 |

### 3.1 コンポーネント目録(18 種 = Container 3 + Leaf 15。詳細は `docs/dsl/README.md` §3.1)

| 区分 | type | 優先 | 使用シナリオ |
|---|---|---|---|
| Container | `vstack` / `hstack` / `section`(children は Leaf のみ・`area` は Screen 直下のみ) | P0 | 全部 |
| Leaf 表示 | `text` / `spacer` / `divider` / `image` | P0 | 全部 / C |
| Leaf 入力 | `textField` / `numberField` / `datePicker` / `picker` / `toggle` / `photoField` | P0 | A,B,C |
| Leaf データ | `list`(entity・item は field id・sort/filter・swipeToDelete・onItemTap) / `stat`(クライアント集計) | P0 | 全部 |
| Leaf 操作 | `button` / `voiceInput` | P0 | 全部 / 初期状態 |
| Leaf グラフ | `chart`(bar/line/pie) | P1 | A,B |
| (予約) | `map` | P1 | C |

- 全種に任意キー `id`・`area`。Claude に渡すのは「この目録 + examples 3 本 + グリッド指示」だけ。目録外は F06 で落とす(iOS 側はプレースホルダで落ちない=二重防御)。
- 式言語(`{{...}}`)・`state`・汎用 `native` は **v1 に無し**。データ束縛は `screen.entity` + 入力 Leaf の `field` 参照のみ。

## 4. 非機能要件

| 項目 | 目標 | 備考 |
|---|---|---|
| 生成レイテンシ | 発話終了→画面切替 **15 秒以内**(STT≤3s / Claude≤10s / 反映≤2s)。20 秒で警告、30 秒でフォールバック | 構造化出力 + 目録を短く + sonnet 既定。`area` 再生成 1 回は予算内(≤10s 追加)。計測は F25 |
| STT 精度 | 日本語デモ文 3 本が実機で安定。WhisperKit `small` を事前 DL(F36)。失敗時はテキスト入力へ即切替(F04) | 会場ノイズ想定で外付けマイク不要。`small` で遅ければ `base` に落とす判断は Day1 夜 |
| オフライン | 最後に受け取った DSL と records キャッシュで**閲覧**は継続。保存・生成は不可(バナー表示) | 会場 Wi-Fi 不安定でも画面は壊れない。保存はオンライン復帰後に手動再送 |
| フォールバック | Claude 失敗/スキーマ不正/`area` 重なり→エラー列挙付きで再生成 1 回→キーワード一致でテンプレ DSL(F19)→テンプレも無ければ「聞き取れませんでした」で音声画面へ。`area` だけ壊れていれば `area` を落として積む(F31) | デモ 3 本は必ずテンプレで救える。「安定稼働」として説明 |
| レイアウト整合 | `area` の重なり・範囲外は backend で 100% 検出(F32)。iOS は不正 `area` を受けても落ちない(無視して積む) | backend 厳格・iOS 寛容 |
| セキュリティ | public リポ: Claude キーは **Secret Manager → Cloud Run 環境変数のみ**。iOS はキーを持たない。Firestore はクライアント直アクセス deny(F39 の DSL 読取のみ例外)・Go 経由。全 API に Firebase ID トークン | GoogleService-Info.plist は gitignore + gitleaks |
| データ | ユーザーデータは uid スコープの Firestore。写真は base64(≈100KB/枚)で 1MB 制限内。URL 化は P1 | 1 ユーザー 1 アプリ前提で件数は少ない |
| 可用性(デモ) | Cloud Run min-instances=1 で冷起動回避(デモ当日のみ)。ローカル Go を Mac で動かす退路 | — |
| コスト | 無料枠 + Claude 数千円以内。生成 1 回≒入力 4k/出力 2k トークン(examples few-shot 込み) | — |
| 観測性 | 生成毎に所要時間・検証結果・再生成回数・使用モデルをログ | 「独自技術」説明に数値を使う |

## 5. 審査観点(README テンプレ)との対応

| README 項目 | 書くこと | 効く要件 |
|---|---|---|
| 背景 | 「アプリを探す/作る」のコストをゼロにし、"話せばアプリが生える"体験 | G1-G3 |
| 製品説明 | 音声→Claude→DSL v1→SwiftUI レンダリングの Server-Driven UI。初期状態は音声画面のみ | F01-F13 |
| 特長1 | **AI がタブ構成と画面レイアウト(6 列グリッドの座標)まで決めて**、10 数秒でネイティブ機能(カメラ/通知)つきの動くアプリになる | G3, G6, F29-F34 |
| 特長2 | 追加で話すとアプリが育つ(JSON Patch 差分・version 管理・「元に戻して」) | G9-G11, F20, F26 |
| 特長3 | LLM が落ちても止まらない(スキーマ+意味検証・レイアウト検証・ルールベースフォールバック)。データはクラウドに残る | F06, F16, F32, G4 |
| 解決できること | 非エンジニアの"自分専用の小さなアプリ"を即席で | §2 シナリオ |
| 今後の展望 | 生成前の対話で要件を固める・Container 内グリッド・map/位置情報・他アプリ連携・複数アプリ・アプリ共有 | G14-G17 |
| こだわり | 15 秒レイテンシ、生成中 UX、目録を絞って品質を出す、backend 厳格/iOS 寛容の二重防御 | §4, F21 |
| 独自技術 | 非再帰 2 段 DSL + グリッド座標 DSL、構造化出力+検証パイプライン、JSON Patch 改変、SwiftUI グリッドレンダラー | F01, F05-F06, F20, F29-F32 |
| 活用技術 | Claude API / WhisperKit / Firestore / Cloud Run / Firebase Auth / Swift Charts / UserNotifications | 00 スタック表 |

## 6. 判断の記録と判断待ち

v1 の Q1〜Q5 は **決着**(詳細は `00_decisions.md` 判断ログ): Q1 ユーザーデータ=**Firestore**(推奨 a を覆してユーザー決定) / Q2 改変=**JSON Patch** / Q3 ネイティブ=**カメラ+通知の両方 P0** / Q4 デモ3本=**家計簿/トレーニング/散歩メモ** / Q5 Sign in with Apple=**やる、詰まれば匿名認証**。

| # | 論点 | 選択肢 | 補佐の推奨 |
|---|---|---|---|
| Q6 | 仮名「メタモン」の商標リスク(ポケモン関連の既存商標) | a) 提出前に別名へ / b) 表示名のみ変更し内部 `Metamon` は維持 / c) そのまま | **a+b で決着(#62)**。正式名 **Tailor It** へ表示名・文言を変更し、内部 `Metamon` は維持 |
| Q7 | Day1 夜にカメラ(F14)が遅れていた場合 | a) カメラを P1 に落とし通知だけで G6 成立 / b) 両方 P0 を維持して Day2 午前も続ける | **a** を発動する時刻を Day1 22:00 と決めておく |
| Q8 | 審査本番の順序 | a) A→B→C(本書案) / b) B→A→C(v1 案) | **a**。C の位置情報が P1 に落ちリスクが下がった |
| Q9 | F39 Firestore リスナー(iOS が DSL ドキュメントを直読)を許すか | a) 許す(ルールで該当パスのみ read 許可) / b) ポーリングのまま | **a** は Day2 午前に余裕があれば。無ければ b で問題ない |
