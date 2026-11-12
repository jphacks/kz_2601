# Universal Links(シェアリンクをアプリで直接開く・#331)

シェアリンク `https://tailor-it.app/s/{token}` を、アプリが入っている端末ではアプリで直接開く。入っていない端末では今までどおり Web の共有ページ(appconfig の `GET /s/{token}`・`backend/internal/appconfig/sharepage.go`)が出る。URL スキーム `tailorit://import/{token}` もそのまま使える(QR・共有ページの「アプリで開く」)。

## 目次

1. [全体の形](#全体の形)
2. [アプリ側(このリポジトリで済み)](#アプリ側このリポジトリで済み)
3. [AASA ファイル](#aasa-ファイル)
4. [人の作業(順番どおりに)](#人の作業順番どおりに)
5. [確かめ方](#確かめ方)
6. [注意](#注意)

## 全体の形

| 部分 | 中身 | 場所 |
|---|---|---|
| リンクの形 | `https://tailor-it.app/s/{token}`(token は 1 区切り・英数字と `_-.:`・128 文字まで) | backend が返す `webUrl`(`SHARE_WEB_BASE_URL` + `/s/{token}`) |
| アプリ | entitlement `applinks:tailor-it.app`。届いた URL を `tailorit://import/{token}` と同じ取り込み / 参加の流れに渡す | `ios/Metamon/Metamon.entitlements`・`ShareURL`・`ShareLinkReceiver` |
| サイト | `/.well-known/apple-app-site-association` を置く。`/s/*` を appconfig の共有ページに中継する | 別リポジトリ(tailor-it-web・非公開)。**人の作業** |
| backend | `SHARE_WEB_BASE_URL=https://tailor-it.app` にする(コードの変更は無し) | appconfig の環境変数。**人の作業** |

2026-11-10 時点の tailor-it.app は Cloudflare の上の Next.js で、`/s/abc` と `/.well-known/apple-app-site-association` はどちらも 404。

## アプリ側(このリポジトリで済み)

- `ios/project.yml` の `METAMON_APPLINKS_DOMAIN`(既定 `tailor-it.app`)を `Metamon.entitlements` の `com.apple.developer.associated-domains` に `applinks:$(METAMON_APPLINKS_DOMAIN)` として展開する。Debug / Staging / Release の全部に入る。
- 審査用の版(Review・`Metamon-Review.entitlements`)には入れない。審査用の版は共有リンクの入口(`SharingHooks` など)をバイナリから外しているため(`docs/ios/review-build.md`)。共有を戻す時に一緒に足す。
- 受け取り: `ShareLinkReceiver` が `onOpenURL`(URL スキーム)と `onContinueUserActivity(NSUserActivityTypeBrowsingWeb)`(Universal Links)の両方を `SharingUI.receive` に渡す。サインイン前に届いても保留し、準備ができてから 1 回だけ開く(#278 と同じ)。
- 判定: `ShareURL.token(from:)`。`https` で host が `tailor-it.app`(大文字小文字は問わない)・ポート指定なし・パスがちょうど `/s/{token}` の時だけトークンを返す。`www.`・`http`・他のパスは扱わない。テスト: `SharingTests.testShareURLTokenFromUniversalLink`・`ShareLinkInboxTests.testReceivesUniversalLinkActivity`・`ShareJoinFlowTests.testOpenUniversalLinkRoutesToSameJoinFlow`。

### Mac(#1286)

- `ios/MetamonMac/MetamonMac.entitlements` にも同じ `applinks:$(METAMON_APPLINKS_DOMAIN)` を入れた。審査用の版(`MetamonMac-Review.entitlements`)には入れない(iOS と同じ理由)。
- 受け取り: `MacLinkReceiver` が `onOpenURL` と `onContinueUserActivity(NSUserActivityTypeBrowsingWeb)` を `MacShareFlow` に渡す。ログインの前に届いたら保留し、ログインできてから 1 回だけ開く(`PendingShareLink`)。
- AASA: Mac の Bundle ID は iOS と同じ `jp.kz2601.metamon` なので、`appIDs` の `CHP337D75C.jp.kz2601.metamon` が Mac にも効く(Mac 用の行は足さない)。Mac App Store の版の App ID の接頭辞が Team ID と違う場合だけ、その `<接頭辞>.jp.kz2601.metamon` を `appIDs` に足す。
- Apple Developer: Mac の App ID(iOS と同じ ID)の Associated Domains がオンになっていること。Mac は AASA を `swcd` が取りに行く。確かめ方: Mac の Safari で `https://tailor-it.app/s/<token>` を開くとアプリで参加の画面が出る(出ない時は `swcutil show` で `tailor-it.app` の状態を見る)。

## AASA ファイル

中身は [`apple-app-site-association`](apple-app-site-association)(このフォルダ)。そのままサイトに置く。

- `appIDs`: `CHP337D75C.jp.kz2601.metamon`(Team ID + Bundle ID)。stg の別 Bundle ID(`.stg`)は入れていないので、stg 版では Universal Links は効かない(URL スキームは効く)。
- `components`: `/s/?*`(token が 1 文字以上)だけ。`/s/*/*`(区切りが 2 つ以上)は除く。サイトの他のページ(`/terms` など)はアプリに来ない。

置き方の決まり(Apple の要件):

| 項目 | 値 |
|---|---|
| URL | `https://tailor-it.app/.well-known/apple-app-site-association`(拡張子なし) |
| Content-Type | `application/json` |
| リダイレクト | しない(200 で直接返す。`www.` への転送や末尾 `/` の付け足しもしない) |
| 認証・Bot 対策 | かけない(Cloudflare の Bot Fight Mode / チャレンジがこのパスに出ないこと。Apple の CDN が取りに来る) |
| 大きさ | 128KB 以下(今は 1KB 未満) |

## 人の作業(順番どおりに)

1. **Apple Developer**: Certificates, Identifiers & Profiles → Identifiers → `jp.kz2601.metamon` → Capabilities で **Associated Domains** をオンにして保存。自動署名(`-allowProvisioningUpdates`)でも付くが、配布用の手動のプロファイルを使っているなら作り直す。
2. **サイト(tailor-it-web)に AASA を置く**: このフォルダの `apple-app-site-association` を `public/.well-known/apple-app-site-association` にコピーする。拡張子が無いので Content-Type が `application/octet-stream` になりやすい。次のどれかで `application/json` にする。
   - Next.js のルートハンドラ `app/.well-known/apple-app-site-association/route.ts` で JSON を返す(`Response.json(...)`)
   - Cloudflare(Workers / Pages)の `_headers` に `/.well-known/apple-app-site-association` → `Content-Type: application/json`
   - Next.js の `next.config` の `headers()` で同じ指定
3. **サイトの `/s/*` を appconfig の共有ページに中継する**: `https://tailor-it.app/s/{token}` を `https://appconfig-prod-378953239426.asia-northeast1.run.app/s/{token}` に**プロキシ**する(URL は tailor-it.app のまま)。例: Next.js の `rewrites()` に `{ source: '/s/:token', destination: 'https://appconfig-prod-378953239426.asia-northeast1.run.app/s/:token' }`。
   - appconfig の応答ヘッダ(`Content-Security-Policy`・`Referrer-Policy: no-referrer`・`Cache-Control`・404 の `no-store`)は消さずにそのまま返す。サイト側で CDN キャッシュしない(取り消しが最大 1 分で効く前提・`docs/backend/features/sharing.md`)。
   - プロキシが無理なら 302 で appconfig の `/s/{token}` に転送でもよい(未インストールの人は共有ページを見られる)。ただしアドレス欄が Cloud Run の URL になる。
4. **backend の環境変数**: 3 が本番で動くのを確かめてから、appconfig(prod)に `SHARE_WEB_BASE_URL=https://tailor-it.app` を入れる(JPHACKS5 の作業)。これで新しく出すシェアリンクの `webUrl` と共有ページの og:url が `https://tailor-it.app/s/{token}` になる。**3 より先に入れるとリンクが 404 になる。** stg は今のまま(stg の Bundle ID は AASA に無い)。
5. **アプリの配布**: この変更を含むビルド(通常の版)を TestFlight に出す。

## 確かめ方

1. サイト:
   ```sh
   curl -sI https://tailor-it.app/.well-known/apple-app-site-association
   curl -s https://tailor-it.app/.well-known/apple-app-site-association
   curl -sI https://tailor-it.app/s/<有効な token>
   ```
   1 本目が `200` かつ `content-type: application/json`(`location` が無い)、2 本目が上の JSON、3 本目が `200` で共有ページが返ること。
2. Apple の CDN(サイトに置いてから反映まで最大 1 日ほどかかる):
   ```sh
   curl -s https://app-site-association.cdn-apple.com/a/v1/tailor-it.app
   ```
   同じ JSON が返ること。404 や古い中身なら待つ。
3. 端末: 新しいビルドを入れ、メモや LINE などから `https://tailor-it.app/s/<有効な token>` をタップ → アプリが開いて「シェアされた〜を追加しました」(記録を共有するアプリなら参加の確認)。アプリを消した端末では Safari に共有ページが出る。
   - 開発中は 設定 → デベロッパ → Associated Domains Development を使うと CDN を待たずに試せる(その時は entitlement を一時的に `applinks:tailor-it.app?mode=developer` にする。コミットしない)。
   - Safari のアドレス欄に直接打った URL・tailor-it.app のページ内のリンクではアプリは開かない(Apple の仕様)。共有ページの「アプリで開く」は URL スキームのままにしているのはこのため。

## 注意

- 共有ページを Safari で開いた後に右上の「tailor-it.app」で Safari に戻すと、その端末ではしばらく Universal Links が Safari 優先になる(Apple の仕様)。長押し →「"Tailor It"で開く」で戻る。
- QR は今も `tailorit://import/{token}`。`https://tailor-it.app/s/{token}` にすると未インストールの人も共有ページを見られるが、上の人の作業が全部終わってからの別の変更にする。
