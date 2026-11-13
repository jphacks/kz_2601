# iOS の開発用メニュー(stg の版だけ)

動きが変わる分かれ道(課金の有無・日付・サーバーのエラー・アカウントの枠など)を、端末の上で切り替えて試すメニューです(#2079・親 #2078)。この文書は、出る条件・入口・つまみの決まり・セクションの足し方をまとめます。

- 表のファイル名は `origin/epic/god` の `f502d2543` で確かめました。コードは `ios/Metamon/Features/Debug/` にあります。
- 「推測」と書いた所は、コードから読み取れなかったことです。

## 目次

- [入口](#入口)
- [決まり](#決まり)
- [項目の足し方](#項目の足し方)
- [いまあるセクション](#いまあるセクション)
- [ほかの OS の状況](#ほかの-os-の状況)

## 入口

| 項目 | 内容 | コード |
|---|---|---|
| 出る版 | `METAMON_STG` が立つ構成(Debug・Staging)だけ。Release(TestFlight・App Store)と Review(審査用)にはコードごと入らない | `ios/project.yml`・[review-build.md](review-build.md) |
| 出る条件 | 上に加えて、接続先の URL が prod(`-prod-` を含む Cloud Run の URL)でないこと。Staging の構成で prod に向けて作った時の取りこぼしを防ぐ。URL が無い(モック)時は出す | `DebugMenuPolicy.isAvailable` |
| 開く所 | 設定の末尾の「開発用メニュー」(レンチとドライバーのアイコン)。説明は「stg の版だけに出ます。」 | `SettingsSheet`・`DebugMenuEntrySection` |
| 画面 | 先頭が「今の環境」(構成・版・ai・appconfig・Firebase・uid・匿名か・appId)、続いてセクション、最後に「全部元に戻す」 | `DebugMenuView` |
| 言語 | 開発者向けなので日本語だけ。xcstrings には入れない | `DebugMenuView` |

- 判定の型 `DebugMenuPolicy` だけは全部の版に入ります(判定の単体テストを Release の条件でも書くため。文言は持たない)。
- 「今の環境」の uid は、横のボタンでコピーできます。

## 決まり

### つまみ(`DebugKnobs`)

| 決まり | 内容 |
|---|---|
| 名前 | 起動時の環境変数と同じ `METAMON_…`(Android の `DebugKnobs.kt` とも同じ名前) |
| 優先 | 環境変数があればそれ、無ければメニューで保存した値。環境変数で決まっているつまみは、メニューで変えても効かない(行に「環境変数 `METAMON_…` が優先」と出る) |
| 保存先 | UserDefaults のスイート `jp.kz2601.metamon.debug`。キーは頭に `metamon.debug.` を付けた `metamon.debug.<名前>`(例: `metamon.debug.METAMON_DEBUG_TODAY`) |
| 型 | 文字列と真偽。真偽は `1` / `true` / `yes` が true、`0` / `false` / `no` が false、それ以外と無しは nil |
| 消す | `set(名前, nil)` で既定に戻す |
| 読む側 | stg の版は `DebugKnobs`、それ以外の DEBUG は環境変数だけ(例: `DebugPlanKnobs.string`)。読む側は `#if DEBUG \|\| METAMON_STG` の中に置く(Release・Review に入れない) |

### 効くまで(`DebugKnobEffect`)

| 値 | 意味 | 画面 |
|---|---|---|
| `.immediate` | すぐ効く | 行の下に「すぐ効く」 |
| `.relaunch` | 起動し直しが要る(起動時に 1 回だけ読むつまみ) | 行の下に警告色で「起動し直しが要る」。変えるとメニューの下に「アプリを終了する」が出る(押すと `exit(0)`) |

- 起動し直しが要るつまみの例: 課金の画面(`METAMON_MOCK_STORE`)・コイン(`METAMON_COINS`)・アカウントの枠。

### 全部元に戻す

1. `DebugKnobs.resetAll()` が `metamon.debug.` で始まるキーを全部消す。
2. 各セクションの `reset()` を呼ぶ。`metamon.debug.` の外に保存したもの(例: 課金の制限、サーバーの応答の作り物)だけをここで戻す。
3. 起動時に読んだつまみは開き直すまで残るので、「アプリを終了する」を出す。

- 起動時の環境変数は消えません(そのまま効く)。

## 項目の足し方

1. `ios/Metamon/Features/Debug/Sections/` に **1 ファイル 1 つ** `DebugMenuSection` の subclass を書く。共有の一覧は書き換えない(メニューは起動中のクラスから subclass を探して並べる。孫のクラスも拾う・`DebugMenuRegistry`)。
2. ファイル全体を `#if METAMON_STG` で囲む(DEBUG の版だけなら `#if DEBUG && METAMON_STG`)。
3. 次を上書きする。

| 上書き | 役目 |
|---|---|
| `order` | 小さいほど上。同じなら型の名前の順。戻し忘れに気づきたいものは小さくする |
| `title` | セクションの見出し |
| `footer` | 見出しの下の説明(任意) |
| `makeBody()` | セクションの中の行(`AnyView`) |
| `reset()` | 「全部元に戻す」で呼ばれる。`DebugKnobs` の分は消えるので、それ以外に保存したものだけ戻す |

```swift
final class DebugFooSection: DebugMenuSection {
    override class var order: Int { 300 }
    override class var title: String { "Foo" }
    override class func makeBody() -> AnyView { AnyView(DebugFooRows()) }
    override class func reset() { /* metamon.debug.* の外に保存したものを戻す */ }
}
```

4. 行の見出しは `DebugKnobLabel(title:effect:envName:)` を使う(「すぐ効く / 起動し直しが要る」と環境変数の優先を同じ書き方で出す)。
5. 値は `DebugKnobs` に保存する(名前は起動時の環境変数と同じ)。起動し直しが要るつまみは `effect: .relaunch` にし、変えたら `DebugMenuState.shared.markNeedsRelaunch()` を呼ぶ。
6. UI テストで触る行には `accessibilityIdentifier`(`debugMenu.<セクション>.<項目>`)を付ける。
7. 確かめるテストは `ios/MetamonTests/` に足す(`DebugMenuRegistryTests` が並びと探し方、`DebugKnobsTests` が保存の決まりを確かめる。`#if METAMON_STG` で包む)。

- サーバーに書く物・本番の権利のデータは、つまみにしない(stg は本番と同じデータベースを使う。#2078 の方針)。

## いまあるセクション

| order | 型 | 見出し | 内容 |
|---|---|---|---|
| 0 | `DebugClockSection` | 日付 | 式の評価の「今日」(`DSLClock`)をずらす。記録の作成日時はずらさない(#2082) |
| 50 | `DebugAccountSection` | アカウントの枠 | 枠ごとの匿名アカウントに切り替える。端末のデータを消し、終了して開き直す(#2084) |
| 100 | `DebugProSection` | 課金 | 課金の制限を無効にする |
| 110 | `DebugPlanSection` | 課金と量 | Pro / 無料・無料の回数・コイン・使っている量の見本(#2080) |
| 200 | `DebugExtCallSection` | つないだサービス | 試験用のサービスを呼ぶ(#2045) |
| 250 | `DebugContactPickSection` | 連絡先から 1 人選ぶ | 連絡先の 1 人選ぶ口を試す。DEBUG の版だけ(#2217) |
| 300 | `DebugFaultsSection` | サーバーの応答 | 402 / 409 / 429 / 5xx・圏外・生成の途中で切れるを作り物で起こす(#2083) |

## ほかの OS の状況

| OS | 状況 | Issue |
|---|---|---|
| Android | `DebugKnobs.kt`(debug のビルドだけ・環境変数と同じ名前のつまみ)はあるが、メニューはない | #2088 |
| iOS | この文書のとおり | #2079 |
| Mac | メニューはない。判定(`DebugKnobs`・`DebugMenuPolicy`)を `ios/Shared` に移す Issue から始める | #2087(判定は #2247・日付は #3247・アカウントの枠は #3246・文書は #3226) |
| デスクトップ | メニューはない。コマンドパレットに出す形で設計されている(`TAILORIT_API=stg` の時だけ) | #2089(土台は #2210) |

- 推測: Mac の Issue #3226 の題は「設定の『開発用』のタブ」なので、Mac は iOS の設定の末尾の入口ではなくタブになる見込み。
- サーバーの Pro / 無料の判定を stg で変える口は、持ち主の承認が要ります(#2085・#2086)。この文書の範囲には入れません。
