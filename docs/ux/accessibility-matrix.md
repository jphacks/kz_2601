# アクセシビリティの OS ごとの対応表(文字の大きさ・動きを減らす・読み上げ・コントラスト)

iOS・Android・Mac・デスクトップ(Windows・Linux)が、OS の使いやすさの設定のどれに従っているかを 1 枚に並べた表です(#3059)。決めることはしません。iOS の画面ごとの決まりは別の文書(#2661・まだありません)で書く予定で、ここは OS を横に並べて穴を見つけるための表です。

- 内容は `origin/epic/god` のコード・文書と、Issue の状態から書きました。コードを実機で動かして確かめたものではありません。
- 各マスは「状態 + 見たところ + Issue 番号」です。状態の言葉は次の 5 つだけ使います。

| 状態 | 意味 |
| --- | --- |
| 確認中 | コードか文書はあるが、実機・支援ソフトでの確認が終わっていない |
| 済み | コードがあり、テストか文書で裏づけがある(実機での確認は別) |
| 一部 | 読む・従う所はあるが、一部の部品・条件だけ |
| 未 | コードが見当たらない。Issue があれば番号を書く |
| 未(Issue なし) | コードが見当たらず、Issue も見つからない。決めること |

## 目次

- [対応表](#対応表)
- [項目ごとの見たところ](#項目ごとの見たところ)
- [穴(Issue が無い・開いたまま)](#穴issue-が無い開いたまま)
- [決めること](#決めること)

## 対応表

行は五十音順です。

| 項目 | iOS | Android | Mac | デスクトップ(Windows・Linux) |
| --- | --- | --- | --- | --- |
| 動きを減らす | 済み(`accessibilityReduceMotion` を iOS・Mac 共有を含む 21 ファイルが読む)。止まらない動きの残りは #3071 #3072 #3076 #3084 | 一部(アニメーションの倍率が 0 の時だけ止める・一部の部品)。#915 | 済み(carousel・disclosure・一言の出入り・#937) | 未。#2998 #2999 |
| コントラスト | 一部(色は固定の AA 配色。OS の「コントラストを上げる」は読まない)。未(Issue なし) | 未(Issue なし) | 済み(「コントラストを上げる」「透明度を下げる」・#1206) | 未。#2207 |
| 文字の大きさ | 済み(Dynamic Type・生成アプリの部品)。固定の大きさが残る所がある。#2724 | 確認中(`sp` で `fontScale` に追従。2.0 倍で崩れないかは未確認・#915) | 済み(⌘+ ⌘- ⌘0・コードあり。#2854 は開いたまま) | 未。#3000(Windows の「文字の大きさ」)。Ctrl + / Ctrl - は Issue なし |
| 読み上げ | 確認中(VoiceOver・部品ごとの名前と値。見出しの印・星・一覧の操作は未。#3073 #3061 #1845) | 確認中(TalkBack・`contentDescription`・お知らせの読み上げ。Accessibility Scanner は未・#915) | 済み(VoiceOver・テストあり。Accessibility Inspector の結果は未・#2855) | 確認中(Windows の Narrator・Java Access Bridge。実機の確認 #2068 #2321 #2209)。Linux の Orca は対象外(#2066 で閉じた・#2322 は調査メモ) |

## 項目ごとの見たところ

### 動きを減らす

| OS | 何を読むか | 場所 | 見たところ |
| --- | --- | --- | --- |
| iOS | `accessibilityReduceMotion`(「視差効果を減らす」) | `ios/Packages/MetamonRenderer/Sources/MetamonRenderer/`(`GridScreenLayout`・`ButtonLeaf`・`CalendarLeaf`・`ProgressBarLeaf` ほか)、`ios/Metamon/App/` | [`docs/ios/architecture.md`](../ios/architecture.md) の「Reduce Motion」の行。止まらない動きの残りは #3071(マイクの光)・#3072(読み込みの棒)・#3076(文言のフェード)・#3084(一覧を出す道具) |
| Android | `Settings.Global.ANIMATOR_DURATION_SCALE` が 0 の時(開発者向けの「アニメーションなし」と同じ値) | `android/renderer/.../theme/Adaptive.kt`(`rememberSystemReduceMotion`)、`LocalReduceMotion` を読む `ProgressGaugeLeaves.kt`・`ListLeaf.kt`・`RecordsPlaceholder.kt` | 倍率が 0 より大きい(0.5 倍など)時は止めない。ほかの部品や生成中の演出が読んでいるかは未確認(推測: 一部だけ)。#915 |
| Mac | `accessibilityReduceMotion` | `ios/MetamonMac/App/Shell/MacRootView.swift`・`AppCanvas.swift`、Renderer は iOS と同じ | [`docs/ios/architecture.md`](../ios/architecture.md) の「Mac(#937 / #940)」の行 |
| デスクトップ | 読んでいない | (該当なし) | 読む口は #2998(Windows のアニメーション効果・GNOME の `enable-animations`)、止める所は #2999(スケルトンの明滅・画面の移り) |

### コントラスト

| OS | 何を読むか | 場所 | 見たところ |
| --- | --- | --- | --- |
| iOS | 読まない。生成アプリの色は固定の AA 配色(4.5:1 以上をテストで検算) | `ios/Packages/MetamonRenderer/.../Support/SemanticColor.swift` | OS の「コントラストを上げる」(`colorSchemeContrast`)は iOS では見当たらない。primary button の白文字 × AccentColor は 3.85:1 と文書にある([`docs/ios/architecture.md`](../ios/architecture.md)) |
| Android | 読まない | (該当なし) | 色の作りは `android/renderer/.../theme/RenderTheme.kt`。OS の高コントラストの設定を読む所は見当たらない |
| Mac | `colorSchemeContrast`・`accessibilityReduceTransparency` | `ios/MetamonMac/App/Theme/MacAccessibility.swift` | 枠を濃く太く・補足の文字を本文の色に・材質を不透明に。テスト `MacAccessibilityStyleTests` |
| デスクトップ | 読まない(明暗だけ OS に従う・`theme/Theme.kt`) | (該当なし) | #2207(Windows のコントラストのテーマ・GNOME の `high-contrast`) |

### 文字の大きさ

| OS | 何に従うか | 場所 | 見たところ |
| --- | --- | --- | --- |
| iOS | Dynamic Type | Renderer の各部品(`dynamicTypeSize`・`@ScaledMetric`・`FadingScrollView`) | アクセシビリティサイズでは部品の中でスクロールに逃がす。iOS のコード全体(本体・パッケージ・共有)に固定の大きさ(`.system(size:)`)が 55 か所ある(推測: 見出しなど一部は意図した固定)。確認の入口は #2724(リリース申請のシートの固定 22pt) |
| Android | 端末の「フォントサイズ」(`fontScale`) | `android/renderer/.../theme/RenderTheme.kt`(文字は `sp`) | 追従はコードのコメントにある。2.0 倍まで崩れないかの確認は #915 の項目のまま |
| Mac | ⌘+ ⌘- ⌘0 でアプリ内の `dynamicTypeSize` を上げ下げ(Mac には Dynamic Type が無いため) | `ios/MetamonMac/App/Shell/MacCommands.swift`・`AppCanvas.swift`(`MacTextZoom`) | 中央のアプリだけに効く。サイドバーなどの殻は対象外(推測)。#2854 は開いたままで、実装は `450a5b1cb`(#1206)に入っている |
| デスクトップ | 従わない | (該当なし) | #3000(Windows の `TextScaleFactor` を読む)。Ctrl + / Ctrl - の拡大縮小は Issue が見つからない |

### 読み上げ

| OS | 読み手 | 場所 | 見たところ |
| --- | --- | --- | --- |
| iOS | VoiceOver | `ios/Metamon/Features/Accessibility/VoiceOverAnnouncer.swift`、Renderer の各部品 | 部品 1 つ = 1 要素・お知らせの読み上げ・削除の取り消しの時間延長(`docs/ios/architecture.md`)。残りは #3073(見出しの印)・#3061(星)・#1845(一覧のカスタムアクション)・#2726(飾りの絵)。親は #1789 |
| Android | TalkBack | `android/app/.../feedback/TalkBackAnnouncements.kt`・`android/renderer/.../a11y/TalkBack.kt`、`contentDescription` 153 か所 | お知らせの読み上げは #1262 で閉じた。#915 は開いたまま(Accessibility Scanner での点検・タップの大きさ 48dp など) |
| Mac | VoiceOver | `ios/MetamonMac/App/`(`MacVoiceOver.swift`)、`ios/MetamonMacTests/MacVoiceOverTests.swift` | #937 で閉じた。Accessibility Inspector の点検の結果を文書に残す作業は #2855 |
| デスクトップ | Windows の Narrator(Java Access Bridge 経由・`jabswitch -enable` が要る) | `desktop/app/.../desktop/A11y.kt`、`desktop/app/build.gradle.kts`(`jdk.accessibility`) | 実機の確認 #2068・手順の文書 #2321・Java Access Bridge が無効な時の案内 #2209。親は #1759。Linux の Orca は対象外(`desktop/README.md`)で、調査メモは #2322 |

## 穴(Issue が無い・開いたまま)

- **Issue が見つからない**(推測を含む): iOS の「コントラストを上げる」・Android のコントラスト(高コントラストの文字・色)・デスクトップの Ctrl + / Ctrl - の拡大縮小。
- **開いたまま**: #915(Android の確認項目が残る)・#2854(Mac の文字の大きさ。実装はあるので閉じてよいかは人が見る)・#1206(Mac。残りは点検の結果の文書だけで #2855)。
- **文書の食い違い**: Linux の Orca は `desktop/README.md` で「対象外」、#2322 は調査メモ。方針が変わったのかは文書から読めない。

## 決めること

- iOS・Android に「コントラストを上げる」への追従を入れるか(今の固定の AA 配色で足りるとするか)。入れる場合は Issue を立てる。
- デスクトップに Ctrl + / Ctrl - の拡大縮小を入れるか(#3000 は両者を掛け合わせる前提で書かれている)。
- 表の「確認中」を「済み」に変える条件(実機・支援ソフトで誰が確かめたら良いとするか)。

関連: [`docs/ios/architecture.md`](../ios/architecture.md)・[`docs/mac/interactions.md`](../mac/interactions.md)・[`docs/desktop/architecture.md`](../desktop/architecture.md)・[`desktop/README.md`](../../desktop/README.md)・[`android/README.md`](../../android/README.md)
