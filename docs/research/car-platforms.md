# CarPlay・Android Auto で生成アプリが当てはまるアプリの種類の調査メモ

> 調べた日: 2026-11-13。親: #1841 #1842(まとめ #1788)。調査だけで、決めない。コード・正典(`docs/00_decisions.md`・`docs/decisions/*`)は変えていない。
> 出典の内容は、下の URL を開いて読んだ範囲。読めなかった所は「未確認」と書く。**入るかどうかの判断は人**(推測は「推測」と書く)。

## 目次

1. [前提](#前提)
2. [CarPlay(Apple)の種類](#carplayappleの種類)
3. [Android Auto(Google)の種類](#android-autogoogleの種類)
4. [生成アプリが当てはまるか](#生成アプリが当てはまるか)
5. [決めること](#決めること)
6. [出典](#出典)

## 前提

- 生成アプリは、ユーザーが DSL で作る任意の中身(記録・stat・ボタンなど)。どの種類のアプリになるかが作るたびに違う。両社とも、車の画面に出せるのは **決まった種類のアプリだけ**で、種類は申請・審査で見られる(推測: 「何でも作れる殻」としては申請しにくい)。
- Android は今、車を対象外にしている([354-android-tech.md](../decisions/354-android-tech.md) の「Wear OS・TV・車は対象外」)。CarPlay・Android Auto を対象にするなら、この決定の見直しが要る(決めること 1)。
- 運転中の通話は、生成アプリの中で映像通話を使わせない方がよいと既に書いてある([424-calls-and-realtime.md](../decisions/424-calls-and-realtime.md))。

## CarPlay(Apple)の種類

Apple の CarPlay のページ([出典 1](#出典))が挙げる種類。権限(entitlement)は「Request CarPlay app entitlement」から申請する。手順の細かい中身はページに無く、未確認(CarPlay Developer Guide の PDF に全種類とテンプレートがあると書かれている・未読)。

| 種類 | 何をするアプリか | 生成アプリが入りうるか(推測) |
| --- | --- | --- |
| EV 充電・給油・駐車・公共安全・クイックフード・運転中の用事(driving task) | 電話を出さずに用事を済ませる | 作る中身が合う時だけ。汎用の殻としては入らない |
| 音声の会話(voice-based conversational) | 声で話すアプリ | aiChat の声(#743)が近いが、車向けの条件は未確認 |
| ナビ | 道案内。計器・ヘッドアップに案内を出せる | 入らない(経路の探索がアプリの本体) |
| ビデオ | 駐車中に動画を再生する | 入らない(動画再生が本体) |
| 音声(audio) | 音楽・ポッドキャスト | 読み上げ・音声ファイル(#737)を使うアプリだけ。汎用の殻としては入らない |
| メッセージ・VoIP | SiriKit の intent に対応する必要がある | 入らない(通話・メッセージが本体。生成アプリの通話は運転中に使わせない方針) |
| ウィジェット・Live Activity | 小さいウィジェットと Live Activity は CarPlay に自動で出る。CarPlay に最適化しないと印を付けることもできる | **入りうる**: 申請なしで出る種類(今の stat のウィジェットがそのまま出るかは未確認) |

## Android Auto(Google)の種類

Google の Car App Library のページ([出典 2](#出典))と、メディアのページ([出典 3](#出典))から。Google Play に出すには、決まった種類に入り、車用のアプリ品質の条件を満たす必要がある。

| 種類 | 何をするアプリか | 生成アプリが入りうるか(推測) |
| --- | --- | --- |
| IoT | 機器の操作(Car App Library) | 作る中身が合う時だけ |
| 天気 | 天気の表示(Car App Library) | 天気の `feed`(#659)を使うアプリだけ |
| ナビ | 道案内 | 入らない |
| メディア(音声) | `MediaBrowserService` / `MediaSession` で音楽などを出す。運転中のアプリ内の再生は、ナビの音声案内とメディアのアプリ以外は許されない | 汎用の殻としては入らない |
| 場所(POI) | 場所の情報(Car App Library) | `placeSearch`(#717)を使うアプリだけ |
| 駐車中のアプリ(ビデオなど) | 駐車中だけ使う | 入らない |

メッセージのアプリの扱いは、読んだページに無く未確認。

## 生成アプリが当てはまるか

- 両社とも、種類が決まっていて、アプリを 1 つの用途として申請する。ユーザーが好きに作る殻は、そのままでは種類に入らない(推測)。
- 入りうる物の候補: (a) CarPlay のウィジェット・Live Activity に今の stat を出す、(b) 天気・場所・IoT のように、中身が種類に合う生成アプリだけを出す。(c) 対象にしない。
- 運転中の操作の制限(画面の作り・文字数)に、生成アプリの自由な画面は合わない(推測)。

## 決めること

| 番号 | 決めること | 案 |
| --- | --- | --- |
| 1 | 車を対象にするか(Android の「車は対象外」の見直しを含む) | 対象外のまま / CarPlay のウィジェットだけ / 種類に合う物だけ |
| 2 | ウィジェットの stat が CarPlay にそのまま出るか | 実機・シミュレータで確かめる(未確認) |
| 3 | 申請(entitlement・Play の審査)を誰がいつ出すか | 人の作業 |

## 出典

1. Apple: CarPlay <https://developer.apple.com/carplay/>(種類・ウィジェット・entitlement の申請)
2. Android Developers: Car App Library <https://developer.android.com/training/cars/apps>(種類・Play の条件)
3. Android Developers: メディアのアプリ <https://developer.android.com/training/cars/media>(メディア・運転中の再生の制限)
4. Android Developers: 車のアプリ品質 <https://developer.android.com/docs/quality-guidelines/car-app-quality>(未読・条件の本体)
5. Apple: CarPlay Developer Guide <https://developer.apple.com/download/files/CarPlay-Developer-Guide.pdf>(未読・全種類とテンプレート)
