# クラッシュ報告(Firebase Crashlytics・#296)

## 目次
- [方針](#方針)
- [コードの場所](#コードの場所)
- [送るもの・送らないもの](#送るもの送らないもの)
- [dSYM のアップロード](#dsym-のアップロード)
- [動作確認(わざと落とす)](#動作確認わざと落とす)
- [人がやること](#人がやること)

## 方針
- Firebase は認証で入っているので、同じ `firebase-ios-sdk`(13.1.1 固定)の `FirebaseCrashlytics` を足した。MetricKit の集計は使わない(端末から自前で送る仕組みとダッシュボードが別に要るため)。
- 依存は `MetamonServices` パッケージ(`Package.swift`)。アプリ側は Firebase を直接 import しない。
- 既定はオン。設定の「プライバシー」→「クラッシュ報告を送る」でオフにできる(プライバシーポリシー 1.8 に記載)。AI ではないので、AI への送信の同意とは別。
- 審査用の版(Review・`APP_STORE_REVIEW`)にも入れる。プライバシーマニフェスト(`ios/Metamon/PrivacyInfo.xcprivacy`)に「クラッシュのデータ」「パフォーマンスのデータ」(本人に紐づく・トラッキングなし・アプリの機能)を書いた。
- `GoogleService-Info.plist` が無い環境(CI・plist 未同期)では何もしない。

## コードの場所
| 何 | ファイル |
| --- | --- |
| 送り先の protocol・設定の保存(UserDefaults `metamon.crashReporting.enabled`) | `ios/Packages/MetamonServices/Sources/MetamonServices/Consent/CrashReporting.swift` |
| Crashlytics 実装 | `ios/Packages/MetamonServices/Sources/MetamonServices/Live/FirebaseCrashReporter.swift` |
| 起動時の反映・uid の付け外し | `ios/Metamon/App/MetamonApp.swift` |
| 設定の切り替え・テスト用クラッシュ | `ios/Metamon/Features/Consent/PrivacySettingsSection.swift` |
| dSYM を送るスクリプト | `ios/scripts/crashlytics-dsym.sh`(`project.yml` の postBuildScripts) |
| テスト | `ios/MetamonTests/CrashReportingTests.swift` |

## 送るもの・送らないもの
- 送る: SDK が自動で集めるクラッシュの情報(スタック・端末・OS・アプリの版・空きメモリ等)と uid。
- 送らない: 話した内容・入力した文・記録・DSL・名前・メール。
- `CrashReporter` protocol には `log` / カスタムキーをわざと持たせていない。足す時はポリシーとマニフェストを先に直す。
- オフにすると `setCrashlyticsCollectionEnabled(false)`(SDK が保存するので次の起動の最初から効く)+ 未送信の報告を消す + uid を外す。

## dSYM のアップロード
- Metamon ターゲットの最後のビルドフェーズ「Upload dSYM to Crashlytics」が `ios/scripts/crashlytics-dsym.sh` を呼ぶ。
- 送るのは **実機向けの Release / Review だけ**。Debug・シミュレータ・plist 無しは即終了する(ローカルのビルドを遅くしない・手元の確認ビルドで Firebase に書き込まない)。
- 中身は SPM のチェックアウトにある `firebase-ios-sdk/Crashlytics/run`。送信は裏で走るのでビルドは待たない。失敗は Console.app で `upload-symbols` を探す。
- Crashlytics の `run` はサンドボックスの外(SourcePackages・ネット)を使うので、Release / Review は `ENABLE_USER_SCRIPT_SANDBOXING: NO`。
- アーカイブ(`ios/scripts/build-review.sh archive` や Xcode の Archive)で送られる。送れなかった時は手で送る:

```sh
<DerivedData>/SourcePackages/checkouts/firebase-ios-sdk/Crashlytics/upload-symbols \
  -gsp ios/Metamon/Resources/GoogleService-Info.plist -p ios <Archive>.xcarchive/dSYMs
```

- CI(`ios-ci.yml`)はシミュレータ向けで plist も無いので送らない。CI から送るなら、plist を secrets から置いて実機向けアーカイブを作る工程が要る(今は無い)。
- 送り先の Firebase プロジェクトは `GoogleService-Info.plist` のもの。prod の plist への切り替えは #292。

## 動作確認(わざと落とす)
1. Debug か Release の版を実機に入れる(審査用の Review にはボタンが無い)。シンボル付きで見るには Release をアーカイブして dSYM を送る。
2. Xcode のデバッガを外す(Xcode で止めてから、ホーム画面のアイコンで起動する。デバッガが付いていると Crashlytics が拾えない)。
3. 設定 → プライバシー →「テスト用にクラッシュさせる」(Debug と、Xcode から入れた版・TestFlight の版にだけ出る。審査用の版には無い)。
4. もう一度起動する(報告は次の起動で送られる)。数分で Firebase Console の Crashlytics に出る。

## 人がやること
- Firebase Console で、対象のプロジェクト・iOS アプリの Crashlytics を有効にする(初回の報告が届くまでダッシュボードは「待機中」)。
- 上の手順でテスト用のクラッシュを 1 回送り、シンボル付きで見えることを確かめる(#296 の完了の条件)。
- クラッシュなし率は Crashlytics のダッシュボード(「クラッシュフリー ユーザー」)で見る。
- App Store Connect のプライバシーの表示(栄養ラベル)に「クラッシュのデータ」「パフォーマンスのデータ」(本人に紐づく・トラッキングなし・アプリの機能)を足す(#328)。
- プライバシーポリシー 3 章の保存期間(90 日)の [要確認] を、Firebase の現在の説明と照らして外す。
