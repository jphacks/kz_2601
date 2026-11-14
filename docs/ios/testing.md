# iOS・Mac のテスト

パッケージのテスト・アプリのテスト・UI テストの回し方と重さ、CI が回すもの、テストを足す時の置き場所。

## 目次

1. [テストの種類](#テストの種類)
2. [回し方](#回し方)
3. [CI が回すもの](#ci-が回すもの)
4. [手元で回さない物](#手元で回さない物)
5. [テストを足す時の置き場所](#テストを足す時の置き場所)
6. [決めること](#決めること)

## テストの種類

ターゲットの定義は `ios/project.yml`。重さは目安(UI テストだけ [architecture.md の 23](architecture.md#23-ui-テストxcuitestissue-165) の実測、ほかは推測)。

| ターゲット | 場所 | 中身 | 重さ |
|---|---|---|---|
| `MetamonDSLTests` | `ios/Packages/MetamonDSL/Tests/MetamonDSLTests/` | DSL のモデル・式・ロジック(macOS で動く) | 軽い |
| `MetamonMacTests` | `ios/MetamonMacTests/` | Mac アプリをホストにした単体テスト | 中(アプリのビルドの続きで数十秒・mac-ci のコメント) |
| `MetamonMacUITests` | `ios/MetamonMacUITests/` | Mac のキーボード操作の UI テスト(XCUITest) | 重い・scheme で `skipped: true` |
| `MetamonRendererTests` | `ios/Packages/MetamonRenderer/Tests/MetamonRendererTests/` | 描画・レイアウト・DSL の適合テスト(`docs/dsl/conformance`) | 軽い〜中 |
| `MetamonServicesTests` | `ios/Packages/MetamonServices/Tests/MetamonServicesTests/` | 通信・保存・同期・認証などのサービス | 軽い〜中 |
| `MetamonTests` | `ios/MetamonTests/` | iOS アプリをホストにしたテスト(StoreKitTest の購入など Bundle ID が要るもの) | 中(シミュレータ) |
| `MetamonUITests` | `ios/MetamonUITests/` | 指の操作(タップ・ドラッグ・長押し)の UI テスト(XCUITest・並列なし) | 重い(テスト本体で 80〜100 秒・シミュレータの起動込みで 2〜4 分。本数が増えた今はもっとかかる) |
| `MetamonWidgetTests` | `ios/MetamonWidgetTests/` | ウィジェットの View を `ImageRenderer` で描く | 中(シミュレータ) |

- scheme `Metamon` の test に入るもの: 3 つのパッケージのテスト・`MetamonTests`・`MetamonWidgetTests`・`MetamonUITests`。
- scheme `MetamonMac` の test に入るもの: `MetamonMacTests`(`MetamonMacUITests` は skipped)。
- `MacSnapshotTests`(`MetamonMacTests` の中)は既定で飛ばす。macOS とフォントの版で文字の描き方が変わるため。

## 回し方

xcodegen でプロジェクトを作ってから回す(`cd ios && xcodegen generate`)。

| すること | コマンド |
|---|---|
| Mac の UI テスト | `cd ios && xcodebuild test -scheme MetamonMac -destination 'platform=macOS' -only-testing:MetamonMacUITests`(初回はアクセシビリティ / オートメーションの許可が要る) |
| Mac の単体テスト | `cd ios && xcodebuild test -project Metamon.xcodeproj -scheme MetamonMac -configuration Debug -destination 'platform=macOS' CODE_SIGNING_ALLOWED=NO` |
| Mac のスナップショット(比べる) | `TEST_RUNNER_METAMON_SNAPSHOTS=1 xcodebuild -scheme MetamonMac test -only-testing:MetamonMacTests/MacSnapshotTests …`(撮り直しは `TEST_RUNNER_METAMON_SNAPSHOT_RECORD=1`) |
| iOS の UI テスト | `cd ios && xcodebuild -scheme Metamon -destination 'id=<UDID>' -only-testing:MetamonUITests -parallel-testing-enabled NO test CODE_SIGNING_ALLOWED=NO` |
| iOS のテスト(UI 以外を全部) | `cd ios && xcodebuild -scheme Metamon -destination 'platform=iOS Simulator,name=iPhone 17' -skip-testing:MetamonUITests test CODE_SIGNING_ALLOWED=NO` |
| パッケージ 1 つ(macOS) | `ios/scripts/mac-package-test.sh <MetamonDSL\|MetamonRenderer\|MetamonServices>`(mac-ci・ios-ci の `ios-dsl` と同じ) |
| パッケージ 1 つ(速いが英語のテストが落ちる) | `cd ios/Packages/MetamonDSL && swift test` |
| ターゲットを絞る | `-only-testing:<ターゲット>[/<クラス>[/<メソッド>]]` を足す |
| ウィジェットの描画を PNG で出す | `TEST_RUNNER_WIDGET_SHOTS_DIR=<dir>` を付けて `-only-testing:MetamonWidgetTests` |

- `swift test` は String Catalog を訳さないので `EnglishCopyTests` だけ落ちる。パッケージは `mac-package-test.sh` を使う(理由は `ios/scripts/mac-package-test.sh` の先頭のコメント)。
- repo か DerivedData が `~/Desktop`・`~/Documents`・`~/Downloads` の下にあると、macOS のテストの実行役が読めずに落ちる。`MAC_DERIVED_DATA` などを保護の外に置く(`ios/scripts/mac-package-test.sh` の先頭)。
- シミュレータは 1 台だけ使い、新しく作らない(初回起動のキャッシュ作りで PC が重くなる)。xcodebuild は同時 1 本。

## CI が回すもの

正しいのは workflow の先頭のコメントと判定の処理(`.github/workflows/ios-ci.yml`・`.github/workflows/mac-ci.yml`)。ここは要約。

| きっかけ | workflow | 回すもの |
|---|---|---|
| develop / main への push(iOS の変更あり) | ios-ci | scheme `Metamon` の全部(UI テスト・`MetamonWidgetTests` を含む) |
| PR・`.github/workflows/ios-ci.yml` の変更 | ios-ci | 3 つのパッケージ + `MetamonTests`(UI テストは除く) |
| PR・`ios/MetamonUITests/**` だけの変更 | ios-ci | テストは回さない(build-for-testing でコンパイルだけ) |
| PR・`ui-tests` ラベル付き | ios-ci | 上の対象 + `MetamonUITests`(ラベルを付けてから Re-run か次の push) |
| PR・アプリの変更(`ios/Metamon/**`・`ios/Shared/**`・`ios/MetamonWidget/**` など) | ios-ci | `MetamonTests` |
| PR・DSL だけの変更 | ios-ci(`ios-dsl`) | macOS の `MetamonDSLTests` だけ(シミュレータ無し) |
| PR・DSL と他の iOS の変更 | ios-ci | 3 つのパッケージ + `MetamonTests` |
| PR・Renderer の変更 | ios-ci | `MetamonRendererTests` + `MetamonTests` |
| PR・Services の変更 | ios-ci | `MetamonServicesTests` + `MetamonTests` |
| PR / push・Mac アプリ・Shared の変更 | mac-ci | Mac アプリのビルド + `MetamonMacTests` |
| PR / push・パッケージの変更 | mac-ci | 変わったパッケージの macOS のテスト(DSL の変更は 3 つ全部) |
| すべて | ios-ci | `.xcstrings` の整形チェック(ubuntu・ブロッキング) |

- `MetamonMacUITests` と `MacSnapshotTests` は CI では回らない。
- `MetamonWidgetTests` は PR では回らない(`-only-testing` の対象に入らない)。push の時だけ回る。
- macOS の runner は分単価が高いので、変わった所のテストだけ回す。

## 手元で回さない物

| 物 | 理由 | 代わり |
|---|---|---|
| `MetamonMacUITests` | 重い・権限の許可が要る・画面を奪う | 必要な時だけ `-only-testing` で 1 クラス |
| `MetamonUITests` を全部 | 重い(シミュレータ・並列なし) | PR に `ui-tests` ラベルを付けて CI で回す。手元は `-only-testing` で 1 本 |
| scheme `Metamon` の test を絞らずに | UI テストまで回る | `-skip-testing:MetamonUITests` を付ける |
| 複数の xcodebuild を同時に | PC が重くなる | 1 本ずつ |

## テストを足す時の置き場所

| 足すもの | 置き場所 |
|---|---|
| DSL のモデル・式・ロジック | `ios/Packages/MetamonDSL/Tests/MetamonDSLTests/` |
| DSL の式の共有テストデータ(Android・backend と共有) | `docs/dsl/conformance/expr/` |
| Mac だけの動き | `ios/MetamonMacTests/`(名前は `Mac…Tests.swift`) |
| Mac のキーボード操作 | `ios/MetamonMacUITests/`(CI では回らない) |
| Renderer の描画・レイアウト | `ios/Packages/MetamonRenderer/Tests/MetamonRendererTests/` |
| Services の通信・保存・同期 | `ios/Packages/MetamonServices/Tests/MetamonServicesTests/` |
| アプリ(iOS)の動き・Bundle ID が要るもの | `ios/MetamonTests/` |
| ウィジェットの見た目 | `ios/MetamonWidgetTests/` |
| 指の操作(タップ・ドラッグ・長押し) | `ios/MetamonUITests/`(PR では既定で回らない) |

- まずパッケージのテストに置けないかを考える(軽く、mac-ci でも回る)。アプリやシミュレータが要る時だけアプリのテストに置く(推奨)。
- 新しいテストのファイルは xcodegen がフォルダごと拾うので、`ios/project.yml` は触らなくてよい。
- UI テストで要素を探す時は accessibilityIdentifier を使う(決まりは [architecture.md の 23](architecture.md#23-ui-テストxcuitestissue-165))。

## 決めること

- `MetamonWidgetTests` を PR でも回すか(今は push の時だけ)。
- [architecture.md の 23](architecture.md#23-ui-テストxcuitestissue-165) の末尾の「CI」の段落は、今の ios-ci(PR では UI テストを除く・`ui-tests` ラベル)と合っていない。どちらを直すか。
- 重さの目安を実測で埋めるか(今は UI テスト以外は推測)。
