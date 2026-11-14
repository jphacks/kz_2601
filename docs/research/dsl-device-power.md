# 端末の電池・充電・省電力を式で読む DSL の案(下書き・決めない)

> 下書きです。**どの案にするかは決めません**(決めるのは人)。今のコードを読んだ事実と、そこからの推測・決めることを分けて書きます。
> 親: #1835(まとめ #1787)・この下書き: #3026。正典の [`../dsl/expressions.md`](../dsl/expressions.md)・[`../dsl/dsl.schema.json`](../dsl/dsl.schema.json) は変えていません。

## 目次

- [1. 何を決めたいか](#1-何を決めたいか)
- [2. 今あるもの(事実)](#2-今あるもの事実)
- [3. 案](#3-案)
- [4. 案の比べ表](#4-案の比べ表)
- [5. 値の型と、使えない OS の値](#5-値の型と使えない-os-の値)
- [6. AI に教える時の注意](#6-ai-に教える時の注意)
- [7. 決めること](#7-決めること)

## 1. 何を決めたいか

端末の「電池の残り・充電中・省電力」を、生成したアプリの式(`stat.value` や通知の `when` など)から読めるようにするかと、読めるなら**どんな形で書かせるか**。

## 2. 今あるもの(事実)

- iOS: `DevicePowerSource`(`ios/Packages/MetamonServices/Sources/MetamonServices/Native/DevicePowerSource.swift`)が残り(0〜1・不明は nil)・充電(`unknown` / `unplugged` / `charging` / `full`)・低電力モードを返す。Mac は電池を読まず、低電力モードだけ読む。ファイル先頭に「式・スキーマ・AI の決まりにはまだ出さない(#1775 の決定待ち)」とある。
- デスクトップ: `desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/power/DevicePowerSource.kt` が同じ形(残り・充電・省電力)で、Linux は sysfs・Windows は `GetSystemPowerStatus` を 60 秒間隔で読み直す。Mac のデスクトップ版は読む実体が無い(`DevicePowerSource.kt` の OS 分けは Linux / Windows)。
- Android: 同じ形の口は見つからない。電池は `DashcamRecorder.kt` が `BatteryManager` を直接使っているだけ。
- 値は端末の中だけで使い、サーバーには送らない(上の 2 つのファイルの注記)。
- 式の今の仕組み: `op` は閉じた一覧で、引数なしの `now` / `today` が前例([`../dsl/expressions.md` §3](../dsl/expressions.md#3-op-一覧))。`setting` は専用のキーを持つ式の形。新しい op や機能は `supportedFeatures`(例 `expr.addMinutes`)で古い版に使わせない([`../dsl/README.md` §5.3](../dsl/README.md#53-backend-が古い-ios-に新しい部品を出さない仕組み))。

## 3. 案

### 案 A: 引数なしの op を足す(`now` / `today` と同じ形)

```json
{"op": "batteryLevel", "args": []}
{"op": "isCharging", "args": []}
{"op": "isLowPower", "args": []}
```

例: 残りが 20% 未満なら「充電して」と出す → `if(lt(batteryLevel(), 0.2), "充電して", "")`。

### 案 B: 専用のキーを持つ式の形を足す(`setting` と同じ形)

```json
{"device": "batteryLevel"}
{"device": "charge"}
{"device": "lowPower"}
```

`device` の値は閉じた一覧(enum)。op でなく式の 1 形になるので、スキーマの `Expr` の `oneOf` に 1 つ足す。

### 案 C: 式に出さず、保存する時点の値を項目に入れる

式からは読ませず、`field.defaultValue` に `"now"`(date の前例)のような決まった文字列(例 `"$battery"`)を許し、記録を作った時の値を数値の項目に残す。式は残った項目を普通の `ref` で読む。

### (参考)案 D: 出さない

今の `DevicePowerSource` は内部の部品のままにして、DSL には出さない。

## 4. 案の比べ表

| 案 | 良い点 | 困る点 | iOS | Android | Mac | デスクトップ(Win / Linux) |
|---|---|---|---|---|---|---|
| A: op(引数なし) | `now` / `today` と同じ書き方で AI が書き間違えにくい(推測)。backend の op 一覧に足すだけ | op が「純粋な計算」でなく「端末の今の値」になる。値が変わった時の再描画を式の評価側が持つ必要がある(決めること) | 作れる(口あり) | 作れない(口が無い。`BatteryManager` / `PowerManager.isPowerSaveMode` で作れる見込み・推測) | 低電力のみ(電池は不明) | 作れる(口あり・60 秒間隔の読み直し) |
| B: `device` の式の形 | 「端末の値」だと見て分かる。値の一覧を enum で閉じられ、検証しやすい | スキーマの `Expr` の形を 1 つ増やす(正典の変更が大きい)。評価の実装が iOS / Android / デスクトップ / backend の検証で全部増える | 同上 | 同上 | 同上 | 同上 |
| C: 保存時の値を項目に入れる | 式の仕組みは変わらない。「記録した時の残り」が履歴として残る | 今の値で画面を変える用途(残りが少ないなら…)に使えない。記録に端末の状態が残る(個人の記録の扱いは決めること) | 作れる | 作れない(口なし・同上) | 低電力のみ | 作れる |
| D: 出さない | 追加の仕組みが要らない。使えない OS の差を AI に教えずに済む | 「電池が減ったら静かな画面に」のようなアプリは作れない | — | — | — | — |

## 5. 値の型と、使えない OS の値

案によらず共通で決めたいこと。

| 名前(仮) | 型 | 値 | 不明の時(推測の案) |
|---|---|---|---|
| `batteryLevel` | 数値 | 0〜1(0.2 = 20%)。「0〜100」にするかは決めること | 空(nil)。`coalesce` で既定値を足せる([`../dsl/expressions.md` §3](../dsl/expressions.md#3-op-一覧)) |
| `isCharging` | 真偽 | 充電中と満充電が true(今のコードの `isCharging` と同じ) | 電池が無い・読めない OS は false か空かを決める |
| `charge` | 文字列(enum) | `unknown` / `unplugged` / `charging` / `full` | `unknown` |
| `isLowPower` | 真偽 | iOS の低電力モード・Android / デスクトップの省電力 | 読めない OS は false(今のコードは false) |

- 空(nil)は式の他の部分と同じく「—」になる前提(`expressions.md` の評価ルール。細かい扱いは決めること)。
- Mac: 電池は不明で低電力モードだけ読める(事実)。Mac で `batteryLevel` を使った式がどう見えるかを決める。

## 6. AI に教える時の注意

- 値は**空になることがある**(電池の無いデスクトップ・Mac・シミュレータ)。残りを使う式は `coalesce` で既定値を持たせるよう教える(推測)。
- 端末の値は**その端末の中だけ**で使う。サーバーの自動の動き(`serverRules`)と、別の人の端末には使えないと教える(今のコードは「サーバーには送らない」が前提)。
- 使える OS を `supportedFeatures` / `disabledFeatures` でどう申告するか(例 `expr.devicePower`。Android 未対応の間は送らない)を決める。申告の仕組みは [`../dsl/platforms.md`](../dsl/platforms.md) の値の意味を参照。
- 画面を消さない(#3029)・振動(#3027)と組み合わせる使い方(省電力の時は画面を消さない等)がありうるが、ここでは決めない。

## 7. 決めること

1. DSL に出すか(案 D を含む)。出すなら案 A / B / C のどれか。
2. 残りの単位(0〜1 か 0〜100)と、不明な時の値(空・false)。
3. 値が変わった時に画面を作り直すか、記録時・画面を開いた時の 1 回だけ読むか。
4. Android の `DevicePowerSource` 相当を先に作るか(今は口が無い)。
5. 通知の `when`(端末内で評価される)でも使えるようにするか。
6. 使えない OS の申告のしかた。
