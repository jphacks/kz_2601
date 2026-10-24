# 生成アプリのデザインシステム(Renderer 共通 UI)

> 対象: `ios/Packages/MetamonRenderer`(DSL から生成されるアプリ側の全部品)。本体(音声画面・生成中画面)の `ios/Metamon/App/Theme.swift` と**同じ値**を `Support/RenderTheme.swift` に持つ(依存が 本体→Renderer なので import できず二重定義。片方を変えたらもう片方も揃える)。
> 目的: 「どんなアプリが欲しい?」画面・「作成中」画面と、生成後のアプリ(家計簿・筋トレ・散歩…)が**同じブランドの作り込み**に見えること。アプリごとに変えない(`app.theme.primaryColor` は使わず、常に Tailor It ブランド)。

## 1. 原則
1. **1 ブランド**: 紫→水色のグラデーションが「押せる・選ばれている・いま注目」の 3 つだけを指す。装飾には使わない。
2. **面で読ませる**: canvas(画面)> surface(カード)> elevated(入力の器)の 3 段。影は薄く、枠は hairline。
3. **数字は主役**: 金額・回数は rounded + monospacedDigit で大きく。ラベルは小さく secondary。
4. **触った手応え**: 押下 0.97 のスプリング、出現はスタガー、保存はチェックの一拍。Reduce Motion では止める。
5. **崩れない**: すべて Dynamic Type の text style ベース(固定 pt は使わない)。accessibility-large でも 1 画面に収まる。

## 2. トークン(`RenderTheme`)

### 2.1 色
| トークン | 値 | 使い所 |
|---|---|---|
| `Colors.brandA` | `#8E6CFF`(紫) | tint / フォーカスリング / 選択中 / stat のアイコン |
| `Colors.brandB` | `#4CC9FF`(水色) | グラデーションの終点 / チャートの 2 色目 |
| `brandGradient` | brandA → brandB(topLeading→bottomTrailing) | primary ボタン / segmented の選択チップ / 塗り星 / ゲージ |
| `Colors.brandSoft` | brandA 12% | secondary ボタン面 / 選択行の背景 / list のアイコン背景 |
| `Colors.canvas` | `systemGroupedBackground` | 画面背景 |
| `Colors.surface` | `secondarySystemGroupedBackground` | カード |
| `Colors.surfaceElevated` | `tertiarySystemGroupedBackground` | 入力の器(カードの中で一段沈む) |
| `Colors.hairline` | primary 8% | 枠線・区切り |
| `Colors.positive / negative / warning` | green / red / orange | semantic(達成・削除・警告)。ブランドと混ぜない |
| `Colors.chartPalette` | brandA, brandB, mint, indigo, orange, pink, teal, purple | chart / カテゴリ色ドット(文字列ハッシュで安定割当) |

本体 `Theme.Colors.brandA/brandB/canvas/surface/surfaceElevated/hairline` と同値。

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
| ボタン | 14 | primary: brandGradient / secondary: brandSoft / destructive: red 12% | primary のみ brandA 28% ・radius 12・y 6 |
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
| picker segmented | チップ型。選択中は brandGradient + 白文字、未選択は surfaceElevated。menu は入力と同じ器 + chevron |
| slider / rating | トラックとつまみ brandA、値は monospaced。星は brandGradient で塗る(未選択は tertiary の輪郭) |
| button | 高さ 52・角丸 14・押下 0.97。primary = brandGradient + 白 + 影、secondary = brandSoft + brandA 文字、destructive = red 12% + red 文字、plain = brandA 文字 |
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
