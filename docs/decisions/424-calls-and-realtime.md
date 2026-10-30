# #424 通話・ビデオ通話(WebRTC)と WebSocket のリアルタイム通信

> 状態: **提案(オーナーの判断待ち)**。2026-10-28 作成。
> 対象 Issue: [#424](https://github.com/jphacks/kz_2601/issues/424)
> 調べた日: 2026-10-28。価格は変わるので、決める前にもう一度「参考」のページを見ること。

## 目次

1. [結論(推奨)](#結論推奨)
2. [選択肢の表](#選択肢の表)
3. [根拠](#根拠)
4. [決めてほしいこと](#決めてほしいこと)
5. [参考](#参考)

## 結論(推奨)

**M0 では通話を入れない。チャット・位置の共有は、今ある SSE の変化通知(#82)で足りる範囲に留め、WebSocket も足さない。**

- 通話は **M4(サーバー機能の自動構築)の後**に、独立した Issue 群として検討する。理由: 着信には CallKit + VoIP プッシュ(サーバーからの APNs 送信)が要り、「誰が誰にかけてよいか」(1 対 1 の関係 #271・権限 #272)と通報・ブロック(#321)が先に要る。どれも M2〜M4 の土台の上に乗る。
- 入れる時は**自前の TURN を立てず、マネージドのサービス**から選ぶ。候補は Cloudflare Realtime(SFU + TURN)か LiveKit Cloud。Twilio・Agora は価格を確かめていない(要確認)。
- 運転中のアプリの例は、まず「電話をかける」ボタン(`openUrlButton` の `tel:`、実装済み)と、位置・状況の共有(SSE)で代替する。

## 選択肢の表

| 選択肢 | 意味 | 費用 | 審査 | 作業量 |
|---|---|---|---|---|
| **A. 入れない・今の SSE で代替(推奨・M0)** | 通話は `tel:` で OS の電話を開く。チャット・位置の共有は公開/共有レコード + SSE 通知(数百 ms) | 追加なし(Firestore の読み取りは増える。下の根拠 4) | 論点なし | 小(AI が「通話はできない、代わりに」と答える文言) |
| B. 通話をアプリを開いている時だけ(着信なし) | 双方がアプリを開いて同じ部屋に入る形。WebRTC + マネージドの SFU/TURN。CallKit・VoIP プッシュは使わない | 通話の分数・転送量に比例(根拠 3 の試算)。無料枠あり | 通話の部品が新しく増えるのでバイナリ更新(審査)が要る。1.2(ユーザー同士のやり取り)の通報・ブロックが要る見込み | 中〜大(部品・部屋のサーバー・トークン発行・権限) |
| C. 着信あり(CallKit + PushKit) | B に加えて、アプリを閉じていても着信画面が出る | B + APNs の送信サーバー | VoIP プッシュは必ず CallKit に通話として報告する決まり(根拠 2)。背景モード voip は本来の用途のみ(2.5.4) | 大(B + サーバーからの VoIP プッシュ・着信の権限管理・スパム対策) |
| D. 自前の TURN(coturn 等)を立てる | B/C の中継サーバーを自分で運用 | VM と転送量。運用の手間が大きい | B/C と同じ | 大(監視・認証情報の発行・地域の分散) |
| E. WebSocket のサーバーを足す | Cloud Run で常時接続し、チャット・位置を即時に流す | 接続中はインスタンスが課金され続ける(根拠 4) | 論点なし | 中(複数インスタンス間の同期に Firestore や Redis が要る) |

## 根拠

### 1. 通話に要る部品

| 部品 | 役割 | Tailor It での置き場 |
|---|---|---|
| シグナリング | 相手の接続情報(SDP・ICE 候補)を交換する | マネージドの SFU を使えば、そのサービスの SDK が持つ。自前なら appconfig か新しいサービス |
| STUN | 端末が自分の外向きの住所を知る | 無料の公開 STUN もあるが、本番は使うサービスのものを使う |
| TURN | 直接つながらない時(会社・携帯回線の NAT 等)に音声・映像を中継する | **転送量に比例して費用がかかる部分**。マネージドを使う |
| SFU(多人数時) | 3 人以上で、各自の映像を 1 本ずつ上げて配る | マネージドを使う |
| 部屋に入る権利(トークン) | 誰がどの部屋に入れるか | backend で発行(Firebase の ID トークンを確かめて発行) |
| 着信 | アプリを閉じていても呼び出す | CallKit + PushKit(VoIP プッシュ)+ APNs の送信サーバー |

### 2. CallKit・VoIP プッシュの決まり(Apple の文書)

- **iOS 13 SDK 以降でビルドしたアプリは、PushKit の VoIP 通知を受けたら CallKit を使って通話として報告しなければならない。報告しないとシステムがアプリを終了させ、繰り返すと VoIP プッシュ自体が届かなくなる**(PushKit の `pushRegistry(_:didReceiveIncomingPushWith:for:completion:)` の説明)。→ VoIP プッシュは「着信」専用。チャットの新着通知などには使えない。
- CallKit を使わずに通話を始めたいなら、PushKit ではなく通常の通知(User Notifications)を使うよう書かれている。
- 審査 2.5.4: 背景の動作(VoIP 等)は本来の用途だけ。
- 審査 2.5.12: CallKit で番号のブロック・迷惑電話の判定をする場合の決まり。Tailor It は着信を受けるだけなら該当しない見込み。
- 生成アプリから「誰にでも着信を鳴らせる」と、スパムの温床になる。M4 の脅威の想定(#339)・プッシュの宛先の制限と同じ扱いが要る。
- 中国本土のストアでの CallKit の扱いは**要確認**(配信地域を絞るなら関係ない)。

### 3. 費用の目安(TURN・SFU の中継)

**試算の前提(仮定・要実測)**: 音声 1 本 約 40 kbps(≒ 0.3 MB/分)、映像 720p 1 本 約 1.5 Mbps(≒ 11 MB/分)。1 対 1 の通話が中継を通ると、中継から出ていく流れは 2 本(A へ B の分、B へ A の分)。

| 通話 | 中継から出る量の目安 | 1 時間あたり |
|---|---|---|
| 1 対 1 音声 | 約 0.6 MB/分 | 約 0.04 GB |
| 1 対 1 映像 + 音声 | 約 23 MB/分 | 約 1.4 GB |

公開されている価格(2026-10-28 に各社のページで確認):

| サービス | 価格(公式ページの記載) | 上の試算に当てると |
|---|---|---|
| Cloudflare Realtime(SFU / TURN) | SFU と TURN 合わせて**毎月 1,000 GB まで無料、その後 $0.05/GB**(外向きの転送)。TURN は SFU と一緒に使えば無料、単体なら $0.05/GB | 映像 1 時間 ≒ $0.07(無料枠の後)。無料枠 1,000 GB ≒ 1 対 1 映像で約 700 時間/月 |
| LiveKit Cloud | Build $0/月(5,000 参加者分・50 GB)、Ship $50/月〜(150,000 参加者分・250 GB、超過 $0.0005/参加者分・$0.12/GB)、Scale $500/月〜 | 1 対 1 映像 1 時間(超過分) ≒ 参加者分 120 × $0.0005 + 1.4 GB × $0.12 ≒ $0.23 |
| Twilio Video | **要確認**(今回価格ページを確認していない) | — |
| Agora | **要確認**(今回価格ページを確認していない) | — |
| 自前の TURN(coturn 等) | VM の費用 + クラウドの外向き転送の費用。**要確認**(GCP の転送単価は地域・量で変わる) | 運用の人手が一番重い |

- すべての通話が中継を通るわけではない(直接つながれば中継の転送は発生しない)。中継を通る割合は**要実測**。上は「全部中継を通る」悪い方の見積もり。
- 費用は利用者の行動に比例し、無料ユーザーが長時間使うと赤字になる。入れるなら Pro 限定か、1 日の分数の上限(`docs/backend/rate-limit.md` と同じ数え方)を前提にする。

### 4. 今の仕組みでどこまで足りるか(Firestore + SSE)

- **注意: iOS は Firestore の SDK で直接購読していない。** backend(appconfig)が Firestore の snapshot listener を持ち、iOS へは SSE `GET /v1/apps/{appId}/events` で「何が変わったか」だけ流して、本体は API で取り直す(`docs/backend/architecture.md` §5.1・`docs/ios/architecture.md`)。
- 実測: 共有の掲示板に投稿 → 別の端末の一覧に出るまで **442 ms**(SSE の到達は 38 ms・残りはデバウンス 250 ms と取り直し)。
- 足りる用途: グループのチャット(新着が数百 ms で出る)、出欠・ランキング、数秒ごとの位置の共有(レコードの更新として流す)。
- 足りない用途: 音声・映像(そもそも Firestore に流すものではない)、アプリを閉じている人への通知(M4 のプッシュ #269)、毎秒何回も変わる値(1 ドキュメントへの書き込みは毎秒 1 回程度が目安・書き込みの費用も増える)。
- 費用の注意: SSE は再接続ごとに最大 100 件読む(50 秒周期で 1 クライアント 1 entity あたり ≒7,200 読み取り/時、§5.1)。位置を 5 秒ごとに書くと 1 人 720 書き込み/時。無料枠(読み 5 万・書き 2 万/日)を多数の常時接続で超える。使い方が増えたら `EVENTS_MAX_STREAM` と Cloud Run の timeout を伸ばす(§5.1 に書いてある対策)。
- WebSocket(案 E): Cloud Run は WebSocket を使えるが、接続は request timeout(最大 60 分)で切れ、**接続中はインスタンスが動き続けて課金される**。インスタンス間の同期には Firestore か Redis が要ると Google の文書にある。今の SSE + Firestore の構成とほぼ同じことをするので、今は足す理由が無い。

### 5. 審査・その他

- 通話・チャットはユーザー同士のやり取り(1.2)。通報・ブロック・運営による非公開化(#321・M2)が先に要る。
- 通話の部品はバイナリに入れる(2.5.2 の論点は無い)。ただし新しい部品は審査付きの更新でしか増やせない。
- 運転中の通話は、OS の電話(`tel:`)・CarPlay の方が安全。生成アプリの中で運転中に映像通話を使わせる設計はしない方がよい(プロダクトの判断)。

## 決めてほしいこと

| # | 決めること | 推奨 |
|---|---|---|
| 1 | M0 で通話を入れるか | **入れない**(`tel:` で代替) |
| 2 | 通話をいつ検討するか | **M4 の後**(#269 プッシュ・#271 1 対 1・#272 権限・#321 通報の後)。独立したエピックにする |
| 3 | 入れる時のサービス | **マネージド**(Cloudflare Realtime か LiveKit Cloud を比較)。自前の TURN はしない |
| 4 | 着信(CallKit + VoIP プッシュ)まで入れるか | 最初は**入れない**(B: アプリを開いている時だけ)。需要を見てから C |
| 5 | WebSocket を足すか | **足さない**(今の SSE で足りる。足りなくなったら §5.1 の timeout を伸ばす) |
| 6 | 通話を Pro 限定にするか | 入れるなら **Pro 限定 + 1 日の分数の上限** |

## 参考

- [PushKit: Responding to VoIP Notifications from PushKit(Apple)](https://developer.apple.com/documentation/pushkit/responding-to-voip-notifications-from-pushkit)
- [PushKit: pushRegistry(_:didReceiveIncomingPushWith:for:completion:)(Apple)](https://developer.apple.com/documentation/pushkit/pkpushregistrydelegate/pushregistry(_:didreceiveincomingpushwith:for:completion:))
- [CallKit(Apple)](https://developer.apple.com/documentation/callkit)
- [App Store Review Guidelines(1.2・2.5.4・2.5.12)](https://developer.apple.com/app-store/review/guidelines/)
- [Cloudflare Realtime TURN](https://developers.cloudflare.com/realtime/turn/)
- [Cloudflare Realtime SFU の価格](https://developers.cloudflare.com/realtime/sfu/pricing/)
- [LiveKit Cloud の価格](https://livekit.com/pricing)
- [Cloud Run で WebSocket を使う(Google Cloud)](https://docs.cloud.google.com/run/docs/triggering/websockets)
- リポジトリ内: `docs/backend/architecture.md` §5.1(SSE)、`docs/ios/architecture.md`(RealtimeDocumentSync)、`docs/roadmap/roadmap-m1-m6.md`(M4: #269〜#273・#336〜#339)、`docs/roadmap/m0-discovery.md`(D-20・D-21)
