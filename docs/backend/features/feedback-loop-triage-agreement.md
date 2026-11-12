# 自己改善ループ: 振り分けと人の判断の一致を測る(#376)

> ループ全体: [feedback-loop.md](feedback-loop.md)。測る道具の中身: [振り分けの精度を測る](feedback-loop.md#振り分けの精度を測る)。
> #376 の完了の条件は「1 週間分のフィードバックの振り分けが、人の判断と 8 割以上一致する」。この文書は、それを人が測る手順。

## 目次

1. [流れ](#流れ)
2. [手順](#手順)
3. [正解の付け方](#正解の付け方)
4. [結果の読み方](#結果の読み方)
5. [守ること](#守ること)

## 流れ

| 段 | する人 | コマンド | できるもの |
|---|---|---|---|
| 1. 書き出す | 人 | `triage-label-export -out FILE` | 直近 1 週間の `loop_inbox` の文(伏せ字済み)と、空の正解欄 |
| 2. 正解を付ける | 人 | (エディタ) | `category` / `priority` / `bundle` を埋めた同じファイル |
| 3. 測る | 人 | `triage-eval -set FILE -model ai` | 分類・優先度・束ねの一致率、取り違えの表、8 割に届いたか |

`triage-eval` は書き出した文をメモリの中だけで振り分け、下書きまで通して比べる。Firestore には書かない。

## 手順

```sh
cd backend
export GOOGLE_CLOUD_PROJECT=jphacks-kz2601
mkdir -p ~/loop-labels   # リポジトリの外に置く

# 1. 直近 7 日に取り込んだ分を書き出す(既にあるファイルは上書きしない)
go run ./cmd/feedback-loop triage-label-export -out ~/loop-labels/2026-w46.jsonl

# 2. ファイルを開き、各行の category / priority / bundle を埋める(下の「正解の付け方」)

# 3. 振り分けのモデルで測る(トークンを使う。段のスイッチと月の上限を見る)
LOOP_TRIAGE=on LOOP_ANTHROPIC_API_KEY=... \
  go run ./cmd/feedback-loop triage-eval -set ~/loop-labels/2026-w46.jsonl -model ai | tee ~/loop-labels/2026-w46-report.txt
```

- `-days N` で期間を変えられる(既定 7)。期間は取り込んだ日時(`ingestedAt`)で数える
- 書き出すのは、振り分けのモデルが読んでよい項目だけ(`aiAllowed`、またはモデルの答えが検査で外されて `held` になったもの)。同意の版・伏せ字の多さ・個人情報の疑いで止めた項目は出ない
- 200 件(`triage-eval` の上限)を超える時は、期間の中から均等に 200 件を選ぶ
- 正解が空の行が残っていると `triage-eval` は `not labelled yet` で止まる(その行番号と id を出す)
- 費用を使わずに形だけ確かめる時は `-model ai` を外す(キーワードの基準線で回る)

## 正解の付け方

1 行が 1 件。`id` と `text` は触らない。

| 欄 | 入れるもの |
|---|---|
| `category` | `bug`(不具合)/ `feature`(要望・作れなかった依頼)/ `question`(使い方の質問)/ `praise`(感想)/ `spam`(対象外・悪用) |
| `priority` | その件が入る束に人が付ける優先度 `P1` / `P2` / `P3`。束の件数も数えて決める |
| `bundle` | 同じ問題の行に同じ短い名前(例 `photo-crash`)。1 件だけの問題は空のまま |

迷ったら、Issue にする時にどう書くかで決める。モデルの答えは見ずに付ける(書き出しにはモデルの答えを入れていない)。

## 結果の読み方

```text
triage-eval: 120 cases
  category: 101/120 (84%)
  priority: 92/120 (77%)
  bundle:   together 40/46 (87%), apart 7000/7094 (99%) (drafts 61)
  verdict:  below 80%: priority

  category (rows: person, columns: model)
              bug  feature  question  praise  spam  (held: triage_output_rejected)
    bug        40        3         1       0     0                               1
    ...
```

- 上の 3 行が一致率。`verdict` が 8 割を下回った欄を出す(`every field at or above 80%` なら全部届いた)
- 表は行が人の正解、列がモデルの答え。対角線の外が取り違え。`(held: …)` はモデルが分類しなかった件(外れとして数える)
- 束ねは 2 件の組で数える。`together` は人が同じ束にした組をモデルも同じ束にした割合、`apart` は別の束にした組を別にした割合。組の数は `apart` が大きくなるので、`together` を主に見る
- `miss` の行は外れた id。文はファイルの同じ id の行で見る(結果には文を出さない)

#376 を閉じる時は、`verdict` の行と 3 つの一致率を Issue にコメントする(文・id は貼らない)。

## 守ること

- 書き出したファイルと結果は**リポジトリに置かない・コミットしない・チャットに貼らない**。伏せ字済みでも本物の利用者の文
- ファイルは作った人だけが読める権限(0600)で作られる。測り終えたら消す(`loop_inbox` と同じく 90 日を超えて持たない)
- 文は書き出す時にもう一度伏せ字にし、それでも個人情報に見える行は書き出さない。`triage-eval` もモデルに渡す前に伏せ字にする
