# クラウドのヘルス API の比較メモ(Google Health API・Garmin Connect)

> #2315(親 #1646)。入れるかどうかは #1666(Fitbit・Google のヘルス)・#1667(Garmin)で決める。これは調べたメモで、決定ではない。**料金は公式ページで確認**(ここには書かない)。確認日 2026-11-13。確認できなかったものは **未確認**。

## 目次

1. [結論(推奨・未確定)](#結論推奨未確定)
2. [比較表](#比較表)
3. [Google Health API](#google-health-api)
4. [Garmin Connect](#garmin-connect)
5. [端末の仕組みとの重なり](#端末の仕組みとの重なり)
6. [人が直す所](#人が直す所)

## 結論(推奨・未確定)

- **推奨・未確定**: どちらも最初は入れず、端末の仕組み(HealthKit #1663・Health Connect #1664)で足りる項目を先に出す。
- Google Health API は全 scope が「制限付き」で、プライバシーとセキュリティの審査が要る。サーバーにデータを置く runner は重い側に入る。
- Garmin は新規の申し込みが止まっていると報じられている(下記)。今は申し込めるか **未確認**。
- 入れる時の順: 端末の仕組み → (需要があれば)Google Health API の審査 → Garmin は再開を待つ。

## 比較表

| 項目 | Google Health API | Garmin Connect(Developer Program) |
|---|---|---|
| 位置づけ | 旧 Fitbit Web API の後継。旧 API は 2026-09-30 にサポート終了、2026-11-02 に止まる(以降 HTTP 410)と案内 | Garmin 端末の運動・健康データを渡すプログラム |
| 申し込み・審査 | 開発者の登録 + 全 scope が制限付きのため、プライバシーとセキュリティの審査(Google の App Verification) | 事業者向けは Garmin の申し込みと承認。個人向けの枠は自分のデータだけ |
| 今の状況 | v4beta は本番で使わない決まり。安定版 v4 は後で | 新規の申し込みが止まっている(報道。公式の発表は **未確認**) |
| 認可の方式 | Google OAuth 2.0(Google の標準ライブラリー)。scope は読む・書くを別々に頼む | OAuth の流れの詳細は公式の概要ページに記載なし。**未確認** |
| runner の OAuth に合うか | 合う(Google のカレンダー等と同じ系統。ただし scope が別で審査も別) | 流れが未確認のため判定できない |
| 取れる項目 | 活動・睡眠・栄養・ECG・女性の健康などのデータ型(詳しい一覧は公式の endpoint ページで確認。この調査では全項目を確認できず **未確認**) | 活動(JSON・.FIT)・健康・女性の健康・コース・トレーニング(層による) |
| 届き方 | 取得 + webhook。日中のデータあり | 層による(個人: 取りに行く / 入口の層: 押してくる / 上位: 両方) |
| 再許可 | 旧 Fitbit の利用者は再同意が要る(トークンは引き継げない) | — |
| サーバーに保存するか | 規約上の可否は **未確認**。保存しない(都度取って返すだけ)形が安全側 | 同上 **未確認** |

## Google Health API

- 位置づけ: Fitbit Web API からの移行先。Fitbit と Pixel Watch のデータ + 手入力を含む。([About](https://developers.google.com/health/about)・[移行ガイド](https://developers.google.com/health/migration)。確認日 2026-11-13)
- 認可: Google OAuth 2.0。scope の頭は `https://www.googleapis.com/auth/googlehealth`。登録にはプライバシーポリシーの URL・利用規約の URL・承認済みドメインが要る。([データの取得と認可](https://developers.google.com/health/migration/data-access))
- 審査: 「すべての scope が Restricted(制限付き)」でプライバシーとセキュリティの審査が要る。([About](https://developers.google.com/health/about))
- scope の変更: 2026-05-26 に、活動と運動・栄養・睡眠などの scope は「読む」か「書くだけ(`.writeonly`)」を明示する形になった。([リリースノート](https://developers.google.com/health/release-notes))
- 回数: トークンの発行に 1 分あたりと 24 時間ごとの上限がある(1 日に 1 万件超の同意が見込まれる時は事前に申請)。値は公式で確認。([データの取得と認可](https://developers.google.com/health/migration/data-access))
- 表示・保存・再配布の規約: 公式のこれらのページには記載なし。**未確認**(審査の申請書と API の規約を読む)。
- 健康のデータは要配慮として扱う(#1646)。

## Garmin Connect

- 層(個人・Starter・Enterprise)と使える API は [Garmin Connect Developer Program の概要](https://developer.garmin.com/gc-developer-program/overview) のとおり。個人の枠は自分のデータだけを取りに行く形。Starter は申し込みフォーム、Enterprise は問い合わせ。(確認日 2026-11-13)
- 新規の申し込み: 2026-05-20 に Garmin が「API プログラムの大きな見直し」を理由に、待ちの申し込みを処理できないと伝えたと報じられている。申し込みフォームは外されている。既存の連携は動いている。公式の発表は **未確認**(報道: [the5krunner 2026-09-14](https://the5krunner.com/2026/09/14/garmin-developer-api-access-paused/))。
- 規約(表示・保存・再配布): 概要ページに記載なし。利用規約・ブランドガイドラインを読む。**未確認**
- 結論: runner の「利用者ごとの OAuth」に載るかが不明。再開まで入れない。

## 端末の仕組みとの重なり

端末(HealthKit #1663・Health Connect #1664)で足りるか。端末の列はこの調査で公式を引いていない一般的な理解で、**未確認**(実装側の項目一覧で確かめる)。

| 項目 | 端末(HealthKit / Health Connect) | Google Health API | Garmin |
|---|---|---|---|
| 歩数・運動 | 足りる(端末に同期された分) | 取れる | 取れる(Activity) |
| 睡眠 | 足りる(連携アプリが書いた分) | 取れる | 取れる(Health) |
| 心拍 | 足りる | 取れる(日中データあり) | 取れる(Health) |
| 端末ベンダー独自の指標(回復・ストレスなど) | 載らないことがある。**未確認** | Fitbit の分は取れる | Garmin 独自の指標が取れる(項目は **未確認**) |
| 端末が手元に無い時のサーバー側の取得 | できない(端末が要る) | できる | できる |

読み方: 端末の仕組みで足りる項目は端末で読む。クラウドが要るのは「アプリを開いていない時にサーバーが取る」か「ベンダー独自の項目」だけ。

## 人が直す所

入れると決めた時だけ。

- プライバシーポリシー(`docs/legal/`)に、読む項目・保存の有無・保存期間・退会時の消去を足す。
- App Store の App Privacy と、Google Play のデータ セーフティの申告を直す。
- Google の OAuth の審査(動画・説明・ドメイン)を出す。
- Garmin は申し込みの可否を公式に問い合わせる。

※ 正典(`docs/00_decisions.md`・`docs/legal/*`・`docs/decisions/*`)はこのメモでは変えない。
