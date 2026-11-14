# アカウント削除(`DELETE /v1/me`・#119)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §4.1)。

App Store 審査ガイドライン 5.1.1(v)(アカウントを作れるアプリはアプリ内から削除を開始できること)への対応。iOS の設定シート「アカウントを削除」から呼ぶ。**論理削除**(2026-10-25 ユーザー判断: 物理削除と Firebase Auth のユーザー削除はしない。Cloud Run に `roles/firebaseauth.admin` を足さずに済む)。

| 対象 | 扱い |
|---|---|
| `users/{uid}` | `{status: "deleted", deletedAt}` を足す(他に入るのは `firstSeenAt` だけ・#339 の通知の 24 時間。`status` が無ければ有効なユーザー)。`firstSeenAt` は 30 日後の消去で消える |
| `users/{uid}/apps/*` | `status: "archived"`(`DELETE /v1/apps/{appId}` と同じ)。versions・records は残る |
| `shares/*`(`ownerUid == uid`) | `revoked: true`(受け取り側の取り込み・プレビューは 404) |
| `public_apps/*`(`ownerUid == uid`) | 取り下げ(「探す」から消える) |
| `users/{uid}/devices/*`(プッシュ通知の端末・#269) | 消す(決定 #337: プッシュをすぐ止める。[notifications.md](notifications.md)) |
| `users/{uid}/connections/*`(外の API の接続・#1716) | runner の `revokeAll` で provider 側の許可も取り消す。失敗してもログだけで 204。トークンは 30 日後の消去で消える([external-apis.md](external-apis.md#oauth-のトークン)) |
| `apps_public/*`(共有アプリの DSL と共有レコード) | 残す。他人の投稿も自分の投稿も消えない |
| Firebase Auth のユーザー | サーバーは削除しない(iOS がクライアント SDK で削除する。下記「再登録」) |

- **以後のリクエスト**: `appconfig.RejectDeleted`(auth.Middleware の内側)が削除済み uid の認証済みリクエストに **410 `account_deleted`** を返す。`DELETE /v1/me` だけは通して何度でも 204。uid ごとに「削除済み」は永続キャッシュ、「有効」は 30 秒キャッシュ(別インスタンスへの反映は最大 30 秒遅れる)
- **冪等性**: 削除済みの印を最初に書き(トランザクションで最初の `deletedAt` を保持)、アプリ・シェア・掲載の更新は何度やっても同じ結果。途中で失敗しても再実行で終わる
- **実装**: `store.Accounts`(`DeactivateUser` / `UserDeleted`。`account.go` / `memory_account.go` / `firestore_account.go`)。`ownerUid` の等価クエリだけなのでインデックス追加は不要
- **再登録**: サーバーは Firebase Auth のユーザーを消さないが、iOS が `DELETE /v1/me` の 204 の後にクライアント SDK で自分のユーザーを削除する(`User.delete()`・管理者権限不要)。そのため同じ Apple ID でサインインし直すと**新しい uid = 新規ユーザー**になる(古い uid のデータは論理削除のまま残る)。クライアント側の削除が失敗した場合だけ、同じ uid に戻って 410 になる(ログに残る)
- **Sign in with Apple のトークン取り消し**: iOS が削除直前に Apple の再認証で `authorizationCode` を得て `Auth.revokeToken(withAuthorizationCode:)`(Firebase が Apple の `auth/revoke` を呼ぶ)。Firebase コンソールの Apple プロバイダに Services ID・Team ID・Key ID・秘密鍵(OAuth コードフロー構成)が要る。失敗しても削除は続ける
