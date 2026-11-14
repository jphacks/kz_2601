# App Links(Android でシェアリンクをアプリで直接開く・#1272)

シェアリンク `https://tailor-it.app/s/{token}` を、Android でもアプリが入っている端末ではアプリで直接開く。iOS の Universal Links([../universal-links/README.md](../universal-links/README.md))と同じ URL・同じサイトを使う。URL スキーム `tailorit://import/{token}` はそのまま使える。

## アプリ側(このリポジトリで済み)

- `android/app/src/main/AndroidManifest.xml`: `https` / `tailor-it.app` / `/s/` の intent-filter(`android:autoVerify="true"`)。
- 判定: `ShareLinks.token`(`android/services/.../sharing/ShareSheetModel.kt`)。iOS の `ShareURL.token(from:)` と同じ規則で、`https`・host がちょうど `tailor-it.app`・ポート指定なし・パスが `/s/{token}`(末尾の `/` とクエリは可)の時だけトークンを返す。
- 届いたリンクは `tailorit://` と同じ受け口(`ShareLinkIntents`)に入り、追加 / 参加の確認を出してから取り込む。

サイトに `assetlinks.json` が置かれて確認が通るまでは、端末はこのリンクをブラウザ(appconfig の共有ページ)で開く。アプリ側に害は無い。

## assetlinks.json

ひな形は [`assetlinks.json`](assetlinks.json)(このフォルダ)。`sha256_cert_fingerprints` に**配布に使う署名鍵の SHA-256 指紋**を入れてから置く。

- Play で配る版は Play App Signing の鍵の指紋(Play Console → アプリの完全性 → アプリの署名 → 「アプリ署名鍵の証明書」の SHA-256)。
- Play を通さずに配る版(内部の APK など)も開きたいなら、その鍵(アップロード鍵など)の指紋も配列に足す。
- stg(`jp.kz2601.metamon.stg`)も開きたいなら、`package_name` を変えた要素を配列に足す。足さなければ stg は URL スキームだけで開く(iOS の stg と同じ)。

置き方:

| 項目 | 値 |
|---|---|
| URL | `https://tailor-it.app/.well-known/assetlinks.json` |
| Content-Type | `application/json` |
| リダイレクト | しない(200 で直接返す) |
| 認証・Bot 対策 | かけない(Google が取りに来る) |

## 人の作業(順番どおりに)

1. **どの鍵を載せるかを決める**(上の「Play App Signing の鍵」が推奨)。指紋を Play Console から写す。
2. **サイト(tailor-it-web)に置く**: 指紋を入れた `assetlinks.json` を `public/.well-known/assetlinks.json` に置く。`/s/*` の中継は iOS の手順(../universal-links/README.md の 3)と共通。
3. **アプリの配布**: この変更を含むビルドを Play に出す。

## 確かめ方

```sh
curl -sI https://tailor-it.app/.well-known/assetlinks.json
curl -s "https://digitalassetlinks.googleapis.com/v1/statements:list?source.web.site=https://tailor-it.app&relation=delegate_permission/common.handle_all_urls"
adb shell pm get-app-links jp.kz2601.metamon
```

1 本目が `200` かつ `application/json`、2 本目に指紋が出ること、3 本目で `tailor-it.app: verified` になること。確認を取り直すには `adb shell pm verify-app-links --re-verify jp.kz2601.metamon`。
