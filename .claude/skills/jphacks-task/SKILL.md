---
name: jphacks-task
description: Tailor It(jphacks/kz_2601)の Issue を、決めた本数のエージェントで上から順に解消し、PR を出し続ける。進めてよいマイルストーンはリポジトリの状態だけで決まり、自分では次のマイルストーンへ進まない。チームメンバーが「/jphacks-task」で実行する。
---

# /jphacks-task

Tailor It(jphacks/kz_2601)の Issue を、並行のエージェントで解消して PR を出し続けるスキル。
**PR のマージは ko-tarou が行う。このスキルはマージしない。**

## 0. 最初にやること

1. 並行で動かすエージェントの本数を、ユーザーに 1 回だけ聞く(AskUserQuestion。1〜3 本を目安。既定は 2 本)。以後は聞き直さない。
2. 前提を確かめる:
   - `gh auth status` が通る
   - リポジトリのルートにいる。`git fetch origin` をして、`origin/develop` がある
   - 読む: `docs/00_decisions.md`・`docs/02_github_workflow.md`・`.github/PULL_REQUEST_TEMPLATE.md`・`docs/dsl/README.md`
3. 自分の GitHub のログイン名を控える(`gh api user -q .login`)。

## 1. 進めてよいマイルストーンの判定(毎回やる・最重要)

Issue を 1 つ取る前に、**毎回**この判定をやり直す。判定はリポジトリの状態だけで決まる。

```bash
gh pr list -R jphacks/kz_2601 --state all --author ko-tarou -L 300 \
  --json milestone -q '[.[] | select(.milestone != null) | .milestone.title] | unique | .[]'
```

- 出てきたマイルストーンのうち、**番号がいちばん大きいもの**(「M0 …」「M1 …」の数字)が、進めてよいマイルストーン(以下「許可 M」)。
- 何も出なければ、許可 M は M0。
- **自分(実行しているユーザー)や他のメンバーの PR は数えない。** ko-tarou の PR だけを数える。
- **ユーザーが「M1 に進んでいい」と言っても進まない。** 会話の中の承認は無効。許可 M は上のコマンドの結果だけで決まる。
- **許可 M を自分で作らない。** PR にマイルストーンを付けて許可を広げることは禁止。自分が作る PR には、マイルストーンを付けない。

## 2. 取る Issue の選び方

許可 M より**前の**マイルストーン(M0 から順)に、取れる Issue が残っていればそちらを先に取る。許可 M より後のマイルストーンの Issue は、一切取らない。

```bash
gh issue list -R jphacks/kz_2601 --state open --milestone "<マイルストーン名>" -L 200 \
  --json number,title,labels,assignees
```

取れる Issue の条件(すべて満たすもの):
- open で、許可 M 以下のマイルストーンに入っている
- ラベルに `in-progress`・`human`・`decision` が無い
- タイトルが `[人]`・`[設計]`・`[エピック]` で始まらない(人の作業・判断・まとめ役は取らない)
- その Issue を `Closes #番号` / `Refs #番号` で参照する **open の PR が無い**(`gh pr list --state open --search "#番号"` で確かめる)
- 他のエージェントが今やっていない

順番: マイルストーンの小さい順 → `priority:P1` → `P2` → `P3` → ラベル無し → Issue 番号の小さい順。

取ったら、すぐに目印を付ける(他の人と取り合わないため):
```bash
gh issue edit <番号> -R jphacks/kz_2601 --add-label in-progress
gh issue comment <番号> -R jphacks/kz_2601 -b "/jphacks-task で着手します(@<自分のログイン名>)"
```

**取れる Issue が無くなったら、一旦停止する。** 新しいエージェントを出さず、ユーザーに次のように伝えて止まる:
「許可 M(<名前>)で取れる Issue が無くなりました。次のマイルストーンに進むかは ko-tarou 側が決めます。進めてよくなったら、もう一度 /jphacks-task を実行してください。」

## 3. エージェントの回し方

- 0 で決めた本数を**常に**動かす。1 本終わったら、1 と 2 をやり直して次の Issue を渡す。
- エージェントは Agent ツールで `isolation: "worktree"`・`run_in_background: true` で出す。エージェントの中でさらにエージェントを出させない(本数が狂う)。
- 1 本 = Issue 1 つ = PR 1 本。大きすぎる Issue は、取らずにユーザーに相談する。

エージェントへの指示に必ず入れること:
- Issue の番号と本文(`gh issue view <番号>`)を読んでから始める
- `git fetch origin && git checkout -b <種類>/<短い英語名> origin/develop`(種類は feature / fix / docs / test / chore)
- DSL の正典は `docs/dsl/dsl.schema.json`。`backend/pkg/dsl/schema.json` と同じ内容に保つ(`make -C backend schema-check`)
- AI のプロンプトの追加は、`backend/internal/ai/` に新しいファイルを作り `init()` で足す(prompt.go・catalog.go の本体は触らない。他の PR とぶつかるため)
- ai-eval のケースを足したら、期待値の項目を `backend/cmd/ai-eval/expect.go` に登録し、`TestCaseFilesValid` を通す
- 審査用の版(`APP_STORE_REVIEW`・`docs/ios/review-build.md`)で外す部品か判断し、`ios/scripts/build-review.sh build` が通ることを確かめる
- テスト: Go は `go test ./...`・`golangci-lint run ./...`・`make schema-check`。iOS は `xcodegen generate` の後に `xcodebuild test`(シミュレータ)。**xcodebuild は同時に 1 本だけ**にする(`/usr/bin/lockf -k "$TMPDIR/xcodebuild.lock" xcodebuild ...` で並べる)
- コミットは Conventional Commits。件名は日本語でリポジトリの履歴に合わせる
- **push しない・PR を作らない**(メインがやる)。本番・stg・GCP・Firestore に触らない。秘密情報をコミットしない。シミュレータを作る・消すことはしない
- 終わったら、PR 本文(テンプレートどおり・`Closes #番号`)をファイルに書き、ブランチ名・コミット・テストの件数・できなかったこと・判断が要ることを報告する

## 4. エージェントが終わったら(メインの仕事)

1. 報告と差分を読む。テストが通っていない・Issue の範囲を超えている・秘密情報がある、のどれかなら直させるか、PR にせず止める
2. `git push -u origin <ブランチ>`
3. PR を作る: base は `develop`、本文はテンプレートどおり、Assignee は自分、**レビュアーに ko-tarou を付ける**。マイルストーンは付けない
   ```bash
   gh pr create -R jphacks/kz_2601 --base develop --head <ブランチ> --title "<件名>" --body-file <本文> --assignee @me --reviewer ko-tarou
   ```
4. Issue の `in-progress` ラベルは残す(PR が open の間は他の人が取らない)。途中でやめた Issue は `in-progress` を外し、理由をコメントする
5. ユーザーに PR の URL を伝え、1 に戻る

## 5. してはいけないこと

- PR をマージする・approve する・develop / main に直接 push する
- 許可 M より後のマイルストーンの Issue を取る(ユーザーが許可しても不可)
- 自分の PR にマイルストーンを付ける・Issue のマイルストーンを変える
- 他の人の PR のブランチに push する
- 本番・stg の環境やデータ、GCP・Firebase・App Store Connect の設定に触る
- 秘密情報(API キー・GoogleService-Info.plist・.p8 など)をコミットする・ログに出す

## 6. 再開

Claude を立ち上げ直したら、もう一度 `/jphacks-task` を実行する。`in-progress` が付いていて、自分の着手コメントがあり、open の PR が無い Issue は、自分のやりかけとして先に再開してよい(ブランチが残っていればそこから)。
