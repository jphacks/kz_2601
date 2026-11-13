# スモーク(cmd/smoke)の使い方(#287)

`backend/cmd/smoke` は、デモの発話を動いている ai(と appconfig)に実際に流し、ステップごとの結果を Markdown の表で出す。どれか 1 つでも FAIL なら終了コードが 1。デプロイの後の確かめと、デモの前の確かめに使う。

## 目次

1. [確かめる流れ](#1-確かめる流れ)
2. [回し方](#2-回し方)
3. [要る環境変数とフラグ](#3-要る環境変数とフラグ)
4. [どの環境で回すか](#4-どの環境で回すか)
5. [片付け(stg-cleanup との関係)](#5-片付けstg-cleanup-との関係)
6. [失敗した時の見方](#6-失敗した時の見方)
7. [決めること](#7-決めること)

## 1. 確かめる流れ

appId は `smoke-<UNIX 秒>`(`-app` で変えられる)。発話は `internal/ai/eval/cases.json` で測っているものと同じ文。

| 順 | ステップ | 確かめること |
|---|---|---|
| 1 | `health <URL>` | ai と appconfig の `GET /health` が 200 |
| 2 | トークン | `-token` > `-firebase-api-key`(Firebase の匿名サインアップ)> `$SMOKE_UID` か `dev-smoke` の順に決める |
| 3 | `auth probe` | `GET /v1/apps/<appId>-probe/dsl` が 200。401 なら下の「フォールバック」に切り替える |
| 4 | `A-0 GET /dsl (自動作成)` | 新しい appId が version 1 で作られる(appconfig がある時) |
| 5 | `A-1 家計簿 create` | mode が `full`・`dsl.Parse` が通る・area の重なりが 0 |
| 6 | `A-2 「今月の合計をもっと大きく表示して」 edit` | version が 1 つ進む・重なりが 0(patch でなく full になったら note に出す) |
| 7 | `A-3 「元に戻して」 undo` | mode が `restore`・create の版から戻す・DSL が create の時と同じ |
| 8 | `A-4 appconfig GET /versions` | 最新の版が `source.kind=restore` |
| 9 | `B 筋トレ create(通知)` | 毎日 08 時台の通知がある(appId `…-training`) |
| 10 | `C 散歩 create(写真)` | photoField の source が camera か both(appId `…-walk`) |
| 11 | `cleanup DELETE /v1/apps/<id>` | `-cleanup` の時だけ。作った appId を全部消す(204 = PASS・404 = WARN) |

- `-only kakeibo|training|walk` で 1 本だけ。Claude を呼んだステップは表の `claude ms` と `tokens` に数が入る。
- 記録(レコード)と共有(共有リンク・メンバー)は今は確かめていない。

**フォールバック**: appconfig がトークンを 401 で断った時(Firebase のキーが無い・匿名サインアップが失敗した)は、生成をせずに「トークン無しの `POST /v1/generate` が 401」だけを確かめる。表の上に `**FALLBACK**` と出る。

## 2. 回し方

`backend/` で打つ。入口は `backend/Makefile`。

| 入口 | 何をするか |
|---|---|
| `make smoke-local` | 手元で appconfig(memory store・`AUTH_MODE=dev`・:18080)と ai(`../.env` の API キー・:18081)を起動 → smoke → 止める。stg・本番に触れない |
| `FIREBASE_WEB_API_KEY=<key> make smoke-stg` | stg の ai / appconfig に流す |
| `FIREBASE_WEB_API_KEY=<key> make smoke-prod` | 本番に家計簿 1 本だけ流し、`-cleanup` で消す |
| `SMOKE_ONLY=kakeibo make smoke-local` | 1 本だけ(`smoke-local` / `smoke-stg`) |
| `deploy-cloudrun.yml` の「Smoke」 | stg と本番のデプロイの後に自動で回す。**止めない**(`continue-on-error`)。表は job summary に貼る |

直接打つ時:

```sh
cd backend
go run ./cmd/smoke -ai http://localhost:8081 -appconfig http://localhost:8080   # make dev で起動した手元(AUTH_MODE=dev)
go run ./cmd/smoke -ai <ai の URL> -appconfig <appconfig の URL> -firebase-api-key <key> -only kakeibo
```

## 3. 要る環境変数とフラグ

| 名前 | 要る時 | 意味 |
|---|---|---|
| `FIREBASE_WEB_API_KEY` | stg・本番 | `-firebase-api-key` の既定。Firebase Web API キー(アプリに入って配られる値。GitHub では repo の Variables) |
| `SMOKE_UID` | 手元(`AUTH_MODE=dev`) | dev のトークンにする uid。無ければ `dev-smoke` |
| `ANTHROPIC_API_KEY` | 手元 | smoke ではなく、手元で起動する ai が読む(`make smoke-local` は `../.env` から読む) |

| フラグ | 既定 | 意味 |
|---|---|---|
| `-ai` | `http://localhost:8081` | ai の URL |
| `-app` | `smoke-<UNIX 秒>` | appId の頭 |
| `-appconfig` | `http://localhost:8080` | appconfig の URL。空にすると appconfig での確かめ(A-0・A-4・auth probe・cleanup)を飛ばす |
| `-cleanup` | false | 最後に作った appId を `DELETE /v1/apps/{id}` で消す。`-appconfig` が要る |
| `-firebase-api-key` | `$FIREBASE_WEB_API_KEY` | 匿名サインアップに使うキー |
| `-only` | 空 | `kakeibo` / `training` / `walk` のどれか 1 本 |
| `-timeout` | `90s` | 1 リクエストの上限(デプロイの後は `120s`) |
| `-token` | 空 | Authorization に使うトークン(一番強い) |

## 4. どの環境で回すか

- 今のコードには「stg 以外では動かない」ような止め方は無い。どこに流すかは `-ai` / `-appconfig` で決まる。
- 本番で回すのは、デプロイの後の家計簿 1 本(`-only kakeibo -cleanup`)と `make smoke-prod` だけ。費用(Claude の呼び出し)と本番のデータを増やさないため。
- 手で試す時は `make smoke-local` を先に使い、stg は stg のデプロイを確かめたい時だけにする(推測: 手元で足りる確かめに stg の Claude と Firestore を使わないという今までのやり方)。
- App Check を `enforce` にした環境では、smoke は App Check のトークンを持たないので失敗する([app-check.md](app-check.md))。

## 5. 片付け(stg-cleanup との関係)

| 環境 | 作った物 | 片付け |
|---|---|---|
| 手元 | memory store の中だけ | 止めれば消える |
| stg | `smoke-<秒>`・`-training`・`-walk`・`-probe` のアプリ(`-cleanup` を付けないので残る) | [stg-cleanup](stg-cleanup.md) の `-smoke` で消す |
| 本番 | 同じ形のアプリ | `-cleanup` がその場で消す。FAIL の行が出たら残っている |

- stg-cleanup の `-smoke` が消すのは、appId が `smoke-<9〜11 桁の数字>` と、その `-training` / `-walk` / `-probe` / `-noauth` に**完全に一致**するものだけ(`store.SmokeAppIDRe`)。`-app` で頭を変えて回した分は消えないので、`-cleanup` を付けるか手で消す。
- Firebase の匿名サインアップで作られたユーザーは、どちらも消さない(stg-cleanup は Firestore だけを触る。[stg-cleanup.md の消すもの・消さないもの](stg-cleanup.md#消すもの消さないもの))。

## 6. 失敗した時の見方

出力の最後の行が `N steps, F failed, W warnings, total … ms (claude … ms)`。表の `result` が `**FAIL**` の行と `note` を見る。

| 出方 | 考えられること |
|---|---|
| `health …` が FAIL | サービスが起動していない・URL の打ち間違い・Cloud Run が落ちている |
| `Firebase 匿名サインアップ` が WARN | キーが違う・Firebase Auth で匿名が無効。dev のトークンに切り替わり、たいてい次の `auth probe` で FALLBACK になる |
| `auth probe` が WARN(401)で `**FALLBACK**` | appconfig が `AUTH_MODE=firebase` なのにトークンが無い。`FIREBASE_WEB_API_KEY` を渡す |
| 生成のステップが `status=4xx/5xx` | ai のログを見る([observability.md](observability.md))。429 は回数の上限(利用上限か Claude 側)・503 `ai_paused` は AI を止めている時([ai-pause.md](ai-pause.md)) |
| `dsl.Parse: …` | ai が返した DSL がスキーマに合わない。生成の不具合 |
| `mode …, want full` / `overlapping areas` / `no daily 08:xx notification` / `no photoField` | 生成の中身が期待と違う。1 回の揺れのこともあるので、`-only` でもう一度回してから Issue にする |
| `restored DSL differs …` | undo が create の版に戻っていない |
| `A-4` が WARN(`appconfig latest=… but ai returned …`) | ai が appconfig でなく自分のメモリに保存している(`APPCONFIG_BASE_URL` が無い) |
| `cleanup …` が FAIL | 本番に smoke のアプリが残っている。appId を見て消す |

デプロイの後の smoke は止めない設定なので、FAIL でもデプロイは成功になる。job summary の表を見る。

## 7. 決めること

- 記録・共有まで smoke で確かめるか(今は作成・編集・undo・版だけ)。
- stg 以外で回らないようにする止め方を入れるか(今は本番でも 1 本回している)。
- 匿名サインアップで増える Firebase Auth のユーザーをどう片付けるか。
