# 生成アプリのデザインシステム(Renderer 共通 UI)

> 対象: `ios/Packages/MetamonRenderer`(DSL から生成されるアプリ側の全部品)。**色の正典は `Support/SemanticColor.swift` の `SemanticPalette`**(ライト/ダーク対)で、`RenderTheme.Colors` がそれを公開し、本体(音声画面・生成中画面)の `ios/Metamon/App/Theme.swift` の色は `RenderTheme.Colors` を参照する(#102 で色の二重定義を解消)。余白・角丸は従来どおり本体と二重定義なので片方を変えたらもう片方も揃える。ウィジェット(`MetamonWidget`)だけは Renderer を import できないので brandA/brandB を複製している。
> 目的: 「どんなアプリが欲しい?」画面・「作成中」画面と、生成後のアプリ(家計簿・筋トレ・散歩…)が**同じブランドの作り込み**に見えること。アプリごとに変えない(`app.theme.primaryColor` は使わず、常に Tailor It ブランド)。

## 1. 原則
1. **1 ブランド**: 藍→青緑のグラデーション(アプリアイコンの紺 × ツルの生成りから取った「藍 × 生成り」・#102)が「押せる・選ばれている・いま注目」の 3 つだけを指す。装飾には使わない。
2. **面で読ませる**: canvas(画面)> surface(カード)> elevated(入力の器)の 3 段。影は薄く、枠は hairline。
3. **数字は主役**: 金額・回数は rounded + monospacedDigit で大きく。ラベルは小さく secondary。
4. **触った手応え**: 押下 0.97 のスプリング、出現はスタガー、保存はチェックの一拍。Reduce Motion では止める。
5. **崩れない**: すべて Dynamic Type の text style ベース(固定 pt は使わない)。accessibility-large でも 1 画面に収まる。

## 2. トークン(`RenderTheme`)

### 2.1 色
アイコン(折り鶴)から抽出した色が出発点: 背景の紺 `#335267`・ツルの生成り `#F4F1E8`・メジャーの金 `#F0D060`・とさかの朱 `#AE4639`。紫は撤去した(#102)。
値は「ライト / ダーク」。ダークで暗い面に沈まないよう、操作色はダークで一段明るくする。

| トークン | 値(ライト / ダーク) | 使い所 |
|---|---|---|
| `Colors.brandA` | `#2E6A93` / `#4E8DB8`(藍) | tint / フォーカスリング / 選択中 / stat のアイコン。面に対し非文字 3:1 以上、かつ白いアイコンを載せて 3:1 以上(`BrandContrastTests`) |
| `Colors.brandB` | `#3E9A9C` / `#3B9E9F`(青緑) | グラデーションの終点 / チャートの 2 色目。白は 3.2〜3.3:1 なのでアイコン・大きい文字まで(本文の白文字は brandFill に載せる) |
| `brandGradient` | brandA → brandB(topLeading→bottomTrailing) | 塗り星 / ゲージ / chart の bar・line / 本体のマイク・送信・ブランドマーク(白アイコン 3:1 以上) |
| `Colors.brandSoft` | brandA 12% | secondary ボタン面 / 選択行の背景 / list のアイコン背景 |
| `Colors.brandText` | `#1F5A82` / `#8CC3E6` | ブランド色の**文字**(リンク・secondary/plain ボタン・stepper/slider の値・フォーカス中ラベル)。面に対し AA(4.5:1)。`SemanticPalette.brand` として #68 の `SemanticColorContrastTests` が検算 |
| `brandFill` | `#245E86` → `#286F7E`(両モード共通) | **白文字を載せる塗り**(primary ボタン・segmented 選択チップ・calendar 選択日・timer の数字)。両端とも白文字 4.5:1 以上(`BrandContrastTests`) |
| `Colors.canvas` | `#F6F3EC`(生成り) / `#0F1A22`(紺) | 画面背景 |
| `Colors.surface` | `#FFFDF8` / `#16232D` | カード。contrast テストの基準面 |
| `Colors.surfaceElevated` | `#F9F6EF` / `#1E2E3A` | 入力の器(ライトはカードより一段沈み、ダークは一段浮く) |
| `Colors.hairline` | 紺 `#335267` 12% / 生成り 10% | 枠線・区切り |
| `Colors.positive / negative / warning` | green / red / orange(AA トーン) | semantic(達成・削除・警告)。ブランドと混ぜない |
| `Colors.chartPalette` | 藍, 青緑, 金 `#C99A22`, 朱 `#B5523F`, 藍鼠 `#6C8BA3`, 灰緑 `#6F966E`, 砂色 `#A88B5E`, 紺桔梗 `#54629E`(ダークは各一段明るく) | chart / カテゴリ色ドット(文字列ハッシュで安定割当)。ブランド以外はアイコンにある色と同系の落ち着いた色 |

本体 `Theme.Colors.brandA/brandB/brandText/canvas/surface/surfaceElevated/hairline` は上を参照(同じもの)。録音中の `recordGradient`(赤〜珊瑚)と `warning`(orange)は本体だけの状態色で、ブランドには含めない。

### 2.2 タイポグラフィ(すべて text style 相対・`design: .rounded` は数字と見出しだけ)
| 役割 | Font | 用途 |
|---|---|---|
| Display | `.system(.title, design: .rounded).bold().monospacedDigit()` | stat の値 |
| Number | `.system(.title2, design: .rounded).semibold().monospacedDigit()` | numberField の入力値 |
| Title | `.headline` | card / disclosure の見出し |
| Section | `.footnote.semibold` + kerning 0.3 | section 見出し(secondary) |
| Body | `.body` | 入力値・text |
| Row title | `.body.semibold` / Row subtitle `.caption` secondary | list |
| Row trailing | `.body.semibold.monospacedDigit()` | list の金額 |
| Caption | `.caption.medium` | 入力ラベル・凡例・軸 |
| Button | `.body.semibold` | ボタン |

### 2.3 面・角丸・影
| 要素 | 角丸 | 面 | 影 |
|---|---|---|---|
| 入力(control) | 12 | surfaceElevated + hairline | なし(フォーカス時 brandA 1.5pt リング) |
| ボタン | 14 | primary: brandFill / secondary: brandSoft / destructive: red 12% | primary のみ brandA 28% ・radius 12・y 6 |
| カード | 16 | surface + hairline | black 6% ・radius 10・y 3 |
| セル・地図・シート | 24(`Radius.sheet`)| — | — |
| サムネ | 10 | — | — |

本体は card=22(大きな面)、Renderer はセルが密集するので 16。shadow も本体(10%/18/8)より控えめ。

### 2.4 余白
`xs 4 / sm 8 / md 16 / lg 24 / xl 32`。カード内 padding 16、フォームの入力間 gap 12、list 行の内側 12、グリッドのセル間 10、画面端 14。

### 2.5 モーション
| 名前 | 値 | 使い所 |
|---|---|---|
| `Motion.press` | `.spring(response: 0.28, dampingFraction: 0.7)` scale 0.97 | すべてのボタン・チップ・星 |
| `Motion.appear` | easeOut 0.32・stagger 0.03/セル・scale 0.98→1 | 変身直後のセル出現 |
| `Motion.select` | `.snappy(duration: 0.25)` | segmented の選択移動・toggle |
| 保存成功 | ボタン内のラベルが checkmark に 0.6 秒置き換わる → list の新規行が brandSoft で 1.2 秒ハイライト | フォーム保存 |

Reduce Motion ではスタガーとハイライトを省略(即時表示)。

### 2.6 アイコン(SF Symbols)
- 重みは**文字と同じ weight**(`.font(.body.weight(.semibold))` の中に置く)。単独の装飾アイコンは `.medium`。
- 入力の左アイコンは field 型から推定(`FieldIcon`): currency → `yensign` / date → `calendar` / datetime → `calendar.badge.clock` / integer,number → `number` / enum → `list.bullet` / photo → `camera` / location → `mappin.and.ellipse` / boolean → `checkmark.circle` / string は id・ラベルで memo,note,comment → `note.text`、pin,password → `lock`、mail → `envelope`、url → `link`、それ以外 → `pencil.line`。component に `icon` があればそれを優先。
- stat のアイコンは aggregate から: count → `number` / sum → field 型のアイコン / avg → `chart.bar` / max → `arrow.up.right` / min → `arrow.down.right`。

## 3. 部品ごとの仕様(第 2 弾 #89)
| 部品 | 見た目 |
|---|---|
| textField / secureField / textEditor | 上ラベル(caption secondary)+ 器の左にアイコン(secondary、フォーカス中は brandA)+ フォーカス時 brandA リング。placeholder は tertiary |
| numberField | 同上。値は Number フォント、currency なら `¥` プレフィックス(secondary)、unit は末尾 |
| datePicker / timePicker / colorPicker / toggle / stepper | 同じ器の中に「アイコン + ラベル … コントロール」の 1 行。toggle / stepper / picker のつまみは brandA |
| picker segmented | チップ型。選択中は brandFill + 白文字、未選択は surfaceElevated。menu は入力と同じ器 + chevron |
| slider / rating | トラックとつまみ brandA、値は monospaced。星は brandGradient で塗る(未選択は tertiary の輪郭) |
| button | 高さ 52・角丸 14・押下 0.97。primary = brandFill + 白 + 影、secondary = brandSoft + brandText 文字、destructive = red 10% + red(AA) 文字、plain = brandText 文字。高さは @ScaledMetric、新規フォームでは deleteRecord を出さない(#73) |
| list | 1 枚のカード。先頭に画像サムネ(角丸 10)/ アイコン(brandSoft の角丸)/ カテゴリ色ドット(subtitle の文字列から chartPalette を安定割当)。title semibold、subtitle secondary、trailing の金額は monospaced semibold。区切りは先頭要素ぶん inset した hairline。空状態は symbol + 1 行 + brandA の薄い CTA |
| stat | 左上に 20pt の brandSoft 角丸アイコン + ラベル、値は Display。カードの左上に brandA 10% → 透明のグラデーション |
| section / card / disclosure | section 見出しは footnote semibold secondary + kerning。card/disclosure の見出しは headline。カード内 padding 16、区切り hairline |
| chart | bar/line は brandGradient、pie は chartPalette。凡例・軸は caption |
| gauge / progressBar | brandGradient の塗り。100% 以上(予算超過など)は warning に切り替える |
| calendar / timeline / table / badge | 選択日・ドット・レールは brandA。table 見出しは caption semibold secondary。badge の `accent` は brandA |
| タブバー | tint = brandA(`RootView` の `.tint`) |
| フォーム画面 | 入力はカード内に縦積み(gap 12)。toolbar の `saveRecord` は画面下の固定バー(primary ボタン・`safeAreaInset`・`FormSaveBar`)に出し、ナビバーからは外す。キーボードが出てもバーはその上に逃げ、入力は `.scrollDismissesKeyboard(.interactively)` で隠れない。保存中はスピナー→完了でチェックを 0.6 秒 |

## 4. 守ること
- 挙動(`field` 束縛・action・area 計算・schema)は見た目の変更で触らない。
- 新しい色・サイズは `RenderTheme` に足してから使う(leaf に生の数値や `Color.blue` を書かない)。
- a11y(#68)の `accessibilityLabel` 等の行は見た目の変更で壊さない。
- 本体 `Theme` を変えたら `RenderTheme` の対応表(ファイル冒頭)も更新する。
