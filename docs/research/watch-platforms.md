# Apple Watch・Wear OS に stat とボタンを出す時の作りの調査メモ

> 調べた日: 2026-11-13。親: #1843 #1844(まとめ #1788)。調査だけで、決めない。コード・正典(`docs/00_decisions.md`・`docs/decisions/*`)は変えていない。
> 「読んだ」と書いた物は出典のページを開いて読んだ内容。Apple のページは本文が取れなかったので、**Apple 側の細かい点は一般の知識で書いた「未確認」**(出典の URL を人が開いて確かめる)。

## 目次

1. [前提](#前提)
2. [作りの案(Apple Watch)](#作りの案apple-watch)
3. [作りの案(Wear OS)](#作りの案wear-os)
4. [データの受け渡し](#データの受け渡し)
5. [決めること](#決めること)
6. [出典](#出典)

## 前提

- 見せたい物は、生成アプリの **stat(数字)とボタン(記録を足すなど)**。iPhone のウィジェットでは `ios/MetamonWidget/` が stat を出し、`Controls/` が開く・操作の入口を出している([architecture.md の §12](../ios/architecture.md#12-ホーム画面ウィジェットwidgetkit実装済みissue-39))。stat の数え方は `WidgetSnapshotWriter` にそろえてある([system-integration.md](../mac/system-integration.md))。
- Android は今、Wear OS を対象外にしている([354-android-tech.md](../decisions/354-android-tech.md))。Wear OS を足すなら、この決定の見直しが要る。

## 作りの案(Apple Watch)

| 案 | 中身 | 出来ること | 手間(推測) | 状態 |
| --- | --- | --- | --- | --- |
| A. コンプリケーション(WidgetKit の accessory)だけ | `accessoryCircular`・`accessoryCorner`・`accessoryRectangular`・`accessoryInline` の stat。タップでアプリを開く | stat の表示。ボタンは出せない(未確認: 小さい枠は操作のボタンを持てず、タップはアプリか URL を開くだけ) | 小。iPhone のウィジェットと同じ WidgetKit | 未確認 |
| B. Smart Stack のウィジェット | watchOS 10 以降の大きめのウィジェット | stat とボタン(App Intent)が置けるかもしれない(未確認・版で変わる) | 小〜中 | 未確認 |
| C. 腕時計のアプリ(watchOS app) | 画面を持つアプリ。ボタンで記録を足す | stat とボタンの両方。声の入力も | 大。ターゲットを足し、画面を作る。コンプリケーションも結局は腕時計のアプリに同梱する(未確認) | 未確認 |

- iPhone のアプリの中身(DSL)を腕時計で全部動かすのは重い(推測)。stat とボタン数個に絞る案が現実的。

## 作りの案(Wear OS)

| 案 | 中身 | 出来ること | 手間(推測) |
| --- | --- | --- | --- |
| A. コンプリケーションだけ | 文字盤に stat を出す | stat の表示 | 小〜中(読んでいない・Wear OS の別のページ) |
| B. タイル(Tiles) | 文字盤を横にスワイプして出る面。小さい情報と操作を出す | stat と、タップで進む操作(読んだ)。通信や重い処理はタイルの中でせず、WorkManager で先に済ませて保存した物を出す(読んだ) | 中。Compose でなく protolayout で宣言して書く(読んだ) |
| C. 腕時計のアプリ | 画面を持つアプリ | stat とボタン | 大 |

- Wear OS の指針は「手首に向く大事な作業だけに絞り、全部は移さない」「面ごとに優先を分け、足りなければアプリへ誘導する」(読んだ)。stat は文字盤・タイル、ボタンはタイルかアプリ、という分け方が指針に近い(推測)。

## データの受け渡し

| 仕組み | 出来ること | 向く物 |
| --- | --- | --- |
| WatchConnectivity `updateApplicationContext`(Apple・未確認) | 最新の 1 件だけ送る(上書き) | stat の今の値 |
| WatchConnectivity `sendMessage`(同) | 相手が起きている時の即時のやり取り。失敗しても待たない | 記録を足すボタンの即時の反映 |
| WatchConnectivity `transferUserInfo`(同) | 順番どおり確実に届く(キュー) | ボタンで足した記録の送り返し |
| WatchConnectivity `transferFile`(同) | ファイルを確実に送る | 写真・音声 |
| Wear OS の Data Layer(読んだ) | 腕時計と、つないだ Android の電話の間でだけ使える。Bluetooth、つながらない時は Google のサーバー経由(暗号化) | 電話をリモコンにする・「電話で開く」 |
| 自前のクラウド(読んだ) | Data Layer ではなく、保存・取得・同期はクラウドの API でやるのが勧められている | 記録の保存・stat の取得 |

- Wear OS の Data Layer は、iPhone とつないだ Wear OS の腕時計では使えず、クラウドの API を使う(読んだ)。
- この repo の記録の置き場・同期の方式(クラウドか端末か)で、受け渡しの形が変わる。腕時計は電話を持たずに使う場面もある(推測)。

## 決めること

| 番号 | 決めること | 案 |
| --- | --- | --- |
| 1 | 腕時計を対象にするか(Wear OS は「対象外」の見直しを含む) | しない / Apple Watch だけ / 両方 |
| 2 | 最初に出す面 | コンプリケーション(stat だけ)/ Smart Stack・タイル(stat+ボタン)/ アプリ |
| 3 | ボタンの効果の反映の方式 | 電話へ送る(WatchConnectivity・Data Layer)/ 腕時計から直接クラウドへ |
| 4 | 実機・シミュレータの確認(Smart Stack のボタン・コンプリケーションの操作の可否) | 人の作業(未確認) |

## 出典

1. Android Developers: Wear OS の原則 <https://developer.android.com/training/wearables/principles>(読んだ)
2. Android Developers: タイル <https://developer.android.com/training/wearables/tiles>(読んだ)
3. Android Developers: Data Layer <https://developer.android.com/training/wearables/data/overview>(読んだ)
4. Apple: アクセサリのウィジェットと腕時計のコンプリケーション <https://developer.apple.com/documentation/widgetkit/creating-accessory-widgets-and-watch-complications>(本文が取れず未確認)
5. Apple: WCSession <https://developer.apple.com/documentation/watchconnectivity/wcsession>(本文が取れず未確認)
