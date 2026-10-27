# m0disc の回帰(#281)

M0 の洗い出し([docs/roadmap/m0-discovery.md](../../roadmap/m0-discovery.md))で試した依頼を、期待値(`expect`)つきの回帰ケースにしたもの。

- 生成: `backend/internal/ai/eval/m0disc_cases.json`(作成 30 本 + 修正 4 本)
- チャット: `backend/internal/ai/eval/m0disc_chat_cases.json`(10 本)

## 目次

1. [API を呼ばずに確かめる](#1-api-を呼ばずに確かめる)
2. [実 API で回す手順](#2-実-api-で回す手順)
3. [M0 のリリース判定に使う数字](#3-m0-のリリース判定に使う数字)
4. [生成ケースの期待値](#4-生成ケースの期待値)
5. [チャットケースの期待値](#5-チャットケースの期待値)
6. [PR #378 / #379 との関係](#6-pr-378--379-との関係)
7. [D-xx が入った時に足すもの](#7-d-xx-が入った時に足すもの)

## 1. API を呼ばずに確かめる

```sh
cd backend
go run ./cmd/ai-eval -check -cases internal/ai/eval/m0disc_cases.json
go run ./cmd/ai-eval -check -chat -cases internal/ai/eval/m0disc_chat_cases.json
go test ./cmd/ai-eval/
```

- `-check` はケースファイルを読んで、id の重複・修正ケースの `base`・`expect` のキー名と型(bool / 文字列 / 数 / 文字列の配列)・`notification` と `scope` の値を確かめて終わる。API キーは要らない。
- 実 API で回す時も、最初の呼び出しの前に同じ確認をする。キーの打ち間違いで「何も確かめずに ○」になるのを防ぐ。
- 使えるキーと型の一覧は `backend/cmd/ai-eval/expect.go` の `expectKeys`。キーを足す時はここにも足す。
- `go test` は、ほかのケースファイル(`cases.json` `demo_cases.json` `chat_cases.json`)も同じ確認にかける。m0disc の生成ケースは全部に `expect` があること(下の「保留」を除く)、m0disc のチャットは全ステップに `textNot` があることも確かめる。

## 2. 実 API で回す手順

このリポジトリの作業では回していない。回す時は次のとおり(生成 34 回 + チャット 12 回ほど。raw JSON はリポジトリに置かない)。

```sh
cd backend
go run ./cmd/ai-eval -env-file ../.env -cases internal/ai/eval/m0disc_cases.json -out <scratch dir> -timeout 150s
go run ./cmd/ai-eval -chat -env-file ../.env -cases internal/ai/eval/m0disc_chat_cases.json -out <scratch dir>
```

- 生成の表の `misses` 列に、外れた期待値が一覧で出る(例: `shared=[post:private]`、`substitution=false`)。
- 表の後に、グループごとの「期待どおり」の割合が出る(次の節)。
- 修正ケース(`d_edit_*`)は `base` の作成ケースを同じ実行に含める(`-only` で絞る時は両方渡す)。

## 3. M0 のリリース判定に使う数字

実行の最後に出る表:

```text
| group | pass / checked | pass% | no expect |
|---|---|---|---|
| m0-social | … | … | … |
| total | … | … | … |
```

- **pass** = 生成が成功し、`expect` の外れが 0 本のケース。`expect` の無いケースは `no expect` に数え、割合には入れない。
- M0 の判定には `total` の pass% と、共有系のグループ(`m0-social` `m0-booking` `m0-work` `m0-edit`)の pass% を使う。
- 洗い出しの「○ 53%・共有系 35%」は人が DSL を読んだ判定で、この pass% とは別物。期待値には「M4 に回す機能は正直に断る(`substitution: true`)」も含むので、○ でなくても pass になるケースがある。最初の実 API の実行で基準値を取り、以後はそこから下がらないことを見る。

## 4. 生成ケースの期待値

付けた期待値は、M0 で守る線(できるものは作れる・できないものは summary で断る)。M4 に回す機能(プッシュ・フォロー・1 対 1・役割・一意制約)は `substitution: true` にしている。

| id | expect | 理由 |
|---|---|---|
| d_friends_vague | `shared: true` | 友達と使う = 共有。public / shared のどちらでもよい(D-06 で shared になっても通る) |
| d_sns_post | `photo: true, scope: public`(前から) | — |
| d_sns_follow | `substitution: true, shared: true` | フォローのタイムラインは M4(D-22) |
| d_group_chat | `substitution: true, shared: true` | 他人の書き込みで届くプッシュは M4(D-21) |
| d_circle_attend | `scope: shared`(前から) | 洗い出しでは public だった。D-06 の目標どおり残す |
| d_ranking | `scope: public`(前から) | — |
| d_reserve | `substitution: true, shared: true` | 重複防止は M4(D-25) |
| d_habit | `calc: true` | 連続日数(streak) |
| d_sleep | `computed: true` | hoursBetween |
| d_medicine | `notification: any`(前から) | — |
| d_period | `calc: true` | 次の予定日の計算 |
| d_vocab | `substitution: true` | めくるカードは M2(D-16) |
| d_pet | `photo: true, shared: true` | 家族で共有・写真 |
| d_baby | `computed: true` | 前回からの経過時間 |
| d_collection | `photo: true, has: [stat]` | 写真と合計 |
| d_travel | `photo: true, has: [locationPicker, mapView]` | 地図のピン |
| d_video_diary | `has: [videoField]` | 動画 |
| d_split_trip | `settlement: true, shared: true` | 精算・友達のスマホからも |
| d_lost_found | `scope: public, photo: true`(前から) | — |
| d_edit_comment | `shared: true` | 修正で共有が外れない |
| d_edit_mine_only | `substitution: true` | 今は断るのが正しい。D-03 が入ったら入れ替える |
| d_edit_notify_new | `substitution: true` | プッシュは M4(D-21) |

`shared` は今回足したキー(1 つ以上の entity が public か shared)。`scope` は値を 1 つに決めるので、D-06 で public / shared の選び方が変わると外れる。共有されていれば良いケースは `shared` を使う。

### 保留(この PR では付けていない 12 本)

PR #378 / #379 が同じ行か隣の行を変えるため、ここで書くと git が衝突する。2 本がマージされた後に、このブランチ(か次の PR)で次の値を足す。テストの `m0discPending` から消す。

| id | 足す expect | 理由 |
|---|---|---|
| d_family_todo | #378 / #379 の値をそのまま使う | 同じ行(2 本どうしも衝突する) |
| d_inventory | #379 の `substitution: true` + `computed: true` | 同じ行。在庫数の計算 |
| d_shift | #378 / #379 の値をそのまま使う | 同じ行 |
| d_quiz_game | #379 の `substitution: true` | 同じ行。D-14 で `computed: true` |
| d_recipe_share | #379 の `noOmission: true` | 同じ行 |
| d_dm | `substitution: true, shared: true` | 1 対 1 は M4(D-23)。隣が d_ranking(#378) |
| d_event_rsvp | `computed: true, shared: true` | 候補日ごとの件数。隣が d_inventory(#379) |
| d_task_team | `shared: true` | チームで共有。隣が d_shift |
| d_study_time | `has: [timerDisplay], chart: true` | ストップウォッチと週のグラフ。隣が d_quiz_game(#379) |
| d_dice | `substitution: true` | ランダムは D-15 まで断る。隣が d_quiz_game(#379) |
| d_subscription | `notification: any, has: [stat]` | 更新日の通知と月額の合計。隣が d_recipe_share(#379) |
| d_edit_like | `shared: true` | 修正で共有が外れない。D-07 で 1 人 1 回のキーを足す。隣が d_lost_found(#378) |

## 5. チャットケースの期待値

- 全ステップに `textNot: [public, private, shared, entity, scope, disclosure, computed]`(D-10)。返答の本文に DSL の英語の名前が出たら外れ。
  - `op` は入れていない(部分一致なので英単語の一部にも当たる)。#379 の `ai.LeakedTerms` は単語の境目で見るので、チップや案のタイトルも含めてそちらで拾う。
- `dc_friends_vague`: `expect: question`(ふわっとした依頼は聞き返す)。
- M4 に回す 4 本(`dc_sns_can` `dc_chat_push_can` `dc_reserve_conflict` `dc_role_can`): `textAny: [できません, できない, 出せません]`(正直に断る)。
- `dc_family_only_can`・`dc_random_can`・`dc_flashcard_can` は D-06 / D-15 / D-16 で答えが変わるので、断り方の期待は付けていない。

## 6. PR #378 / #379 との関係

| PR | ai-eval で足すもの | この PR との関係 |
|---|---|---|
| #378(feature/shared-record-update) | `editMembers` キー(Metrics・checkExpect)、m0disc 4 本の expect | `expectKeys` に `editMembers` を先に入れてある。どちらが先にマージされても `-check` とテストが通る |
| #379(feature/no-silent-omission) | `noOmission` キー(runCase)、`mentionsSubstitution` に「できません」を足す、チャットの全ステップで `ai.LeakedTerms`、m0disc 5 本の expect | `expectKeys` に `noOmission` を先に入れてある。`substitution: true` の判定は #379 の後のほうが正確(今は「代替」「replaced」しか見ない) |

この PR が触る行は、2 本と重ならないようにした。

- `m0disc_cases.json`: 2 本が変える行とその隣の行は変えていない(上の保留)。
- `main.go`: 先頭コメントは 2 本が変える行から離れた所に 1 段落、`checkExpect` は switch の先頭に `shared` を 1 つ、`main()` に `-check` とグループの表。2 本が変える `Record`・`Metrics`・`measure`・`runCase`・`mentionsSubstitution`・`noShared` の周りは変えていない。
- 新しいファイル: `expect.go` `expect_test.go`。

## 7. D-xx が入った時に足すもの

| Issue | ケース | 足す / 変える expect |
|---|---|---|
| D-02(投稿者名) | d_sns_post / d_circle_attend / d_event_rsvp | 名前の入力欄が無い(キーを新しく足す) |
| D-03(自分の投稿だけ) | d_edit_mine_only | `substitution: true` を外し、author で絞るキーを足す |
| D-05(公開の注意) | d_reserve / d_pet | summary に公開の注意(キーを新しく足す) |
| D-06(参加者だけの共有) | d_family_todo / d_pet | `scope: shared` |
| D-07(自分だけの印) | d_sns_post / d_edit_like / d_recipe_share | 1 人 1 回の印のキー |
| D-08(親から子) | d_sns_post / d_edit_comment / d_quiz_game | 子のフォームに親が入るキー |
| D-13(グループ集計) | d_ranking | グループ集計の一覧のキー |
| D-14(クイズの採点) | d_quiz_game | `computed: true` |
| D-15(ランダム) | d_dice | `substitution: true` を外し、ランダムのキーを足す |
