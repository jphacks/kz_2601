# 画面を消さない(keepAwake)を DSL で書けるようにする案と、今の自動の判定との違い(下書き・決めない)

> 下書きです。**どの案にするかは決めません**(決めるのは人)。事実と、推測・決めることを分けて書きます。
> 親: #1840・この下書き: #3029。正典の [`../dsl/dsl.schema.json`](../dsl/dsl.schema.json)・[`../dsl/README.md`](../dsl/README.md) は変えていません。

## 目次

- [1. 何を決めたいか](#1-何を決めたいか)
- [2. 今の自動の判定(事実)](#2-今の自動の判定事実)
- [3. 案](#3-案)
- [4. 案の比べ表](#4-案の比べ表)
- [5. OS ごとの実装](#5-os-ごとの実装)
- [6. 今の自動との違い](#6-今の自動との違い)
- [7. 決めること](#7-決めること)

## 1. 何を決めたいか

調理・読み上げ・タイマーのように、画面を見続けるアプリで「画面を消さない」を、生成アプリの作り手(AI)が**明示的に書ける**ようにするか。それとも今のように、部品の種類で自動に決めたままにするか。

## 2. 今の自動の判定(事実)

画面を消さない仕組みは、DSL の props からは決められず、**使っている部品や処理が自分で持つ**。

| どこ | 何をする | 場所 |
|---|---|---|
| iOS 共通の持ち手 | `ScreenAwake.shared.hold()` で持ち、`release()` で戻す。持っている所が 1 つでもあれば消さない(他の使い手の分まで戻さないため) | `ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/ScreenAwake.swift` |
| iOS の使い手 | ドラレコ(`DashcamRecorder`)・道案内(`RouteGuideSession`)・`fileImport` の文字読み取り中 | `Support/DashcamRecorder.swift`・`Support/RouteGuideSession.swift`・`Components/Leaves/FileImportLeaf.swift` |
| iOS の実体 | iOS は `UIApplication.isIdleTimerDisabled`、Mac はディスプレイのスリープを止める電源の assertion(`kIOPMAssertionTypePreventUserIdleDisplaySleep`) | `Support/Platform/PlatformSystem.swift` の `setIdleTimerDisabled` |
| Android | ドラレコの部品が録画中だけ `view.keepScreenOn = true`(外れる時に false) | `android/renderer/src/main/kotlin/jp/kz2601/metamon/renderer/components/dashcam/DashcamLeaf.kt` |
| デスクトップ | 見当たらない(確認した範囲) | — |

- つまり今は、**`dashcam` と `routeGuide` と `fileImport` だけ**が画面を消さない。`timerDisplay`(筋トレ・ポモドーロ)・`speakButton`(読み上げ)・レシピのように見続ける画面は、OS の既定の時間で消える(推測: 確認した範囲では他の部品に keepAwake は無い)。
- 画面の props(`Screen`)にも部品の props にも、画面を消さない指定は無い(`dsl.schema.json` を確認)。

## 3. 案

### 案 A: 画面(`Screen`)の props に `keepAwake: true` を足す

```json
{"id": "cook", "title": "調理", "keepAwake": true, "components": [ ... ]}
```

その画面を表示している間だけ画面を消さない。画面から離れる・アプリが背面に回る時に戻す。

### 案 B: 部品の props に足す(動いている間だけ)

```json
{"type": "timerDisplay", "mode": "countdown", "seconds": 180, "keepAwake": true}
```

タイマーが動いている間など、部品が動いている間だけ。画面が複数の部品を持つ時は、今の `ScreenAwake` の持ち手の数え方(1 つでも持てば消さない)がそのまま使える。

### 案 C: 自動のまま(部品の種類で決める)

DSL は変えず、`timerDisplay` の動作中・`speakButton` の読み上げ中なども、部品が自分で持つ種類を増やす(OS 側の実装だけ)。

### 案 D: 自動を既定にして、画面で上書きできる

画面に `keepAwake: "auto" | true | false`(省略は `auto` = 今の自動)。`false` で「この画面ではドラレコ以外も消えていい」、`true` で明示。

## 4. 案の比べ表

| 案 | 良い点 | 困る点 | iOS | Android | Mac | デスクトップ |
|---|---|---|---|---|---|---|
| A: 画面の props | 書き方が 1 つで AI が書きやすい(推測)。「調理の画面は消さない」が言える | 画面を開いている間ずっと消えないので、電池を使う(省電力 #3026 との組み合わせは決めること)。書き忘れると消える | 作れる(`ScreenAwake`) | 作れる(`keepScreenOn` / `FLAG_KEEP_SCREEN_ON`) | 作れる(電源の assertion) | 未実装(下の表) |
| B: 部品の props | 動いている間だけで無駄が少ない。持ち手の数え方が今のまま使える | 対応する部品を数える(スキーマの変更が多い)。AI が「どの部品に付けるか」を間違えうる | 作れる | 作れる | 作れる | 未実装 |
| C: 自動のまま | スキーマ・AI の説明を変えずに済む。作り手が書き忘れても動く | どの部品が消さないかは作り手に見えない。「この画面だけ消したくない」が言えない | 作れる | 作れる | 作れる | 未実装 |
| D: 自動 + 上書き | 今の動きを壊さずに、明示もできる | 値が 3 通りで説明が増える。`auto` の中身を AI に教えるか決める必要がある | 作れる | 作れる | 作れる | 未実装 |

## 5. OS ごとの実装

| OS | 画面を消さない方法 | 今の状態 | 案を作る時に足すもの |
|---|---|---|---|
| iOS | `UIApplication.isIdleTimerDisabled`(`ScreenAwake` が持ち手を数える) | あり | 画面・部品から `hold()` / `release()` する橋渡しだけ |
| Mac | `IOPMAssertionCreateWithName`(ディスプレイのスリープを止める) | あり(同じ `ScreenAwake` の実体) | 同上 |
| Android | `View.keepScreenOn` か `Window.FLAG_KEEP_SCREEN_ON` | ドラレコの部品だけ | 共通の持ち手(iOS の `ScreenAwake` 相当)。複数の使い手が同時に持つ時の数え方(推測: Compose の `DisposableEffect` で足し引き) |
| デスクトップ(Windows・Linux) | Windows は `SetThreadExecutionState`・Linux は D-Bus の inhibit(推測。どちらも今のコードに無い) | なし | OS ごとの実体を新しく作る。JVM から呼ぶ方法は決めること |

## 6. 今の自動との違い

- 今の自動は「**処理が重要な間だけ**」(録画中・案内中・読み取り中)。案 A は「**画面を開いている間ずっと**」で、時間が長くなる。
- 今の自動は処理が終われば戻す。案 A・B は画面を離れる・背面に回る・部品が消える時に戻す仕組みが要る(`ScreenAwake` の `Hold` は `deinit` でも手放す設計)。
- 今は作り手が選べない。案 A〜D は作り手(AI)の判断を DSL に出すので、**使いすぎ**(すべての画面で true)を AI に避けさせる説明が要る。

## 7. 決めること

1. DSL に出すか(案 C のみ)。出すなら A / B / D のどれか。
2. 画面の props か部品の props か。
3. 電池が少ない・省電力の時は無視するか(#3026 の値を使うか)。
4. デスクトップの実体を作るか(作るまでは `disabledFeatures` で申告する案)。
5. 使える OS の申告のしかた([`../dsl/platforms.md`](../dsl/platforms.md)・[`../dsl/README.md` §5.3](../dsl/README.md#53-backend-が古い-ios-に新しい部品を出さない仕組み))。
