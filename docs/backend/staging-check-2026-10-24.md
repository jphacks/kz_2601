# staging 実ログイン疎通確認(2026-10-24)

> Issue #148。Firebase 匿名サインインで作った**本物の ID トークン 2 人分(u1 / u2)**で、staging の appconfig / ai を一通り叩いた結果。
> 本番(appconfig-prod / ai-prod)には一切リクエストしていない。GCP / Firestore の設定は変えていない。

## 目次
1. [対象と条件](#1-対象と条件)
2. [結果の一覧](#2-結果の一覧)
3. [遅延](#3-遅延)
4. [見つかった問題と起票した Issue](#4-見つかった問題と起票した-issue)
5. [本番と同じ Firestore を共用している件](#5-本番と同じ-firestore-を共用している件)
6. [後片付けと残ったテスト用データ](#6-後片付けと残ったテスト用データ)
7. [再実行の手順](#7-再実行の手順)

## 1. 対象と条件

| 項目 | 値 |
|---|---|
| appconfig | `https://appconfig-stg-378953239426.asia-northeast1.run.app`(デプロイ済み `GIT_SHA=fa597ee` = #135 マージ時点。以降の develop の差分は iOS / renderer のみ) |
| ai | `https://ai-stg-378953239426.asia-northeast1.run.app`(モデル `claude-sonnet-5-5`) |
| 認証 | `identitytoolkit.googleapis.com/v1/accounts:signUp` の匿名ユーザー 2 人。u1 = `c6fIdgLP6qai5afN77WpyYc0bUF3` / u2 = `HiQz2YWNfiTZiNxCl4bOzW4BbXc2`(確認後に両方削除済み) |
| 実施 | 2026-10-24 21:55〜22:05 JST、手元の Mac(回線は自宅/会場相当)から curl |
| Claude 呼び出し | 5 回(生成 ja / 生成 en / 対話 2 / 編集 1)。上限 15 回の範囲内 |
| 対象外 | リリース申請(PR #145 が未マージのため staging に無い) |

## 2. 結果の一覧

所要時間は curl の `time_total`(クライアントから見た往復)。

### 2.1 ヘルスチェック・認証

| 確認 | 結果 | HTTP | ms |
|---|---|---|---|
| appconfig `GET /health` | OK(`ok`) | 200 | 138 |
| ai `GET /health` | OK(`ok`) | 200 | 82 |
| appconfig / ai `GET /healthz` | Cloud Run の GFE が HTML 404(既知・`gcp-setup.md` 記載どおり)。ただし `openapi-appconfig.yaml` がまだ `/healthz` を載せている → #154 | 404 | 65 / 142 |
| `GET /v1/apps` トークン無し | OK(`unauthenticated`) | 401 | 78 |
| `GET /v1/apps` u1 の実トークン | OK(`{"items":[]}`) — **Firebase Admin SDK での ID トークン検証が staging で通ることを確認** | 200 | 208 |

### 2.2 生成・対話・編集(ai → appconfig)

| 確認 | 結果 | HTTP | ms |
|---|---|---|---|
| `GET /v1/apps/{新規id}/dsl`(seed) | OK。version 1(音声画面のみ) | 200 | 348 |
| `POST /v1/generate`「家計簿アプリが欲しい」 | OK。mode=full / source=claude / locale=ja / version 1→2。アプリ名「家計簿」、画面「家計簿」「支出を記録」、entity `expense`。Claude 5.6 s | 200 | 6,055 |
| `POST /v1/generate` "I want a budget app" + `locale:"en"` | OK。locale=en / version 1→2。アプリ名 "Budget"、画面 "Budget" / "New expense" / "Monthly budget"。**DSL 内にかな・漢字 0 件**(ラベルはすべて英語。1 つだけ `note` と小文字の id がそのままラベルに出ていた) | 200 | 8,320 |
| `GET /dsl` で version が上がったか | OK(version 2 を返す) | 200 | 129 |
| `POST /v1/chat`「日記」 | OK。`kind=question`「写真も一緒に残したいですか?」+ 選択肢 3 つ | 200 | 2,247 |
| `POST /v1/chat` 具体的な長文(日付・タイトル・本文・気分 5 段階・写真・新しい順・今月の平均・22 時通知) | OK。`kind=ready`、brief と生成用 transcript が返る(brief.entities にフィールド名が並ぶのは表示上の癖) | 200 | 2,631 |
| `DELETE /v1/chat/{appId}` | OK | 204 | 162 |
| `POST /v1/generate`(edit)「集計のタブを足して」 | OK。mode=patch / version 2→3 / ops 2(`/screens/-` に「集計」画面、`/app/tabs` にホーム・集計)。円グラフ(今月のカテゴリ別)と棒グラフ(月ごと) | 200 | 3,888 |

### 2.3 リアルタイム(`scripts/realtime-smoke.sh`)

| 回 | 結果 | u1 の public 投稿 → u2 の SSE に `records` が届くまで |
|---|---|---|
| 1 | OK | 286 ms |
| 2 | OK | 287 ms |
| 3 | OK | 218 ms |

スクリプト全体は 1 回 16 秒ほどかかる(SSE の購読プロセスの後始末待ち。結果には影響しない)。

### 2.4 シェア

| 確認 | 結果 | HTTP | ms |
|---|---|---|---|
| u1 が `POST /apps/{id}/share` | OK。token 32 文字、`tailorit://import/...` と web URL | 201 | 141 |
| u2 が `GET /shares/{token}`(プレビュー) | OK。`mine:false` / `sharedData:false` / components 一覧 | 200 | 95 |
| u2 が `POST /shares/{token}/import` | OK。`created:true` / `joined:false`、**新しい UUID の appId** に version 2(`source.kind=import`) | 201 | 456 |
| 複製 DSL と元 DSL(v3)の比較 | **完全一致**(`jq -S` で diff 0) | 200 | 93 |
| u2(他人)が `DELETE /shares/{token}` | OK(403 `only the owner can change this share`) | 403 | 186 |
| u1 が取り消し / 2 回目 | OK(冪等) | 204 / 204 | 139 / 199 |
| 取り消し後のプレビュー / import | OK(404 `share has been revoked`) | 404 / 404 | 111 / 117 |

### 2.5 探す

| 確認 | 結果 | HTTP | ms |
|---|---|---|---|
| `GET /public-apps?sort=popular` | OK。seed 済み 3 本(catalog-walk / catalog-training / catalog-kakeibo) | 200 | 151 |
| u2 が `POST /public-apps/catalog-kakeibo/install` | OK。新しい UUID に複製、installs 0 → 1 | 201 | 301 |
| u2 が同じものを 2 回目 install | **別の UUID にもう 1 つ複製され installs 1 → 2**(同じ人の連打で水増しできる)→ #154 | 201 | 283 |
| `GET /public-apps?q=家計簿` | OK(catalog-kakeibo のみ) | 200 | 109 |

### 2.6 認可(u2 が u1 のアプリに触る)

| 確認 | 結果 | HTTP | ms |
|---|---|---|---|
| u2 が u1 の `GET /apps/{id}` | OK(404) | 404 | 99 |
| u2 が u1 の `GET /apps/{id}/dsl` | OK(u1 の DSL は見えない)。仕様(#47)どおり **u2 自身の木に同じ appId の空アプリが seed** され、それが返る | 200 | 239 |
| u2 が u1 の appId で share | u2 の空アプリのシェアになる(u1 の DSL は出ない)= 漏えいなし | 201 | 129 |
| u2 が u1 の appId で publish | **NG: 200 で「探す」に掲載できた**。その後 u1 が自分のアプリを publish すると 403 → **#153(P1)** | 200 | 251 |
| u2 が u1 の versions / generate(undo) | u2 の空アプリに対する操作になり、u1 の DSL は version 3 のまま変わらない | 200 | 157 / 332 |
| 共有アプリ(u2 も参加済み)を u1 が先に掲載 → u2 が publish / unpublish | OK(403 `only the owner can change this listing`) | 403 / 403 | 130 / 173 |

### 2.7 リリース申請

PR #145 が OPEN(未マージ)なので staging に API が無く、**未確認**。マージ後に同じ要領で確認が必要。

## 3. 遅延

| 経路 | 実測 |
|---|---|
| appconfig の CRUD(GET/POST/DELETE) | 93〜456 ms(大半 100〜250 ms) |
| 生成 create(ja) | 6.1 s(Claude 5.6 s) |
| 生成 create(en) | 8.3 s(Claude 7.8 s) |
| 編集 patch | 3.9 s(Claude 3.6 s) |
| 対話 1 ラリー | 2.2〜2.6 s |
| リアルタイム(投稿 → 他端末の SSE) | 218〜287 ms |

## 4. 見つかった問題と起票した Issue

| Issue | 優先 | 内容 |
|---|---|---|
| [#153](https://github.com/jphacks/kz_2601/issues/153) | P1 | 他人の appId で「探す」に先に掲載でき、持ち主が publish できなくなる(appId の squat)。中身は初期 DSL のまま掲載される |
| [#154](https://github.com/jphacks/kz_2601/issues/154) | P2 | 軽微まとめ: 同じ人の install 連打で installs 水増し / OpenAPI の `/healthz` 記載 / appId の形式チェック無し(`null` でも seed) / summary のボタン位置が ja「右下」en "bottom left" で食い違う |

デモの主経路(生成 → 編集 → シェア → 探す → リアルタイム)は**すべて実トークンで通った**。

## 5. 本番と同じ Firestore を共用している件

**結論: 今回のテスト用データは本番からも見える状態(同じ DB・同じコレクション)。**

本番には一切リクエストせず、設定とコードで確認した。

- `gcloud run services describe` で両サービスの環境変数を確認: appconfig-stg も appconfig-prod も `GOOGLE_CLOUD_PROJECT=jphacks-kz2601`。違うのは `APP_ENV=stg|prod` だけ
- backend のコードは `APP_ENV` を参照しておらず、コレクション名は固定(`users/{uid}/apps/...`・`public_apps`・`apps_public`・`shares`)。DB は `(default)`
- Firebase Auth も同じプロジェクトなので、同じ uid のトークンなら本番でも同じアプリが見える

本番の利用者から実際に見えるもの:
- **「探す」の catalog-kakeibo の installs が 2 増えている**(今回のテストの install 2 回)。API では減らせない
- テスト掲載(RT-SMOKE-1792846653 と u2 の squat 掲載)は確認直後に取り下げ済みで、「探す」には残っていない
- それ以外(u1 / u2 のアプリ・シェア)は u1 / u2 本人にしか見えないので、本番の他ユーザーからは見えない

## 6. 後片付けと残ったテスト用データ

実施したこと:
- 全シェアを取り消し(u1 のもの、u2 の空アプリのもの)
- 掲載はすべて取り下げ(public_apps に残っていないことを確認)
- RT-SMOKE 3 本の公開レコードを `DELETE .../records?confirm=true` で全削除(残数 0 を確認)
- u1 / u2 の全アプリを `DELETE /v1/apps/{id}` でアーカイブ(論理削除。物理削除 API は無い)
- 匿名アカウント u1 / u2 を `accounts:delete` で削除。ID トークンのファイルも削除(refresh token は保存していない)

Firestore に残っているもの(物理削除するなら Firebase コンソールか `gcloud firestore` で。**本番と共用の DB なので、下記以外を消さないこと**):

| パス | 中身 |
|---|---|
| `users/c6fIdgLP6qai5afN77WpyYc0bUF3`(u1)配下すべて | `apps/` の `stgchk-ja-1792846555` `stgchk-en-1792846555` `RT-SMOKE-1792846618` `RT-SMOKE-1792846636` `RT-SMOKE-1792846653`(versions・records を含む。全部 archived) |
| `users/HiQz2YWNfiTZiNxCl4bOzW4BbXc2`(u2)配下すべて | `apps/` の `stgchk-ja-1792846555`(空)`null`(空)`07269ef5-8e04-498a-a282-afc761f06f6b`(シェアの複製)`0e610568-15c0-46a0-921c-03a0adc217bc` `f81ca3e0-4dba-40ae-97fd-f261daa98d2e`(探すの複製)`RT-SMOKE-1792846618/1792846636/1792846653`(参加) |
| `apps_public/RT-SMOKE-1792846618` `…1792846636` `…1792846653` | 共有アプリの DSL(レコードは削除済み) |
| `shares/` の取り消し済み 2 件 | appId が `stgchk-ja-1792846555` のもの |
| `public_apps/catalog-kakeibo.installs` | 2 増えた(戻すなら 2 減らす) |

例(u1 の木を消す。実行前に必ずパスを確認):

```sh
firebase firestore:delete "users/c6fIdgLP6qai5afN77WpyYc0bUF3" --recursive --project jphacks-kz2601
firebase firestore:delete "users/HiQz2YWNfiTZiNxCl4bOzW4BbXc2" --recursive --project jphacks-kz2601
firebase firestore:delete "apps_public/RT-SMOKE-1792846618" --recursive --project jphacks-kz2601
firebase firestore:delete "apps_public/RT-SMOKE-1792846636" --recursive --project jphacks-kz2601
firebase firestore:delete "apps_public/RT-SMOKE-1792846653" --recursive --project jphacks-kz2601
```

補足: `accounts:delete` の後も、発行済みの ID トークンで appconfig は 200 を返した(Admin SDK の検証で失効チェックをしていないため。トークンの寿命 1 時間で切れる)。匿名アカウント運用なら実害は小さい。

## 7. 再実行の手順

- トークン: `FIREBASE_WEB_API_KEY` を `GoogleService-Info.plist` の `API_KEY` から読み、`accounts:signUp?key=...` に `{"returnSecureToken":true}` を POST して `idToken` を使う(`scripts/seed-public-apps.sh` の `token()` と同じ)。キーとトークンは画面・ログに出さない
- リアルタイム: `U1_TOKEN=… U2_TOKEN=… scripts/realtime-smoke.sh https://appconfig-stg-378953239426.asia-northeast1.run.app`
- import / install の 201 本文はアプリ本体(キーは `id`)。`appId` ではない点に注意
- `DELETE /v1/apps/{id}/entities/{entity}/records` は `?confirm=true` が必須
- staging で作ったものは本番からも同じ DB に見えるので、appId に `stgchk-` などの接頭辞を付け、終わったら掲載取り下げ・アーカイブまで行う
