# shared の参加者(#254)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §3.4)。

`scope:"shared"` は「シェアリンク / QR で参加した人だけ」(家族・サークル・友達・チーム)。`public` は従来どおり誰でも(掲示板・ランキング)。レコードの置き場はどちらも `apps_public/{appId}/entities/{e}/records` のまま変えず、**誰が読み書きできるか**だけを参加者表で絞る。

```
apps_public/{appId}/members/{uid}   # {displayName?, via: share|legacy, invitedBy?, joinedAt}
```

| 項目 | 決定 |
|---|---|
| 参加者 | 作成者(`app_owners/{appId}`・ドキュメント無しで常に参加者)+ `members/{uid}` がある人。作成者の判定は publish と同じ `appOwner`(記録が無ければ「探す」の掲載者 → `apps_public` の持ち主) |
| 参加 | `POST /shares/{token}/import`(任意で `{displayName}`)。シェアの DSL に shared の entity があれば呼び出したユーザーを `members/{uid}` に入れる(`via: share`・`invitedBy` = リンクの発行者)。既に参加していれば表示名だけ更新。**リンクの発行者がもう参加者でなければ 403 `inviter_left`**(外された人の古いリンクを止める)。上限 100 人(`members_full`) |
| 招待 | shared の entity を含むアプリの `POST /apps/{appId}/share` は参加者だけ(appId を知っているだけの人が自分宛てにリンクを作って入るのを防ぐ)。参加者が友だちを招けるのは #118 のまま |
| records | shared の entity の records の全操作(一覧・取得・作成・更新・削除・全件削除)で `entityOf` が参加者か確かめ、違えば **403 `forbidden` / `details.reason: not_member`**。`GET /entities/{e}`(定義)は DSL に入っているので絞らない。public の entity は確かめない |
| 更新の権限(#252 / PR #378 と合わせる) | `entity.edit: "members"` の「参加者なら誰でも更新」の参加者はこの表の参加者。参加者でなければ `ownRecord` に届く前に 403 になるので、#378 の「同じ appId を開いた全員」は shared ではこの表に絞られる(public の `edit: members` は従来どおり全員) |
| 退出・外す | `DELETE /apps/{appId}/members/me` で退出(冪等)。`DELETE /apps/{appId}/members/{uid}` は作成者だけ。作成者は抜けられない(400)。外された人の木のコピーと過去の投稿は残る。作成者が出したリンクは取り消すまで有効なので、再参加させたくなければ `DELETE /shares/{token}` も行う |
| 一覧 | `GET /apps/{appId}/members`(参加者だけ)。作成者を先頭に参加順。`{uid, displayName, role: owner / member, admin, via?, joinedAt?, you}`。最上位に呼び出した人の `role` と `admin` |
| 管理者(#272) | 作成者は常に管理者(外せない)。管理者は `PATCH /apps/{appId}/members/{uid}` `{"admin": true / false}` で参加者を管理者にする / 外す(`me` で自分がやめる)。`members/{uid}.admin` に持つ。管理者でない参加者は 403 `admins_only`。権限の使い道は [DSL §3.13](../../dsl/README.md) |
| GET /events | 参加者でなければ shared の entity の records を購読しない(何も漏らさない・エラーにもしない) |
| GET /dsl | appId を開けば DSL のインストール(seed)は従来どおり起きる。レコードは参加しないと読めない |
| 探す | 誰でも見られるので、共有が shared だけのアプリは**新しい appId の独立コピー**(追加した人が作成者)。public も含むアプリは元 appId に参加するが、shared の entity は参加者でないので 403 |
| アプリ削除 | 作成者の `DELETE /apps/{appId}` は `apps_public/{appId}` ごと消すので参加者表も消える(Firestore の再帰削除)。参加者が自分のコピーを消しても参加は残る(後続) |
| コスト | shared の records の 1 リクエストにつき `app_owners` と `members/{uid}` の読み取りが最大 2 回増える。キャッシュはしない(外した直後から確実に 403 にするため) |

**移行(#254 以前に shared で共有していたアプリ)**

- 作成者は計算で常に参加者なので、既存アプリの作成者が急に読めなくなることは無い。
- それ以外の人(リンクや appId で開いて使っていた家族)は、既定では参加し直しが要る(作成者がリンクを送り直す)。急に 403 にしたくない環境では **`SHARED_MEMBERS_LEGACY_BEFORE`(RFC3339)** をデプロイ時刻にして出す。その時刻より前に作られた自分のコピー(`users/{uid}/apps/{appId}.createdAt`)を持つ人は、初回アクセスで `via: legacy` の参加者として記録される。時刻より後に開いた人は対象外なので、appId を知っているだけの人は入れない。全員が一度開いた頃に外してよい。
- `public` の挙動は何も変えていない。今の AI は共有の依頼を主に `public` で作るので(#385)、影響を受ける既存データは `scope:"shared"` を明示したアプリだけ。

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `SHARED_MEMBERS_LEGACY_BEFORE` | o | - | 未設定 = 移行しない。RFC3339。上の「移行」を参照 |
