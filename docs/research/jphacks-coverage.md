# 過去の JPHACKS 作品を Tailor It で作れるか(2022〜2025)

> 調べた日: 2026-11-06(マージされた部品で判定を見直した日: 2026-11-08)。目的: オーナーの目標「過去の JPHACKS のプロダクトを含め、ほぼ全部のプロダクトのモバイル版を Tailor It で作れる状態にする」(期限 2026-11-14)のために、今の部品で作れる作品・作れない作品と、足すべき部品を数で出す。
> 調査だけで、コード・決定事項は変えていない。他チームの作品は公開情報(GitHub の `jphacks` organization の公開リポジトリの README と、JPHACKS 公式サイトの結果ページ)だけを使い、作品名・リポジトリの URL・何をするものかだけを載せる。作品の良し悪しは書かない。

## 目次

1. [要約](#要約)
2. [調べ方](#調べ方)
3. [判定の基準](#判定の基準)
4. [機能の分類ごとの数](#機能の分類ごとの数)
5. [足りない部品・機能(多い順)](#足りない部品機能多い順)
6. [× のもの: 難しい理由と代わりの案](#-のもの-難しい理由と代わりの案)
7. [受賞・決勝の作品](#受賞決勝の作品)
8. [測り方](#測り方)
9. [相談が要ること](#相談が要ること)
10. [作品の一覧](#作品の一覧)

## 要約

- **調べた作品: 484 チーム**(2022: 106・2023: 111・2024: 134・2025: 133)。このうち README に中身があって判定できたのは **341 作品**。残りの 143 作品は README が JPHACKS の雛形のまま(または無い)で判定していない(?)。
- **判定(341 作品のうち・2026-11-08)**: ◎ 57(17%)・○ 107(31%)・△ 77(23%)・× 100(29%)。**今の部品で本体が作れる(◎○)のは 164 作品 = 48%**(11-06 は ◎ 51・○ 97・△ 93 で 148 作品 = 43%)。
- **11-06 からの変化**: develop に入った部品で 17 作品を紙で判定し直した(△ → ◎ 5・△ → ○ 11・○ → ◎ 1。× は変えていない)。効いたのは 文字起こし `transcribeField`・手書き `drawingField`・読み上げ `speakButton`・写真の `labels`・`fileImport`・天気の `feed`(#659)、`addToCalendar`(#660)、`alarm`(#665)、`follow`(#679)、serverRules の `lookup`(#701)。管理者・一意・宛先つきメッセージ(#678)とプッシュの配信(#683・#691)は、11-06 の判定で共有・通知を既にある物として数えていたので、紙の判定は動かない。変えた行はメモに部品と PR を書いた。
- △ の部品を足していくと ◎○ は増える(下の表の「足した後の ◎○ の累計」)。**上位 5 つを足すと 214 作品(63%)、上位 10 で 246 作品(72%)**。残りの約 3 割は × で、自作の機器・独自の機械学習モデル・AR・カメラの連続認識が本体の作品。
- **足すべき上位 5 つ**(作品の数は、その物が要ると判定した作品。部品が入って ◎○ になった作品も含む):
  1. **外部 API・Web サービスの連携**(60 作品・大・#348)— 場所・施設の検索、カレンダー、メッセージ(Slack・LINE)、ニュースなどのデータが本体の作品。天気・祝日・RSS の `feed`(#659)だけ入った
  2. **クラウドの生成 AI**(56 作品・中)— 端末内の `aiButton` では足りない長い文・会話・画像の理解
  3. **サーバーの処理を増やす**(36 作品・中〜大)— serverRules の語彙。記録の集計 `agg`・`each`(#658)と突き合わせ `lookup`(#701)は入った。推薦・割り当てが残る
  4. **文字起こし**(30 作品・小〜中)— `transcribeField`(#659)で入った。残る △ 4 は背景の常時録音・クラウド AI も要る作品
  5. **手書き・描画の入力**(25 作品・中)— `drawingField`(#659)で入った。残る △ 5 は自由な配置・物理の動き・写真の編集が要る作品
- 開いている PR(#702 長い文・#703 警告・圧力、本の情報の取得、ハイライト)の部品は、マージされるまで数えていない。
- 受賞・決勝の作品(公開リポジトリのある 61 作品)は **× が 32**(自作の機器が 12・AR が 9)で、全体より × の割合がずっと高い。「受賞作のモバイル版」を目標に数えるなら、× の作品を「機能を落とした版」で数えるかを決める必要がある([相談が要ること](#相談が要ること))。

| 年 | 作品(チーム) | ◎ | ○ | △ | × | ?(README が雛形) |
|---|---|---|---|---|---|---|
| 2022 | 106 | 11 | 22 | 18 | 21 | 34 |
| 2023 | 111 | 19 | 21 | 12 | 24 | 35 |
| 2024 | 134 | 15 | 35 | 22 | 21 | 41 |
| 2025 | 133 | 12 | 29 | 25 | 34 | 33 |
| 計 | 484 | 57 | 107 | 77 | 100 | 143 |
| 割合(判定できた 341 作品のうち) | | 17% | 31% | 23% | 29% | |

## 調べ方

| 項目 | 内容 |
|---|---|
| 対象 | `jphacks` organization の公開リポジトリのうち、名前がチームの番号(`A_2201` `tk_a_2515` など。上 2 桁が年)で 2022〜2025 のもの 506 本。同じチームの分割リポジトリ(`_1` `_2`)は 1 作品にまとめて 484 チーム |
| 中身 | 各リポジトリの README(`gh api repos/jphacks/<repo>/readme`)の冒頭 約 1300 字と「開発技術」の節。20 本は README が無いか取れなかった |
| 受賞 | 公式サイトの結果ページ(`https://jphacks.com/<年>/result/`)の受賞・決勝の作品。公開リポジトリが見つからない作品(2022 C_2201「ディズニーAIナビ」・2025 OS_2504「Mogufinder」)は一覧に入っていない |
| 判定 | README の要約を読んで、AI(サブエージェント 6 本・同じ基準)が 1 作品 1 行で判定し、集計した。README の冒頭だけを見ているので、1 作品ずつの判定には誤りがありうる。数の傾向を見るための表として使う |
| 対象外 | 2021 年以前(約 700 本)は今回は数えていない。同じ方法で足せる |

## 判定の基準

照らした Tailor It の能力は `docs/dsl/README.md`・`docs/dsl/dsl.schema.json`(Container 7・Leaf 61(スキーマの定義の数)・Action 12)と、サーバーの機能(共有データ `scope`・仲間探し `match`・条件の通知 `when`・`serverRules`)。11-06 の判定では、外部 API は無い(#348 未着手)、AI は端末内の `aiButton` と `photoField.analyze` だけ、とした。11-08 の見直しでは、その後 develop に入った部品([要約](#要約)の「11-06 からの変化」)を足して、影響する行だけを紙で判定し直した(迷う時は低い方にした)。

| 判定 | 意味 |
|---|---|
| ◎ | 今の部品だけで本体の体験が作れる |
| ○ | 少し機能を減らせば作れる(欠けるものを「メモ」に書いた) |
| △ | 部品・機能を 1〜2 個足せば作れる(足すものを「足りない物」に書いた) |
| × | 今の方式では難しい(自作の機器・AR・独自モデルなど。代わりの案を「メモ」に書いた) |
| ? | README が雛形のままで判定できない |

Web の作品はモバイルに置き換えて考えた(Web だから × にはしていない)。審査用の版(`docs/ios/review-build.md`)で外している部品(カメラの撮影・共有データ・センサーなど)も、通常の版にあるものとして数えた。審査用の版で数えると ◎○ はもっと減る。

## 機能の分類ごとの数

判定できた 341 作品が使っている機能(1 作品で複数)。

| 機能 | 作品の数 | ◎ | ○ | △ | × |
|---|---|---|---|---|---|
| AI(文章・画像の理解・生成) | 131 | 7 | 39 | 45 | 40 |
| 他の人との共有・チャット・SNS | 125 | 37 | 51 | 21 | 16 |
| 記録・一覧・集計・グラフ | 107 | 25 | 50 | 23 | 9 |
| カメラ・写真 | 74 | 10 | 23 | 11 | 30 |
| 位置・地図 | 56 | 14 | 20 | 15 | 7 |
| 外部 API・Web サービスとの連携 | 51 | 0 | 12 | 29 | 10 |
| 音声 | 44 | 3 | 10 | 10 | 21 |
| 通知・リマインダー | 44 | 11 | 18 | 6 | 9 |
| ランキング・ゲーム | 38 | 5 | 14 | 6 | 13 |
| センサー・ハードウェア(BLE・IoT・ウェアラブル) | 35 | 0 | 2 | 1 | 32 |
| 入力フォーム | 25 | 7 | 13 | 4 | 1 |
| Web 専用(ブラウザ拡張など) | 20 | 1 | 3 | 2 | 14 |
| 独自の機械学習モデル | 17 | 0 | 0 | 0 | 17 |
| AR・3D | 16 | 0 | 1 | 1 | 14 |
| その他(QR・通話・フォローなどを含む) | 18 | 4 | 6 | 2 | 6 |
| 決済 | 4 | 0 | 3 | 0 | 1 |

読み方: **共有・記録・通知・地図・フォームの作品は ◎○ が多い**(今の部品の得意な所)。**AI・外部 API は △ が多い**(足せば届く)。音声は文字起こし・読み上げ(#659)で △ が 17 → 10 に減った。**ハードウェア・独自モデル・AR はほぼ全部 ×**。

## 足りない部品・機能(多い順)

「必要な作品」= 判定できた 341 作品のうち、その物を「足りない物」に挙げた数(今の判定の △ / ○ / × / ◎。◎ は部品が入って ◎ になった作品。「足りない物」の列は 11-06 に挙げたまま残している)。「1 つで ◎○ になる」= 足りない物がこれ 1 つだけの今の △ ×(これを足せば ◎○ になる)。「足した後の ◎○ の累計」= 今の ◎○ 164 に、表の上から順に足していった時の数(足りない物がすべて足された △ × を数える)。

| 順 | 足す物 | 必要な作品(△ / ○ / × / ◎) | 大きさ | 審査への影響 | 1 つで ◎○ になる | 足した後の ◎○ の累計 |
|---|---|---|---|---|---|---|
| 1 | **外部 API・Web サービスの連携**(天気・店・交通・ニュース・SNS。許可リストの連携を宣言から呼ぶ・#348) — 天気・祝日・RSS の `feed` は #659 で入った | 60(28 / 22 / 10 / 0) | 大 | 2.5.2 にはかからない見込み(コードは配らず、サーバーが呼ぶ)。連携先を後から足すのは「許可リストのデータ」と説明する(#349)。連携先の利用規約と 5.2.2(第三者のサービスの利用許可) | 8 | 172 |
| 2 | **クラウドの生成 AI**(生成アプリから backend 経由で LLM を呼ぶ。長い文・会話・画像の理解) | 56(27 / 10 / 19 / 0) | 中 | 4.7(生成 AI・チャットボット)の説明、1.2(生成物のフィルタ・通報)、プライバシーの表示(「データは端末外に出ない」を直す)。費用は利用量に比例(`docs/backend/rate-limit.md` に乗せる)。#426 の案 F | 11 | 188 |
| 3 | **サーバーの処理を増やす**(serverRules の語彙: 定期の集計・並べ替え・推薦・複数人の割り当て・日程の重なり) — 集計 `agg`・`each` は #658、突き合わせ `lookup` は #701 で入った。推薦・割り当てが残る | 36(18 / 13 / 5 / 0) | 中〜大(宣言の語彙を足す。任意のコードは書かせない) | 小(方式 A = 宣言だけ・`docs/decisions/336`〜`339`) | 4 | 205 |
| 4 | **文字起こし**(`audioRecorder` / 発話の文字を text field に入れる。`voiceInput` と同じ端末内の音声認識) — **入った**(`transcribeField`・#659) | 30(4 / 8 / 15 / 3) | 小〜中 | 小。マイク・音声認識の許可の文言(5.1.1)。審査用の版では `audioRecorder` と一緒に外れる(4.7.2) | 0 | 209 |
| 5 | **手書き・描画の入力**(PencilKit で描き、写真の field に保存) — **入った**(`drawingField`・#659) | 25(5 / 9 / 9 / 2) | 中 | 小 | 2 | 214 |
| 6 | **写真の理解を広げる**(`photoField.analyze` の kind: 物・動植物・料理 / 2 枚の比較。端末内 Vision。2 と合わせるならクラウドでも) — `labels` など 5 つの kind は #659 で入った。料理・名所の細かい種類と 2 枚の比較が残る | 18(5 / 4 / 8 / 1) | 中 | 小(端末内)。分類の種類はバイナリに固定(2.5.2) | 0 | 220 |
| 7 | **OS との連携**(ウィジェット・アラーム(AlarmKit)・スクリーンタイム・カレンダーへの書き込み) — カレンダーへの書き込みは `addToCalendar`(#660)、アラームは `alarm`(#665)で入った。ウィジェット・スクリーンタイムが残る | 17(5 / 7 / 5 / 0) | 中〜大(1 つずつ別の作業) | スクリーンタイムは Family Controls の権限の申請が要る。アラームは AlarmKit(iOS 26) | 1 | 225 |
| 8 | **画像の生成**(Image Playground の端末内 API か、2 のクラウド) | 14(8 / 1 / 5 / 0) | 中 | 端末内なら小。クラウドなら 1.2・4.7(生成物のフィルタ) | 3 | 234 |
| 9 | **読み上げ**(AVSpeechSynthesizer。ボタン・画面を開いた時に読む) — **入った**(`speakButton`・#659) | 14(1 / 2 / 11 / 0) | 小 | なし | 0 | 239 |
| 10 | **ファイルの取り込み**(PDF・文書・音声のファイルを選び、文字を取り出して field へ) — CSV・文書・PDF の `fileImport` は #659 で入った。音声のファイルが残る | 11(2 / 4 / 5 / 0) | 小〜中 | 小 | 1 | 246 |
| 11 | リアルタイムの同時操作・対戦(WebSocket・#424 案 E) | 14(2 / 2 / 10 / 0) | 大 | 小。接続中は費用がかかり続ける | 1 | 248 |
| 12 | カメラ映像のリアルタイム認識(回数のカウント・ジェスチャー・連続の姿勢推定) | 28(5 / 0 / 23 / 0) | 大 | カメラの撮影は審査用の版で外している(4.7.2・2.5.14)。認識の種類はバイナリに固定し、DSL では選ぶだけ(2.5.2) | 7 | 261 |
| 13 | フォロー・タイムライン(いいねは `entity.marks` で既にある) — **入った**(`follow`・#679) | 5(1 / 4 / 0 / 0) | 小〜中 | 1.2(通報・ブロック・#321) | 0 | 262 |
| 14 | 背景での常時計測(睡眠・加速度・マイク) | 6(2 / 1 / 3 / 0) | 中 | 2.5.4(背景モードは本来の用途だけ)・4.7.2 | 0 | 265 |
| 15 | ゲームの画面(物理・当たり判定・アニメーション) | 7(1 / 2 / 4 / 0) | 大 | 2.5.2(遊び方を DSL で書かせると実質コード) | 0 | 267 |
| 16 | 通話・ビデオ通話(#424) | 11(0 / 4 / 7 / 0) | 大 | #424 の通り。M4 以降 | 0 | 274 |
| - | 決済 | 4(0 / 4 / 0) | 0(#413 の案 B = `openUrlButton` で外の支払いページを開く、で 4 作品とも ○) | 3.1.1 / 3.1.3 は #413 の通り | - | - |
| - | その他 1 作品ずつ: 立体音響・目的地に着いたら通知・トーナメント表・ブロックチェーン | 各 1 | - | - | - | - |

**目標への道筋(数の見込み)**: 11-06 の 148(43%)→ 今 164(48%)→ 1〜5 を足して 214(63%)→ 6〜10 を足して 246(72%)。4・5・9・13 は入り、1・3・6・7・10 は一部が入った(残る △ は、ほかの物も要る作品)。ここから先は × の作品が中心で、部品を足しても本体にならない物が多い。「ほぼ全部」に近づけるには、× を「機能を落とした版」で作れたと数えるかどうかの決め方による(次の節)。

## × のもの: 難しい理由と代わりの案

× の 100 作品の主な理由(1 作品で複数)。

| 理由 | 作品の数 | どうしても無理な理由 | 代わりの案(機能を落とした版) |
|---|---|---|---|
| 自作の機器・BLE・IoT | 24 | 機器ごとに通信の取り決めが違い、それを DSL に書かせると実質のコード(2.5.2)。機器そのものは Tailor It の外 | 機器の値を手入力・`sensorValue`・`healthStat` で代わりに記録し、グラフ・通知・共有にする。標準の BLE の形(心拍計・体重計など)だけを読む部品なら作れる(相談) |
| カメラ映像のリアルタイム認識 | 23 | 認識の種類ごとに別のモデル・処理が要る。審査用の版ではカメラの撮影を外している | 写真を 1 枚撮って `photoField.analyze`(関節の角度・分類)で読む版。回数は手で数える・`timerDisplay` で時間を計る |
| 独自の機械学習モデル | 21 | 作品の価値がモデルそのもの。後からモデルを配ると 2.5.2 に近い(#426 は「重みをデータとして配り、使える機能は増やさない」の整理) | `aiButton` の classify / `analyze` の classify で候補から選ぶ版、チェックリストで人が判定する版 |
| AR・3D | 16 | 作品ごとに 3D の中身・置き方が違い、部品 1 つにまとめにくい | 写真・地図(`mapView`)・記録で同じ情報を見せる版 |
| クラウド AI が本体で、ほかの物も複数要る | 19 | 音声の会話(文字起こし + 生成 + 読み上げ)・動画の生成など、2 と 4 と 9 を全部足しても届かない物がある | 文字で質問して端末内の AI が短く返す版、カードで練習する版 |
| 音声(文字起こし・読み上げ)を複数組み合わせる | 15・11 | 通話・音声の翻訳・会話の練習など | 録音の記録 + 手入力のメモ |
| ブラウザ拡張・PC の操作 | 9 | モバイルの外の機能 | 共有シートから URL を受けて記録する版(共有の拡張を足せば作れる) |
| 通話・リアルタイムの対戦 | 7・10 | #424 の通り | `openUrlButton` の `tel:`・ターン制・ランキング(`public` の entity)版 |

## 受賞・決勝の作品

公式サイトの結果ページに載った作品のうち、公開リポジトリがある 61 作品(2022: 15・2023: 15・2024: 16・2025: 15。リポジトリの見つからない 2 作品を除く。1 作品は README が雛形で ?)。

| 判定 | 数 |
|---|---|
| ◎ | 7 |
| ○ | 14 |
| △ | 7 |
| × | 32 |
| ? | 1 |

× の理由の多くは自作の機器(12)・AR(9)・クラウド AI を含む複数の組み合わせ(8)・カメラの連続認識(5)。受賞作は「ハードウェアを作った」「新しい体験を作った」作品が多いため、日常の記録・共有が中心の全体よりも × の割合が高い。一覧の「受賞など」の列に、どの作品かを載せた。

## 測り方

上の ◎○ は README を読んだ見込みで、AI が本当にその作品を作れるかは数えていない。そこで、過去の作品を元にした ai-eval のケースで「作れた割合」を測る。実際に Claude を呼ぶのは人が決めた時だけ(下のコマンド)。

### ケース

| ファイル(`backend/internal/ai/eval/`) | 作品 | group | 中身 |
|---|---|---|---|
| `jphacks_coverage_cases.json` | 39 | `cov-full`(◎ 21)・`cov-reduced`(○ 18) | develop にある部品だけで作れる作品(記録を読むルール `ruleAgg`・`ruleEach` は #658、文字起こし・手書き・読み上げ・写真の labels・fileImport・feed は #659、カレンダーへの追加は #660 で入った) |
| `jphacks_coverage_pending_cases.json` | 0 | なし | 開いている PR の部品が要る作品(今は無い。新しい部品の PR を待つ作品ができたら `cov-pr<N>` で入れる) |

- **選び方**: 判定 ◎○△ の 241 作品から 39 作品。受賞・決勝の ◎○△ 28 作品のうち 18 作品を入れ、残りは機能の分類(共有・記録・計算・通知・地図・写真・QR・AI・音声・マッチング・音楽)が偏らないように選んだ。× は入れていない(本体が作れないため)。△ は、足りない物が開いている PR で入る作品だけを PR 待ちのファイルに入れた。
- **依頼の文**: 利用者が話す形で、その作品の本体の体験を 1〜2 文で頼む。作品名・チーム名は入れず、`note` の先頭にリポジトリの名前(`jphacks/<repo>`)・判定・受賞を書いた。
- **期待の付け方**: 本体の体験に要る物だけを、既存の期待のキーで書く(部品 `has`・共有 `shared` / `scope`・計算 `calc` / `computed`・通知 `notification`・写真の読み取り `photoAnalyze`・AI `aiGenerate` / `aiTask` / `aiLength`・曲のリクエスト `musicRequests`・マッチング `matchProfile`・ルール `serverRules` / `ruleTrigger` / `ruleAction` など)。見た目・画面の数・タブは見ない。○ の作品は「欠けるもの」(一覧のメモ)を期待に入れない。
- **PR 待ちの期待**: develop に無い部品は `has` の名前(`transcribeField` など)で書く。PR が足す期待のキー(`addToCalendar` など)は `pendingExpect` に置き、PR がマージされてキーが入ると自動で確かめる対象になる(それまでは実行のログに「not checked until the PR is merged」と出て、確かめない)。
- **作れた** = 生成が成功し、期待をすべて満たした作品。試験(`backend/cmd/ai-eval/coverage_test.go`)が、作品の数(30〜40)・依頼の文にチーム名が無いこと・develop のファイルが develop の部品だけを使うこと・PR 待ちのケースが本当に新しい部品を要ることを確かめる。

### コマンド(`backend/` で)

```sh
# 期待の書き方だけを確かめる(API を呼ばない)
go run ./cmd/ai-eval -check -cases internal/ai/eval/jphacks_coverage_cases.json
go run ./cmd/ai-eval -check -cases internal/ai/eval/jphacks_coverage_pending_cases.json

# 実際に回す(人が決めた時だけ。-logic-pass / -server-rules は本番の AI_LOGIC_PASS / AI_SERVER_RULES に合わせる)
go run ./cmd/ai-eval -env-file ../.env -cases internal/ai/eval/jphacks_coverage_cases.json -logic-pass -server-rules -tag jphacks
go run ./cmd/ai-eval -env-file ../.env -cases internal/ai/eval/jphacks_coverage_pending_cases.json -logic-pass -server-rules -tag jphacks-pending  # PR 待ちの作品がある時

# 件数を絞る(受賞作の 3 件だけ・落ちた作品だけを回し直す)
go run ./cmd/ai-eval -env-file ../.env -cases internal/ai/eval/jphacks_coverage_cases.json -logic-pass -server-rules -tag jphacks -only cov_poker_chips,cov_trash_dex,cov_car_dj

# 保存した結果から作れた割合を出す(API を呼ばない。同じ id は後のファイルの結果を使う)
go run ./cmd/ai-eval -score '../docs/backend/ai-eval/raw/*-jphacks.json'
go run ./cmd/ai-eval -score '../docs/backend/ai-eval/raw/*-jphacks*.json'  # PR 待ちも合わせて
```

実行の最後と `-score` に group ごとの「pass / checked」(= 作れた / 作品)と total が出る。`-score` は作れなかった作品の外れた期待も一覧にする。

### 件数・費用の目安

| 回し方 | 新規作成の回数 | 費用の目安 |
|---|---|---|
| develop のファイル | 39 | $1.2〜3.9(2 回目の呼び出し `-logic-pass` / `-server-rules` を含めると多くて $6 程度) |
| PR 待ちのファイル | 0 | $0 |
| `-only` で 3 件 | 3 | $0.1〜0.3 |

1 回の新規作成を $0.03〜0.10 と置いた(`docs/backend/rate-limit.md` の目安。モデルは ai-eval の既定 `claude-sonnet-5-5`・effort low)。実測の `usage` は結果のファイルに残るので、1 回目の後に直す。ケースは直列に回るので、1 件 30〜60 秒として 39 件で 20〜40 分。

### 目標との関係

- 目標の指標は「判定できた 341 作品のうち ◎○ の割合」(11-06 の 43% → 今 48% → 60% 以上)。この割合は README からの見込みなので、**ケースの作れた割合を掛けて実際の見込みにする**: 実際の ◎○ ≒ ◎ 57 × `cov-full` の割合 + ○ 107 × `cov-reduced` の割合。例えば両方 80% なら 164 × 0.8 ≒ 131 作品(38%)で、見込みの 48% より低い。
- 63% の見込み(214 作品)は「足すべき上位 5 つ」が入った後の数。PR 待ちのケース(`cov-pr*`)は、部品が入って △ から ◎○ になるはずの作品が本当に作れるかを測る。マージ後に `cov-pr*` の割合が低ければ、部品はあっても AI が使えていない(プロンプト・カタログの直し)ことが分かる。
- 1 作品で約 3 ポイント動く標本なので、1 回の差は揺れとみる。比べる時は同じモデル・effort で回し、揺れを見るなら `-repeat 2`(実行の最後の表は 2 回分を数える。`-score` は同じ id の最後の 1 件だけを使う)。

## 相談が要ること

| # | 相談 | 理由 |
|---|---|---|
| 1 | 目標の「ほぼ全部」を、◎○(本体が作れる)で数えるか、× の「機能を落とした版」も入れて数えるか | 部品を足しても × の約 100 作品(受賞作の半分)は本体にならない。2026-11-14 までに上位 5 つを足しても 63%(214 作品)。機能を落とした版を数えるなら、判定できた 341 作品のうち 339 作品に作り方か代わりの案がある(× 100 のうち 98 に代わりの案) |
| 2 | クラウドの生成 AI を生成アプリから使えるようにするか | 56 作品が要る(2 位)。費用・4.7・プライバシーの文言(「データは端末外に出ない」)に関わる。#426 では案 F として参考に載せただけ |
| 3 | 外部 API の最初の連携先(#348)を何にするか | 60 作品が要る(1 位)。今回の作品の説明・メモに多いのは 地図の施設・場所の検索、カレンダー(Google)、メッセージ(Slack・LINE)、GitHub、ニュース・天気・SNS・音楽(Spotify) |
| 4 | 標準の BLE の形(心拍計・体重計など)だけを読む部品を作るか | 自作の機器の作品(24)は作れないままだが、市販の機器で代わりにできる作品がある |
| 5 | カメラの連続認識(回数・ジェスチャー)を足すか | 28 作品が要るが、審査用の版でカメラの撮影を外していることとの関係を先に決める |
| 6 | 2021 年以前の作品も数えるか | 約 700 本ある。同じ方法で 1〜2 時間で足せる |

## 作品の一覧

列: チーム(リポジトリ)・作品名(README の題。公式の結果ページの名前と違う時は併記)・何をするものか(1 文)・使っている機能・判定・足りない物(上の表のタグ)・メモ(○ = 欠けるもの / △ = 足すもの / × = 理由と代わりの案)・受賞など(公式サイトの結果ページ)。

足りない物のタグ: `EXT_API` 外部 API / `CLOUD_AI` クラウドの生成 AI / `SERVER_LOGIC` サーバーの処理 / `STT` 文字起こし / `CANVAS` 手書き・描画 / `IMAGE_AI` 写真の理解 / `OS_INTEG` OS との連携 / `IMAGE_GEN` 画像の生成 / `TTS` 読み上げ / `FILE_IMPORT` ファイルの取り込み / `REALTIME` リアルタイム / `CAMERA_LIVE` カメラの連続認識 / `FEED_UI` フォロー・タイムライン / `BG_SENSE` 背景の常時計測 / `GAME` ゲームの画面 / `CALL` 通話 / `PAYMENT` 決済 / `BLE_IOT` 機器との通信 / `WEARABLE` ウェアラブル / `AR3D` AR・3D / `CUSTOM_ML` 独自モデル / `WEB_ONLY` ブラウザ拡張・PC / `新:…` その他

### 2022 年(106 作品)

| チーム(リポジトリ) | 作品名 | 何をするものか | 使っている機能 | 判定 | 足りない物 | メモ | 受賞など |
|---|---|---|---|---|---|---|---|
| [A_2201](https://github.com/jphacks/A_2201) | Zoo Saver | ブックマークに理由や評価を添え、動物で識別しチームで共有する | 記録,共有 | ◎ | - | - |  |
| [A_2202](https://github.com/jphacks/A_2202) | いえサガシ | 現在地周辺の物件をARで重ねて表示し探せる | 地図,外部API,AR3D | △ | EXT_API | 物件API連携が必要。ARは地図ピンで代替 |  |
| [A_2203](https://github.com/jphacks/A_2203) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [A_2204](https://github.com/jphacks/A_2204) | - | ダイエット記録でキャラが育ち他人と競うアプリ | 記録,ゲーム,共有 | ○ | CANVAS | キャラの成長演出は段階画像で簡素化 |  |
| [A_2205](https://github.com/jphacks/A_2205) | DigLike | Twitterのいいねを自動分類しPodcastにする | 外部API,AI,音声 | × | EXT_API,TTS,CLOUD_AI | Twitter連携と音声生成が本体。手動貼付メモ+要約で代替 |  |
| [A_2206](https://github.com/jphacks/A_2206) | Chill-Chill Yoga Fire | ゲーム感覚でヨガを続けるアプリ | カメラ,ゲーム,AI | △ | CAMERA_LIVE | ポーズ判定は写真の関節角度で代替し連続判定を足す |  |
| [A_2207](https://github.com/jphacks/A_2207) | VRooM | アバターが見守る作業タイマーとスクワット休憩 | AR3D,カメラ,音声,記録 | × | AR3D,CAMERA_LIVE,TTS | 3Dアバターと姿勢連続認識が本体。タイマー+継続記録版 | 企業賞 |
| [A_2208](https://github.com/jphacks/A_2208) | CookinGit | 調理工程をフローチャートで編集するレシピアプリ | フォーム,その他 | ◎ | CANVAS | drawingField(手書き・#659)でフローチャートを描いて手順と一緒に保存する | 決勝 |
| [A_2209](https://github.com/jphacks/A_2209) | Sound Direction(公式の一覧: Sound Directions) | 目的地の方角から音が鳴る立体音響の道案内 | 地図,音声 | △ | 新:立体音響 | 空間音響の再生部品が要る。routeGuideで代替可 | 企業賞 |
| [A_2210](https://github.com/jphacks/A_2210) | こんだTech | 食材在庫と作り置きの経過日数を管理しレシピを探す | 記録,通知,外部API | ○ | EXT_API | レシピ自動提案が欠ける。手入力のレシピ登録で代替 |  |
| [A_2211](https://github.com/jphacks/A_2211) | Priority TOMATO | タスクをトマトの大きさと色で優先度・期限表示する | 記録,通知 | ○ | CANVAS | トマト絵の可変表示は段階アイコンで代替 |  |
| [A_2212](https://github.com/jphacks/A_2212) | Teval | 大学の履修データから取消率や評価を可視化する | 外部API,記録 | △ | SERVER_LOGIC,EXT_API | 大学データの取得・差分集計が必要 |  |
| [A_2213](https://github.com/jphacks/A_2213) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [A_2214](https://github.com/jphacks/A_2214) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [A_2215](https://github.com/jphacks/A_2215) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [A_2216](https://github.com/jphacks/A_2216) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [A_2217](https://github.com/jphacks/A_2217) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [A_2218](https://github.com/jphacks/A_2218) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [A_2219](https://github.com/jphacks/A_2219) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [A_2220](https://github.com/jphacks/A_2220) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [B_2201](https://github.com/jphacks/B_2201) | (仮)オフラインワークショップ支援 with Post itアプリ | 付箋を文字起こししLINE経由で可視化する | カメラ,AI,外部API | ○ | EXT_API | LINE連携と形態素解析が欠ける。OCR+AI抽出で代替 |  |
| [B_2202](https://github.com/jphacks/B_2202) | te AR ai | 手を映すとAR のウイルスが手洗いで弱るゲーム | カメラ,AR3D,ゲーム | × | CAMERA_LIVE,AR3D | 手の連続追跡と3Dが本体。手洗いタイマー+gauge版 |  |
| [B_2203](https://github.com/jphacks/B_2203) | SUGAO | マスク顔の写真から素顔をAIで生成する | AI,カメラ | × | IMAGE_GEN,CUSTOM_ML | 独自の画像生成モデルが本体で代替が難しい |  |
| [B_2204](https://github.com/jphacks/B_2204) | ペンギン学習塾 | 勉強時間を記録しペンギンを育てランキングで競う | 記録,ゲーム,共有 | ○ | CANVAS | ペンギンの動く育成表示は静止画の段階表示で代替 |  |
| [B_2205](https://github.com/jphacks/B_2205) | Political Debate Chat | トピックを作って参加し話し合える議論チャット | 共有 | ◎ | - | - |  |
| [B_2206](https://github.com/jphacks/B_2206) | SumiMatch | 客が希望物件を登録し不動産会社が提案する逆フリマ | 共有,フォーム,通知 | ○ | EXT_API | 住所補完APIが欠ける。手入力で代替 |  |
| [B_2207](https://github.com/jphacks/B_2207) | - | 別ブランチに成果物がありREADMEから判定不能 | その他 | ? | - | 内容が読めない |  |
| [B_2208](https://github.com/jphacks/B_2208) | AI.pic | 部屋写真と好みから合う絵をAI生成して合成する | AI,カメラ | △ | IMAGE_GEN | 画像生成が要る。既存画像から選ぶ版で代替可 |  |
| [B_2209](https://github.com/jphacks/B_2209) | 自己紹介 de AR | ARマーカーで名前や趣味を重ね表示し出会いを記録する | AR3D,記録,カメラ | ○ | AR3D | AR表示が欠ける。QRで相手のプロフィール表示で代替 |  |
| [B_2210](https://github.com/jphacks/B_2210) | AIcon Maker | タグや心理テストからアイコンを画像生成する | AI,フォーム | △ | IMAGE_GEN | 画像生成が要る。テスト結果から既存アイコン選択で代替 |  |
| [B_2211](https://github.com/jphacks/B_2211) | Qsahaiel | GitHubの草や日課の達成度を光るインテリアで見せる | HW,記録 | × | BLE_IOT | 光る自作機器が本体。streak記録とgaugeで画面版を作る |  |
| [B_2212](https://github.com/jphacks/B_2212) | MCM | データ分析の流れに沿って学習コードを生成する | AI,フォーム | △ | CLOUD_AI | コード生成にクラウドLLMが要る |  |
| [B_2213](https://github.com/jphacks/B_2213) | Dealer(公式の一覧: P-Dealer) | ポーカーのチップ管理を全員のスマホで進める | 共有,記録,ゲーム | ◎ | - | - | 決勝 |
| [B_2214](https://github.com/jphacks/B_2214) | まどりーだー | 間取り図から部屋の寸法を測りVRで内見する | AI,AR3D,カメラ | × | CUSTOM_ML,AR3D | 独自セグメンテーションとVRが本体。寸法手入力の距離計算版 | 審査委員特別賞 |
| [B_2215](https://github.com/jphacks/B_2215) | AIdist | 顔画像がAI生成かを判定し怪しい箇所を示す | AI,カメラ | × | CUSTOM_ML | 独自判定モデルが本体。見分け方チェックリスト版で代替 |  |
| [B_2216](https://github.com/jphacks/B_2216) | Health Chair | 椅子の荷重センサーで体重と座り時間を記録する | HW,記録 | × | BLE_IOT | 自作センサーが本体。体重・座位時間の手入力と推移グラフ | 企業賞 |
| [B_2217](https://github.com/jphacks/B_2217) | SpeakDuck | アヒルに話しかけた内容を文字起こしして残す | 音声,記録 | ◎ | STT | transcribeField(文字起こし・#659)で話しかけた内容を文字にして記録に入れる |  |
| [B_2218](https://github.com/jphacks/B_2218) | おてがるAI秘書 | 話し言葉から予定と課題を抽出して登録する | AI,音声,記録 | ○ | STT | 声は transcribeField(#659)で文字にでき、予定と課題の抽出は端末内 AI で簡易 |  |
| [B_2219](https://github.com/jphacks/B_2219) | Sutrace | GitHubのコミット順に写経して学ぶサイト | 外部API,Web専用 | × | EXT_API,CANVAS | コード差分エディタが本体。学習記録ノート版で代替 |  |
| [B_2220](https://github.com/jphacks/B_2220) | あんしん夜道 | 偽の通話を流し応答が途絶えると通報する | 音声,HW,地図 | × | CALL,TTS,STT | 通話と自動音声が本体。位置付きSOS共有ボタン版 | 審査委員特別賞 |
| [B_2221](https://github.com/jphacks/B_2221) | フェイス・サーチ | 顔写真から相手を探しSNSアカウントを交換する | カメラ,独自ML,共有 | × | IMAGE_AI,CUSTOM_ML | 顔認識が本体。QRでSNS交換する版で代替 |  |
| [B_2222](https://github.com/jphacks/B_2222) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [B_2223](https://github.com/jphacks/B_2223) | 零→Fifty | 動画内の外国語の文字を認識し翻訳して重ねる | カメラ,外部API | × | CAMERA_LIVE,EXT_API | 動画処理が本体。写真1枚のOCR+AI言い換え版 |  |
| [B_2228](https://github.com/jphacks/B_2228) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [B_2229](https://github.com/jphacks/B_2229) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [B_2230](https://github.com/jphacks/B_2230) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [C_2202](https://github.com/jphacks/C_2202) | HYOKA | AmazonのURLから商品情報を表とチャートで比較する | 外部API,記録 | △ | EXT_API,SERVER_LOGIC | 商品ページ取得が要る。手入力なら○ |  |
| [C_2203](https://github.com/jphacks/C_2203) | ひろうけん | ゴミを検知して拾うペット型ロボットとAR表示 | HW,AR3D,カメラ | × | BLE_IOT,AR3D | 自作ロボットが本体で代替が難しい |  |
| [C_2204](https://github.com/jphacks/C_2204) | Parallel Memory | 写真の人物を切り抜き背景や写真に合成して投稿する | カメラ,共有,AI | △ | IMAGE_AI,CANVAS | 切り抜きと配置編集が要る |  |
| [C_2205](https://github.com/jphacks/C_2205) | 気配 × 斬り | VRゴーグルで気配斬りの制限範囲を共有して遊ぶ | AR3D,ゲーム,共有 | × | AR3D,GAME,REALTIME | AR/VRが本体。遊ぶ範囲の地図共有版で代替 |  |
| [C_2206](https://github.com/jphacks/C_2206) | Plan Plant | 目標を達成するとマンドラゴラを収穫できる習慣アプリ | 記録,ゲーム | ○ | CANVAS | 育つ絵は段階画像で代替。収穫演出は簡素 |  |
| [C_2207](https://github.com/jphacks/C_2207) | 推し隊 | 推し活の予定とメモをカレンダーで管理する | 記録,通知 | ◎ | - | - |  |
| [C_2208](https://github.com/jphacks/C_2208) | まもるくん | 情報リテラシーを○×クイズで学ぶLINE Bot | 記録,その他 | ◎ | - | - |  |
| [C_2209](https://github.com/jphacks/C_2209) | らくらくトリセツ | 画面録画から操作手順の紙マニュアルを自動作成する | カメラ,その他 | ○ | FILE_IMPORT,IMAGE_AI | 動画からの自動抽出が欠ける。写真手動追加+PDF出力 | 決勝 |
| [C_2210](https://github.com/jphacks/C_2210) | - | 宣伝文のみで中身なし | その他 | ? | - | 内容が読めない |  |
| [C_2211](https://github.com/jphacks/C_2211) | しらべるん(公式の一覧: LINE Bot「しらべるん」) | 話し言葉から検索語を作りGoogle検索につなぐ | AI,外部API | ○ | EXT_API | 検索結果の取得が欠ける。語抽出+検索リンクで代替 | 企業賞 |
| [C_2212](https://github.com/jphacks/C_2212) | JunSche | 前後関係のあるタスクを図で管理しリスケを促す | 記録,通知 | ○ | CANVAS | フロー図が欠ける。依存つきリストと条件通知で代替 |  |
| [C_2213](https://github.com/jphacks/C_2213) | YOULINK | 学生エンジニアと日程を選んで話せる募集サービス | 共有,フォーム,通知 | ◎ | - | - |  |
| [C_2214](https://github.com/jphacks/C_2214) | コワプラ | 条件に合うワーキングスペースを探すアプリ | 外部API,地図 | △ | EXT_API | 施設データAPIが要る。READMEは骨子のみ |  |
| [C_2215](https://github.com/jphacks/C_2215) | Dish/Dining | カメラで机の皿の占有率を測り注文を支援する | カメラ,独自ML | × | CAMERA_LIVE,CUSTOM_ML | 映像の物体検出が本体。占有率の手入力版で代替 |  |
| [C_2216](https://github.com/jphacks/C_2216) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [C_2217](https://github.com/jphacks/C_2217) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [C_2218](https://github.com/jphacks/C_2218) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [C_2219](https://github.com/jphacks/C_2219) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [D_2201](https://github.com/jphacks/D_2201) | Look for Stars | 衛星データから星が見える場所を地図で提案し共有する | 地図,外部API,共有 | △ | EXT_API,SERVER_LOGIC | 衛星・土地データの計算が要る | 決勝 |
| [D_2202](https://github.com/jphacks/D_2202) | Narcissus | 近くの植物を地図で探し、写真と位置を投稿する | 地図,カメラ,共有,AI | ○ | IMAGE_AI | 植物名の推測は Tailor It では簡易になる。タグ検索と手入力で代替 |  |
| [D_2203](https://github.com/jphacks/D_2203) | - | ブラウザ上で攻撃を体験するセキュリティ学習サイト | Web専用,その他 | × | WEB_ONLY,SERVER_LOGIC | 実習環境と自動判定が本体。解説+クイズ版で代替 |  |
| [D_2204](https://github.com/jphacks/D_2204) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [D_2205](https://github.com/jphacks/D_2205) | mapper | 旅行先を地図に記録し友人や公開で共有するSNS | 地図,カメラ,共有 | ◎ | - | - |  |
| [D_2206](https://github.com/jphacks/D_2206) | 曖昧警察 | ニュース記事の文を事実と意見に分けて表示する | AI,外部API | △ | CLOUD_AI,EXT_API | 記事全文の判別と取得が要る。短文分類なら○ |  |
| [D_2207](https://github.com/jphacks/D_2207) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [D_2208](https://github.com/jphacks/D_2208) | スマートポインター | スマホからホストPCのスライドを共同操作する | Web専用,共有 | × | WEB_ONLY,REALTIME | PCの操作が本体。発表タイマーと順番共有版で代替 | 決勝 |
| [D_2209](https://github.com/jphacks/D_2209) | Airdapt | なぞった地域に雰囲気が似た街を衛星画像で探す | 地図,独自ML | × | CUSTOM_ML | 衛星画像の特徴検索が本体。街の条件タグ照合版で代替 |  |
| [D_2210](https://github.com/jphacks/D_2210) | スケジュールマジック | グループの予定から全員が空く日を探す | 共有,記録 | ○ | SERVER_LOGIC | 空き日の濃淡集計は Tailor It では簡易になる。予定の重ね表示で代替 |  |
| [D_2211](https://github.com/jphacks/D_2211) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [D_2212](https://github.com/jphacks/D_2212) | Complete Menu! | 外食店のメニュー制覇を記録しおすすめを出す | 記録,ゲーム | ○ | SERVER_LOGIC | 店メニューは手入力。おすすめ提案が欠ける |  |
| [D_2213](https://github.com/jphacks/D_2213) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [D_2214](https://github.com/jphacks/D_2214) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [D_2215](https://github.com/jphacks/D_2215) | わだいほりだー | 趣味から話題を出し合ってマッチングする | 共有 | ○ | - | 自動マッチングは簡易な絞り込みになる |  |
| [D_2216](https://github.com/jphacks/D_2216) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [D_2217](https://github.com/jphacks/D_2217) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [D_2218](https://github.com/jphacks/D_2218) | - | README雛形のまま | その他 | ? | - | 内容なし |  |
| [E_2201](https://github.com/jphacks/E_2201) | Play with Learn | 野菜などを学べるバカゲー風TPSアクション | ゲーム | × | GAME,AR3D | ゲームエンジン級。野菜クイズの暗記カード版で代替 |  |
| [E_2202](https://github.com/jphacks/E_2202) | YouQuery | エラー文から検索クエリと重要語を提案する | フォーム,AI | ○ | - | 重要箇所のハイライト表示が欠ける | Best Team Collaboration |
| [E_2203](https://github.com/jphacks/E_2203) | レコメニュー | 食べたい料理をおすすめして検索へ飛ばす | AI,外部API | △ | SERVER_LOGIC,EXT_API | 全ユーザー学習の推薦と天気連携を足す |  |
| [E_2204](https://github.com/jphacks/E_2204) | デスマTV | 開発作業をライブ配信しコメントし合う | 共有,AI | × | CALL,REALTIME | 動画配信が難しい。代案は作業ログとコメント共有 |  |
| [E_2205](https://github.com/jphacks/E_2205) | TwiCordGram | 複数SNSのタイムラインを一括表示する | 外部API | △ | EXT_API,OS_INTEG | SNS API連携を足す |  |
| [E_2206](https://github.com/jphacks/E_2206) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [E_2207](https://github.com/jphacks/E_2207) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [E_2208](https://github.com/jphacks/E_2208) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [E_2209](https://github.com/jphacks/E_2209) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [E_2210](https://github.com/jphacks/E_2210) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [F_2201](https://github.com/jphacks/F_2201) | HackathonRunner | 診断と体験投稿でハッカソン参加を支援する | フォーム,共有 | ○ | SERVER_LOGIC | 似たレベルの人の自動推薦が欠ける |  |
| [F_2202](https://github.com/jphacks/F_2202) | ほびすま | 好きな施設の近くの物件を探して推薦する | 地図,外部API | △ | EXT_API,SERVER_LOGIC | 物件API・周辺施設検索・類語推薦を足す | 決勝 |
| [F_2203](https://github.com/jphacks/F_2203) | オタクの履歴書 | 推し活の履歴を地図に残して共有する | 記録,地図,共有 | ○ | EXT_API | Spotifyでのアーティスト自動取得が欠ける |  |
| [F_2204](https://github.com/jphacks/F_2204) | Yoriai | 手料理の会を宣言し通知とチャットで集う | 共有,通知 | ◎ | - | - |  |
| [F_2205](https://github.com/jphacks/F_2205) | のみぱら | オンライン飲み会で並行して会話できる通話 | HW,共有 | × | CALL,REALTIME | 通話が難しい。代案は話題別のチャットルーム | 審査委員特別賞 |
| [F_2206](https://github.com/jphacks/F_2206) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [F_2207](https://github.com/jphacks/F_2207) | リカプセル | 友達と写真を埋めて現地で開けるタイムカプセル | 地図,カメラ,共有 | ○ | AR3D | AR探索が欠ける。地図と現地での開封は残る | 企業賞 |
| [F_2208](https://github.com/jphacks/F_2208) | 貯Pay箱 | 家族でTodoを共有し報酬を入出金する | 記録,共有,決済 | ○ | PAYMENT | PayPay入出金が欠ける |  |
| [F_2209](https://github.com/jphacks/F_2209) | - | アバターと一緒に休めるメンタル回復アプリ | 記録,ゲーム | ○ | CANVAS | アバターの動きが欠ける。ポイント記録は残る |  |
| [F_2210](https://github.com/jphacks/F_2210) | Fishery Eye | 海洋のクロロフィル濃度を地図で確認できる | 地図,外部API | △ | EXT_API | 海洋データAPIが要る |  |
| [F_2211](https://github.com/jphacks/F_2211) | A cup of coffee | コーヒー一杯の値段でカフェを地図で探す | 地図,共有 | ◎ | - | - |  |
| [F_2212_1](https://github.com/jphacks/F_2212_1) | Info_Checker | 登録URLの更新をウィジェットで知らせる | 通知,外部API | △ | SERVER_LOGIC,OS_INTEG | サイト監視とウィジェットが要る |  |
| [F_2213](https://github.com/jphacks/F_2213) | プロリス | 音声の一部にタグを付けて聞き返し口の動きも見る | 音声,記録 | △ | FILE_IMPORT | 音声取込が要る。口の動き表示は省く |  |
| [Z_2210](https://github.com/jphacks/Z_2210) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |

### 2023 年(111 作品)

| チーム(リポジトリ) | 作品名 | 何をするものか | 使っている機能 | 判定 | 足りない物 | メモ | 受賞など |
|---|---|---|---|---|---|---|---|
| [KB_2301](https://github.com/jphacks/KB_2301) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [KB_2302](https://github.com/jphacks/KB_2302) | 探しAI | カメラ映像で物の出し入れを追跡し探し物を検索する | カメラ,独自ML | × | CAMERA_LIVE,CUSTOM_ML | 常時物体追跡は難しい。代案は置き場所を写真付きで記録検索 |  |
| [KB_2303](https://github.com/jphacks/KB_2303) | てるみー | LINEで学習者に定期ヒアリングしAIが進捗をまとめる | AI,通知,共有,記録 | △ | CLOUD_AI | 対話AIが必要。定期質問と管理者閲覧は現行で可 |  |
| [KB_2304](https://github.com/jphacks/KB_2304) | 着いたぞ!降りるぞ!!! | 目的地に近づくと距離を通知し乗り過ごしを防ぐ | 地図,通知 | △ | 新:位置到達通知 | 目的地への接近で鳴る通知を足す |  |
| [KB_2305](https://github.com/jphacks/KB_2305) | Specialize Todo! | 優先度マトリックス付きのチーム共有Todo | 記録,共有,フォーム | ◎ | - | - |  |
| [KB_2306](https://github.com/jphacks/KB_2306) | LyricScribe | 曲ファイルをWhisperで文字起こしし歌詞を同期表示する | 音声,AI | × | STT,FILE_IMPORT,CLOUD_AI | 自動文字起こしが難しい。代案は歌詞を手入力し曲と表示 |  |
| [KB_2307](https://github.com/jphacks/KB_2307) | BuccalTimer | スマホを伏せて集中するポモドーロと専用ライト連動 | HW,ゲーム,AR3D | × | BLE_IOT,AR3D | 専用ライトと3D演出が本体。代案は伏せ検知付きタイマー | 企業賞 |
| [KB_2308](https://github.com/jphacks/KB_2308) | UnMap | 現在地を地図でなく文章(地名)で共有するアプリ | 地図,共有 | ○ | EXT_API | 座標から文章への自動変換が欠ける(地名は手動選択) |  |
| [KB_2309](https://github.com/jphacks/KB_2309) | PosturePro | Webカメラで姿勢を常時判定し悪化時に通知する | カメラ,通知,AI | △ | CAMERA_LIVE | 連続の姿勢推定を足す。写真1枚の角度判定なら現行で可 |  |
| [KB_2310](https://github.com/jphacks/KB_2310) | TECH MATCH | 複数AIがプレゼン/面接の練習に講評を返す | AI,音声 | × | CLOUD_AI,STT,FILE_IMPORT | 複数LLMと音声評価は難しい。代案は練習記録と自己評価表 |  |
| [KB_2311](https://github.com/jphacks/KB_2311) | FillHome | 内見前の物件に実際に泊まれる予約サービス | フォーム,共有,記録 | ○ | PAYMENT | 宿泊の決済と鍵の受け渡しが欠ける |  |
| [KB_2312](https://github.com/jphacks/KB_2312) | rewekey | 階層なしで1ページ1内容に整理するwiki風メモ | 記録 | ○ | - | ページ間リンクのwiki記法は Tailor It では簡易になる |  |
| [KB_2313](https://github.com/jphacks/KB_2313) | Go! Gomibako | ゴミ箱の位置を地図に出しQRで満杯報告できる | 地図,共有,カメラ | ◎ | - | - |  |
| [KB_2314](https://github.com/jphacks/KB_2314) | 健康習慣 | 運動しないと止まらない目覚ましと就寝前ロック | HW,通知 | △ | OS_INTEG,BG_SENSE | アラームは alarm(#665)で可。スマホの利用制限が残る。回数計測は可 |  |
| [KB_2315](https://github.com/jphacks/KB_2315) | 革靴メンテなんです!! | 温湿度センサで革靴の手入れ時期を通知する装置 | HW,通知 | × | BLE_IOT | 自作装置が本体。代案は履いた日の記録と手入れ周期の通知 | Best Hack Award / Innovator |
| [KB_2316](https://github.com/jphacks/KB_2316) | 老人見守りIoT エコー | トイレの人感センサで1日無反応ならLINEで警告 | HW,通知 | × | BLE_IOT | センサー機器が本体。代案は毎日の安否ボタンと未記録の通知 | 決勝 |
| [KB_2317](https://github.com/jphacks/KB_2317) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [KB_2318](https://github.com/jphacks/KB_2318) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [KB_2319](https://github.com/jphacks/KB_2319) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [KB_2320](https://github.com/jphacks/KB_2320) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [NG_2301](https://github.com/jphacks/NG_2301) | 気圧変化確認ツール | 地点を登録して気圧の予測グラフを見る | 外部API,記録 | ○ | EXT_API | 天気の予報は feed の weather(#659)で出せる。気圧の予測グラフは欠ける |  |
| [NG_2302](https://github.com/jphacks/NG_2302) | DJふっきん(公式の一覧: DJ腹筋) | 車内の全員が曲をリクエストして順に流す | 音声,共有 | ○ | STT,BG_SENSE | 音声リクエストと音量の自動調整が欠ける | 決勝 |
| [NG_2303](https://github.com/jphacks/NG_2303) | Adversarial Armor | AIが誤認識する画像を当てるクイズ | AI,ゲーム | × | CUSTOM_ML,IMAGE_AI | AI判定が本体。代案は固定問題の画像クイズ |  |
| [NG_2304](https://github.com/jphacks/NG_2304) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [NG_2305](https://github.com/jphacks/NG_2305) | PredicTalk | 英会話中の次の単語を予測して視覚的に補助する | 音声,AI,AR3D | × | STT,CLOUD_AI,AR3D | 音声認識と予測表示が本体。代案はフレーズ集 | Best Audience Award / Innovator |
| [NG_2306](https://github.com/jphacks/NG_2306) | Alexander | 予定のクイズに声で答えると止まるアラーム | 音声,通知 | ○ | OS_INTEG,STT | alarm(止めるまで鳴る・#665)と transcribeField(#659)の声の回答で作れる。答えないと止まらない制御は欠ける |  |
| [NG_2308](https://github.com/jphacks/NG_2308) | fifteel | 検索活動を見て15分悩んだら知らせる | Web専用,通知 | × | WEB_ONLY | ブラウザ監視が本体。代案は15分タイマー通知 |  |
| [NG_2309](https://github.com/jphacks/NG_2309) | CalmEnginer | コミット間隔からストレス値を推定して通知 | 外部API,独自ML | × | EXT_API,CUSTOM_ML | 異常検知モデルが本体。代案は手入力の気分記録 |  |
| [NG_2310](https://github.com/jphacks/NG_2310) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [NG_2311](https://github.com/jphacks/NG_2311) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [NG_2312](https://github.com/jphacks/NG_2312) | LenBow | 友達との本の貸し借りをLINEとQRで管理する | 記録,通知,共有,カメラ | ○ | EXT_API | 書籍情報の自動取得が欠ける(書名は手入力) | 企業賞 |
| [NG_2313](https://github.com/jphacks/NG_2313) | T.H.U.R.S.D.A.Y. | 手元のPDFを読んで答えるデスクトップAIチャット | AI | × | CLOUD_AI,FILE_IMPORT | RAGチャットは難しい。代案はメモ記録とaiButton要約 |  |
| [NG_2314](https://github.com/jphacks/NG_2314) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [NG_2315](https://github.com/jphacks/NG_2315) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [NG_2316](https://github.com/jphacks/NG_2316) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [OL_2301](https://github.com/jphacks/OL_2301) | Cranberry ほめちゃん | AIが何でも褒めて相談に乗るiOSチャットと気分記録 | AI,記録 | △ | CLOUD_AI | 長い会話のAIが必要。気分の記録と振り返りは現行で可 |  |
| [OL_2302](https://github.com/jphacks/OL_2302) | pills(公式の一覧: pease) | 低用量ピルの服用記録と時刻・在庫の通知、パートナー共有 | 記録,通知,共有 | ◎ | - | - | Innovator |
| [OL_2303](https://github.com/jphacks/OL_2303) | メール・カレンダー自動同期システム | 届いたメールから日時を抽出しGoogleカレンダーに追加する | AI,外部API | ○ | EXT_API,OS_INTEG | 端末のカレンダーへは addToCalendar(#660)で入る。メールの自動取得は欠ける(貼り付けて抽出) |  |
| [OL_2304](https://github.com/jphacks/OL_2304) | 野良猫マップ | 野良猫の目撃を地図で共有し新顔を近所に通知する | 地図,共有,通知,カメラ | ○ | - | 近所限定の通知が弱く全員または地域単位になる |  |
| [OL_2305](https://github.com/jphacks/OL_2305) | Make Playlist With... | Spotifyの曲をBPMでクラスタリングし新プレイリストを作る | 外部API,音声 | △ | EXT_API,SERVER_LOGIC | 楽曲データ取得とクラスタ計算を足す |  |
| [OL_2306](https://github.com/jphacks/OL_2306) | フォトマ | 撮りたい人と撮られたい人を投稿とDMでつなぐ | 共有,カメラ | ◎ | - | - |  |
| [OL_2307](https://github.com/jphacks/OL_2307) | ゆる友 | イベントごとにカードを送り合い疎遠を防ぐSNS | 共有,通知 | ◎ | - | - |  |
| [OL_2308](https://github.com/jphacks/OL_2308) | どれすたいる | 服の仮想試着やクローゼット管理をするファッションアプリ | AI,カメラ,記録 | × | IMAGE_GEN | 仮想試着は画像生成が必要。代案はクローゼットとコーデの記録 |  |
| [OL_2309](https://github.com/jphacks/OL_2309) | 簡単名刺交換 | 相手に合わせたSNSをQRでまとめて共有する | 共有,カメラ | ◎ | - | - |  |
| [OL_2310](https://github.com/jphacks/OL_2310) | 本棚管理アプリ | 部活の本棚で本の貸し借りとタグ分類を管理する | 記録,共有 | ◎ | - | - |  |
| [OL_2311](https://github.com/jphacks/OL_2311) | Kanpai! | カップホルダー型デバイスの乾杯でSNSを交換する | HW,共有 | × | BLE_IOT | 乾杯検知の機器が本体。代案はQR交換と乾杯数の共有カウンタ | 審査委員特別賞 / Innovator |
| [OL_2312](https://github.com/jphacks/OL_2312) | PaaD: Paper as a Device | 紙のメモや図をカメラで切り出しPDFに貼り込む | カメラ,Web専用 | × | FILE_IMPORT,CANVAS,IMAGE_AI | PDFへの貼り込みが本体。代案は紙のメモを写真で記録整理 |  |
| [OL_2313](https://github.com/jphacks/OL_2313) | ismet | 自炊の写真とレシピを投稿し食べ物でモンスターを育てる | 共有,ゲーム,AI,カメラ | ○ | GAME,CLOUD_AI | モンスター描画と献立自動生成が欠ける(段階表示で代替) |  |
| [OL_2314](https://github.com/jphacks/OL_2314) | Emotter | 表情と文章の感情が一致する投稿を優遇するSNS | カメラ,AI,独自ML | × | CAMERA_LIVE,CUSTOM_ML | 表情認識が本体。代案は感情を選んで投稿しAI分類で照合 |  |
| [OL_2315](https://github.com/jphacks/OL_2315) | DIGICRE | 個人開発者の発信と支援者をつなぐSNS | 共有,決済 | ○ | PAYMENT | アプリ内の支援決済が欠ける(外部リンクで代替) |  |
| [OL_2316](https://github.com/jphacks/OL_2316) | ECOSNAP | ゴミ写真からリサイクル方法とCO2削減量を表示する | カメラ,AI | ○ | IMAGE_AI | 独自物体検知の精度が欠ける(候補からの分類で代替) |  |
| [OL_2317](https://github.com/jphacks/OL_2317) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [OL_2318](https://github.com/jphacks/OL_2318) | みんなでTodo | 講義の課題を共有し進捗は各自で管理するTodo | 記録,共有 | ◎ | - | - |  |
| [OL_2319](https://github.com/jphacks/OL_2319) | Dev Study | 初心者エンジニア向けのタグ付きチャットチャンネル | 共有 | ◎ | - | - |  |
| [OL_2320](https://github.com/jphacks/OL_2320) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [OL_2321](https://github.com/jphacks/OL_2321) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [OL_2322](https://github.com/jphacks/OL_2322) | Message-Log-Viewer | Slackの履歴を保存して閲覧しAIで要約するツール | 外部API,AI,共有 | △ | EXT_API,OS_INTEG | Slack連携を足す。履歴閲覧と要約の画面は現行で可 |  |
| [OL_2323](https://github.com/jphacks/OL_2323) | Farmatch | 後継者を探す農家と就農希望者をつなぐマッチング | 共有 | ◎ | - | - |  |
| [OL_2324](https://github.com/jphacks/OL_2324) | 持ち物ロック画面通知 | 持ち物や移動教室の情報をロック画面の通知で見せる | 通知,記録 | ◎ | - | - |  |
| [OL_2325](https://github.com/jphacks/OL_2325) | Peek | 友人同士で部屋の写真を見せ合い片付けの動機にする | 共有,カメラ | ◎ | - | - |  |
| [OL_2326](https://github.com/jphacks/OL_2326) | ProdTalk Master | 英語プレゼンの発話をAIが採点し改善案を出す | 音声,AI | × | STT,CUSTOM_ML | 発話採点が本体。代案は練習記録と改善案の文章生成 |  |
| [OL_2327](https://github.com/jphacks/OL_2327) | Finger Drawing | Webカメラで指の動きを捉え空中で絵を描く | カメラ,AI | × | CAMERA_LIVE,CANVAS | 指追跡と描画が本体。代案は写真とメモの記録 | 企業賞 |
| [OL_2328](https://github.com/jphacks/OL_2328) | TECH MATCH | エンジニア同士をスワイプでマッチングしイベントも載せる | 共有,外部API | ○ | EXT_API,SERVER_LOGIC | イベント自動取得と機械学習の相性判定が欠ける |  |
| [OL_2329](https://github.com/jphacks/OL_2329) | SlackJack | 勉強・開発の時間を記録してグラフで見る | 記録 | ◎ | - | - |  |
| [OL_2330](https://github.com/jphacks/OL_2330) | ほんびより | 図書館の日本十進分類をランダムに表示する | その他 | ◎ | - | - |  |
| [OL_2331](https://github.com/jphacks/OL_2331) | ChocoAlate | 到着時刻から出発時間と移動手段を調べる | 地図,外部API | △ | EXT_API | 乗換検索の外部APIを足す |  |
| [SD_2301](https://github.com/jphacks/SD_2301) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SD_2303](https://github.com/jphacks/SD_2303) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SD_2304](https://github.com/jphacks/SD_2304) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SD_2305](https://github.com/jphacks/SD_2305) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2301](https://github.com/jphacks/SP_2301) | Mooooosic | 自撮りの表情から気分に合う曲を流す | カメラ,AI | ○ | - | 曲は感情ごとの固定リストになる |  |
| [SP_23018](https://github.com/jphacks/SP_23018) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2302](https://github.com/jphacks/SP_2302) | 今夜の献立AI | 冷蔵庫の写真から在庫を出し献立を考える | カメラ,AI,HW | ○ | IMAGE_AI | 写っている物の名前は photoField.analyze の labels(#659・最大 5 つ)で読み、献立は端末内 AI。食材の細かい見分けは簡易 |  |
| [SP_2303](https://github.com/jphacks/SP_2303) | Umap | 行った店を地図に記録し友達と交換する | 地図,記録,共有 | ○ | - | スワイプ推薦と育成キャラが欠ける。「まだ行っていない店」を知らせる・印をつけるのは serverRules の lookup(notExists)で書ける(推薦の土台) | Innovator |
| [SP_2304](https://github.com/jphacks/SP_2304) | ラー麺罪符 | 罪悪感を和らげる言い訳をAIが出す | AI,記録 | ○ | - | 顔文字連打の演出が弱くなる |  |
| [SP_2305](https://github.com/jphacks/SP_2305) | カラクル | 予定とタスクを同時管理し量を動的に割り当てる | 記録,通知 | △ | SERVER_LOGIC | 行動履歴から割り当てる計算を足す |  |
| [SP_2306](https://github.com/jphacks/SP_2306) | おもメロ♪ | 場所に思い出の曲とコメントを紐づけて共有 | 地図,共有 | ◎ | - | - |  |
| [SP_2307](https://github.com/jphacks/SP_2307) | ご意見マップ | 住民が地図に行政への要望を投稿する | 地図,共有 | ◎ | - | - |  |
| [SP_2309](https://github.com/jphacks/SP_2309) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2310](https://github.com/jphacks/SP_2310) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2311](https://github.com/jphacks/SP_2311) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2312](https://github.com/jphacks/SP_2312) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2313](https://github.com/jphacks/SP_2313) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2314](https://github.com/jphacks/SP_2314) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2315](https://github.com/jphacks/SP_2315) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [SP_2316](https://github.com/jphacks/SP_2316) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [TK_2301](https://github.com/jphacks/TK_2301) | 密話廊 | AIでマーダーミステリーを作り皆で遊ぶ | AI,ゲーム,共有 | × | IMAGE_GEN,CLOUD_AI,REALTIME | 生成と同時進行が本体。代案はシナリオ共有とチャット | 決勝 |
| [TK_2302](https://github.com/jphacks/TK_2302) | おかえりPush | 帰宅の合図でオートロック解錠と来客通知 | HW,通知 | × | BLE_IOT | ラズパイ・錠前が本体。代案は帰宅と来客の共有通知 |  |
| [TK_2303](https://github.com/jphacks/TK_2303) | ホッとお風呂 | 浴室のセンサーで入浴中の異常を家族へ通知 | HW,通知 | × | BLE_IOT | センサー自作が本体。代案は入浴タイマーと長風呂通知 | Innovator |
| [TK_2304](https://github.com/jphacks/TK_2304) | Camembert Chat | 翻訳とおすすめ返信つきの英語チャット | 共有,AI | △ | CLOUD_AI | 翻訳とスラング対応の生成AIを足す |  |
| [TK_2305](https://github.com/jphacks/TK_2305) | 花火スポット共有 | 花火の写真と位置を投稿し穴場を探す | 地図,カメラ,共有 | ◎ | - | - |  |
| [TK_2306](https://github.com/jphacks/TK_2306) | Junan Shift | 条件に合うシフトを最適化しカレンダー連携 | フォーム,外部API | △ | SERVER_LOGIC,EXT_API | 最適化計算とGoogleカレンダー連携を足す |  |
| [TK_2307](https://github.com/jphacks/TK_2307) | しぇあまど | 電車の窓からの景色の写真を位置で共有する | 地図,カメラ,共有 | ○ | - | 路線に沿った近くの写真の自動表示は Tailor It では簡易になる |  |
| [TK_2308](https://github.com/jphacks/TK_2308) | 秘書っぴ | Slackで日程調整と会話要約をするAI秘書 | AI,外部API | × | EXT_API,CLOUD_AI,OS_INTEG | Slack・カレンダー連携が本体。代案は日程候補の投票板 |  |
| [TK_2309](https://github.com/jphacks/TK_2309) | Twins Link | お題に近い文をAIに出させる力をLINEで競う | AI,ゲーム | △ | CLOUD_AI,SERVER_LOGIC | 文章の類似度採点と生成AIを足す | 審査委員特別賞 / Innovator |
| [TK_2310](https://github.com/jphacks/TK_2310) | SAFA | 企業の報酬つきゴミ拾いと参加者のマッチング | 地図,カメラ,決済 | ○ | PAYMENT,SERVER_LOGIC | 報酬の支払いと最適な組み合わせ検索が欠ける |  |
| [TK_2311](https://github.com/jphacks/TK_2311) | PicSchedule | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [TK_2312](https://github.com/jphacks/TK_2312) | ATS System | 利用規約の不利な点を抜き出して表示する | AI,Web専用 | ◎ | - | 長文(16KB まで)を貼って aiButton の length long で抜き出す(追記)。規約の自動取得はしない | Innovator |
| [TK_2313](https://github.com/jphacks/TK_2313) | スマートホームメイド | MR空間のAIキャラで家電を操作する | AR3D,AI,HW | × | AR3D,CLOUD_AI,BLE_IOT | MRと家電制御が本体。代案はAIなしの家電メモ | 決勝 |
| [TK_2314](https://github.com/jphacks/TK_2314) | つくってくん | 消費期限順の食材リストと余り物レシピ提案 | 記録,通知,AI | ◎ | - | - |  |
| [TK_2315](https://github.com/jphacks/TK_2315) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [TK_2316](https://github.com/jphacks/TK_2316) | SADISTECH | クリックの度にキャラを鞭打つブラウザ拡張 | Web専用,ゲーム | × | WEB_ONLY,CANVAS | ブラウザ拡張が本体。代案はタップ連打カウンター |  |
| [TK_2317](https://github.com/jphacks/TK_2317) | GoalBuddies | 同じ期間のタスクを持つ人とマッチして励まし合う | 共有 | ◎ | - | - |  |
| [TK_2318](https://github.com/jphacks/TK_2318) | Task Brancher | タスクを細分化しブランチ名と期限を提案する | AI,通知 | ○ | - | Slack連携が欠ける。短い生成文で代替する |  |
| [TK_2319](https://github.com/jphacks/TK_2319) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [TK_2320](https://github.com/jphacks/TK_2320) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [TK_2321](https://github.com/jphacks/TK_2321) | カードバトル通話 | 手持ちのカードで通話しながら対戦する | ゲーム,共有 | × | CALL,REALTIME | 通話が本体。代案は場の状況を共有する盤面ボード |  |
| [TK_2322](https://github.com/jphacks/TK_2322) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [TK_2323](https://github.com/jphacks/TK_2323) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [TK_2324](https://github.com/jphacks/TK_2324) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |
| [Z_2301](https://github.com/jphacks/Z_2301) | - | README が雛形のまま | その他 | ? | - | 判定不能 |  |

### 2024 年(134 作品)

| チーム(リポジトリ) | 作品名 | 何をするものか | 使っている機能 | 判定 | 足りない物 | メモ | 受賞など |
|---|---|---|---|---|---|---|---|
| [fk_2401](https://github.com/jphacks/fk_2401) | UECS Navi | ノーコードのワークフローでハウス内機器を制御する | HW,フォーム | × | BLE_IOT | 制御機器との通信が本体。代案は温度条件で通知する記録アプリ | 決勝 |
| [fk_2402](https://github.com/jphacks/fk_2402) | ワーキングサポート | 社内文書をRAGで検索しAIが業務の質問に答える | AI,共有 | △ | CLOUD_AI,FILE_IMPORT | LLMのRAG回答と文書取込を足す |  |
| [fk_2403](https://github.com/jphacks/fk_2403) | プロとも | 対面でプロフィールを交換しアバターで自己表現する | 共有,地図 | ○ | REALTIME | 相手の方向を示すライブ表示が欠ける |  |
| [fk_2404](https://github.com/jphacks/fk_2404) | Yurushiru | ガスセンサ検知でNFTの遅延証明書を発行する | HW,決済 | × | BLE_IOT | センサ装置とNFTが本体。代案は遅延理由の記録とPDF出力 |  |
| [fk_2405](https://github.com/jphacks/fk_2405) | WHOAMI | 名札のNFCで会話し相手が埋めるプロフィールで競う | 共有,ゲーム,HW | ○ | OS_INTEG | NFC名札が欠ける(QRで代替) |  |
| [fk_2406](https://github.com/jphacks/fk_2406) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [fk_2407](https://github.com/jphacks/fk_2407) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [fk_2408](https://github.com/jphacks/fk_2408) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [hs_2401](https://github.com/jphacks/hs_2401) | Communication名刺 | 共通の話題だけを名刺風に見せて会話を促す | 共有 | △ | SERVER_LOGIC | 共通項目の自動算出を足す |  |
| [hs_2402](https://github.com/jphacks/hs_2402) | Tripper | 他人の旅行プランを複製して自分用に編集する | 記録,共有 | ○ | FEED_UI | フォロー(#679)は入った。プランの複製は欠ける |  |
| [hs_2403](https://github.com/jphacks/hs_2403) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |
| [hs_2404](https://github.com/jphacks/hs_2404) | taskEcho | 講義の音声を認識しキーワード検知と要約で聞き逃しを防ぐ | 音声,AI | ◎ | STT | transcribeField(文字起こし・#659)で講義を文字にし、キーワードの検知と要約は端末内 AI | 企業賞 |
| [hs_2405](https://github.com/jphacks/hs_2405) | Blue Moon | 外見を隠し価値観の質問でつながる匿名マッチング | 共有 | ○ | SERVER_LOGIC | 価値観の近さの自動算出が欠ける |  |
| [hs_2406](https://github.com/jphacks/hs_2406) | ずんだもんのオールナイトニッポン | ずんだもんが好みに合わせたラジオを生成して読み上げる | AI,音声 | ○ | CLOUD_AI,TTS | 読み上げは speakButton(#659)、台本は aiButton の length long(#702)。キャラクターの声は無い(端末の声) |  |
| [hs_2407](https://github.com/jphacks/hs_2407) | SubVision | サブスクの利用時間を可視化し他ユーザと比較する | 記録,共有 | △ | OS_INTEG | アプリ利用時間の自動取得を足す |  |
| [hs_2408](https://github.com/jphacks/hs_2408) | Brew Buddy | 少ない質問で好みのコーヒー豆を提案する | AI,記録 | ○ | EXT_API | 商品画像のスクレイピング解析が欠ける(固定の候補表で代替) |  |
| [hs_2409](https://github.com/jphacks/hs_2409) | LiCC | ビデオ通話しながらコードを共同編集するWebアプリ | 共有,通話 | × | CALL,REALTIME,CANVAS | 通話と同時編集が本体。代案はコードメモの共有掲示とチャット |  |
| [hs_2410](https://github.com/jphacks/hs_2410) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [hs_2411](https://github.com/jphacks/hs_2411) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [hs_2412](https://github.com/jphacks/hs_2412) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [kz_2401](https://github.com/jphacks/kz_2401) | IoT学習管理支援システム | スマホをロックするケースと顔認識で集中度を測る学習管理 | HW,カメラ,記録 | × | BLE_IOT,CAMERA_LIVE | ロック機構が本体。代案: ポモドーロと学習時間の記録・共有 |  |
| [kz_2402](https://github.com/jphacks/kz_2402) | Share Fit | ジオフェンス内の近くの運動者と運動データを見せ合う | 地図,共有,記録 | △ | REALTIME | 近くの人の運動データのライブ共有を足す |  |
| [kz_2403](https://github.com/jphacks/kz_2403) | MoreReaction! | Slackでメッセージに合うリアクションを薦め催促する | 共有,外部API,通知 | × | WEB_ONLY,EXT_API | Slack内で動くBotが本体。代案: 独自の掲示板にランキングと未返信通知 | 企業賞 |
| [kz_2404](https://github.com/jphacks/kz_2404) | Let's Pics(公式の一覧: Lets Pics) | お題の英単語に合う写真を撮りAI採点で競う | カメラ,ゲーム,AI | ◎ | IMAGE_AI | 写っている物を photoField.analyze の labels(#659)で読み、お題と比べて採点する | 決勝 |
| [kz_2405](https://github.com/jphacks/kz_2405) | リンクル店 | 地域の複数店の売上を集めて見比べ傾向を分析する | 記録,共有,AI | ○ | CLOUD_AI | 高度なAI分析は無く、集計とグラフと短い提案まで |  |
| [kz_2406](https://github.com/jphacks/kz_2406) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [kz_2407](https://github.com/jphacks/kz_2407) | Common | 参加者の自己紹介から全員の共通点を見つける | 共有,AI,フォーム | ○ | CLOUD_AI | 共通点の抽出は端末内AIの短い文まで |  |
| [kz_2408](https://github.com/jphacks/kz_2408) | ゲームコネクト | ボードゲームの開催を地図で探しチームで交流する | 地図,共有 | ◎ | - | - |  |
| [kz_2409](https://github.com/jphacks/kz_2409) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [kz_2410](https://github.com/jphacks/kz_2410) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [ng_2401](https://github.com/jphacks/ng_2401) | レオナルド・ダ・ふっきん | BLEでつながるロボットが文字や絵を書く | HW | × | BLE_IOT | 自作ロボットが本体。代案: メッセージ送受信と表示のみのアプリ |  |
| [ng_2402](https://github.com/jphacks/ng_2402) | Free AI at api.airforce(公式の一覧: OriCube) | タイトルのみで内容が読めない | - | ? | - | 内容が読めず判定不可 | Best Hack Award / Best Audience Award / Innovator |
| [ng_2403](https://github.com/jphacks/ng_2403) | カンターン | インターンの予定管理と経路提案・メモのAI要約 | 記録,地図,AI,共有 | ○ | EXT_API | 最適経路の自動提案は無く、道案内まで |  |
| [ng_2404](https://github.com/jphacks/ng_2404) | 悩みをポップな文章に変換するアプリ | 入力した悩みをAIが明るい文章に言い換える | AI | ◎ | - | - |  |
| [ng_2405](https://github.com/jphacks/ng_2405) | HackDrill | 言語と記法を指定するとAIが難易度別の課題を作る | AI,記録 | △ | CLOUD_AI | 課題文の生成にクラウドAIを足す |  |
| [ng_2406](https://github.com/jphacks/ng_2406) | 安心打診おばあ | 予定を入れるとおばあ口調で危険と助言をくれる | AI,記録 | ◎ | - | 長めの助言は aiButton suggest の length long(Pro のオンライン AI・追記) | 企業賞 |
| [ng_2407](https://github.com/jphacks/ng_2407) | 駅くぐる | 駅の設備を地図で調べ利用者の投票で情報を補う | 地図,共有,外部API | ○ | EXT_API | 鉄道会社の設備データは取り込めず手入力と投票で集める |  |
| [ng_2408](https://github.com/jphacks/ng_2408) | What's Up! | 近くの人と自動でグループを作りチャットする | 共有,地図 | △ | SERVER_LOGIC | 距離で自動グルーピングするサーバー処理を足す |  |
| [ng_2409](https://github.com/jphacks/ng_2409) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [ng_2410](https://github.com/jphacks/ng_2410) | モットーク！ | AIと音声で会話し質問力を採点して助言する | AI,音声 | × | CLOUD_AI,STT,TTS | 長い会話AIと音声が本体。代案: 質問を文字で書き端末AIが短く講評 |  |
| [ng_2411](https://github.com/jphacks/ng_2411) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [ng_2412](https://github.com/jphacks/ng_2412) | タスクにゃん | 締切と必要日数から作業日程を自動で組む | 記録,フォーム | ○ | SERVER_LOGIC | 作業不可日を避ける自動割付は無く日割り計算まで |  |
| [ng_2413](https://github.com/jphacks/ng_2413) | Meacle | 身体情報から必要栄養を計算し献立を自動で出す | 記録,フォーム | ○ | SERVER_LOGIC | 栄養バランスの最適化は無くランダム提案と合計表示 |  |
| [ng_2414](https://github.com/jphacks/ng_2414) | 留守るちゃん | インターホン映像の顔認証で知人と不審者を判定する | カメラ,独自ML,HW | × | CAMERA_LIVE,CUSTOM_ML | 顔認証が本体。代案: 訪問者を撮影し知人か不審者かを手で記録 |  |
| [ng_2415](https://github.com/jphacks/ng_2415) | StandTogether | 球場の音から応援歌を特定して歌詞を表示する | 音声,独自ML | × | CUSTOM_ML | 音の照合が本体。代案: 応援歌リストの検索と歌詞表示 |  |
| [ng_2416](https://github.com/jphacks/ng_2416) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [ng_2417](https://github.com/jphacks/ng_2417) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [ng_2418](https://github.com/jphacks/ng_2418) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [os_2401](https://github.com/jphacks/os_2401) | 国会劇場 | 国会議事録をAIで要約しアニメ風に見せる | 外部API,AI | △ | EXT_API,CLOUD_AI | 議事録API取得とクラウドAI要約を足す |  |
| [os_2402](https://github.com/jphacks/os_2402) | Notoria | 会話形式で日記を書きVR空間で振り返る | 記録,AR3D,AI | × | AR3D | VR空間が本体。代案: 会話風の日記入力と感情別の背景色表示 |  |
| [os_2403](https://github.com/jphacks/os_2403) | HubMe | サークルがタグで学生にスカウトを送るマッチング | 共有 | ○ | SERVER_LOGIC | タグ一致の一括スカウトは通知ルールで近似 |  |
| [os_2404](https://github.com/jphacks/os_2404) | すもトーク！ | 少人数ボイスチャットの日程調整と組み合わせ自動化 | 共有,音声 | ○ | CALL | 通話自体は無く、日程調整とグループ分けまで |  |
| [os_2405](https://github.com/jphacks/os_2405) | CartShare | 家族や友人で買い物リストをメモ帳単位で共有する | 共有,記録 | ◎ | - | - |  |
| [os_2406](https://github.com/jphacks/os_2406) | EnergyMate | エナドリと睡眠、作業の出来を記録しAIが助言する | 記録,AI | ◎ | - | - |  |
| [os_2407](https://github.com/jphacks/os_2407) | Eventpix | ポスターを撮影するとAIが予定を抽出して登録する | カメラ,AI,記録 | ○ | OS_INTEG | 端末のカレンダーへは addToCalendar(#660)で入る。ポスターからの抽出は写真の文字 + 端末内 AI で簡易 | Innovator |
| [os_2408](https://github.com/jphacks/os_2408) | Task_yell | やりたいことと予定を管理し重要度で通知、AIで細分化 | 記録,通知,AI | ○ | CALL | 電話での通知は無く、AI分割は短い箇条書きまで |  |
| [os_2409](https://github.com/jphacks/os_2409) | SmartThrone(公式の一覧: スマートイレ) | 人感センサーで既存トイレを自動洗浄しAIと会話する | HW,AI | × | BLE_IOT | 自作機器が本体。代案: 滞在時間と便の記録アプリ | 審査委員特別賞 / Innovator |
| [os_2410](https://github.com/jphacks/os_2410) | へるぴぃ | 生活習慣を記録しキャラクターのHPで健康度を見せる | 記録,ゲーム | ◎ | - | - |  |
| [os_2411](https://github.com/jphacks/os_2411) | Chronotes | GitHubやSlackの作業ログからAIが日次ノートを作る | 外部API,AI,記録 | △ | EXT_API | 外部サービスの作業ログ取り込みを足す |  |
| [os_2412](https://github.com/jphacks/os_2412) | ScanJourney | 観光名所を撮ると名称や歴史をAIが説明する | カメラ,AI,音声 | △ | IMAGE_AI,CLOUD_AI | 名所の認識と長い解説を足す。読み上げは無し |  |
| [os_2413](https://github.com/jphacks/os_2413) | マイレポ | GitHubのコミットからAIが日報を自動生成する | 外部API,AI,記録 | △ | EXT_API,CLOUD_AI | GitHub取り込みとクラウドAI要約を足す |  |
| [os_2414](https://github.com/jphacks/os_2414) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [os_2415](https://github.com/jphacks/os_2415) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [os_2416](https://github.com/jphacks/os_2416) | 決算カード倶楽部 | 自治体の決算カードの読み方を学ぶチュートリアル | 記録,Web専用 | ◎ | - | - |  |
| [os_2417](https://github.com/jphacks/os_2417) | ユーズミー | IT用語を含む文を専門用語なしの言葉に言い換える | AI,音声 | ◎ | STT | 声でも transcribeField(#659)で入れられ、言い換えは端末内 AI |  |
| [os_2418](https://github.com/jphacks/os_2418) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [os_2419](https://github.com/jphacks/os_2419) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [os_2420](https://github.com/jphacks/os_2420) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [sd_2401](https://github.com/jphacks/sd_2401) | EIGOイスト | 好きなキーワードからお題を作り英語スピーキングを練習 | AI,音声,外部API | × | CLOUD_AI,STT,EXT_API | 発音採点が本体。代案: 端末AIでお題を作りカードで英語学習 | Innovator |
| [sd_2402](https://github.com/jphacks/sd_2402) | KINOKEN | ポモドーロ中の姿勢と笑顔、ストレッチをカメラで検知 | カメラ,記録 | △ | CAMERA_LIVE | カメラ映像の連続認識を足す | 審査委員特別賞 / Innovator |
| [sd_2403](https://github.com/jphacks/sd_2403) | フードチェーン | ブロックチェーンで農産物の流通履歴を追えるQRシステム | 記録,QR | ○ | 新:BLOCKCHAIN | 改ざん防止は無く、QR付きの履歴記録まで |  |
| [sd_2404](https://github.com/jphacks/sd_2404) | QuickScribe | 手書きの数式をAIで認識して数式表示する | AI,Web専用 | × | CANVAS,IMAGE_AI | 手書き認識が本体。代案: 数式の写真をOCRして文字で記録 |  |
| [sd_2405](https://github.com/jphacks/sd_2405) | RegiStock | AIとの会話で家の食品を登録し賞味期限を管理 | 記録,音声,AI | ○ | STT,CLOUD_AI | 会話登録は無く、フォーム入力と期限通知で管理 |  |
| [sd_2406](https://github.com/jphacks/sd_2406) | アンチ・フードロス | 食材と消費期限を管理し献立が作れるか判定する | 記録,通知 | ○ | - | 作れるかの自動判定は無く、不足は一覧で目視確認 |  |
| [sd_2407](https://github.com/jphacks/sd_2407) | なにはな | 対面のグループで好みの類似度を出し話題にする | 共有,フォーム | ○ | SERVER_LOGIC | 類似度は共通項目の数の集計に簡略化 |  |
| [sd_2408](https://github.com/jphacks/sd_2408) | Hack Mach | 技術者同士がプロフィールとタグで探しチャットする | 共有 | ◎ | - | - |  |
| [sd_2409](https://github.com/jphacks/sd_2409) | 食べタグルーレット | 学食の自動会計でラベル付けとルーレット演出を行う | HW,独自ML | × | BLE_IOT,CUSTOM_ML | カメラ認識と自作機器が本体。代案: 料理の手動ラベルとルーレット |  |
| [sd_2410](https://github.com/jphacks/sd_2410) | Wake Mate | アバターとカメラ確認で一人暮らしの起床を支援する | HW,AI,AR3D | × | BLE_IOT,AR3D | TV操作やXRアバターが本体。代案: 起床時刻の通知と写真での起床記録 | 企業賞 |
| [sp_2401](https://github.com/jphacks/sp_2401) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [sp_2402](https://github.com/jphacks/sp_2402) | Labely | ラベルを剥がした写真で妖精との物語が進む分別促進アプリ | カメラ,ゲーム,記録 | ○ | AR3D | 3D妖精は無く、画像と文章でのシナリオ進行 |  |
| [sp_2403](https://github.com/jphacks/sp_2403) | まにあえ！ | シャトルバス時刻と乗換案内から出発時刻を逆算する | 外部API,記録 | △ | EXT_API | 乗換案内の経路取得を足す |  |
| [sp_2404](https://github.com/jphacks/sp_2404) | Mirodyssey | 説明文から架空の旅行写真を画像生成してSNS投稿する | AI,共有 | △ | IMAGE_GEN,FEED_UI | 画像生成を足す。タイムラインは follow(#679)で可 |  |
| [sp_2405](https://github.com/jphacks/sp_2405) | ぽもとも | 他の人と同時にポモドーロを行い進捗を共有する | 共有,記録 | ○ | REALTIME | 全員一斉のタイマー同期は無く、各自のタイマーと進捗共有 |  |
| [sp_2406](https://github.com/jphacks/sp_2406) | あけおめリンク | 年賀状を自由に作ってSNS共有、ネットプリント印刷 | 共有,その他 | ◎ | CANVAS | drawingField(手書き・#659)で年賀状を描いて共有する。ステッカーは描き込みで代わる | 企業賞 |
| [sp_2407](https://github.com/jphacks/sp_2407) | 飛び出せ！我々の家 | 観光地へチェックインして陣取りを競う地図ゲーム | 地図,ゲーム,共有 | ○ | GAME | 地図の領地の塗り分けは無く、ポイント集計のランキング |  |
| [sp_2408](https://github.com/jphacks/sp_2408) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [sp_2409](https://github.com/jphacks/sp_2409) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [sp_2410](https://github.com/jphacks/sp_2410) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [tk_2401](https://github.com/jphacks/tk_2401) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [tk_2402](https://github.com/jphacks/tk_2402) | Maclay Rush | 司書体験ゲームで本を探しレビューや本棚を共有するSNS | ゲーム,外部API,共有 | △ | EXT_API,GAME | 書籍検索APIとゲーム画面を足す |  |
| [tk_2403](https://github.com/jphacks/tk_2403) | 二次退避くん | 避難場所を貸す人と借りたい人をマッチングする | 共有,地図 | ○ | EXT_API | 地域の安全度データは取り込めず手入力 |  |
| [tk_2404](https://github.com/jphacks/tk_2404) | Music-Walk | すれ違った人と聴いている曲を交換する | 地図,共有,外部API | ○ | EXT_API | Spotify連携は無く、Apple Musicの再生まで |  |
| [tk_2405](https://github.com/jphacks/tk_2405) | シェアトレ | 筋トレをカメラで認識し回数を同室の人と共有する | カメラ,共有 | △ | CAMERA_LIVE,REALTIME | 連続の姿勢認識とライブ共有を足す |  |
| [tk_2406](https://github.com/jphacks/tk_2406) | Yoi | 注文のアルコール量を計算し飲み過ぎを警告する | 記録,フォーム,通知 | ◎ | - | - |  |
| [tk_2407](https://github.com/jphacks/tk_2407) | つむぎ | 場所の近くにいる時だけ書ける寄せ書きアプリ | 地図,共有 | ○ | - | 近くにいる時だけ閲覧・投稿する制限は無く距離表示まで |  |
| [tk_2408](https://github.com/jphacks/tk_2408) | Vany | 動画を自分の声のまま多言語に自動翻訳する | AI,音声,Web専用 | × | CLOUD_AI,STT,TTS | 音声の翻訳と合成が本体。代案: 動画の投稿と字幕の手入力共有 | Innovator |
| [tk_2409](https://github.com/jphacks/tk_2409) | - | README が雛形のまま | - | ? | - | 雛形のまま |  |
| [tk_2410](https://github.com/jphacks/tk_2410) | ひとひら文庫 | 電子ペーパー端末に小説の一行を表示しお気に入りを記録 | HW,記録 | × | BLE_IOT | 電子ペーパー端末が本体。代案: 日替わり一行の通知とお気に入り記録 |  |
| [tk_2411](https://github.com/jphacks/tk_2411) | Life DiaLog | 日記を記録しAIが自己分析し対話で検索できる | 記録,AI | △ | CLOUD_AI | 蓄積した日記への会話型の相談・検索を足す |  |
| [tk_2412](https://github.com/jphacks/tk_2412) | Calm Sense | 皮膚電気センサーで焦りを検知し英文読解にヒントを出す | HW,独自ML,AI | × | BLE_IOT,CUSTOM_ML | センサーと学習モデルが本体。代案: 自己申告の焦り度で出すヒント |  |
| [tk_2413](https://github.com/jphacks/tk_2413) | フジヱ | 履修登録の誤りの学生情報を名簿から突合して一覧する | フォーム,その他 | ○ | FILE_IMPORT,SERVER_LOGIC | 名簿の CSV は fileImport(#659)、突合は serverRules の lookup(#701・notExists で名簿に無い登録に印)で作れる。一覧の見せ方は簡易 |  |
| [tk_2414](https://github.com/jphacks/tk_2414) | FairEdu | 生成AIと対話しながら学ぶ中高生向けオンライン教材 | AI | △ | CLOUD_AI | 長い対話AIと教材の自動生成を足す |  |
| [tk_2415](https://github.com/jphacks/tk_2415) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2416](https://github.com/jphacks/tk_2416) | Turtle Stamina | グループで体力を共有し亀が疲労を表すアプリ | 記録,共有 | ◎ | - | 歩数・体力メーター・共有・休憩通知で作れる |  |
| [tk_2417](https://github.com/jphacks/tk_2417) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2418](https://github.com/jphacks/tk_2418) | サンプル（研究SNS） | 学生の研究を支えるSNSコミュニティアプリ | 共有 | ○ | FEED_UI | 論文情報の自動更新は無し・掲示板とチャットで代用 |  |
| [tk_2419](https://github.com/jphacks/tk_2419) | 映画館検索 | 見たい映画を近くで上映する映画館を探すWebアプリ | 地図,外部API | △ | EXT_API | 上映館・上映情報の外部データ連携が必要 |  |
| [tk_2420](https://github.com/jphacks/tk_2420) | Where is your chugyeog? | K-pop動画の人物に名札を重ねて表示する | カメラ,独自ML | × | CUSTOM_ML,CAMERA_LIVE | 動画の人物認識は不可。メンバー図鑑の写真記録版なら可 | 決勝 |
| [tk_2421](https://github.com/jphacks/tk_2421) | らくらくトーナメント | エントリー情報から自動でトーナメント表を作る | フォーム,共有 | △ | 新:トーナメント表 | 対戦表の自動生成と勝ち上がり表示の部品を足す |  |
| [tk_2422](https://github.com/jphacks/tk_2422) | LuggPal | 荷物を預けたい人と預かる人のマッチング掲示板 | 共有,フォーム | ◎ | - | 公開掲示板と地域日時の絞り込みで作れる |  |
| [tk_2423](https://github.com/jphacks/tk_2423) | HearU | 手話認識とARグラスで聴覚障害者の会話を支える | AR3D,独自ML,音声 | × | CUSTOM_ML,CAMERA_LIVE,AR3D,STT | 手話認識とARは不可。定型文カードと警告通知の版なら可 | Innovator |
| [tk_2424](https://github.com/jphacks/tk_2424) | ゴミンゴ | ゴミを撮影して種類判別し拾って集める | カメラ,地図,ゲーム | ◎ | - | 写真分類・地図ピン・図鑑で作れる | 企業賞 |
| [tk_2425](https://github.com/jphacks/tk_2425) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2426](https://github.com/jphacks/tk_2426) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2427](https://github.com/jphacks/tk_2427) | Shopiris | 視覚障害者の店内買い物を物体検出と音声で案内 | カメラ,音声,独自ML | × | CAMERA_LIVE,TTS,IMAGE_AI | リアルタイム検出と読み上げは不可。撮影して説明1文の版なら可 |  |
| [tk_2428](https://github.com/jphacks/tk_2428) | PRISM | 政治記事から人物と政策の関係を3D相関図にする | AI,Web専用 | △ | CLOUD_AI,CANVAS | 長文解析と関係図の描画部品が必要 |  |
| [tk_2429](https://github.com/jphacks/tk_2429) | AI裁判官 | 主張を入力するとAIが判決を下す複数人ゲーム | AI,共有,ゲーム | △ | CLOUD_AI | GPT級の判決文生成が必要。端末内AIの短文なら代用可 |  |
| [tk_2430](https://github.com/jphacks/tk_2430) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2431](https://github.com/jphacks/tk_2431) | 脱だからモテないんだよbot | LINEのメッセージの感情を分析しレポートする | AI,記録 | ○ | OS_INTEG | LINE連携は無し。入力文を分類し記録とグラフにする版 |  |
| [tk_2432](https://github.com/jphacks/tk_2432) | Drift | ポジティブな投稿だけが風船で届くSNS | 共有,ゲーム | ○ | CANVAS | 風船の演出と色の混ざりは簡略。シール付き投稿一覧で代用 |  |
| [tk_2433](https://github.com/jphacks/tk_2433) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2434](https://github.com/jphacks/tk_2434) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2435](https://github.com/jphacks/tk_2435) | Text Tuning | チャット文のハラスメント表現を言い換え提案する拡張 | AI,Web専用 | ○ | WEB_ONLY | チャット画面上の自動検知は無し。文を貼って言い換える版 |  |
| [tk_2436](https://github.com/jphacks/tk_2436) | CosmeCanvas | 顔写真の色を調整し理想に合う化粧品を提案する | カメラ,AI | △ | IMAGE_AI,CANVAS | 写真の色調編集と化粧品データの突き合わせが必要 |  |
| [tk_2437](https://github.com/jphacks/tk_2437) | Nazomate | 謎解き適性診断と謎解きイベント情報の集約 | フォーム,記録 | ○ | EXT_API | イベント情報は手入力。診断は選択式と計算式で作れる |  |
| [tk_2438](https://github.com/jphacks/tk_2438) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2439](https://github.com/jphacks/tk_2439) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2440](https://github.com/jphacks/tk_2440) | ヘルスハンター | 体調管理をクエスト化しAIと音声で対話する | 記録,ゲーム,音声,AI | ○ | CLOUD_AI,STT,TTS | 音声対話AIは無し。クエスト達成の記録と連続日数で作る |  |
| [tk_2441](https://github.com/jphacks/tk_2441) | 起きた人だけのSNS | アラーム後の通知に反応できた人だけ投稿を見られる | 通知,共有 | ○ | OS_INTEG | 止めるまで鳴るアラームは alarm(#665)で可。通知への反応の判定は簡略 |  |
| [tk_2442](https://github.com/jphacks/tk_2442) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2443](https://github.com/jphacks/tk_2443) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2444](https://github.com/jphacks/tk_2444) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [tk_2445](https://github.com/jphacks/tk_2445) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [Z_2401](https://github.com/jphacks/Z_2401) | - | READMEが雛形のままで内容なし | その他 | ? | - | 判定不能 |  |

### 2025 年(133 作品)

| チーム(リポジトリ) | 作品名 | 何をするものか | 使っている機能 | 判定 | 足りない物 | メモ | 受賞など |
|---|---|---|---|---|---|---|---|
| [fk_2501](https://github.com/jphacks/fk_2501) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [fk_2502](https://github.com/jphacks/fk_2502) | PillPal | 処方箋を撮影して服薬予定を作り、通知・家族への知らせ・QR共有をする | カメラ,通知,記録,共有 | ○ | - | 処方箋の構造化抽出の精度が欠ける。手入力併用で可 |  |
| [fk_2503](https://github.com/jphacks/fk_2503) | ゴブジェクター | 不要品を撮影するとAIがリサイクル工作案を提案する | カメラ,AI | ○ | - | 図解や3D完成イメージが欠ける。短い手順の提案は可 |  |
| [fk_2504](https://github.com/jphacks/fk_2504) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [fk_2505](https://github.com/jphacks/fk_2505) | Reply Mate | 複数チャットアプリの通知を集め、緊急度をAIで判定して返信を助ける | 外部API,AI,Web専用 | × | OS_INTEG,EXT_API,WEB_ONLY | 他アプリの通知取得が不可。代案は自分用の返信待ちタスク管理 |  |
| [fk_2506](https://github.com/jphacks/fk_2506) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [fk_2507](https://github.com/jphacks/fk_2507) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [fk_2508](https://github.com/jphacks/fk_2508) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [fk_2509](https://github.com/jphacks/fk_2509) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [fk_2510](https://github.com/jphacks/fk_2510) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [hs_2501](https://github.com/jphacks/hs_2501) | AIbum | 写真と絵文字から日記文を生成し継続日数を見せるアプリ | AI,カメラ,記録 | ◎ | - | 日記の本文は aiButton generate の length long(追記)。写真そのものは送らず、端末内の analyze の説明を材料にする |  |
| [hs_2502](https://github.com/jphacks/hs_2502) | LangBridge | 英文を利用者の語彙レベルに合わせ日英混在で表示する | AI,Web専用 | △ | CLOUD_AI,SERVER_LOGIC | 語彙レベル判定と混在翻訳の処理を足す |  |
| [hs_2503](https://github.com/jphacks/hs_2503) | hinavi | 災害情報と避難所までの道案内、現地情報の投稿ができる防災アプリ | 地図,共有,外部API | ○ | EXT_API | 気象庁の災害情報が欠ける。避難所データと道案内は可 | 企業賞 |
| [hs_2504](https://github.com/jphacks/hs_2504) | PoseTrack | Webカメラで座り姿勢を常時監視し悪いと知らせる | カメラ,AI | △ | CAMERA_LIVE | 連続した姿勢推定を足す。1枚撮影の角度判定は可 |  |
| [hs_2505](https://github.com/jphacks/hs_2505) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [hs_2506](https://github.com/jphacks/hs_2506) | TaskEcho ver2.0 | 授業を文字起こしし、キーワード周辺を要約してカレンダーやNotionに登録する | 音声,AI,外部API | × | STT,EXT_API,CLOUD_AI | 常時文字起こしと外部連携が難しい。代案は期限の手動入力と通知 |  |
| [hs_2507](https://github.com/jphacks/hs_2507) | NearTalk | 会場の卓上マイクで会話を要約し、各テーブルの話題と参加者を見せる | 音声,AI,HW,共有 | × | HW,STT,BLE_IOT | 卓上マイクの常時収録が不可。代案は話題を自己申告する卓ボード |  |
| [hs_2508](https://github.com/jphacks/hs_2508) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [hs_2509](https://github.com/jphacks/hs_2509) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [hs_2510](https://github.com/jphacks/hs_2510) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [hs_2511](https://github.com/jphacks/hs_2511) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [hs_2512](https://github.com/jphacks/hs_2512) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [kz_2501](https://github.com/jphacks/kz_2501) | つぶやきチャット | 感情を投稿し似た感情の人数を見るSNS | 共有,AI | ○ | SERVER_LOGIC | 類似度は無し。ポジネガ分類の人数集計で代用 |  |
| [kz_2502](https://github.com/jphacks/kz_2502) | MemoryChat | AIとの会話から日記を自動作成する | AI,記録 | △ | CLOUD_AI | AI対話の長い会話が必要。会話メモの要約は端末AIで可 |  |
| [kz_2503](https://github.com/jphacks/kz_2503) | WinCook | ウィンクでレシピをめくりAIと話しながら料理する | カメラ,音声,AI | × | CAMERA_LIVE,STT,TTS,CLOUD_AI | ウィンク検出と音声AIは不可。手動でめくるレシピ版 |  |
| [kz_2504](https://github.com/jphacks/kz_2504) | 4DX@HOME | 動画を解析し振動・光・風・水で4DX化する | HW,AI | × | BLE_IOT,IMAGE_AI | 外部の物理装置が本体で不可。見た場面の記録版 | 審査委員特別賞 / Innovator |
| [kz_2505](https://github.com/jphacks/kz_2505) | マナープロダクト | 参加イベントのマナーや持ち物をまとめて調べる | 記録,フォーム | ◎ | - | 検索付き一覧・Markdown・購入リンクで作れる |  |
| [kz_2506](https://github.com/jphacks/kz_2506) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [kz_2508](https://github.com/jphacks/kz_2508) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [kz_2509](https://github.com/jphacks/kz_2509) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [kz_2510](https://github.com/jphacks/kz_2510) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [ng_2501](https://github.com/jphacks/ng_2501) | SUDO | 数式の入力から数学解説動画をAIで自動生成する | AI,その他 | × | CLOUD_AI,IMAGE_GEN,CANVAS | 動画生成は不可。数式解説をMarkdownカードにする版 | 審査委員特別賞 / Innovator |
| [ng_2502](https://github.com/jphacks/ng_2502) | SNR(公式の一覧: SNR~SNSの一歩前~) | AI相手にSNS投稿を練習する子供向けアプリ | 共有,AI | ○ | FEED_UI | タイムラインは follow(#679)とフォロー中の人の投稿の一覧、いいねは entity.marks で可。AI のコメントは短文 | 企業賞 |
| [ng_2503](https://github.com/jphacks/ng_2503) | VocaRise | 登録単語を含む英語長文をAIで生成し復習する | AI,記録 | △ | CLOUD_AI | 長文と設問の生成が必要。暗記カードは可 |  |
| [ng_2504](https://github.com/jphacks/ng_2504) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [ng_2505](https://github.com/jphacks/ng_2505) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [ng_2506](https://github.com/jphacks/ng_2506) | StopMeeting | 会議の音声から議論の停滞をAIが検知し介入する | 音声,AI,共有 | △ | STT,CLOUD_AI | 会議音声の文字起こしと分析が必要。匿名シグナルは可 |  |
| [ng_2507](https://github.com/jphacks/ng_2507) | AIメッセージ | 送信前にメッセージのトゲをAIが判定し優しく言い換える | AI,共有 | ◎ | - | 端末内AIの言い換えと夫婦の共有で作れる |  |
| [ng_2508](https://github.com/jphacks/ng_2508) | Habitech | 習慣の達成を仲間と共有して続けるアプリ | 記録,共有,通知 | ◎ | - | 共有リスト・記録・通知・チャットで作れる |  |
| [ng_2509](https://github.com/jphacks/ng_2509) | らくらく・おしゃべり日記 | 音声でAIと話すだけで日記を代筆してもらう | 音声,AI,記録 | △ | STT,CLOUD_AI | 音声対話と長い会話が必要。5項目入力から日記生成は可 |  |
| [ng_2510](https://github.com/jphacks/ng_2510) | デコ×めし！ | 贈ったアイテムが旅して装飾され戻る運動促進ゲーム | 共有,ゲーム,記録 | ○ | SERVER_LOGIC | 旅の経路と加工は簡略。贈り合いと集めるだけ |  |
| [ng_2511](https://github.com/jphacks/ng_2511) | ミライかもね | 日記の内容で未来の自分の姿が変わる日記アプリ | AI,記録 | ○ | IMAGE_GEN | 姿は生成せず、点数ごとに用意した画像を切替 |  |
| [ng_2512](https://github.com/jphacks/ng_2512) | Recall | 過去写真の人物をAIで認識し再会を匿名提案する | AI,共有,カメラ | × | IMAGE_AI,SERVER_LOGIC | 顔認識と自動提案は不可。友人を手選びし匿名で誘う版 |  |
| [ng_2513](https://github.com/jphacks/ng_2513) | RoadNote | 危険な道路を地図に投稿して共有するアプリ | 地図,共有 | ◎ | - | 公開の地図ピン投稿と外部地図リンクで作れる |  |
| [ng_2514](https://github.com/jphacks/ng_2514) | デスク・ぬい・メイト | ぬいぐるみアバターと作業するAI対話つきタイマー | AI,ゲーム,記録 | △ | IMAGE_GEN,CLOUD_AI | アバター生成とAI対話が必要。タイマーは可 |  |
| [ng_2515](https://github.com/jphacks/ng_2515) | KIBARASHI | 入力した不安な言葉を投げて散らし消す | その他 | △ | CANVAS | 投げて飛散する物理アニメが必要。入力と消去は可 |  |
| [ng_2516](https://github.com/jphacks/ng_2516) | 景歩 | 現在地から目的地まで景観優先の徒歩ルートを提案 | 地図,外部API | △ | EXT_API,SERVER_LOGIC | 観光スポット取得とルート生成が必要 |  |
| [ng_2517](https://github.com/jphacks/ng_2517) | 偉人トーーク！ | 歴史上の偉人とAIが音声つきで会話する | AI,音声 | △ | CLOUD_AI,TTS | AI会話と読み上げが必要 |  |
| [ng_2518](https://github.com/jphacks/ng_2518) | StepMuchle | カメラで骨格を検知しスクワット回数を数える | カメラ,ゲーム,記録 | × | CAMERA_LIVE | 姿勢の連続検知は不可。回数を手入力し週ミッション管理 |  |
| [os_2501](https://github.com/jphacks/os_2501) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [os_2502](https://github.com/jphacks/os_2502) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [os_2503](https://github.com/jphacks/os_2503) | ひびコレ！ | 道路のヒビを撮影して卵を孵化させる育成ゲーム | カメラ,地図,ゲーム | ◎ | - | 写真分類・地図ピン・集めるゲームで作れる |  |
| [os_2505](https://github.com/jphacks/os_2505) | 前を向いて歩こう | 歩行中にスマホが下向きだと画面をロックする | HW,その他 | × | BG_SENSE,OS_INTEG | 常時の傾き監視と画面ロックは不可。歩数ごとの休憩通知版 |  |
| [os_2506](https://github.com/jphacks/os_2506) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [os_2507](https://github.com/jphacks/os_2507) | 組みTalk | 家具の取説PDFを読みAIが手順とチャットで案内 | AI,Web専用 | × | FILE_IMPORT,CLOUD_AI,IMAGE_GEN | PDF解析と画像生成は不可。手順を手入力するチェック版 | 企業賞 |
| [os_2508](https://github.com/jphacks/os_2508) | ARROW | イベントの告知・参加登録・出席管理を一元化するWebアプリ | 記録,フォーム,共有 | ◎ | - | 公開イベント一覧+参加登録+QR出席で作れる |  |
| [os_2509](https://github.com/jphacks/os_2509) | つながり日記(公式の一覧: つながり絵日記) | 子供の音声と位置から絵日記を自動生成し家族で共有する | 音声,地図,AI,共有 | △ | IMAGE_GEN,STT | クレヨン風画像生成と音声起点の推定を足す | 企業賞 |
| [os_2510](https://github.com/jphacks/os_2510) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [os_2511](https://github.com/jphacks/os_2511) | PDFAPTER | PDFから文を抽出しヘッダー除去・文字数カウントする | その他 | ○ | FILE_IMPORT | PDF の本文は fileImport の pdf(#659)で取り込み、文字数は計算で出す。ヘッダーの自動除去は欠ける |  |
| [os_2512](https://github.com/jphacks/os_2512) | Oton-Zzz | カメラで寝落ちを検知し赤外線でテレビを消す | カメラ,HW,AI | × | BLE_IOT,CAMERA_LIVE | 赤外線機器制御が不可。代案は就寝時刻の自動タイマー通知 |  |
| [os_2513](https://github.com/jphacks/os_2513) | えっ？なんつった？ | 常時会話を聞き取り、聞き逃した直前の話を要約する | 音声,AI | △ | STT,BG_SENSE | 背景の常時録音と文字起こしを足す |  |
| [os_2514](https://github.com/jphacks/os_2514) | ゆびサッカー | 手の動きで操作する最大6人のオンライン対戦サッカー | ゲーム,カメラ,共有 | × | GAME,REALTIME,CAMERA_LIVE | 対戦ゲーム画面が不可。代案は試合結果の記録とランキング | 審査委員特別賞 / Innovator |
| [os_2515](https://github.com/jphacks/os_2515) | なるはや | タスク期限を内緒で前倒しし通知で早めの着手を促す | 記録,通知 | ○ | - | ランダムな前倒し計算が欠ける。固定の前倒し通知なら可 |  |
| [os_2516](https://github.com/jphacks/os_2516) | 魔法少女になりたい | 位置情報と向きで現実空間を動き回って戦う対戦ゲーム | ゲーム,地図,HW | × | REALTIME,GAME,WEARABLE | リアルタイム対戦が不可。代案は走った距離で競うランキング |  |
| [os_2517](https://github.com/jphacks/os_2517) | Emo | 座標と日付から当日の天気で思い出の風景画像を生成する | AI,外部API,地図 | △ | IMAGE_GEN,EXT_API | 過去天気の取得と画像生成を足す |  |
| [os_2518](https://github.com/jphacks/os_2518) | Lingua Bridge | 言語交換の相手を探しチャットや日程調整ができるマッチングアプリ | 共有,記録 | ○ | CALL | ビデオ通話と自動翻訳が欠ける。チャットと予定は可 |  |
| [os_2519](https://github.com/jphacks/os_2519) | 隙マスター | SNS風スワイプで雑学やニュースを会話形式で読めるアプリ | AI,外部API,記録 | △ | EXT_API,SERVER_LOGIC | Wikipedia等からの自動生成と好み推薦を足す |  |
| [os_2520](https://github.com/jphacks/os_2520) | あんしんクイズ便 | 祖父母が出すクイズに家族が答え、未投稿ならアラートを送る | 記録,通知,共有 | ◎ | - | 共有リストとサーバー側の毎日通知で作れる |  |
| [os_2521](https://github.com/jphacks/os_2521) | ひとやすみ通信 | Meet中のまばたきから疲労を検知し、全員に休憩を促す拡張機能 | Web専用,カメラ,通知 | × | WEB_ONLY,CAMERA_LIVE | 会議画面への拡張が不可。代案は一定間隔の休憩通知と匿名申請 |  |
| [os_2522](https://github.com/jphacks/os_2522) | Dare Dakke? | カメラで人の顔を識別し、名前と前回の会話要約を表示する | カメラ,独自ML,音声,AI | × | CUSTOM_ML,CAMERA_LIVE,STT | 顔識別と会話の文字起こしが不可。代案は会った人の写真付き記録 |  |
| [sd_2501](https://github.com/jphacks/sd_2501) | Mokuhyo-! | 目標を読み上げないと止まらないアラーム | 音声,通知 | ○ | STT,OS_INTEG | alarm(#665)で鳴らし、読み上げた目標は transcribeField(#659)で記録。読まないと止まらない制御は欠ける |  |
| [sd_2502](https://github.com/jphacks/sd_2502) | Re:ポケベル | マイコンとWebで簡易メッセージを送り合う | HW,共有,通知 | × | BLE_IOT | マイコン端末は不可。OK/SOSの定型メッセージ送信版 |  |
| [sd_2503](https://github.com/jphacks/sd_2503) | Re:view | 話す速さや口癖を分析する面接練習ツール | 音声,カメラ,記録 | ○ | STT | 発話は transcribeField(#659)で文字にでき、録画と記録も可。話す速さ・口癖の分析は端末内 AI で簡易 |  |
| [sd_2504](https://github.com/jphacks/sd_2504) | Co-食 | 同じ料理を今食べている人と写真でつながる共食アプリ | カメラ,共有 | ○ | CALL | 通話は外部リンクで開く。写真分類と投稿共有で作る |  |
| [sd_2505](https://github.com/jphacks/sd_2505) | すきま文庫 | AIで小説を短く区切り時間を置いて読ませる | AI,外部API,記録 | △ | EXT_API,CLOUD_AI | 青空文庫の取得とAI区切りが必要 |  |
| [sd_2506](https://github.com/jphacks/sd_2506) | ホジョリンク | 顔認識で生徒の集中度を測り講師に知らせる | カメラ,AI | × | CAMERA_LIVE | 顔の常時認識は不可。自己申告の集中記録とタイマー版 |  |
| [sd_2507](https://github.com/jphacks/sd_2507) | Bonne App | カメラで咀嚼回数を数え食事をミッション化する | カメラ,ゲーム,記録 | × | CAMERA_LIVE | 咀嚼のカメラ計測は不可。ミッションと食事記録の版 |  |
| [sd_2508](https://github.com/jphacks/sd_2508) | 見守りカード mimoca | LoRaとBLEのカードで幼児や高齢者の位置を見守る | HW,地図 | × | BLE_IOT | 専用機器は不可。家族のスマホの位置共有版 | Innovator |
| [sd_2509](https://github.com/jphacks/sd_2509) | やらなオカン | AIのおかんが電話でタスクの確認をしてくる | AI,音声,通知 | × | CALL,STT,TTS,CLOUD_AI | AI通話は不可。おかん口調の通知でタスク確認する版 |  |
| [sd_2510](https://github.com/jphacks/sd_2510) | 研助 | 目標からAIが研究スケジュールを作り論文も検索 | AI,外部API,記録 | △ | CLOUD_AI,EXT_API | AI計画生成とarXiv検索が必要 |  |
| [sd_2511](https://github.com/jphacks/sd_2511) | モールスライト | 文字をライトの点滅で送り、撮影した点滅動画から文字を読み取る | カメラ,HW | × | CAMERA_LIVE,IMAGE_AI | 点滅動画の解析が不可。代案は文字とモールス表記の相互変換表 |  |
| [sp_2501](https://github.com/jphacks/sp_2501) | processlog | PC画面を自動撮影しGPTで活動を要約する | Web専用,AI | × | WEB_ONLY,BG_SENSE,CLOUD_AI | PC画面の自動撮影は不可。作業を手入力し要約する版 |  |
| [sp_2502](https://github.com/jphacks/sp_2502) | パキッとクネクネタスク | 音声で登録したタスクをAIで分割しスワイプ仕分け | AI,音声,フォーム | ○ | STT | 声のタスクは transcribeField(#659)で文字にし、分割は端末内 AI。スワイプの仕分けは簡易 |  |
| [sp_2503](https://github.com/jphacks/sp_2503) | TARUFlex | 寸法から洋酒樽の3Dモデルと図面を生成する | HW,AR3D | × | AR3D,CANVAS | 3DCAD連携は不可。寸法から板の長さを計算する版 |  |
| [sp_2504](https://github.com/jphacks/sp_2504) | Funeat | 条件に合う飲食店を投票連動の抽選で決める | 外部API,共有,ゲーム | △ | EXT_API | 飲食店データの外部取得が必要。手入力の店なら代用可 |  |
| [sp_2505](https://github.com/jphacks/sp_2505) | Traverse | 遠回り最適化ルートと3D演出で運動を促す | 地図,ゲーム,AR3D | × | SERVER_LOGIC,AR3D | 遠回りルート生成と3D表示は不可。ミッションと歩数記録の版 |  |
| [sp_2506](https://github.com/jphacks/sp_2506) | SecretSync | 秘密計算で共感だけが分かる匿名マッチング | 共有,その他 | × | SERVER_LOGIC | 暗号照合は不可。承認制の仲間探しで共通趣味を見る版 |  |
| [sp_2507](https://github.com/jphacks/sp_2507) | Nemulert | AirPodsの頭の動きで居眠りを検知しアラームを鳴らす | HW,独自ML | × | BG_SENSE,CUSTOM_ML,OS_INTEG | 常時の動き検知は不可。タイマー通知で起こす版 | Innovator |
| [sp_2508](https://github.com/jphacks/sp_2508) | のりとも | 友人に車で乗せてもらう依頼を送るアプリ | 共有,通知 | ◎ | - | 友人グループ共有と一斉プッシュ通知で作れる |  |
| [sp_2509](https://github.com/jphacks/sp_2509) | AshiArt | 一筆書きの絵から走れるGPSアートコースを作る | 地図,その他 | × | CANVAS,SERVER_LOGIC | 道路に沿う経路生成は不可。定番コースを選び地図に記録する版 | Best Hack Award / Best Audience Award / Innovator |
| [sp_2510](https://github.com/jphacks/sp_2510) | - | サンプルのまま内容なし | その他 | ? | - | README が雛形 |  |
| [sp_2511](https://github.com/jphacks/sp_2511) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [sp_2512](https://github.com/jphacks/sp_2512) | Safer Map | 危険な場所を地図に投稿・共有し、近づくと通知する | 地図,共有,通知,外部API | ○ | EXT_API | 接近通知とニュース自動取得が欠ける。投稿と地図は可 |  |
| [tk_a_2501](https://github.com/jphacks/tk_a_2501) | Don't Stop Running | 交差点を避けた周回ランニングコースを自動生成する | 地図,外部API | △ | EXT_API,SERVER_LOGIC | 地図データ取得と周回経路の探索計算を足す |  |
| [tk_a_2502](https://github.com/jphacks/tk_a_2502) | Shift+ | 出勤時に1分のPCスキル動画を見て、実践と学習実績を記録する | 記録,その他 | ○ | CANVAS | キー入力のミニゲームが欠ける。動画視聴と実績記録は可 |  |
| [tk_a_2503](https://github.com/jphacks/tk_a_2503) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [tk_a_2504](https://github.com/jphacks/tk_a_2504) | TasukeAi | 助けたい人と助けてほしい人を地図と一覧でつなぎ、ポイントで感謝を返す | 共有,地図,記録 | ◎ | - | 公開依頼+地図ピン+ポイント集計で作れる |  |
| [tk_a_2505](https://github.com/jphacks/tk_a_2505) | HiNan! | AIの災害シナリオで近くの避難所まで歩き、バッジを集ぶ避難訓練アプリ | 地図,AI,ゲーム,記録 | ○ | EXT_API | 全国の避難所データの取り込みが欠ける。登録済み地点なら可 | 企業賞 |
| [tk_a_2506](https://github.com/jphacks/tk_a_2506) | YOLOSHIKU | スマートグラスのカメラで危険物を常時検知し音声で警告する | カメラ,独自ML,音声 | × | CAMERA_LIVE,CUSTOM_ML,TTS | 常時の物体認識が不可。代案は撮影した1枚の危険チェック |  |
| [tk_a_2507](https://github.com/jphacks/tk_a_2507) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [tk_a_2508](https://github.com/jphacks/tk_a_2508) | Lazy Busters | やる気の出ない日にタスクを仲間と共有し、ランダム通知で写真を投稿する | 共有,通知,カメラ | ○ | - | 通知時刻が毎回ランダムにならない。固定時刻の通知なら可 |  |
| [tk_a_2509](https://github.com/jphacks/tk_a_2509) | おかえりすごろく | 家族の投稿をAIがマスに変え、印刷して遊ぶ人生ゲームを作る | 共有,AI,その他 | ○ | - | 盤面の印刷レイアウトが欠ける。マス一覧のPDF出力なら可 |  |
| [tk_a_2510](https://github.com/jphacks/tk_a_2510) | Tea-Connect | カフェ内でライブ映像を見て相手を選び、その場で通話を始めるマッチング | 共有,Web専用 | × | CALL,REALTIME | ビデオ通話が不可。代案は同じ店の人のプロフィール承認とチャット |  |
| [tk_a_2511](https://github.com/jphacks/tk_a_2511) | OPENME | フォーム入力から1枚完結のHTMLプロフィールを生成・共有する | フォーム,Web専用 | ○ | FILE_IMPORT | HTMLファイル生成が欠ける。プロフィール画面とPDF出力なら可 |  |
| [tk_a_2512](https://github.com/jphacks/tk_a_2512) | RPGバトルToDo | タスク完了でモンスターのHPを減らし、期限切れで回復させるToDo | 記録,ゲーム | ○ | CANVAS | モンスターのアニメが欠ける。HPゲージの表示は可 |  |
| [tk_a_2513](https://github.com/jphacks/tk_a_2513) | つながり日和 | 近所1km以内の高齢者向け投稿にいいねとコメントができるSNS | 共有,地図 | ○ | FEED_UI | いいねとフィード風の表示が欠ける。掲示板とコメントは可 |  |
| [tk_a_2514](https://github.com/jphacks/tk_a_2514) | EikenTalk | AIが面接官になり、英検2次の面接を音声で練習し評価する | 音声,AI | × | STT,CLOUD_AI,TTS | 音声対話と発音評価が不可。代案は問題カードと自己採点 |  |
| [tk_a_2515](https://github.com/jphacks/tk_a_2515) | PeakSight | 登山向けに3D地形、山の情報、クマ出没、標高グラフを見せる地図 | 地図,外部API | × | AR3D,EXT_API | 3D地形が不可。代案は山のピン地図と標高グラフ | Innovator |
| [tk_a_2516](https://github.com/jphacks/tk_a_2516) | SilentSpeaking | 口の動きをカメラで読み取り、英語の発音を無声で採点する | カメラ,独自ML | × | CUSTOM_ML,CAMERA_LIVE | 読唇の認識が不可。代案は例文カードと自己採点 |  |
| [tk_a_2517](https://github.com/jphacks/tk_a_2517) | QRally | 現地のQRを読むと観光地の説明が出て、スタンプを集めて景品と交換する | 地図,記録,AI | ○ | CLOUD_AI | 興味に合わせた長い説明生成が欠ける。固定の説明文なら可 |  |
| [tk_a_2518](https://github.com/jphacks/tk_a_2518) | FeelBody | 感情と体調を毎日記録し、天気や気圧との関係をAIが分析する | 記録,AI,外部API | ○ | EXT_API,CLOUD_AI | 天気取得とAIの長い分析が欠ける。気圧はセンサーで代用 |  |
| [tk_a_2519](https://github.com/jphacks/tk_a_2519) | AI秘書 | タスクをAIが細分化し、カレンダーの空きに合わせて予定を組む | AI,外部API,記録 | △ | EXT_API,SERVER_LOGIC | 外部カレンダー連携と空き時間の自動割り当てを足す |  |
| [tk_a_2520](https://github.com/jphacks/tk_a_2520) | Michikusa GO | 旅先の小さな発見や危険情報をタグ付きで地図に投稿・共有する | 地図,共有 | ◎ | - | 公開投稿とmapViewのピン、写真添付で作れる |  |
| [tk_a_2521](https://github.com/jphacks/tk_a_2521) | 声出してみない？ | 感情を選んでAI生成のセリフを録音し、演技をAIが採点する | 音声,AI | △ | CUSTOM_ML,CLOUD_AI | 声の感情分析による採点を足す。録音と短いセリフ生成は可 |  |
| [tk_a_2522](https://github.com/jphacks/tk_a_2522) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [tk_a_2523](https://github.com/jphacks/tk_a_2523) | - | Adobe InDesign をClaude Desktopから操作するMCPサーバーの手順 | Web専用 | × | WEB_ONLY,OS_INTEG | PCアプリ操作が本体で不可。代案は文書テンプレートの記録とPDF出力 |  |
| [tk_a_2524](https://github.com/jphacks/tk_a_2524) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [tk_a_2525](https://github.com/jphacks/tk_a_2525) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [tk_b_2501](https://github.com/jphacks/tk_b_2501) | GeoPhoto | 写真を撮影場所で整理し、地図を動かすとその周辺の写真を見せる | カメラ,地図,記録 | ◎ | - | photoFieldと現在地、mapViewのピン表示で作れる |  |
| [tk_b_2502](https://github.com/jphacks/tk_b_2502) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [tk_b_2503](https://github.com/jphacks/tk_b_2503) | Haru | 友だちの思い出投稿の好きな位置にステッカーを貼り合うアプリ | 共有,FEED_UI | △ | CANVAS | 自由な位置・回転のステッカー配置を足す。スタンプ反応のみなら可 |  |
| [tk_b_2504](https://github.com/jphacks/tk_b_2504) | Aicademy | 学習時間を記録し、分からない問題の解答と解説をAIが作る | 記録,AI | △ | CLOUD_AI | 長い解答解説と類似問題の生成を足す。学習記録は可 |  |
| [tk_b_2505](https://github.com/jphacks/tk_b_2505) | First Singin'! | カラオケの1曲目の曲と歌う人をAIが提案するアプリ | AI,フォーム | ○ | - | 機械学習の推薦が欠ける。曲リストからランダム選出なら可 |  |
| [tk_b_2506](https://github.com/jphacks/tk_b_2506) | SympoLink! | 学会参加者が興味タグで交流し、チャットや位置共有で会えるアプリ | 共有,通知,外部API,AI | ○ | EXT_API | ResearchMap連携とLINE通知が欠ける。タグとチャットは可 | 決勝 |
| [tk_b_2507](https://github.com/jphacks/tk_b_2507) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [tk_b_2508](https://github.com/jphacks/tk_b_2508) | MIERUTE | 自然文からAIが手順マニュアルを作り、QRで配って撮影で達成度を判定する | AI,カメラ,共有 | △ | CLOUD_AI,IMAGE_AI | 手順の自動生成と写真での達成度判定を足す | 決勝 |
| [tk_b_2509](https://github.com/jphacks/tk_b_2509) | SerenSpot | レビュー単位で作業スポットを地図に出し、思わぬ場所と出会うSNS | 地図,共有,記録 | ◎ | - | 公開投稿とmapViewのピン、地名検索で作れる |  |
| [tk_b_2510](https://github.com/jphacks/tk_b_2510) | Emolog | 写真とコメントを記録しAIが感情コメントを付け、連続日数を見せる | カメラ,AI,記録 | ○ | - | 月末のタイムラプス動画が欠ける。写真の一覧表示は可 |  |
| [tk_b_2511](https://github.com/jphacks/tk_b_2511) | - | README が雛形のまま | その他 | ? | - | 内容が読めず判定不可 |  |
| [tk_b_2512](https://github.com/jphacks/tk_b_2512) | ozendate | 自分の食卓写真に合う器をAIが薦め、置いた完成画像を生成する | カメラ,AI | △ | IMAGE_GEN,IMAGE_AI | 器を合成した画像の生成と写真理解を足す |  |
| [tk_b_2513](https://github.com/jphacks/tk_b_2513) | Repic | 他人が生成した画像とプロンプトを検索して再利用し、画像生成を助ける | AI,共有 | △ | IMAGE_GEN,SERVER_LOGIC | 画像生成と類似検索を足す。プロンプト共有は可 |  |
| [tk_b_2514](https://github.com/jphacks/tk_b_2514) | りーどみー | 積読の本を登録し、本が話しかける通知で読書のきっかけを作る | 記録,通知,カメラ,AI | ○ | EXT_API | ISBNからの書誌取得が欠ける。手入力か撮影で登録なら可 |  |
| [tk_b_2515](https://github.com/jphacks/tk_b_2515) | 恋AI | 3Dアバターと音声で会話練習し、表情と会話をAIが評価する | AI,音声,AR3D | × | AR3D,TTS,CLOUD_AI | 3Dアバター会話が不可。代案は会話お題カードと自己振り返り記録 | Innovator |
