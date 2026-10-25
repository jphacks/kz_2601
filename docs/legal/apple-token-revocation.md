# Sign in with Apple のトークン失効(アカウント削除時・#119 / #246)

App Store 審査ガイドライン 5.1.1(v): Sign in with Apple を使うアプリは、アカウント削除の時に Apple の REST API(`https://appleid.apple.com/auth/revoke`)でトークンを失効させる。

## 仕組み(実装済み)

1. 設定の「アカウントを削除」→ 確認 → **Apple の再認証シート**(`AppleReauth`・`ios/Metamon/Features/Auth/AccountDeletion.swift`)。
   ここで新しい `authorizationCode`(5 分有効・1 回限り)と `identityToken` を受け取る。
2. `AppleTokenRevocation.run`(`ios/Packages/MetamonServices/Sources/MetamonServices/AccountDeletionFlow.swift`)
   - `identityToken` で Firebase を再認証(`requiresRecentLogin` の回避)
   - `authorizationCode` を **Firebase Auth の `revokeToken(withAuthorizationCode:)`** に渡す。Firebase(Google のサーバー)が
     client secret(.p8 鍵で署名した JWT)を作り、Apple の `auth/token` でトークンに交換して `auth/revoke` を呼ぶ。
   - どちらが失敗しても削除は続ける(ログ `[Metamon] Apple token revoke failed` が出る)。
3. `AccountDeletionFlow.run`: `DELETE /v1/me`(サーバーで論理削除)→ Firebase のユーザー削除 → 端末のデータ削除 → サインアウト。

### なぜ自前の backend ではなく Firebase か
- 失効には、authorization code をトークンに交換する処理と client secret が要り、**どちらもサーバー側**で行う必要がある(.p8 鍵を端末に置けない)。
- Firebase Auth はこの交換と失効を代行する API を持つ(iOS SDK `Auth.revokeToken(withAuthorizationCode:)`)。鍵は Firebase コンソールに登録するので、**リポジトリ・端末・Go の backend のどこにも鍵を置かない**で済む。Go に同じ処理を足すと、鍵の保管場所と失敗の経路が 2 つになるので足していない。
- テスト: `MetamonTests/PrivacyConsentTests.swift`(モックの `AuthService` で、再認証 → 失効の順に code が渡ること、失敗しても続くこと)。

## 人の作業(未実施・GCP / Apple の設定変更)

この作業までは、失効の呼び出しは失敗し(ログに出る)、削除自体は完了する。**審査に出す前に必ず行う。**

1. **Apple Developer**(Certificates, Identifiers & Profiles)
   1. Keys → 「+」→ **Sign in with Apple** にチェック → Primary App ID に `jp.kz2601.metamon` → 作成。
   2. `.p8` をダウンロード(**1 回しか落とせない**)。**Key ID** を控える。**Team ID** を控える(右上)。
   3. `.p8` はリポジトリ外(パスワード管理ツール等)に保管し、**コミット・チャット・ログに貼らない**。
   4. (Services ID が無ければ)Identifiers → Services IDs → 作成し Sign in with Apple を有効化。Return URL に Firebase の
      `https://<project-id>.firebaseapp.com/__/auth/handler` を登録。
2. **Firebase コンソール**(プロジェクト `jphacks-kz2601`)
   1. Authentication → Sign-in method → Apple → 編集。
   2. 「OAuth コードフローの構成」に **Services ID・Apple チーム ID・キー ID・秘密鍵(.p8 の中身)** を入れて保存。
3. **確かめ方**(実機・TestFlight)
   1. Apple でサインインしたアカウントで、設定 → アカウントを削除。
   2. Xcode のコンソールに `Apple token revoke failed` が**出ない**こと。
   3. iPhone の「設定」>「Apple ID」>「サインインとセキュリティ」>「Apple でサインイン」から Tailor It が消えていること。

### Secret Manager を使う場合(代替案・今は不要)
自前の backend で失効させる場合は、`.p8` を **Secret Manager**(例 `apple-siwa-p8`)に置き、Cloud Run に環境変数ではなくシークレットとしてマウントし、ログに出さない。ただし Firebase の構成で足りるので、現状は採っていない。
