# ai-eval の基準と PR 用の表(#371)

生成の品質が下がる PR を赤にするための、`backend/cmd/ai-eval` の 2 つのフラグ。実装は `backend/cmd/ai-eval/gate.go`。

## 目次

1. [基準(-min-pass / -thresholds)](#1-基準-min-pass---thresholds)
2. [PR 用の表(-report md)](#2-pr-用の表-report-md)
3. [使えるモード](#3-使えるモード)
4. [CI での使い方(案)](#4-ci-での使い方案)
5. [人の作業](#5-人の作業)

## 1. 基準(-min-pass / -thresholds)

pass% は今までと同じ数え方(`expect` のあるケースのうち、生成が成功し外れが 0 本のもの。`expect` の無いケースは数えない)。

| フラグ | 意味 |
|---|---|
| `-min-pass 0.9` | `total` の pass% が 90% 未満なら終了コード 1。値は 0〜1(`90` はエラー)。`0`(既定)は基準なし |
| `-thresholds th.json` | グループごとの基準。`{"m0-social": 0.5, "total": 0.8}` の形の JSON |

- どちらも付けなければ、出力も終了コードも今までと同じ。
- 両方が `total` を決めた時は厳しい方を使う。
- 基準を書いたグループに `expect` のあるケースが 1 本も無い時は失敗にする(グループ名の打ち間違いや、空の実行で黙って通らないように)。
- 判定は表を出した後に行う。失敗しても表(と `-report-out` のファイル)は残るので、CI で赤にしつつ表をコメントできる。

`-thresholds` のファイルは `internal/ai/eval/` に置かない(`-check-all` が形の分からない JSON として失敗にする)。

## 2. PR 用の表(-report md)

| フラグ | 意味 |
|---|---|
| `-report md` | 判定の行・グループごとの表(pass / checked・pass%・基準・PASS/FAIL・expect なし)・通らなかったケース(折りたたみ)を Markdown で出す |
| `-report-out r.md` | 表をこのファイルに書く。省略すると標準出力に出し、今までの表の代わりにする |

```sh
cd backend
go run ./cmd/ai-eval -score '<raw dir>/*.json' -min-pass 0.9 -report md -report-out ai-eval-report.md
```

表のセルの `|` と改行はエスケープする(エラー文がそのまま入っても表が崩れない)。

## 3. 使えるモード

| モード | 基準・表 |
|---|---|
| `-score <glob>`(保存した実行を読む・API を呼ばない) | 使える |
| 生成の実行(既定・実 API) | 使える。raw JSON を書いた後に判定する |
| `-check` / `-check-all` / `-chat` / `-compare-*` | 使えない(付けるとエラーで終わる。黙って無視しない) |

テストは `gate_test.go` で、保存した実行の形のデータだけを使う(実 API は呼ばない)。

## 4. CI での使い方(案)

今の backend-ci は `go run ./cmd/ai-eval -check-all internal/ai/eval` でケースの形だけを確かめている(API を呼ばない)。pass% を見るには実 API の実行が要るので、次のような手順が考えられる。

1. 生成に関わる差分(`backend/internal/ai/` など)がある PR だけで、件数を絞って実 API で回す(費用の上限は `-only` とケースファイルで決める)。
2. `-min-pass` か `-thresholds` で赤にする。
3. `-report md -report-out` で書いた表を PR にコメントする。

```sh
go run ./cmd/ai-eval -cases internal/ai/eval/m0disc_cases.json -out "$RUNNER_TEMP/ai-eval" \
  -thresholds "$THRESHOLDS" -report md -report-out "$RUNNER_TEMP/ai-eval.md"
```

## 5. 人の作業

ワークフロー(`.github/`)への組み込みは、人が決めて行う。この PR ではワークフローを変えていない。決めること:

- 実 API を CI で回すか(`ANTHROPIC_API_KEY` を secrets に置くか)、回す条件(どのパスの差分か)と費用の上限
- 基準の値(最初の実 API の実行で基準値を取ってから決める。[m0disc-regression.md](m0disc-regression.md) の 3 節)
- 表を PR にコメントする方法(コメント用のトークンと権限)
- 必須チェック(branch protection)にするか
