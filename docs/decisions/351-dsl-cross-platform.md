# #351 DSL を OS 共通の約束事にする(版・部品の対応表・OS ごとの差の扱い)

> 状態: **提案(オーナーの判断待ち)**。2026-11-02 作成。
> 対象 Issue: [#351](https://github.com/jphacks/kz_2601/issues/351)(依存 #302・この後に #352 #353 #354)
> 調べた元: `origin/develop`(e576869d)のスキーマ・iOS のコード・backend のコード。部品の数・名前はコードから数えた(記憶や Issue の文ではない)。
> **この文書は下書き**。決まったら、対応表と規則を [`docs/dsl/README.md`](../dsl/README.md) に移す(Issue の「完了の条件」)。この PR では README・スキーマ・[`docs/00_decisions.md`](../00_decisions.md) は変えない。

## 目次

1. [結論(推奨)](#結論推奨)
2. [今の数え直し](#今の数え直し)
3. [対応表(部品 66 種・アクション 12 種)](#対応表部品-66-種アクション-12-種)
4. [OS で違ってよい所・同じでなければならない所](#os-で違ってよい所同じでなければならない所)
5. [対応していない部品に出会った時](#対応していない部品に出会った時)
6. [版を OS をまたいで使う(#302 の拡張)](#版を-os-をまたいで使う302-の拡張)
7. [スキーマから各 OS の型を作る](#スキーマから各-os-の型を作る)
8. [適合テストの骨組み(#352)](#適合テストの骨組み352)
9. [決めてほしいこと](#決めてほしいこと)
10. [参考](#参考)

## 結論(推奨)

1. **部品を 3 つに分ける**: 全 OS 必須(A)・OS ごとに作り方が違い、使えない端末では決まった代わりを出す(B)・iOS だけ(C)。今の 66 部品は **A 56 / B 9 / C 1**、12 アクションは **A 10 / B 2**(§3)。
2. **同じでなければならないのは「何が・どこに・どの値で・押すと何が起きるか」**(並び・グリッドの位置・データと計算結果・動き・検証)。違ってよいのは「どう見えるか」(フォント・余白・部品のネイティブの見た目・アイコンの絵・アニメーション)(§4)。
3. **対応外の部品は 2 段で受ける**: (1) appconfig が取得時に対応表の代わりの部品に置き換える(保存はしない)、(2) それでも残った部品はクライアントが「この端末では使えません」の小さい枠を出す。今の iOS のリリース版は**何も描かない(`EmptyView`)**ので、ここは変える(§5)。
4. **版は #302 の仕組みをそのまま OS ごとに持つ**: `MIN_<OS>_BUILD` を OS ごとに、申告は今の `allowedComponents` + `supportedFeatures` に `supportedActions` を足す。`supportedFeatures` には部品の中の機能(`photoField.analyze` など)も入れる(§6)。
5. **型はスキーマから自前の生成器(Go)で作る**。quicktype は今の「1 ノードに全 props を平らに持つ」形と「知らない値は落とさず丸める」寛容さに合わない。Kotlin は最初から生成、Swift はまず enum と「部品ごとのキー一覧」だけ生成して手書きの struct と CI で突き合わせる(§7)。
6. **適合テストは「DSL + レコード + 端末の能力」→「描画の木(JSON)」を比べる**。ピクセルは比べない(§8)。

## 今の数え直し

Issue の「部品 51 種・アクション 9 種」は古い。`docs/dsl/README.md` §3 の「Leaf 44 種」、ロードマップ #357 の「Leaf 44 種」も古い。

| 数えたもの | 数 | 元 |
|---|---|---|
| Container | 7 | [`dsl.schema.json`](../dsl/dsl.schema.json) `$defs.Container.properties.type.enum` |
| Leaf | 59 | 同 `$defs.Leaf.anyOf` の各 `type.const` |
| 部品の合計 | **66** | iOS の [`ComponentType`](../../ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Component.swift)(`unknown` 除く)・[`ComponentRegistry.standard`](../../ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Registry/ComponentRegistry.swift) の登録と一致(`ComponentRegistryTests` が強制) |
| Action | **12** | 同 `$defs.Action.properties.type.enum`・iOS の [`ActionType`](../../ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Action.swift) と一致 |
| Field.type | 15 | string text integer number boolean date datetime enum currency photo video audio location ref refs |

- 審査用ビルド(`APP_STORE_REVIEW`)は 8 部品を登録しない: healthStat locationTracker nearbyShare dashcam sensorValue audioRecorder webView chatThread([`ClientCapabilities`](../../ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift))。**「OS によって描けない部品がある」状態は、すでに iOS の中に 1 つある**。Android はこれと同じ仕組みに乗せられる。
- README §3.8 の Web の共有ページ(#362)は、読むだけの 3 つ目の描画先。対応表を README に移す時は Web 列もこの表にまとめる。

## 対応表(部品 66 種・アクション 12 種)

区分:

| 区分 | 意味 | Android(と以後の OS)の扱い |
|---|---|---|
| **A 全 OS 必須** | OS の標準の UI か、端末の外に出ない計算だけで作れる | 初版から描く。描けない版は出さない(適合テストの必須項目) |
| **B OS ごとに代わり** | OS の固有の仕組み(地図・ヘルス・センサー・端末内 AI・近距離通信など)に頼る。各 OS に近いものはある | 各 OS の仕組みで作る。作る前の版・使えない端末は「代わり」の列のとおりにする |
| **C iOS だけ** | 他の OS に同じものが無い・規約上作れない | 他の OS は描かない。AI は他の OS の作成者に使わない。共有アプリでは代わりに置き換える |

Android の候補は #354 の判断の材料で、ここでは決めない。

### Container(7 種・全部 A)

| type | 区分 | iOS の今 | Android の候補 | 理由 |
|---|---|---|---|---|
| `vstack` | A | VStack | Column | 標準の並び |
| `hstack` | A | HStack | Row | 標準の並び |
| `section` | A | 見出し付きのまとまり | Column + 見出し | 標準の並び |
| `card` | A | 角丸 + 影 | Card | 標準の部品 |
| `disclosure` | A | DisclosureGroup | AnimatedVisibility + 行 | 標準の部品 |
| `carousel` | A | 横スクロール | LazyRow | 標準の部品 |
| `grid` | A | 等幅タイル | LazyVerticalGrid / 自前の行 | 標準の部品 |

### Leaf(59 種)

「代わり」は、B / C の部品を描けない時に出すもの。`—` は A なので要らない。

| type | 区分 | iOS の今(使う仕組み) | Android の候補 | 代わり(描けない時) | 理由(1 行) |
|---|---|---|---|---|---|
| `text` | A | Text | Text | — | 標準 |
| `button` | A | Button | Button | — | 標準 |
| `textField` | A | TextField | TextField | — | 標準 |
| `numberField` | A | TextField(数字) | TextField(数字キーボード) | — | 標準 |
| `datePicker` | A | DatePicker | DatePicker(Material 3) | — | 標準 |
| `picker` | A | Picker(menu / segmented / wheel) | ExposedDropdownMenu / SegmentedButton / 一覧 | — | `wheel` は Material に無いので見た目は違ってよい(値は同じ) |
| `toggle` | A | Toggle | Switch | — | 標準 |
| `photoField` | A(`analyze` は B) | PhotosPicker + カメラ。`analyze` は Vision + Foundation Models | Photo Picker + CameraX。`analyze` は ML Kit(文字・姿勢)+ 端末内 AI | `analyze` だけ無視して写真の入力として動く(古い iOS と同じ) | 撮る・選ぶは標準。読み取りは OS の AI に頼る |
| `image` | A | 画像表示 | Image(Coil 等) | — | 標準 |
| `videoField` | A | カメラ / ライブラリ | CameraX / Photo Picker | — | 標準。ただし動画は端末内のファイルなので**他の端末では再生できない**(iOS どうしでも同じ) |
| `videoPlayer` | A | AVPlayer | Media3 ExoPlayer | — | 標準。同上 |
| `audioRecorder` | A | AVAudioRecorder(審査用では外す) | MediaRecorder | — | 標準。イヤホンのボタンの受け方は OS で違ってよい。音声は端末内のファイル |
| `list` | A | 自前の行 | LazyColumn | — | 標準 + 集計は端末内 |
| `stat` | A | 端末内の集計 | 同じ計算 | — | 計算だけ。**値は全 OS で一致必須** |
| `chart` | A | Swift Charts | Vico 等のライブラリ | — | 描き方は違ってよい。棒・点の値は一致必須 |
| `voiceInput` | A | WhisperKit(端末内) | #354 で決める(SpeechRecognizer / whisper.cpp / サーバー) | — | 製品の中心。認識の仕組みは OS で違ってよい |
| `spacer` | A | Spacer | Spacer | — | 標準 |
| `divider` | A | Divider | HorizontalDivider | — | 標準 |
| `slider` | A | Slider | Slider | — | 標準 |
| `stepper` | A | Stepper | −/+ のボタン(Material に無い) | — | 見た目は違ってよい。丸め・上下限は一致必須 |
| `textEditor` | A | TextEditor | TextField(複数行) | — | 標準 |
| `secureField` | A | SecureField | TextField + PasswordVisualTransformation | — | 標準 |
| `timePicker` | A | DatePicker(時刻) | TimePicker(Material 3) | — | 標準。保存の形(`"HH:mm"` / RFC3339)は一致必須 |
| `colorPicker` | A | ColorPicker | 自前の色の一覧(Material に無い) | — | 保存は `"#RRGGBB"` で一致必須 |
| `rating` | A | 星の行 | 星の行 | — | 自前で簡単 |
| `label` | A | Label | Icon + Text | — | `icon` は SF Symbols の名前(下の「アイコン」) |
| `icon` | A | SF Symbols | Material Symbols へ対応表で写す | — | 同上 |
| `progressBar` | A | ProgressView | LinearProgressIndicator | — | 標準 + 集計 |
| `gauge` | A | Gauge | 自前 / CircularProgressIndicator | — | 見た目は違ってよい |
| `badge` | A | 色付きの文字 | Badge / Surface | — | 標準 |
| `link` | A | Link | ClickableText + Custom Tabs | — | 標準 |
| `markdown` | A | AttributedString(インライン) | 自前の簡単な解釈 / ライブラリ | — | インラインだけなので作れる。解釈できる記法の範囲は一致させる |
| `avatar` | A | 丸い画像 / イニシャル | 同じ | — | 標準 |
| `table` | A | 自前の表 + `TableGrouping` | 同じ計算 | — | 集計は一致必須 |
| `timeline` | A | 自前 | LazyColumn | — | 標準 |
| `emptyState` | A | ContentUnavailableView 風 | 自前 | — | 標準 |
| `searchBar` | A | 検索欄(画面ローカル) | SearchBar / TextField | — | 標準。絞り込みの規則は一致必須 |
| `calendar` | A | 自前の月表示 | 自前の月表示 | — | 自前の UI |
| `gridTable` | A | 自前(`GridTableLayout`) | 同じ計算で自前 | — | マスの配置計算は一致必須 |
| `shareButton` | A | ShareLink / LINE の URL | 共有の Intent(ACTION_SEND)/ LINE の URL | — | 両 OS に共有シートがある |
| `openUrlButton` | A | openURL(allowlist のスキーム) | Intent / Custom Tabs | — | `maps:` は Apple のスキーム。Android は `geo:` か Google マップの URL に写す(写し方を表で決める) |
| `mapView` | **B** | MapKit | Google Maps Compose(API キーが要る)/ OpenStreetMap | 場所の一覧(名前 + 「地図で開く」) | 地図は OS ごとの SDK。キーと費用の条件が違う |
| `locationButton` | A | CoreLocation(使用中だけ) | Fused Location(使用中だけ) | — | 両 OS 標準。値は `"lat,lng"` で一致 |
| `locationPicker` | **B** | MKLocalSearch(キー不要) | Places API(キーと費用)/ Geocoder / 地図のピンだけ | 地図のピン・現在地だけで選ぶ(検索なし) | 場所の検索は OS ごとの仕組み。Android は無料で同じことはできない |
| `routeGuide` | **B** | MKDirections + 読み上げ | Google の経路 API(費用)/ 作らない | 「マップで道案内」ボタン(Google マップのナビの URL を開く) | 経路の計算は OS の地図に頼る。Android は外のアプリに任せる方が安い |
| `qrScanner` | A | AVFoundation | ML Kit Barcode + CameraX | — | 両 OS 標準 |
| `qrCode` | A | CoreImage | ZXing 等 | — | 誤り訂正 M・余白 4 モジュールは一致必須 |
| `timerDisplay` | A(`liveActivity` は B) | Timer。`liveActivity` は ActivityKit | 同じ。`liveActivity` は進行中の通知(Android 16 の Live Updates) | `liveActivity` だけ無視(画面の中のタイマーとして動く) | タイマーは標準。ロック画面の表示は OS の仕組み |
| `webView` | A | WKWebView(審査用では外す) | WebView | — | 両 OS 標準 |
| `settlement` | A | 計算(`Expr`) | 同じ計算 | — | 計算だけ。一致必須 |
| `randomPicker` | A | 自前(`RandomPick`) | 同じ規則 | — | 「直前と同じ 1 件を続けて出さない」は一致必須(乱数そのものは一致しなくてよい) |
| `aiButton` | **B** | Foundation Models(iOS 26+・対応端末だけ)。generate はオンラインの代わりあり | Gemini Nano(ML Kit GenAI / AICore・対応端末だけ)。generate は同じ `POST /v1/text` | 今の iOS と同じ「この端末では AI を使えません」(理由付きで無効)。generate はオンライン | 端末内 AI は OS ごと・対応端末も限られる。代わりの表示はもう iOS にある |
| `nearbyShare` | **B**(同じ OS どうしだけ) | MultipeerConnectivity(+ passby・UWB) | Nearby Connections | 「この端末では使えません」。**iOS と Android の間では届かない**ことを部品に出す | 近距離通信は OS ごとに別の仕組みで、OS をまたいで繋がらない |
| `locationTracker` | **B** | CoreLocation(閉じても停止まで) | 位置の Foreground Service(通知が常に出る・Play の申告が要る) | 「この端末では使えません」 | 裏で続ける作りは OS ごとに規則が違う |
| `chatThread` | A | 自前 + SSE(審査用では外す) | 自前 + SSE | — | UI とサーバーだけで作れる |
| `healthStat` | **B** | CoreMotion(歩数)+ HealthKit(体重) | Health Connect(歩数・体重) | 今の iOS と同じ「ヘルスケアへのアクセスが許可されていません」系の表示 | ヘルスデータは OS ごとの仕組み |
| `dashcam` | **B** | AVFoundation(前面にいる間だけ) | CameraX(Foreground Service なら裏でも撮れる) | 「この端末では使えません」 | カメラの裏での扱いが OS で逆。動き(裏で撮るか)を揃えるか決める |
| `sensorValue` | **B** | CoreMotion / CMAltimeter | SensorManager(加速度・ジャイロ・気圧) | 今の iOS と同じ「このセンサーはこの端末では使えません」 | センサーの有無は端末ごと。単位と相対高度の定義は一致必須 |
| `musicPlayer` | **C** | MusicKit(Apple Music)+ リクエストは MultipeerConnectivity | (Apple の Android 用 MusicKit はあるが、Apple Music アプリと契約が前提・**要確認**) | 「この端末では使えません」+ 曲名で検索する外部リンク(Spotify / YouTube Music) | Apple Music の再生は Apple の仕組みに強く依存。他の OS で同じ体験を約束できない |

### アクション(12 種)

| type | 区分 | iOS の今 | Android の候補 | 代わり | 理由 |
|---|---|---|---|---|---|
| `navigate` | A | push / sheet / fullScreenCover | Navigation の遷移 / ModalBottomSheet / 全画面の Dialog | — | 見た目(シートの形)は違ってよい。行き先・recordId を暗黙に渡す・`prefillRef` は一致必須 |
| `back` | A | pop / dismiss | 同じ(+ OS の戻る操作) | — | 標準 |
| `saveRecord` | A | ドラフトを POST / PATCH → 再取得 → 遷移 | 同じ | — | データの動き。一致必須 |
| `deleteRecord` | A | DELETE → back | 同じ | — | 同上 |
| `openVoice` | A | 音声シート | 同じ | — | 製品の中心 |
| `openUrl` | A | Safari | Custom Tabs | — | 標準 |
| `resetForm` | A | ドラフトを既定値に | 同じ | — | 同上 |
| `scheduleNotification` | A | UNUserNotificationCenter(`dsl.<id>`) | AlarmManager / WorkManager + 通知(13+ は通知の許可) | — | 両 OS にある。**時刻のずれの許容**(Android 14 は正確なアラームに制限)を決める |
| `cancelNotification` | A | 同上 | 同上 | — | 同上 |
| `export` | A | CSV / PDF + 共有シート | CSV / PdfDocument + 共有の Intent | — | 中身(列・書式・CSV インジェクション対策)は一致必須 |
| `startActivity` | **B** | ActivityKit(ロック画面 / Dynamic Island) | 進行中の通知(Live Updates) | 何もしない + トースト「この端末では使えません」 | ロック画面の表示は OS の仕組み |
| `endActivity` | **B** | 同上 | 同上 | 何もしない | 同上 |

### 部品でない DSL の機能

| もの | 区分 | 扱い |
|---|---|---|
| グリッド配置(`area`・Container 内のローカルグリッド・`area` 無しの積み方) | A | **位置の計算は一致必須**(§4)。セルの中の余白は違ってよい |
| `app.tabs` / `app.voiceEntry` / `app.theme` | A | タブ = BottomNavigation / NavigationBar。色は `primaryColor` から各 OS の作法で |
| `app.notifications[]` | A | DSL を当てた時に同期。識別子 `dsl.<id>`・`once` の翌日送りは一致必須 |
| 計算式 `Expr`(`expressions.md`)・`computed`・集計(`stat` / `chart` / `table.groupBy`) | A | **一致必須**。端末ローカル時刻の `thisMonth` / `today` も同じ定義 |
| `entity.scope` / `edit` / `marks` / `$author` / `$createdAt` | A | サーバーの規則なので OS に依らない |
| `Entity.retentionDays` | A | `supportedFeatures: recordRetention` で申告(#482) |
| `Entity.match`(#507・**develop には未マージ**・`origin/feat/matching-507`) | A の予定 | `supportedFeatures: matching` で申告する作りで進んでいる。§6 の一般化と同じ形 |
| `Field.health: "weight"` | B | Health Connect に書く。使えない OS は書かずに保存だけ(今の iOS の失敗時と同じ) |
| アイコンの名前(`icon.name`・`label.icon`・`button.icon`・`tabs[].icon`・toolbar) | A(絵は違ってよい) | **名前の語彙は SF Symbols のまま**。各 OS が対応表で写し、無い名前は決まった既定のアイコン。将来、閉じた enum にするかは §9 |
| `color`(閉じた enum) | A | 各 OS のシステム色に写す |

## OS で違ってよい所・同じでなければならない所

| 同じでなければならない(適合テストで縛る) | 違ってよい(各 OS の作法に合わせる) |
|---|---|
| 画面の並び・タブの順・`initialScreen` | フォント・文字の大きさ・行間(OS の既定と文字サイズの設定に従う) |
| 部品の順と、グリッドのどのセルに置くか(`area` の解決・積み方の規則・Container 内のローカルグリッド) | セルの中の余白・角丸・影・区切り線の太さ |
| 表示する**値**(レコード・計算式・集計・丸め・日付と通貨の書式・空の時の「—」) | 部品の見た目(segmented / wheel / stepper など OS に無い部品の作り方) |
| 押した時の動き(遷移先・recordId を暗黙に渡す・`prefillRef`・保存 → 戻る・削除の確認・`confirmMessage`) | シートの形(下から出る / 中央)・遷移のアニメーション・触覚の反応 |
| 入力の検証と保存の形(必須・`min/max/step` の丸め・`"HH:mm"`・`"#RRGGBB"`・`"lat,lng"`) | アイコンの絵(名前の対応表で写す)・色の正確な濃さ(`color` の enum の意味は同じ) |
| 一覧の並べ替え・絞り込み・検索の一致の規則 | 共有シート・地図のタイル・通知の見た目 |
| 通知の識別子・時刻(許容するずれは §9 で決める)・自動削除(`retentionDays`) | 権限を求める文と出す時機(各 OS の規則に従う) |
| 対応外の時の扱い(§5 の規則・文言の意味) | 読み上げ(VoiceOver / TalkBack)の作り方。ただし読み上げる中身は同じ |

迷った時の基準: **同じアプリを iOS と Android で並べて、利用者が「データが違う」「押したら違う所に行った」と感じるものは同じでなければならない**。「Android らしい見た目だ」で済むものは違ってよい。

## 対応していない部品に出会った時

### 今の iOS

| 場面 | 動き | 場所 |
|---|---|---|
| 知らない部品の type | `ComponentType.unknown` に丸め、レジストリに無ければ `UnsupportedComponent`。**DEBUG は黄色の「未対応: <type>」、リリースは `EmptyView`(何も描かない)** | [`DSLView.swift`](../../ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Screen/DSLView.swift)・[`UnsupportedComponent.swift`](../../ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/UnsupportedComponent.swift) |
| 審査用ビルドで外した部品 | 上と同じ(登録しないのでプレースホルダ扱い) | `ComponentRegistry.standard` の `#if !APP_STORE_REVIEW` |
| 知らないアクション | トースト「未対応のアクション: <type>」 | [`AppRuntime.swift`](../../ios/Metamon/App/AppRuntime.swift) |
| 知らないキー・enum の値 | 無視・既定に落とす(`Lenient`) | `MetamonDSL` |
| 部品の中の機能が使えない(AI・センサー・ヘルス) | 部品ごとに理由を出す(「この端末では AI を使えません」など) | 各 Leaf |

リリースで何も描かないと、グリッドに穴が開くだけで**利用者は何が欠けたか分からない**。iOS だけの時は「新しい backend × 古い iOS」でしか起きなかった(しかも #302 で生成時に防いでいる)が、OS が増えると「iOS で作った共有アプリを Android で開く」で日常的に起きる。

### 選択肢

| 選択肢 | 意味 | 良い点 | 悪い点 |
|---|---|---|---|
| X. 今のまま(リリースは何も描かない) | 変えない | 作業なし | 穴の理由が分からない。共有アプリで「壊れた」と見える |
| Y. クライアントの枠だけ | 描けない部品の場所に「この端末では使えません」の枠 | 各 OS の中で閉じる | 代わりに使える部品(地図 → 場所の一覧)があっても使えない |
| Z. サーバーで置き換えだけ | appconfig が取得時に、申告に無い部品を表の「代わり」に置き換える | 代わりの中身が全 OS で揃う | 申告を送らない古い版・オフラインのキャッシュでは効かない |
| **Y + Z(推奨)** | 先に Z で置き換え、残った分は Y | 二重防御(README §1「iOS は寛容・backend は厳格」と同じ考え方) | 両方作る |

推奨の細かい規則(下書き):

- **置き換えは取得の応答だけ**で行い、保存している DSL は変えない(iOS で開けば元の部品のまま)。置き換えた部品には `id` と `area` を残す(グリッドの位置が変わらない)。
- 置き換えの表は 1 か所(backend の Go の表 + README の対応表の「代わり」列)。#353 の appconfig はこの表を読む。
- クライアントの枠の文言を 2 つに分ける:
  - **型を知っているが、この OS / この端末では描けない**(生成した型には有るが、その OS のレジストリに無い・権限や端末が無い): 「この端末では使えません」+ 部品の説明名(例「道案内」)。
  - **型を知らない**(クライアントより新しい部品): 「アプリを更新すると表示できます」。ストアへのボタンは出さない(`LATEST_<OS>_BUILD` のお知らせに任せる)。
  - 型の一覧をスキーマから生成すると(§7)、この 2 つは機械的に区別できる。
- 枠の大きさは置いた `area` のまま(グリッドを詰めない)。`area` 無しで積まれている時は 1 行分。
- アクションは今の iOS と同じトースト。文言は上の 2 種類に揃える。
- 部品の中の一部の機能(`photoField.analyze`・`timerDisplay.liveActivity`・`Field.health`)が使えない時は、**その機能だけ無視して部品は普通に動かす**(古い iOS が未知キーを無視するのと同じ結果)。

## 版を OS をまたいで使う(#302 の拡張)

### 今あるもの(#302・#482)

| もの | 中身 | 場所 |
|---|---|---|
| `schemaVersion` | 非互換な変更でだけ上げる整数(今は 1) | README §5.1 |
| generate / chat の body `allowedComponents` | その版が描ける部品の全部 | `ClientCapabilities.allowedComponents` → [`restrict.go`](../../backend/internal/ai/restrict.go) `NewRestriction` |
| `disabledFeatures` | 審査用ビルドで止める機能(否定のリスト): fieldHealth cameraCapture sharedData | 同上 |
| `supportedFeatures` | 部品以外で実装済みの動作(肯定のリスト): 今は `recordRetention`(#482)。#507 のブランチで `matching` を追加中 | `NewClientRestriction`。`allowedComponents` を送るのに無い = 古い版とみなし AI に使わせない |
| ヘッダ `X-Client-Platform` / `X-Client-Version` / `X-Client-Build` / `X-Client-DSL-Version` | `ios` など | ログ |
| `GET /v1/client-config` | `MIN_IOS_BUILD` / `LATEST_IOS_BUILD` / `IOS_APP_STORE_URL` | appconfig・README §5.4 |

`supportedFeatures` の作りが一般化の鍵: **「申告しない = 持っていない」**(肯定のリスト)なので、新しい機能を足しても、それを知らない版・知らない OS は自然に外れる。Android の初版は、作った機能だけを並べればよい。

### 一般化の案

| 論点 | 選択肢 | 推奨 |
|---|---|---|
| 最低対応版 | (a) OS ごとに `MIN_<OS>_BUILD` / `LATEST_<OS>_BUILD` / `<OS>_STORE_URL`(`X-Client-Platform` で返す値を選ぶ)・(b) 共通の 1 つ | **(a)**。ビルド番号は OS ごとに別物(iOS の `CFBundleVersion` と Android の `versionCode`)。応答は今の形を保ち、`platform` 別のキーを足す |
| 部品の申告 | (a) 今の全部の一覧(`allowedComponents`)・(b) 「対応の版」番号 1 つ | **(a)**。OS ごとに持つ部分集合(A + 作った B)を表せるのは一覧だけ。番号だと OS の出す順番が縛られる。66 個の名前で 1 KB 未満 |
| アクションの申告 | (a) `supportedActions` を足す・(b) `supportedFeatures` に `action:startActivity` の形で混ぜる・(c) 申告しない | **(a)**。今はアクションを制限する仕組みが無い(`restrict.go` は部品だけ)。Android の初版に `startActivity` が無いと、生成時に防げない |
| 部品の中の機能 | `supportedFeatures` に `<type>.<key>` の名前で入れる(`photoField.analyze`・`timerDisplay.liveActivity`・`field.health`) | **入れる**。今の `recordRetention` と同じ「無ければ使わせない」 |
| 申告を送らない | 今どおり制限なし(開発版・ai-eval) | 変えない。ただし `X-Client-Platform` が `ios` 以外で申告が無い時は、**A だけ**に絞る(未知の OS に B / C を出さない) |
| 取得時の申告(#353 の置き換え) | (a) `GET /dsl` にもヘッダで部品の一覧・(b) (platform, build) から backend が対応を引く表を持つ・(c) 置き換えをクライアントだけでする | **(a)** を推奨(生成と同じ一覧を使える・backend に版の表を持たない)。ヘッダ名は `X-Client-Components` などを #353 で決める。長さが気になれば一覧のハッシュを送り、初回だけ本体を送る |
| `schemaVersion` | OS ごとに別の版にするか | **全 OS で 1 つ**。非互換な変更は全 OS が同時に追いつくまで出さない(README §5.1 の「新しい名前で足す」を優先) |

### 生成の時の選び方(#353 へ)

1. 作成者のクライアントの申告(部品・アクション・機能)で、`restrict.go` が今と同じようにカタログ・索引・生成スキーマを絞る(OS の違いは申告の中身の違いになるだけで、仕組みは変えない)。
2. **共有アプリ**(`scope: shared / public` の entity を持つ)は、他の OS の人も開く。選択肢: (i) A だけに絞る(厳しい)・(ii) A を優先し、B は「代わり」がある部品だけ・C は使わない(推奨)・(iii) 制限しない。
3. 取得時は §5 の置き換え。

## スキーマから各 OS の型を作る

### 今

- 正典は [`dsl.schema.json`](../dsl/dsl.schema.json)(draft 2020-12)。Go は `backend/pkg/dsl` に埋め込み(`make schema-check` で byte 一致)。
- 型は **Go も Swift も手書き**。どちらも**1 ノードに全部品の props を平らに持つ** 1 つの struct(`dsl.Component` / Swift `Component`・Swift は `public var` が 122 個)。Swift は `Lenient` / `Lossy` で知らない値を落とさず丸める。
- 抜けの検出はテスト: iOS `ComponentRegistryTests`(スキーマの type と `ComponentType`・レジストリの一致)・`ExportTests`(Action の一致)、backend `catalog_test.go`(Go の定数とスキーマの一致)。**キー(props)の抜けは検出していない**(スキーマにキーを足して Swift に足し忘れても、Swift は黙って無視する)。

スキーマの形の特徴(生成器を選ぶ時に効く):

- `Leaf` は 59 個の object の `anyOf`(`type` の `const` で区別する判別子付きの和だが、`discriminator` は書いていない)。
- 同じ名前のキー(`title` `label` `field` `entity` など)が部品をまたいで共有され、型も同じ。
- `x-for` / `x-enumDescriptions` / `x-usage` など独自のキーを持つ。
- 構造化出力のために数値の制約はスキーマに書かず Go の `Validate()` に置いている。

### 選択肢

| 選択肢 | 意味 | 良い点 | 悪い点 |
|---|---|---|---|
| Q. quicktype | JSON Schema から Kotlin(kotlinx.serialization)・Swift を生成 | 既製品・多言語 | `anyOf` の 59 個は部品ごとの別クラスか、名前の予測しにくい合成クラスになり、今の「平らな 1 struct」と合わない。知らない enum 値で落ちる(寛容さは手で足す)。draft 2020-12 と `x-*` の扱いが部分的。出力の差分が大きく揺れる |
| O. OpenAPI Generator / jsonschema2pojo 系 | 同上 | 既製品 | Q と同じ問題 + JSON Schema の方言の差 |
| **G. 自前の生成器(Go・推奨)** | `backend/cmd/dslgen` がスキーマを読み、各言語のファイルを書く。`restrict.go` / `catalog.go` がもうスキーマを歩いているので、その読み方を再利用 | 平らな struct・`UNKNOWN` に丸める enum・寛容なデコードなど、**今の設計どおりの形**で出せる。出力が決定的で差分が読みやすい。部品ごとのキー一覧(`x-for` 含む)も一緒に出せる | 生成器を自分で保守する(見込み 300〜500 行)。スキーマの書き方が変わったら直す |
| T. 型を正典にしてスキーマを生成 | Kotlin / Swift / Go の型からスキーマを作る | — | 正典はスキーマ(Claude の構造化出力が読む)。逆転は README §4 の前提を崩す。不採用 |
| H. 手書き + CI で突き合わせ | 今の iOS のやり方を Android にも | 自由 | Issue が「手で書かない」と決めている。キーの抜けは突き合わせを足さないと見えない |

### 推奨の進め方

1. **生成するもの**(G): 各言語に (1) `ComponentType` / `ActionType` / `FieldType` の enum(`unknown` 付き)、(2) 平らな `Component` / `Action` / `Screen` / `Entity` / `Field` / `AppDSL` の型(全キー省略可・寛容なデコード)、(3) 部品ごとの「使ってよいキー」の表(`x-for` とスキーマの各定義から。適合テストと検証に使う)。
2. **Kotlin(Android)は最初から生成だけ**(手書きの型を作らない)。#354 の「kotlinx.serialization」はこの出力を前提にする。
3. **Swift は段階的に**: まず enum と (3) のキー表だけ生成し、手書きの `Component` の `CodingKeys` とキー表をテストで突き合わせる(キーの抜けが赤になる)。全体の置き換えは後で決める(§9)。
4. **Go は enum の定数だけ生成**するか、今の `catalog_test.go` の突き合わせのままにするかを選ぶ(どちらでもよい)。
5. 生成物はリポジトリにコミットする(各 OS のビルドが生成器に依存しない)。

### CI での確かめ方

- backend の CI(`backend-ci.yml`)に `make dsl-gen && git diff --exit-code` を足す。`docs/dsl/**` か生成器が変わった PR で、生成物のコミット忘れが赤になる(今の `schema-check` と同じ考え方)。
- iOS の CI: キー表と Swift の `CodingKeys` の突き合わせテスト・`ComponentRegistryTests`(今のまま)。
- Android の CI(#356 で作る): 生成した型で `docs/dsl/examples/*.json` を全部デコードするテスト・レジストリと「その OS で描く部品(A + 作った B)」の突き合わせ。

## 適合テストの骨組み(#352)

### 置き場と形(案)

```
docs/dsl/conformance/
  cases/<名前>.json      DSL + 試験用のレコード + 端末の条件 + 操作
  expected/<名前>.tree.json  期待する描画の木(全 OS 共通)
```

`cases/<名前>.json` の例:

```json
{
  "dsl": { "...": "examples/kakeibo.json と同じ形" },
  "records": { "expense": [ { "id": "r1", "data": { "amount": 1200, "date": "2026-11-01" }, "createdAt": "2026-11-01T09:00:00+09:00" } ] },
  "device": { "now": "2026-11-02T08:00:00+09:00", "timeZone": "Asia/Tokyo", "locale": "ja_JP", "capabilities": { "components": ["..."], "actions": ["..."], "features": ["recordRetention"] } },
  "steps": [ { "tap": "add_button" } ]
}
```

`expected/<名前>.tree.json` の例:

```json
{
  "screen": "home",
  "nodes": [
    { "type": "stat", "id": "total", "cell": { "col": 0, "row": 0, "colSpan": 6, "rowSpan": 2 }, "value": "¥1,200", "state": "rendered" },
    { "type": "list", "id": "recent", "cell": { "col": 0, "row": 2, "colSpan": 6, "rowSpan": 8 }, "rows": [ { "title": "ランチ", "trailing": "¥1,200", "recordId": "r1" } ], "state": "rendered" },
    { "type": "routeGuide", "id": "go", "cell": { "col": 0, "row": 10, "colSpan": 6, "rowSpan": 2 }, "state": "unavailable" }
  ],
  "afterSteps": [ { "navigatedTo": "expense_form", "presentation": "sheet", "recordId": null } ]
}
```

### 規則

- 木に入れるのは §4 の「同じでなければならない」ものだけ: type・id・解決したセル(`area` と積み方の結果)・表示する値(書式後の文字)・一覧の行・`state`(`rendered` / `substituted:<type>` / `unavailable` / `unknown`)・操作の結果(遷移先・presentation・recordId・保存されたレコード)。フォント・ピクセル・余白は入れない。
- 時刻・タイムゾーン・ロケール・乱数の種は `device` で固定する。
- 各 OS は「DSL → 描画の木」を、**画面を描かずに**出す関数を持つ(iOS は `MetamonDSL` の純関数に寄せる。`GridTableLayout` `TableGrouping` `RandomPick` `Expr` の評価はもう純関数にある)。
- 期待値の更新は iOS だけで行い(`UPDATE_GOLDEN=1` のような明示の操作)、差分を人がレビューする。**Android は期待値を書き換えず、合わせる**。
- 例の集め方: `docs/dsl/examples` の全部 + 部品ギャラリー(`gallery-*.json`)+ 境界値(空・上限・`area` 無しの混在・Container 内のローカルグリッド)+ 対応外(`capabilities` を減らした端末で `unavailable` / `substituted` になること)。
- 部品の網羅は機械で確かめる: 全 cases に出てくる type / action の集合 = スキーマの集合(足りなければテストが赤)。

## 決めてほしいこと

| # | 決めること | 推奨 |
|---|---|---|
| 1 | 対応表の区分(A 56 / B 9 / C 1・アクション A 10 / B 2)で進めてよいか。特に `nearbyShare`(B・同じ OS どうしだけ)と `musicPlayer`(C)・`dashcam`(B) | このまま。`nearbyShare` は Android の初版では「使えません」でもよい(B のまま後で作る) |
| 2 | 違ってよい / 同じでなければならないの線引き(§4) | 表のとおり |
| 3 | 通知の時刻のずれをどこまで許すか(Android 14 の正確なアラームの制限) | 数分以内のずれは許す。正確な時刻が要る用途(服薬など)は別途 |
| 4 | `dashcam` を Android で裏に回っても撮るか(iOS は前面だけ) | **iOS に合わせて前面だけ**(動きを揃える) |
| 5 | 対応外の扱い: Y + Z(サーバーで置き換え + クライアントの枠) | Y + Z。iOS のリリース版の `EmptyView` も枠に変える(別 Issue) |
| 6 | 枠の文言を 2 種類に分けるか(「この端末では使えません」/「アプリを更新すると表示できます」) | 分ける |
| 7 | 最低対応版を OS ごとに持つ(`MIN_<OS>_BUILD`) | 持つ |
| 8 | `supportedActions` を足す・部品の中の機能を `supportedFeatures` に `<type>.<key>` で入れる | 両方する |
| 9 | 申告の無い iOS 以外のクライアントは A だけに絞る | 絞る |
| 10 | 共有アプリの部品の制限 | A 優先・B は代わりがある時だけ・C は使わない |
| 11 | 型の生成: 自前の生成器(Go)・Kotlin は生成だけ・Swift は enum とキー表から | このとおり。Swift の struct を全部生成に置き換えるかは Android の初版の後に決める |
| 12 | アイコンの語彙: SF Symbols の名前のまま各 OS で写すか、閉じた enum に変えるか | 当面は SF Symbols の名前 + 対応表(enum 化は非互換なので `schemaVersion` の判断と合わせる) |
| 13 | `openUrlButton` の `maps:` を Android でどう写すか | Google マップの URL(`https://www.google.com/maps/search/?api=1&query=`)に写す |
| 14 | 決まった後、README に移す範囲 | 対応表(Web 列を含む)・§4 の表・§5 の規則・§6 の申告を README §3 / §5 に移す。Issue の数(51 / 9)とロードマップ #357 の「Leaf 44 種」を直す |

## 参考

- リポジトリ内: [`docs/dsl/README.md`](../dsl/README.md)(§1 寛容さ・§3.8 Web 列・§4 部品の足し方・§5 版)、[`docs/dsl/dsl.schema.json`](../dsl/dsl.schema.json)、[`docs/dsl/expressions.md`](../dsl/expressions.md)、[`backend/internal/ai/restrict.go`](../../backend/internal/ai/restrict.go)、[`docs/backend/openapi-ai.yaml`](../backend/openapi-ai.yaml)(`SupportedFeatures`)、[`docs/backend/openapi-appconfig.yaml`](../backend/openapi-appconfig.yaml)(`/client-config`)、[`ClientCapabilities.swift`](../../ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift)、[`ComponentRegistry.swift`](../../ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Registry/ComponentRegistry.swift)、[`ComponentRegistryTests.swift`](../../ios/Packages/MetamonRenderer/Tests/MetamonRendererTests/ComponentRegistryTests.swift)、[`docs/roadmap/roadmap-m1-m6.md`](../roadmap/roadmap-m1-m6.md)(R-65〜R-68)
- 関連 Issue: [#302](https://github.com/jphacks/kz_2601/issues/302)・[#352](https://github.com/jphacks/kz_2601/issues/352)・[#353](https://github.com/jphacks/kz_2601/issues/353)・[#354](https://github.com/jphacks/kz_2601/issues/354)・[#362](https://github.com/jphacks/kz_2601/issues/362)・[#482](https://github.com/jphacks/kz_2601/issues/482)・[#507](https://github.com/jphacks/kz_2601/issues/507)
- 外部(Android の候補・決める前に #354 で確かめる): [Jetpack Compose](https://developer.android.com/compose)、[kotlinx.serialization](https://github.com/Kotlin/kotlinx.serialization)、[quicktype](https://github.com/glideapps/quicktype)、[Health Connect](https://developer.android.com/health-and-fitness/guides/health-connect)、[Nearby Connections](https://developers.google.com/nearby/connections/overview)、[ML Kit](https://developers.google.com/ml-kit)、[Gemini Nano(ML Kit GenAI)](https://developer.android.com/ai/gemini-nano)、[正確なアラームの制限(Android 14)](https://developer.android.com/about/versions/14/changes/schedule-exact-alarms)、[Live Updates(Android 16)](https://developer.android.com/develop/ui/views/notifications/live-update)
