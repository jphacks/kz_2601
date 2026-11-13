# iOS の文言と多言語(ja / en)

iOS・Mac・パッケージの文言の足し方、String Catalog(`.xcstrings`)の書式とマージ、英語のテストの回し方。仕組みの全体は [architecture.md の「16. 多言語対応」](architecture.md#16-多言語対応ja--enissue-44--45-の-ios-側)。

## 目次

1. [文言を引く関数](#文言を引く関数)
2. [String Catalog の置き場所](#string-catalog-の置き場所)
3. [文言を足す手順](#文言を足す手順)
4. [審査用の版の言い換え(review:)](#審査用の版の言い換えreview)
5. [カタログの書式](#カタログの書式)
6. [merge driver の入れ方](#merge-driver-の入れ方)
7. [英語のテスト](#英語のテスト)
8. [決めること](#決めること)

## 文言を引く関数

キーはどれも**日本語のソース文字列**(`sourceLanguage` は `ja`)。訳が無いと日本語のキーのまま出る。

| 関数 | 使う所 | 定義 |
|---|---|---|
| `L("…")`(DSL) | `MetamonDSL` の中の固定文言 | `ios/Packages/MetamonDSL/Sources/MetamonDSL/Support/PackageLocalization.swift` |
| `L("…")`(Renderer) | `MetamonRenderer` の中の固定文言(リテラルの `Text` も含めて全部。リテラルだと `Bundle.main` を引いてしまう) | `ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/LanguagePreference.swift` |
| `L("…")`(Services) | `MetamonServices` の中の固定文言 | `ios/Packages/MetamonServices/Sources/MetamonServices/Support/Localization.swift` |
| `L10n.key(_:review:)` | アプリの `Text` / `Label` で、審査用の版だけ文言を変える所 | `ios/Shared/Runtime/L10n.swift` |
| `L10n.pick(_:review:)` | アプリの `String` で、審査用の版だけ文言を変える所 | `ios/Shared/Runtime/L10n.swift` |
| `L10n.tr("…")` | アプリ(iOS・Mac)で `String` を返す文言(三項演算子・変数・`String` 返し) | `ios/Shared/Runtime/L10n.swift` |
| `LocalizedStringResource` | ウィジェットのコントロール・App Intents のタイトル | `ios/MetamonWidget/Controls/`・`ios/Metamon/Features/Intents/` |
| リテラル(`Text("…")` など) | アプリの SwiftUI に直接渡す文言。ルートの `\.locale` で引き直される | — |

- 言語の設定は UserDefaults の `metamon.appLanguage`(`system` / `ja` / `en`)。DSL・Services は Renderer に依存できないので、`PackageLocalization` が同じキーを読む。
- `static let` の文言は `static var` にする(起動時の言語で固定されないように)。

## String Catalog の置き場所

| ファイル | 中身 |
|---|---|
| `ios/Metamon/Resources/AppShortcuts.xcstrings` | Siri・ショートカットのフレーズ(Mac も同じファイルを使う・`ios/project.yml`) |
| `ios/Metamon/Resources/InfoPlist.xcstrings` | iOS の Info.plist の文言(権限の説明など) |
| `ios/Metamon/Resources/Localizable.xcstrings` | iOS アプリの文言(`L10n` とリテラル) |
| `ios/MetamonMac/Resources/InfoPlist.xcstrings` | Mac の Info.plist の文言 |
| `ios/MetamonMac/Resources/Localizable.xcstrings` | Mac アプリの文言。`ios/Shared/` の `L10n` は Mac では `Bundle.main` = このカタログを引くので、Shared の文言はこちらにも要る |
| `ios/MetamonMac/Resources/ServicesMenu.xcstrings` | Mac のサービスメニューの文言 |
| `ios/MetamonWidget/Localizable.xcstrings` | ウィジェット(iOS と Mac の両方が使う) |
| `ios/Packages/MetamonDSL/Sources/MetamonDSL/Resources/Localizable.xcstrings` | DSL の固定文言 |
| `ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Resources/Localizable.xcstrings` | Renderer の固定文言 |
| `ios/Packages/MetamonServices/Sources/MetamonServices/Resources/Localizable.xcstrings` | Services の固定文言 |

## 文言を足す手順

1. コードに日本語で書く(パッケージは `L("…")`、アプリは上の表のどれか)。補間はそのまま `\(x)` でよい。
2. 該当のカタログに日本語のキーと `en` の訳を足す。キーの補間は `%@`(文字列)/ `%lld`(整数)。補間ありのキーでは `%` を `%%` にする。Xcode で足しても、JSON を手で足してもよい(今のキーはすべて `"extractionState" : "manual"`)。
3. `python3 ios/scripts/xcstrings-fmt.py <カタログ>` で整える([カタログの書式](#カタログの書式))。
4. 英語の文言のテスト(`EnglishCopyTests`)に代表の 1 文を足すとよい([英語のテスト](#英語のテスト))。
5. Shared の文言は iOS と Mac の両方のカタログに足す。

- 翻訳しないもの: DSL の中の文言(AI が locale に合わせて作る)・Services のログ・FieldIcon のキーワード・音声の意図判定の語彙(architecture.md の 16 と同じ)。

## 審査用の版の言い換え(review:)

審査用の版(`APP_STORE_REVIEW`・[review-build.md](review-build.md))では「アプリを作る / 生成 / 変身」を避け、「記録帳を用意する・直す」と言う(言葉の決まりは [docs/store/wording.md](../store/wording.md))。

```swift
// ios/Metamon/App/AIButton.swift
.accessibilityHint(L10n.key("長押しで変身一覧", review: "長押しで記録帳の一覧"))
// ios/Shared/Settings/SupportInfo.swift
Row(label: L10n.pick(L10n.tr("今のアプリの版"), review: L10n.tr("今の記録帳の版")), value: "v\(appRevision)")
```

- 通常の版は `normal` だけを評価する(`#if APP_STORE_REVIEW` で切り替える)。
- `normal` と `review` の両方のキーをカタログに足す。
- パッケージ(`L("…")`)には `review:` が無い。パッケージの中で言い換えが要る時は、アプリ側から文言を渡す(決めること)。

## カタログの書式

`ios/scripts/xcstrings-fmt.py` は Xcode の書き出しと同じ書式にする(キーはコードポイント順・インデント 2・`"key" : value`・末尾改行なし)。Xcode で保存しても差分が出ないので、キーはどこに足してもよい。

| コマンド | すること |
|---|---|
| `git ls-files -z '*.xcstrings' \| xargs -0 python3 ios/scripts/xcstrings-fmt.py --check` | 全部を検査(CI の `ios-ci` ジョブの「xcstrings の整形チェック」と同じ・ブロッキング) |
| `python3 ios/scripts/xcstrings-fmt.py --check <files…>` | 書き換えずに検査。整っていないファイルを出して exit 1 |
| `python3 ios/scripts/xcstrings-fmt.py <files…>` | 整える(書き換える) |

- 中身が同じ重複キーは 1 つにまとめる。中身が違う重複キーはエラーで止まる。

## merge driver の入れ方

clone ごとに 1 回、repo のルートで実行する。

```sh
scripts/setup-git.sh
```

| 設定 | 値 |
|---|---|
| `.gitattributes` | `*.xcstrings merge=xcstrings` |
| `merge.xcstrings.driver` | `python3 ios/scripts/merge-xcstrings.py %O %A %B` |
| `rerere.enabled` | `false`(解き方のキャッシュが worktree 間で共有され、別のブランチの解き方が当たったことがあるため) |

- キー単位の 3-way マージ。片側だけが変えたキーはその変更を採る。別々のキーの追加はコンフリクトしない。
- 両側が同じキーを違う中身に変えた時だけ止まる(exit 1)。自分側が残るので、手で直して `xcstrings-fmt.py` で整え直す。
- 入れていないと、普通のテキストのマージになり、離れたキーの追加でもコンフリクトしやすい。

## 英語のテスト

`swift test` は String Catalog を訳さないので、英語の文言のテスト(`EnglishCopyTests`)だけ落ちる。パッケージは `xcodebuild test` で回す(`ios/scripts/mac-package-test.sh` がその理由で作られている)。

| テスト | 見るもの | 回し方 |
|---|---|---|
| `IntentEnglishCopyTests`(`ios/MetamonTests/`) | Siri・ショートカットの文言の英訳 | アプリのテスト(`xcodebuild -scheme Metamon … test`) |
| `MetamonDSLTests/EnglishCopyTests` | DSL の固定文言が英語で出る | `ios/scripts/mac-package-test.sh MetamonDSL` |
| `MetamonRendererTests/EnglishCopyTests` | Renderer の固定文言が英語で出る | `ios/scripts/mac-package-test.sh MetamonRenderer` |
| `MetamonServicesTests/EnglishCopyTests` | Services の固定文言が英語で出る・DSL と Services の `L("…")` のキーが全部カタログにあり英訳が付く(`testUsedKeysExistInCatalog`) | `ios/scripts/mac-package-test.sh MetamonServices` |
| `RuntimeCopyIosEnglishTest`(Android) | Android の英語が iOS の `Localizable.xcstrings` の訳と同じ | `android-ci`(iOS のカタログを変えると回る) |

- DSL・Services のテストの中で言語を切り替えるには `setAppLanguage("en")`(テストの補助関数。UserDefaults の `metamon.appLanguage` を書き換える)。Renderer のテストは `LanguagePreference` の設定を書き換える。
- 手元の注意: repo か DerivedData が `~/Desktop`・`~/Documents`・`~/Downloads` の下にあると、テストの実行役が読めずに落ちる(macOS の保護)。`MAC_DERIVED_DATA` などを保護の外に置く(`ios/scripts/mac-package-test.sh` の先頭のコメント)。
- 画面で確かめる: 起動引数 `-metamon.appLanguage en`、または `-AppleLanguages "(en)"`。

## 決めること

- Renderer の `L("…")` のキーがカタログにあるかの検査(`testUsedKeysExistInCatalog` は DSL と Services だけを見る)を Renderer にも広げるか。
- アプリ(`L10n.tr` / リテラル)のキーの抜けを検査するテストを足すか(今は Intent の文言だけ)。
- パッケージの中で審査用の版の言い換えが要る時のやり方(アプリから渡す / パッケージに `review:` を足す)。
