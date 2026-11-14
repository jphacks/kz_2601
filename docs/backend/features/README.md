# backend の機能の文書(索引)

[バックエンド設計](../architecture.md) から機能ごとに分けた文書の一覧(1 行 1 本・名前順)。新しい機能の文書を足したら、この表にも 1 行足す。

| 文書 | 何の文書か |
|---|---|
| [account-deletion.md](account-deletion.md) | アカウント削除(`DELETE /v1/me`・#119)の受け口と、消す物・残す物 |
| [automations.md](automations.md) | 自動の動き(serverRules)が動いたか・止まったかを作成者に見せる仕組み(#347) |
| [book-lookup.md](book-lookup.md) | ISBN から本の情報を入れる `lookupBook`(openBD・既定で止めている) |
| [calls.md](calls.md) | 共有アプリの参加者どうしの音声・ビデオ通話(LiveKit Cloud・#447) |
| [chat.md](chat.md) | 生成の前に足りない点を聞き返す対話(`POST /v1/chat`・#74) |
| [events.md](events.md) | アプリの変化を端末に知らせる SSE(`GET /v1/apps/{appId}/events`・#82) |
| [expr.md](expr.md) | DSL の計算ロジック(Expr・#100) |
| [external-apis.md](external-apis.md) | runner から外部 API を呼ぶ口・数え方・OAuth のトークン(#1493) |
| [external-feeds.md](external-feeds.md) | 鍵の要らない公開データを許可リストから読む feed(#348) |
| [feedback-loop-agent-pr.md](feedback-loop-agent-pr.md) | 自己改善ループで、人が承認した Issue からエージェントの PR を作る手順(#370) |
| [feedback-loop-triage-agreement.md](feedback-loop-triage-agreement.md) | 自己改善ループの振り分けと人の判断の一致を測る(#376) |
| [feedback-loop.md](feedback-loop.md) | 自己改善ループの全体(取り込み・振り分け・Issue の下書き・直ったお知らせ・#364) |
| [feedback.md](feedback.md) | アプリ内のフィードバックの受け口と運用(#319) |
| [follows.md](follows.md) | フォローした人の投稿だけが流れるタイムライン(#270) |
| [matching.md](matching.md) | 相互承認でつながる仲間探しのアプリ(#507) |
| [members.md](members.md) | 共有アプリ(shared)の参加者(#254) |
| [notifications.md](notifications.md) | プッシュ通知の端末の登録と受け取りの同意(#269・#339) |
| [pro.md](pro.md) | 有料機能(`requiresPro`・#77)とサーバーでの強制(#316) |
| [release.md](release.md) | 作ったアプリを運営に出すリリース申請と、運営の処理の手順(#76) |
| [shared-data.md](shared-data.md) | 共有データ(`entity.scope`・#47) |
| [sharing.md](sharing.md) | シェアリンクと公開カタログ「探す」(#49・#50) |
| [text.md](text.md) | 決まった形の文章を作る `POST /v1/text`(#417) |
