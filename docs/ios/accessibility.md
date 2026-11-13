# iOS の読み上げと使いやすさ

VoiceOver・Dynamic Type・動きを減らす・アシスティブアクセスの決まりと、画面ごとの確認の表(#1789)。生成アプリの部品(Renderer)の最低線は [architecture.md の 13](architecture.md#13-アクセシビリティ最低限rendererissue-68)、色のトークンは [design-system.md](design-system.md)。

## 目次

1. [決まり](#決まり)
2. [固定の文字の大きさを使ってよい所](#固定の文字の大きさを使ってよい所)
3. [アシスティブアクセス](#アシスティブアクセス)
4. [確かめ方](#確かめ方)
5. [画面ごとの確認の表](#画面ごとの確認の表)
6. [決めること](#決めること)

## 決まり

| 項目 | 決まり | 例・置き場所 |
|---|---|---|
| 動きを減らす | `@Environment(\.accessibilityReduceMotion)` が真なら、飾りのアニメーション(出てくる動き・押した時の拡大・送りの動き)を止めて即時に反映する。意味のある変化は残す | `ButtonLeaf`・`CalendarLeaf`・`ProgressBarLeaf`・`BuildStageView`・`VoiceFab` |
| 飾り | 意味の無い絵・アイコン・区切り・レールは `.accessibilityHidden(true)` | list のアイコン・chevron・timeline のレール・空の時のアイコン |
| 絵だけのボタン | `accessibilityLabel` を必ず付ける(SF Symbols の名前を読ませない)。長押しなど隠れた操作は `accessibilityHint` / `accessibilityAction(named:)` | `AIButton`・`VoiceFab`(`L10n.key("長押しで変身一覧", review: …)`) |
| 画面に出た一言 | AI の返事・完成の要約・失敗の状態カード・トーストは出すと同時に読む(`VoiceOverAnnouncer.announce`)。VoiceOver の時は消えるまでを長くする(`holdSeconds`) | `ios/Metamon/Features/Accessibility/VoiceOverAnnouncer.swift` |
| 部品 1 つ = 1 要素 | 生成アプリの部品はまとめて意味を読む(stat は「ラベル, 値」・list の行は「title, subtitle, trailing」+ ボタン) | architecture.md の 13 |
| 文字の大きさ | 本文と見出しは文字のスタイル(`.body`・`.headline`・`.caption` など)で Dynamic Type に従う。固定の大きさ(`.font(.system(size:))`)は使わない。下の「使ってよい所」だけ例外 | [design-system.md](design-system.md) の「崩れない」 |
| 大きい文字で崩れない | アクセシビリティの大きさ(`dynamicTypeSize.isAccessibilitySize`)では、文字を切らずに中でスクロールさせるか折り返す。高さは `@ScaledMetric` で追従 | `GridScreenLayout`(`FadingScrollView`)・`ButtonLeaf` |
| タップだけの操作 | `onTapGesture` は VoiceOver から呼べないので、`isButton` の特性と `accessibilityAction` を付ける | `ListRow` |
| 文言 | `accessibilityLabel` などの文言も訳す(リテラルはそのまま、`String` は `L10n.tr` / `L("…")`) | `docs/ios/architecture.md` の 16 |
| UI テストの目印 | `accessibilityIdentifier` は見た目・読み上げを変えない。読み上げの文言で要素を探さない | architecture.md の 23 |

## 固定の文字の大きさを使ってよい所

| 所 | 使ってよいか | 理由 |
|---|---|---|
| 大きな数字(stat・健康の値など) | 推奨: 文字のスタイル + `minimumScaleFactor`。固定にする時は `relativeTo:` か `@ScaledMetric` で大きさに追従させる | 読ませる中身なので Dynamic Type に従う |
| 見出し | 使わない | 読ませる中身 |
| 絵の中の飾りの文字(作っている途中の絵の小さな文字など) | よい(`accessibilityHidden(true)` と一緒に) | 読ませない飾り。大きくすると絵が崩れる |
| 本文 | 使わない | 読ませる中身 |
| アイコン(SF Symbols)の大きさ | よい | 文字ではない。ボタンなら `accessibilityLabel` を付ける |

- 今のコードには固定の大きさが残っている(例: `ios/Metamon/Features/Switcher/AppSwitcherSheet.swift`・`ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Components/Leaves/HealthStatLeaf.swift`)。飾りか中身かを 1 つずつ見る(決めること)。

## アシスティブアクセス

`Info.plist` の `UISupportsFullScreenInAssistiveAccess` で全画面で出す。何を出す・出さないかは `AssistiveAccessPolicy`(`ios/Metamon/Features/Accessibility/AssistiveAccess.swift`)の 1 か所で決める。判定は `AssistiveAccessReader`(`MetamonApp`)だけ。

| 物 | アシスティブアクセスの中 |
|---|---|
| AI ボタン・マイクの FAB(`aiButton`) | 出さない |
| AI 送信の同意(`aiConsent`) | 出さない(「この画面では使えません」) |
| 起動時の同意・チュートリアル・β の案内(`launchPrompts`) | 出さない |
| 課金の画面(`paywall`) | 出さない |
| シェアリンク・「探す」(`shareLinks`) | 開かない(「この画面では使えません」) |
| 最初の画面 | 生成アプリの一覧(`AssistiveAccessAppList`・大きな行・1 行に 1 つ) |
| 開いた生成アプリ | 主な動作の大きなボタン(`AssistiveActions`・チェック → すぐ記録 → stat の順に 4 個まで。共有の entity は選ばない)。選べなければ今の画面 |
| 話しかける画面(`voiceSheet`) | 出さない |
| 変身一覧(`appSwitcher`) | 出さない |

- 新しく「作る・設定・共有・課金」の入口を足す時は、`AssistiveAccessPolicy.Feature` に足すか、既にある項目で隠れるかを確かめる。

## 確かめ方

| 物 | やり方 |
|---|---|
| VoiceOver | 実機で 設定 › アクセシビリティ › VoiceOver。シミュレータは Xcode の Accessibility Inspector |
| アシスティブアクセス | 実機で 設定 › アクセシビリティ › アシスティブアクセス に Tailor It を入れて始める |
| ダーク | `xcrun simctl ui <udid> appearance dark` |
| 動きを減らす | 設定 › アクセシビリティ › 動作 › 視差効果を減らす |
| 文字の大きさ | `xcrun simctl ui <udid> content_size accessibility-large`(最大は `accessibility-extra-extra-extra-large`) |

## 画面ごとの確認の表

空欄で始める。確かめた人が「済(日付・名前)」か「直す(Issue 番号)」を書く。

| 画面 | 場所 | VoiceOver | 文字の大きさ | 動きを減らす | アシスティブアクセス |
|---|---|---|---|---|---|
| アシスティブアクセスの一覧 | `ios/Metamon/Features/Accessibility/AssistiveAccessHome.swift` | | | | |
| アシスティブアクセスの大きなボタン | `ios/Metamon/Features/Accessibility/AssistiveActionsView.swift` | | | | |
| 起動の画面 | `ios/Metamon/App/SplashView.swift` | | | | |
| 課金 | `ios/Metamon/Features/Purchase/PaywallSheet.swift` | | | | |
| サインイン | `ios/Metamon/Features/Auth/SignInView.swift` | | | | |
| 作っている途中 | `ios/Metamon/Features/Voice/GeneratingView.swift`・`BuildStageView.swift` | | | | |
| 生成アプリの画面(部品) | `ios/Packages/MetamonRenderer/` | | | | |
| 設定 | `ios/Metamon/Features/Settings/SettingsSheet.swift` | | | | |
| チュートリアル | `ios/Metamon/Features/Onboarding/TutorialView.swift` | | | | |
| 話しかける画面・対話 | `ios/Metamon/Features/Voice/VoiceInputView.swift`・`ClarifyChatView.swift` | | | | |
| 変身一覧 | `ios/Metamon/Features/Switcher/AppSwitcherSheet.swift` | | | | |
| ホーム・ロック画面のウィジェット | `ios/MetamonWidget/Views/` | | | | |
| マイクの FAB | `ios/Metamon/App/VoiceFab.swift`・`AIButton.swift` | | | | |

## 決めること

- 固定の文字の大きさの残り(上の例など)を、飾りとして残すか文字のスタイルに直すか。1 つずつ Issue にするか。
- 固定の大きさを検査する仕組み(lint・テスト)を入れるか。今は無い。
- 確認の表を誰がいつ埋めるか(リリースの前ごと・大きな UI の変更ごと など)。
- 本体(`ios/Metamon/`)にも Renderer と同じ最低線(architecture.md の 13 は「本体は対象外」)を広げるか。
