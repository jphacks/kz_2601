# 部品とアクションに振動(触覚)を付ける DSL の案(下書き・決めない)

> 下書きです。**どの案にするかは決めません**(決めるのは人)。事実と、推測・決めることを分けて書きます。
> 親: #1839(まとめ #1787)・この下書き: #3027。正典の [`../dsl/dsl.schema.json`](../dsl/dsl.schema.json)・[`../dsl/README.md`](../dsl/README.md) は変えていません。

## 目次

- [1. 何を決めたいか](#1-何を決めたいか)
- [2. 今あるもの(事実)](#2-今あるもの事実)
- [3. 案](#3-案)
- [4. 案の比べ表](#4-案の比べ表)
- [5. 強さの種類と OS ごとの対応](#5-強さの種類と-os-ごとの対応)
- [6. どこで鳴らすか](#6-どこで鳴らすか)
- [7. 振動の無い OS の扱い](#7-振動の無い-os-の扱い)
- [8. 決めること](#8-決めること)

## 1. 何を決めたいか

保存した時・タイマーが終わった時・くじが決まった時などに、生成したアプリが振動を鳴らせるようにするか。鳴らせるなら、DSL のどこに書かせるか。

## 2. 今あるもの(事実)

- Android は、部品の中で固定の振動を鳴らしている。`DSLView.kt`(フォームの保存バー: `HapticFeedbackType.Confirm`)・`RandomPickerLeaf.kt` / `DashcamLeaf.kt` / `SensorValueLeaf.kt`(`LongPress`)・`BracketLeaf.kt` / `FlashcardsLeaf.kt` / `MatchListLeaf.kt`(`TextHandleMove`)。どれも DSL からは変えられない。
- iOS は `PlatformSystem.successHaptic()`(`UINotificationFeedbackGenerator` の成功)があり、`QrScannerLeaf.swift` と `MacCamera.swift` が使う。Mac では何もしない(`#if canImport(UIKit)` の中だけ)。ほかの部品で iOS が振動を鳴らしているかは、この下書きでは確かめていない(推測: 保存・くじは鳴らしていない)。
- `docs/ios/architecture.md` の表に「`native: haptic {style}`・`UIImpactFeedbackGenerator`(実装済み)」とあるが、今のスキーマ(`dsl.schema.json` の `Action.type`)に `haptic` も `native` も無い。古い記述の可能性が高い(推測)。
- デスクトップ(Windows・Linux)は振動の仕組みが無い。部品の振動は見当たらない。
- アクションは「入れ子不可」で、1 つのボタンから 1 つの `type` を実行する(`dsl.schema.json` の `Action` の説明)。

## 3. 案

### 案 A: 部品の props に `haptic` を付ける(部品が鳴る時を指定)

```json
{"type": "timerDisplay", "mode": "countdown", "seconds": 180, "haptic": {"onFinish": "success"}}
{"type": "randomPicker", "...": "...", "haptic": {"onPick": "medium"}}
{"type": "button", "label": "記録", "action": {"type": "saveRecord"}, "haptic": {"onTap": "light"}}
```

鳴らす「時」は部品ごとの決まった名前(`onFinish` / `onPick` / `onTap` / `onSave`)に限る。

### 案 B: アクションに `haptic` を足す

```json
{"type": "haptic", "style": "success"}
```

ボタンの `action`・toolbar から実行する。保存の後に鳴らすには「保存 → 振動」の 2 つが要る。今のアクションは入れ子不可なので、**「保存してから振動」の書き方(`saveRecord` に `thenHaptic` を付ける・または連続実行の仕組みを作る)が別に必要**(決めること)。

### 案 C: DSL には出さず、部品が決めた時に自動で鳴らす(OS ごとに統一)

保存の完了・タイマー終了・くじの確定・QR の読み取り成功を、全 OS で同じ強さの種類(下の表の `success` など)で鳴らす。アプリ全体の「振動 切り」のスイッチだけ DSL(`app` の props)か端末の設定に持つ。

## 4. 案の比べ表

| 案 | 良い点 | 困る点 | iOS | Android | Mac | デスクトップ |
|---|---|---|---|---|---|---|
| A: 部品の props | 部品の「その時」に正しく鳴る(保存の成否・タイマー終了)。AI が部品の説明を読めば書ける | 部品ごとに props を足す(スキーマの変更が多い)。書き忘れた時は鳴らない | 作れる(UIKit の生成器) | 作れる(`HapticFeedbackType`) | 作れない(何もしない) | 作れない(何もしない) |
| B: アクション | 書き方が 1 つ。どのボタンにも付けられる | 「保存の成功の後」のような時を指定できない。入れ子不可の制約と合わない(連続実行の仕組みが別に必要) | 作れる | 作れる | 何もしない | 何もしない |
| C: 自動 | AI も DSL も変えずに済む。全部のアプリで同じ体験 | アプリごとに強さ・有無を変えられない。振動が嫌なアプリで切る口が要る | 作れる(今は成功だけ) | 作れる(今は部品ごとに固定) | 何もしない | 何もしない |

## 5. 強さの種類と OS ごとの対応

DSL の値(仮)と、今のコードにある・ありそうな OS の API の対応。

| 値(仮) | 意味 | iOS | Android(Compose の `HapticFeedbackType`) |
|---|---|---|---|
| `light` / `medium` / `heavy` | 軽い・中・強いタップ | `UIImpactFeedbackGenerator`(推測: 未実装。`docs/ios/architecture.md` は実装済みと書く) | 近い種類を選ぶ(`LongPress` などで代用・推測) |
| `selection` | 選択が変わった | `UISelectionFeedbackGenerator`(推測) | `TextHandleMove`(`BracketLeaf.kt` などが使用) |
| `success` | 成功 | `UINotificationFeedbackGenerator.success`(`PlatformSystem.successHaptic`) | `Confirm`(`DSLView.kt` が使用) |
| `warning` / `error` | 警告・失敗 | `UINotificationFeedbackGenerator`(推測) | `Reject`(推測。使える Android の版に依存) |

- 種類を細かく増やすと OS ごとの差が出る(推測)。**少ない数(例 3〜4)に絞る**かは決めること。
- Android の `HapticFeedbackType` の一部は新しい Android の版でないと効かない場合がある(推測。実装時に版ごとの確認が要る)。

## 6. どこで鳴らすか

| 場面 | 今 | 案 A | 案 B | 案 C |
|---|---|---|---|---|
| 保存(`saveRecord`)の完了 | Android は鳴る(`Confirm`)・iOS は鳴らない(推測) | 保存ボタンの `haptic` | 「保存の後」が書けない | 自動で成功 |
| タイマーの終わり(`timerDisplay`) | 鳴らない(確認した範囲) | `haptic.onFinish` | 書けない(終了の時点がアクションでない) | 自動で成功 |
| くじ(`randomPicker`) | Android は `LongPress` | `haptic.onPick` | 書けない | 自動 |
| 項目の選択・トーナメントの進行など | Android の一部が固定で鳴る | 部品ごと | 書けない | 自動 |

## 7. 振動の無い OS の扱い

- Mac・デスクトップは**何もしない**で、エラーにも代わりの音・点滅にもしない案(今の `successHaptic` の Mac と同じ)。
- 代わりの表示(画面の一瞬の強調・音)を出すかは決めること。出す場合も DSL で書かせず、OS 側で決める方が AI に教えることが増えない(推測)。
- AI には「振動は iPhone と Android だけで、それ以外では何も起きない。振動に意味を持たせた(知らせはそれだけ)アプリは作らない」と教える案。申告は `supportedFeatures` / `disabledFeatures`(例 `haptics`)を使う案(仕組みは [`../dsl/platforms.md`](../dsl/platforms.md)・[`../dsl/README.md` §5.3](../dsl/README.md#53-backend-が古い-ios-に新しい部品を出さない仕組み))。

## 8. 決めること

1. DSL に出すか(案 C のみで出さない案を含む)。
2. 出すなら A / B のどちらか(または併用)。B なら「保存してから振動」の書き方。
3. 強さの種類をいくつにするか。
4. iOS が保存・くじで鳴らすか(今の Android との揃え方)。
5. 振動を切る設定の置き場(アプリごとか端末の設定か)。
6. 古い記述 `native: haptic`(`docs/ios/architecture.md`)をどう扱うか(この下書きでは直していません)。
