# プッシュ通知の端末と受け取りの同意(#269・#339)

> [バックエンド設計](../architecture.md) から分けた機能の節。決定は [#337 隔離](../../decisions/337-isolation-multitenancy.md)・[#339 悪用対策](../../decisions/339-abuse-threat-model.md)。API は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の `notifications`。

```
users/{uid}/devices/{installationId}   # {token, environment: sandbox|production, appVersion?, os?, createdAt, updatedAt}
users/{uid}/notifyConsent/{appId}      # {allowed, updatedAt}
device_tokens/{sha256(token)}          # {uid, installationId}: トークンを今持っている端末
```

| 項目 | 決め方 |
|---|---|
| 登録 | `PUT /v1/me/devices/{installationId}`(本人の分だけ・パスに uid は無い)。同じ `installationId` は上書き。トークンは 16 進(32〜100 バイト)を小文字で保存 |
| 上限 | 1 人 10 台(`store.MaxDevicesPerUser`)。超えたら最後の更新が古いものから消す |
| アカウントを替えた端末 | 同じトークンが別の (uid, installationId) にあれば、そちらを消す(`device_tokens` で引く)。前のアカウントの通知が次の人の端末に出ないように |
| 削除 | `DELETE /v1/me/devices/{installationId}`(サインアウト・通知を切った時・冪等) |
| ログ・レスポンス | トークンは出さない(環境だけ) |
| runner が読む | 宛先の判定(参加者・ブロック・同意・上限)を通った uid の分だけ、appId に縛られた取っ手から読む(#337 の 3 層目)。`PushEnvironment`(既定 production)と同じ環境のトークンだけ送る |
| 410 Unregistered | runner がその uid のそのトークンを消す(失敗はログだけ。次の送信でまた 410 になる) |
| 退会 | `DELETE /v1/me`(論理削除)の時点で全部消す(#337)。30 日後の消去では残り(ログ `devices`)と同意(ログ `notifyConsents`)を消す |
| 同意 | `PUT /v1/me/notify-consent/{appId}` `{allowed}`。保存が無い = 受け取らない(作成者以外には送らない)。有効にするには自分のコピー(404)と、shared のデータがあるアプリでは参加(403 `not_member`)が要る。無効はいつでも |
| 同意の既定(#339) | shared は参加の時に確認つきで有効・public は無効。iOS が参加の画面でこの API を呼んで実現する(画面は未作成) |

iOS 側(APNs の登録・Push の entitlement)は Apple の App ID の設定変更が要るので別の作業。
