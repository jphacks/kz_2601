# 公式サイトのページの構成(下書き)

#331。公式サイトに置くページと、それぞれの中身をこのリポジトリのどの文書から取るかの下書き。サイトのソースは別のリポジトリ(tailor-it-web・非公開。[Universal Links の README](../infra/universal-links/README.md) の「全体の形」)にあり、このリポジトリには無い。

URL は、今の文書・コードに書いてある値をそのまま写した。決まっていないことは最後の「人が決めること」にまとめた。

## 目次

1. [ページの一覧](#1-ページの一覧)
2. [ストアの欄との対応](#2-ストアの欄との対応)
3. [同じドメインに置く仕組み](#3-同じドメインに置く仕組み)
4. [人が決めること](#4-人が決めること)

## 1. ページの一覧

| ページ | パス | 中身 | 中身の出どころ |
|---|---|---|---|
| 紹介 | `/` | 何ができるか・使い方の 3 歩・スクリーンショット・App Store へのリンク | [`README.md`](../../README.md) の「使い方」「製品説明」・[`docs/store/listing.md`](../store/listing.md) の説明文・スクリーンショット |
| サポート(問い合わせ) | `/contact` | 問い合わせ先(support@tailor-it.app)・送ってほしい情報(設定の「詳細をコピー」)・FAQ へのリンク | [`docs/support/operations.md`](../support/operations.md) の 1 章「窓口」 |
| よくある質問 | `/faq` | 質問と答え(日本語・英語) | [`docs/support/faq.md`](../support/faq.md)(公開の前の作業は同じ文書の 1 章) |
| プライバシーポリシー | `/privacy` | 本文 | [`docs/legal/privacy-policy.md`](../legal/privacy-policy.md) |
| 利用規約 | `/terms` | 本文 | [`docs/legal/terms.md`](../legal/terms.md) |
| 特定商取引法に基づく表記 | 未定 | 未定 | 無い。要るかどうかは #330(人)。[`docs/release/app-review-playbook.md`](../release/app-review-playbook.md) の #330 の行 |
| 共有ページ | `/s/{token}` | 共有されたアプリの画面を読むだけで描く(記録は出さない)・「アプリで開く」 | サイトでは作らない。appconfig の `GET /s/{token}`(`backend/internal/appconfig/sharepage.go`)へ中継する。[`docs/backend/features/sharing.md`](../backend/features/sharing.md) |
| アカウントの削除 | `/account/delete` | アプリの中での消し方と、アプリが無い時のメールでの依頼 | サイトでは作らない。appconfig の `GET /account/delete`(`backend/internal/appconfig/accountdelete_page.go`。コメントに「Google Play's account deletion URL」「share page's domain に置く」)。[`docs/backend/features/account-deletion.md`](../backend/features/account-deletion.md) |

サイトに置く文は、出どころの文書を直してから写す(サイトの側だけを直さない)。

## 2. ストアの欄との対応

### App Store Connect

値は [`docs/store/listing.md`](../store/listing.md) の「ASC の欄」から。

| 欄 | 値 | ページ |
|---|---|---|
| マーケティング URL | https://tailor-it.app/ | 紹介 |
| サポート URL | https://tailor-it.app/contact | サポート |
| プライバシーポリシー URL | https://tailor-it.app/privacy | プライバシーポリシー |
| 利用規約 | 説明文に https://tailor-it.app/terms を書く(EULA の欄は標準のまま。同じ文書に「推測」とある) | 利用規約 |

### Google Play Console

このリポジトリの文書には、Play Console の欄に入れる値の表がまだ無い。下は Play Console の一般的な欄と、合いそうなページの対応(**推測**。入れる前に人が確かめる)。

| 欄 | 合いそうなページ |
|---|---|
| ストアの設定の「ウェブサイト」 | 紹介 `/` |
| ストアの設定の「メールアドレス」 | support@tailor-it.app(サポートと同じ) |
| アプリのコンテンツの「プライバシー ポリシー」 | `/privacy` |
| データ セーフティの「アカウントの削除の URL」 | `/account/delete`(appconfig の削除のページ。コメントに「Google Play's account deletion URL」とある) |

## 3. 同じドメインに置く仕組み

紹介などのページと同じドメイン(tailor-it.app)に、次の物も置く。手順の本体はそれぞれの README。

| 物 | パス | サイトがすること | 手順 |
|---|---|---|---|
| Universal Links(iOS・Mac) | `/.well-known/apple-app-site-association` | ファイルを `application/json` で、リダイレクト無し・Bot 対策無しで返す | [`docs/infra/universal-links/README.md`](../infra/universal-links/README.md) |
| App Links(Android) | `/.well-known/assetlinks.json` | 指紋を入れたファイルを同じ条件で返す | [`docs/infra/app-links/README.md`](../infra/app-links/README.md) |
| 共有ページ | `/s/{token}` | appconfig の `/s/{token}` へプロキシする(URL は tailor-it.app のまま) | Universal Links の README の「人の作業」3 |
| アカウントの削除 | `/account/delete` | appconfig の `/account/delete` へ同じように中継する(**推測**。今の手順の文書には書いていない) | 無い |
| backend の設定 | — | appconfig の `SHARE_WEB_BASE_URL` を `https://tailor-it.app` にする(中継ができてから) | [`docs/backend/features/sharing.md`](../backend/features/sharing.md) の環境変数の表 |

## 4. 人が決めること

| 決めること | 関係する Issue・文書 |
|---|---|
| 特定商取引法の表記が要るか・要るならどのパスに置くか | #330 |
| `/account/delete` をサイトから中継するか、appconfig の URL をそのまま Play Console に入れるか | #331 |
| FAQ を公開する時期と、公開する版で使える機能だけに絞るか | [`docs/support/faq.md`](../support/faq.md) の 1 章 |
| 紹介のページに出すスクリーンショット・動画 | [`docs/store/listing.md`](../store/listing.md) の 3 章 |
| サイトの置き場・ドメインの更新・費用 | #331 |
