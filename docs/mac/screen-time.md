# Mac でスクリーンタイムの部品を使えるか(調べた結果)

#1797 の調べた結果です。使いすぎの見守り(スクリーンタイム・アプリの使用時間)の部品を、Mac 版の生成アプリで出せるかをまとめます。入れるか・どの形で入れるかはオーナーが #1792 で決めます。このページは判断の材料で、決まりではありません。

## 目次

- [結論](#結論)
- [OS ごとの対応の範囲](#os-ごとの対応の範囲)
- [entitlement と審査](#entitlement-と審査)
- [今のアプリ](#今のアプリ)
- [OS ごとの扱い(推奨)](#os-ごとの扱い推奨)
- [Mac の代わりの案](#mac-の代わりの案)
- [オーナーに決めてほしいこと](#オーナーに決めてほしいこと)
- [出典](#出典)

## 結論

- FamilyControls・ManagedSettings・DeviceActivity は **iOS / iPadOS と Mac Catalyst だけ**。**ネイティブの macOS(AppKit・SwiftUI の macOS ターゲット)には無い**。
- Tailor It の Mac 版はネイティブの macOS ターゲット(`ios/project.yml` の `MetamonMac`・macOS 14)なので、**今の作りのままでは使えない**。
- 推奨: Mac は `screenTimeStat` を「この Mac では使えません」の枠にする(`docs/decisions/351-dsl-cross-platform.md` の 2 段の受け方)。Mac だけで別の作りは今は入れない。

## OS ごとの対応の範囲

Apple のドキュメントの `platforms` の値です(2026-11-12 に確かめた)。

| フレームワーク | できること | iOS / iPadOS | Mac Catalyst | ネイティブ macOS |
| --- | --- | --- | --- | --- |
| FamilyControls | 見守りの許可を取る(`AuthorizationCenter`)・アプリとカテゴリを選ぶ(`FamilyActivityPicker`) | 15.0 から | 15.0 から | **無し** |
| ManagedSettings | アプリ・カテゴリ・Web サイトに制限をかける(シールド)・購入や設定を止める | 15.0 から | 15.0 から | **無し** |
| DeviceActivity | 決めた時間帯・しきい値の監視(`DeviceActivityMonitor`)・使用時間の表示(`DeviceActivityReport`) | 15.0 から(Report は 16.0 から) | 15.0 から(Report は 16.0 から) | **無し** |

どれにも共通する制限:

- 許可は 2 通り。子ども(同じファミリー共有の保護者が承認)と本人(端末の持ち主が生体認証で承認)。
- 使用時間の値は `DeviceActivityReport` の拡張(`com.apple.deviceactivityui.report-extension`)の中でしか見えない。拡張はサンドボックスで、**ネットワークに出せず、値を拡張の外へ出せない**。アプリは拡張が描いた View を表示するだけ。式・集計・サーバー保存には使えない(#1792 の「気をつけること」と同じ)。
- 選んだアプリ・Web サイトは名前でなくトークン(`ApplicationToken` など)で渡る。AI やサーバーはアプリの名前を知れない。

Mac Catalyst には `platforms` 上は載っているが、Mac で実際に他のアプリの使用時間が取れるかは Apple の文書に書かれていない。Catalyst に移す案は実機で確かめる前提です(下の選択肢)。

## entitlement と審査

- `com.apple.developer.family-controls`(Boolean)。Xcode で Family Controls の capability を足すと付く。開発用の署名ならそのまま試せる。
- **配布(TestFlight・App Store)には Apple の承認が要る**。申請は [Family Controls の配布の申請](https://developer.apple.com/contact/request/family-controls-distribution) から。承認は Bundle ID ごとなので、本体と拡張(Report・Monitor)の分を出す。
- entitlement の `platforms` は iOS・iPadOS・Mac Catalyst・visionOS。**ネイティブ macOS は無い**ので、Mac 版(`MetamonMac`)に付けても使えない。Catalyst にした場合に iOS と同じ申請で足りるかは Apple の文書に書かれていない(#1797 の「確かめること」は未解決。申請の時に Apple に聞く)。
- 審査の決まり 4.10: **Screen Time API など OS の機能そのものにお金を取れない**。`screenTimeStat` を Pro だけの部品にすると審査で落ちる恐れがある。無料の部品にする。
- 審査用の版(`APP_STORE_REVIEW`・`docs/ios/review-build.md`)では、承認が下りるまで `supportedFeatures` に入れず、バイナリからも外す。

## 今のアプリ

`origin/develop` と `origin/epic/ios` で `FamilyControls|ManagedSettings|DeviceActivity|screenTime` を `git grep` した結果(2026-11-12):

- コードには何も無い(iOS・Mac とも)。entitlement のファイル(`ios/Metamon/Metamon.entitlements`・`ios/MetamonMac/MetamonMac.entitlements`)にも無い。
- 書かれているのは `docs/dsl/README.md` の 1 行だけ:「スクリーンタイム(FamilyControls)は権限と entitlement が要るので入れていない」。
- iOS は #1792(DeviceActivityReport)と entitlement の申請の Issue が先。Mac はその後。

## OS ごとの扱い(推奨)

| OS | 扱い | 理由 |
| --- | --- | --- |
| iOS / iPadOS | **作る**(#1792) | API がそろっている。entitlement の承認が下りてから配布 |
| Android | **代わりの作り**(UsageStatsManager・別 Issue) | 使用時間は取れる。特別なアクセスの許可が要る |
| Mac(ネイティブ) | **「この Mac では使えません」** | フレームワークが無い。appconfig で置き換え → 残ったらクライアントの枠(決定 351 の 2 段) |
| Mac(Catalyst にした場合) | 今は作らない | Mac 版を Catalyst に変えるのは大きすぎる(`docs/decisions/363-desktop-strategy.md` でネイティブを選んだ)。実機で動くかも不明 |

Mac の扱いの具体:

- Mac のクライアントは `supportedFeatures` に `screenTimeStat`(名前は #1792 で決まる)を**送らない**。AI は Mac では作らない。
- iOS で作ったアプリを Mac で開いた時は、`screenTimeStat` を「この Mac では使えません」の小さい枠にする。文言は「スクリーンタイムは iPhone で見られます」を添える。

## Mac の代わりの案

ネイティブの macOS で取れる範囲。どれもスクリーンタイムと同じ値にはならない。

| 案 | 取れるもの | 弱い所 |
| --- | --- | --- |
| A. `NSWorkspace.didActivateApplicationNotification` で前面のアプリを記録 | Tailor It が動いている間の、前面にあったアプリと時間 | アプリを閉じると数えられない。Web サイトは分からない。審査で「何のために他のアプリを見るか」の説明が要る |
| B. 生成アプリ自身の使用時間だけ | そのアプリを開いていた時間 | 「SNS の時間」には使えない |
| C. 作らない(推奨) | なし | iPhone で見る前提にする |

A は端末の中だけで値を持つなら作れるが、iOS と別の部品になる(値を式に使えてしまうなど動きが違う)。#1797 の例(Mac での作業中の SNS の時間)には一番近いが、ブラウザの中の SNS は分からない。

## オーナーに決めてほしいこと

#1792 の判断と一緒に決める。推奨は「この Mac では使えません」。

| 選択肢 | 意味 |
| --- | --- |
| この Mac では使えません(推奨) | Mac は `supportedFeatures` に出さず、iOS で作った部品は枠を出す。追加の作りは枠だけ |
| 前面のアプリを記録する Mac 専用の部品(案 A) | Mac だけの別部品を作る。開いている間だけの値で、審査の説明が要る |
| Catalyst で試す | Mac Catalyst の試作で他のアプリの使用時間が取れるか実機で確かめる。取れても Mac 版の作りの見直しが要る |
| iOS も含めて入れない | #1792 ごと見送る。entitlement の申請もしない |
| 明日に送る | 今日は決めない |

## 出典

- [Family Controls](https://developer.apple.com/documentation/familycontrols)(platforms: iOS 15.0・iPadOS 15.0・Mac Catalyst 15.0)
- [Managed Settings](https://developer.apple.com/documentation/managedsettings)(同上)
- [Device Activity](https://developer.apple.com/documentation/deviceactivity)(同上)
- [DeviceActivityReport](https://developer.apple.com/documentation/deviceactivity/deviceactivityreport)(iOS 16.0・iPadOS 16.0・Mac Catalyst 16.0。拡張のサンドボックスの記述)
- [com.apple.developer.family-controls](https://developer.apple.com/documentation/bundleresources/entitlements/com.apple.developer.family-controls)(iOS・iPadOS・Mac Catalyst・visionOS。配布前に申請が要る)
- [App Review Guidelines 4.10](https://developer.apple.com/app-store/review/guidelines/#monetizing-built-in-capabilities)(Screen Time API にお金を取れない)
- [NSWorkspace.didActivateApplicationNotification](https://developer.apple.com/documentation/appkit/nsworkspace/didactivateapplicationnotification)(macOS 10.6 から)
