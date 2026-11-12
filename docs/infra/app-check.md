# Firebase App Check(#318)

ai / appconfig は Cloud Run で公開されていて、認可は Firebase の ID トークンだけです。匿名認証があるので、スクリプトで匿名ユーザーを量産すれば AI を叩けます。App Check は「正規の Tailor It(App Store / TestFlight の版)から来たリクエストか」を確かめます。

> **まだ有効にしていません。** コードは入っていますが、backend の既定は `APP_CHECK_MODE=off`(今と同じ動き)です。下の「人の作業」を順に行い、`log` で様子を見てから `enforce` にします。Firebase・GCP・GitHub の設定は承認を得てから人が行います。

## 目次
- [しくみ](#しくみ)
- [モード](#モード)
- [iOS の構成ごとのプロバイダ](#ios-の構成ごとのプロバイダ)
- [Android の構成ごとのプロバイダ](#android-の構成ごとのプロバイダ)
- [人の作業](#人の作業)
- [モードの切り替え](#モードの切り替え)
- [ログの見方](#ログの見方)
- [注意点](#注意点)

## しくみ

1. アプリは Firebase App Check の SDK でトークン(約 1 時間有効・SDK がキャッシュと更新を管理)を取り、ai / appconfig への全リクエストに `X-Firebase-AppCheck` ヘッダで付けます。iOS は `MetamonServices/Live/HTTP.swift`(SSE は `RealtimeDocumentSync`)、Android は `:services` の `MetamonHttp`(SSE の `openStream` も同じ・#1008)。取れない時はヘッダ無しで送ります(初回の App Attest / Play Integrity が 5 秒を超えた時も)。
2. backend は `internal/appcheck` のミドルウェアで、Firebase Admin SDK(公開鍵 JWKS を 6 時間ごとに取得)でトークンを検証します。サービスアカウントの権限は要りません。
3. ai は appconfig を呼ぶ時に、受け取った `X-Firebase-AppCheck` もそのまま転送します。

| サービス | 対象 | 対象外 |
| --- | --- | --- |
| ai | `/v1/*`(generate・chat・text・chat の削除)。上限を数える(#280)**前**に検証 | `/health`・`/healthz` |
| appconfig | 認証付きの `/v1/*` | `/health`・`/healthz`・共有ページ `/s/{token}`・`/v1/client-config`(古い版に「アップデートしてください」を出すため・#302) |

## モード

環境変数 `APP_CHECK_MODE`(ai / appconfig 共通):

| 値 | 動き | ヘッダが無い古いクライアント |
| --- | --- | --- |
| `off`(既定・未設定) | 何もしない | 通る |
| `log` | 検証して結果をログに書く。拒否しない | 通る(ログに `missing`) |
| `enforce` | 有効なトークン以外は 401 | **401** |

`enforce` で拒否した時の応答:

```json
{"error":{"code":"app_check_failed","message":"app check token is missing or invalid","details":{"reason":"missing"}}}
```

`reason` は `missing` / `invalid` / `wrong_app` / `unconfigured`。`unconfigured` は起動時に検証器を作れなかった時(JWKS の取得失敗など)で、`enforce` では**全部拒否**します(fail closed)。iOS はこのコードを ID トークンの失効と区別し、App Check のトークンを取り直して 1 回だけ再送し、それでも駄目なら「このアプリからの接続を確認できませんでした…」を出します(サインイン画面には戻しません)。

任意で `APP_CHECK_APP_IDS`(Firebase のアプリ ID をカンマ区切り)を入れると、それ以外のアプリのトークンを `wrong_app` にします。同じ Firebase プロジェクトに Web などのアプリを足した時に使います。

## iOS の構成ごとのプロバイダ

| 構成 | 実行先 | プロバイダ | App Attest の環境(entitlement) |
| --- | --- | --- | --- |
| Debug | シミュレータ・実機 | デバッグ用(`AppCheckDebugProviderFactory`) | development |
| Staging | 実機 | App Attest(非対応の端末は DeviceCheck) | development |
| Staging / Release / Review | シミュレータ | デバッグ用 | — |
| Release(TestFlight / App Store) | 実機 | App Attest(非対応の端末は DeviceCheck) | production |
| Review(審査用) | 実機 | App Attest(非対応の端末は DeviceCheck) | production |

- 選び方は `Metamon/App/MetamonApp.swift` の `appCheckMode`(`DEBUG` かシミュレータならデバッグ用)。
- entitlement `com.apple.developer.devicecheck.appattest-environment` は `Metamon.entitlements` / `Metamon-Review.entitlements` に `$(METAMON_APP_ATTEST_ENV)` で入れ、値は `project.yml`(既定 development、Release / Review は production)。
- `GoogleService-Info.plist` が無い時(CI・plist 未同期)は Firebase も App Check も動かず、ヘッダは付きません。CI のテストはモックなので影響しません。

## Android の構成ごとのプロバイダ

| 構成 | プロバイダ | 備考 |
| --- | --- | --- |
| debug(`stgDebug` / `prodDebug`。エミュレータ・実機) | デバッグ用(`DebugAppCheckProviderFactory`) | Firebase コンソールに登録したトークンだけ通る |
| release(`stgRelease` / `prodRelease`。Play・内部テスト) | Play Integrity(`PlayIntegrityAppCheckProviderFactory`) | Play で配った版だけ通る |

- 出し分けは build type のソースセット(`android/app/src/debug/.../AppCheckProviders.kt` と `android/app/src/release/.../AppCheckProviders.kt`)。依存も `debugImplementation` / `releaseImplementation` で分けてあるので、配布版にデバッグ用のプロバイダは入りません。
- 組み立ては `AppContainer.makeLive`(`FirebaseBootstrap.configureIfAvailable(context, AppCheckProviders.factory())` → `FirebaseAppCheckTokenProvider.createIfConfigured()` を `MetamonHttp` に渡す)。
- トークンが取れない・5 秒を超えた・Play 開発者サービスが無い時は null(ヘッダ無し)で先に進みます。**App Check の不調で通信は止めません**(iOS と同じ方針)。
- `google-services.json` が無いビルド(CI・json 未同期)は Firebase も App Check も動かず、ヘッダは付きません。CI のテストはモックなので影響しません。
- デバッグ用のトークンは初回起動時に logcat に出ます(`DebugAppCheckProvider` で検索)。**秘密情報**なのでコード・PR・Issue・チャットに貼らないでください。

## 人の作業

順番どおりに。stg で確かめてから prod。

### 1. Apple Developer: App Attest の capability

- 自動署名(`-allowProvisioningUpdates`)なら、次に実機ビルド / アーカイブした時に App ID `jp.kz2601.metamon` に **App Attest** が付き、プロファイルも作り直されます。
- 手動のプロファイル(配布用など)を使っている場合は、Developer サイトで App ID に App Attest を付け、**プロファイルを作り直して**入れ直します。古いプロファイルのままだと、entitlement が合わずに署名で失敗します。
- `METAMON_STG_BUNDLE_ID_SUFFIX=.stg` で別の Bundle ID を使う場合は、その App ID にも同じことをします。

### 2. Firebase コンソール: アプリを App Check に登録

1. Firebase コンソール →「App Check」→「アプリ」→ iOS アプリ(`jp.kz2601.metamon`)。
2. **App Attest** を登録(Team ID を入れる)。トークンの有効期間は既定(1 時間)のままでよい。
3. 任意: **DeviceCheck** も登録(Apple の DeviceCheck 用の鍵 `.p8`・Key ID・Team ID が要る。鍵は `~/Desktop/JPHACKS2026/secrets/` に置き、リポジトリには入れない)。登録しないと、App Attest 非対応の端末(ごく一部)は `enforce` で弾かれます。
4. stg 用に別の Firebase iOS アプリを作った場合(#288)は、そちらにも同じ登録をします。

### 3. App Check API を有効にする

GCP プロジェクトで `firebaseappcheck.googleapis.com` を有効にします(Firebase コンソールで App Check を開くと案内が出ます)。Terraform で管理するなら `infra/variables.tf` の API の一覧に足す案もあります(今は足していません)。

### 4. デバッグ用のトークンを登録

1. シミュレータ(または Debug 構成の実機)でアプリを起動し、Xcode のコンソールに出る `Firebase App Check debug token: <UUID>` を控える。決まった値を使いたい時は、scheme の環境変数 `FIRAAppCheckDebugToken` に UUID を入れる。
2. Firebase コンソール →「App Check」→ iOS アプリのメニュー →「デバッグ用トークンを管理」に追加(名前は「ko-tarou のシミュレータ」など)。
3. デバッグ用のトークンは**秘密情報**です。コード・PR・Issue・チャットに貼らないでください。漏れたらコンソールで消します。

### 5. Android: Firebase コンソールにアプリを登録(#1008)

Android は 1(App Attest の capability)が要りません。2〜4 の Android 版として行います。

1. Firebase コンソール →「App Check」→「アプリ」→ Android アプリ(`jp.kz2601.metamon` / stg は `jp.kz2601.metamon.stg`)。
2. **Play Integrity** を登録。Play Console と Firebase をリンクしておく(リンクしていないと Play Integrity の検証ができません)。
3. debug ビルドで起動して logcat に出るデバッグ用トークンを「デバッグ用トークンを管理」に追加(名前は「○○ のエミュレータ」など)。貼る先はコンソールだけ。
4. **`APP_CHECK_APP_IDS` を使っている場合は、Android の Firebase アプリ ID(`1:…:android:…`)を足します。**足さないと Android のトークンが `wrong_app` になります。backend のコードは変えません(値は GitHub の Environment 変数)。
5. `enforce` にする前に、App Check 入りの版を Play の最低対応版にする(#302 の Android 相当)。

### 6. `log` で様子を見る(stg → prod)

GitHub の Environment 変数 `APP_CHECK_MODE` を `log` にしてデプロイします([モードの切り替え](#モードの切り替え))。数日、[ログ](#ログの見方)で `valid` の割合を見ます。最新の版の正規のクライアントがほぼ `valid` で、`missing` が古い版(`clientBuild`)だけなら次へ。

### 7. `enforce` にする

- **先に、App Check 入りの版を最低対応版にする**(#302 の `MIN_IOS_BUILD`)。そうしないと古い版がすべて 401 になります(古い版は `/v1/client-config` が対象外なので「アップデートしてください」は出せます)。
- stg で `enforce` → iOS / Android の実機で生成・同期・共有を一通り確認 → prod で `enforce`。
- デプロイ後のスモーク(`cmd/smoke`)は App Check のトークンを持たないので、`enforce` の環境では失敗します(非ブロッキング)。必要なら別 Issue でスモーク用のトークンの取り方を足します。

### 8. 任意: Firebase Auth 自体にも App Check をかける

匿名ユーザーの作成そのもの(identitytoolkit)を守るには、Firebase コンソールの App Check で **Authentication** の「適用」をオンにします。iOS の FirebaseAuth は App Check が入っていれば自動でトークンを付けます。ただしスモークの匿名サインイン(`FIREBASE_WEB_API_KEY` の REST)も弾かれるので、オンにする前にスモークの扱いを決めます。

## モードの切り替え

**ふだんは GitHub の Environment 変数で切り替えます。** `APP_CHECK_MODE` はデプロイ(`.github/workflows/deploy-cloudrun.yml`)が毎回決める値なので、gcloud で手で変えた値は次のデプロイで戻ります([runtime-config.md](runtime-config.md))。

1. GitHub → Settings → Environments → `staging`(または `production`)→ Variables に `APP_CHECK_MODE` = `log` / `enforce` / `off`。
2. Actions →「deploy-cloudrun」を `workflow_dispatch` で流す(または次のマージを待つ)。

すぐに変えたい時(戻すのが急ぎの時など)は gcloud でも変えられます。ただし上の変数も同じ値にしておかないと、次のデプロイで戻ります:

```sh
gcloud run services update ai-stg        --region=asia-northeast1 --update-env-vars=APP_CHECK_MODE=off
gcloud run services update appconfig-stg --region=asia-northeast1 --update-env-vars=APP_CHECK_MODE=off
```

起動ログに `app check mode=<値>`(ai)/ `"msg":"app check","mode":"<値>"`(appconfig)が出ます。値が `off|log|enforce` 以外なら起動しません。

## ログの見方

`log` ではリクエストごとに 1 行、`enforce` では拒否した時だけ(警告)書きます。

```json
{"level":"INFO","msg":"appcheck","appCheck":"missing","mode":"log","method":"POST","path":"/v1/generate","clientBuild":"2026103001"}
```

Cloud Logging の例(`valid` 以外の内訳):

```
resource.type="cloud_run_revision"
jsonPayload.msg="appcheck"
jsonPayload.appCheck!="valid"
```

`clientBuild`(#302 のヘッダ)で、ヘッダ無しが古い版だけかを確かめます。

## 注意点

- App Check はデータを集めません(端末の正当性の確認だけで、ユーザーに紐づくデータも、トラッキングもありません)。`PrivacyInfo.xcprivacy` の収集データ・App Store の栄養ラベルは変わりません。Firebase の SDK は自前のプライバシーマニフェストを同梱しています。
- App Check は「改造していない正規のアプリか」を確かめるだけで、正規のアプリを使った大量の呼び出しは止めません。上限(#280)と匿名ユーザー作成の監視(#295)と組み合わせます。
- トークンは使い回せる種類です(再生防止の limited-use は使っていません)。ai → appconfig の転送はそのためです。
