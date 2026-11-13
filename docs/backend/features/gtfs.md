# バスの時刻の静的な表(internal/gtfsstatic・#1692)

`backend/internal/gtfsstatic` は、事業者が出す静的な GTFS-JP の zip から、バスの「次の便・今の位置」(`gtfsRealtime` の `nextBuses` / `busPositions`)に要る部分だけを読んで表にする。リアルタイムの側の仕様と規約は [external-apis.md の GTFS リアルタイムのフィードと規約](external-apis.md#gtfs-リアルタイムのフィードと規約1692)。

## 目次

1. [何に使うか](#1-何に使うか)
2. [読むファイル](#2-読むファイル)
3. [表の持ち方](#3-表の持ち方)
4. [時刻と運行日](#4-時刻と運行日)
5. [データの入れ替え](#5-データの入れ替え)
6. [置き場](#6-置き場)
7. [テスト](#7-テスト)
8. [未定・決めること](#8-未定決めること)

## 1. 何に使うか

`backend/internal/extapi/gtfs_realtime.go` が、フィードごとに表を受け取る(`newGTFSRealtime` の引数 `static`)。表がある時とない時の違い:

| | 表がある | 表がない(今の本番) |
|---|---|---|
| 系統 | `route_short_name`(無ければ `route_long_name`) | `route_id` のまま |
| 停留所 | `nextBuses` の補足の頭に停留所の名前 | 付けない |
| 遅れだけが来る便 | 時刻表の時刻 + 遅れで出す | 出さない |
| `route_id` の無い便 | `trips.txt` の系統で `route` を絞る | 絞れない |

今の本番は表を渡していない(`static` が nil)。zip を取ってきて読む仕組みがまだ無いため(下の「データの入れ替え」)。

## 2. 読むファイル

GTFS-JP の zip のうち、次の 4 つだけを読む。どれかが無ければ `ErrBad`。zip の中で 1 段のフォルダに入っていても読む(同じ名前が 2 つあれば先の方)。

| ファイル | 読む列 | 要る列 | 使い道 |
|---|---|---|---|
| `stops.txt` | `stop_id`・`stop_name` | 両方 | 停留所の名前 |
| `routes.txt` | `route_id`・`route_short_name`・`route_long_name` | `route_id` と、名前のどちらか | 系統の名前 |
| `trips.txt` | `trip_id`・`route_id` | 両方 | 便の系統 |
| `stop_times.txt` | `trip_id`・`stop_id`・`arrival_time`・`departure_time` | `trip_id`・`stop_id` と、時刻のどちらか | 便ごと・停留所ごとの時刻 |

- 読まないもの: `agency.txt`・`calendar.txt`・`calendar_dates.txt`・`shapes.txt`・`fare_*.txt`・GTFS-JP の拡張(`office_jp.txt` など)・停留所の緯度経度。
- CSV は、先頭の BOM・CRLF・引用符でくくった値・列の順の違い・余分な列を受ける。見出しの前後の空白は除く。
- 大きさと行の数には上限がある(`gtfsstatic.Limits`。値は [external-apis.md の静的な GTFS-JP の読み込み](external-apis.md#静的な-gtfs-jp-の読み込みbackendinternalgtfsstatic))。

## 3. 表の持ち方

`Read` / `ReadFile` が返す `Table` は、メモリ上の 4 つの map。

| フィールド | キー → 値 |
|---|---|
| `Stops` | `stop_id` → 停留所の名前 |
| `Routes` | `route_id` → `route_short_name`、空なら `route_long_name` |
| `TripRoute` | `trip_id` → `route_id` |
| `Times` | (`trip_id`, `stop_id`) → 時刻(秒) |

- 名前は UTF-8 に直し、64 文字(`MaxNameRunes`)で切る。
- `Times` は到着の時刻、空なら出発の時刻。両方空の行(時刻の無い停留所)は入れない。同じ便が同じ停留所を 2 度通る時は最初の 1 回だけ。
- 引く関数: `StopName`・`RouteName`・`RouteOf`・`Scheduled`。表が nil でも呼べる(空を返す)。
- 表は読むだけで、DB にもファイルにも書かない。

## 4. 時刻と運行日

- 時刻は `H:MM:SS` / `HH:MM:SS`(`ParseTime`)。GTFS の決まりどおり 24 時を過ぎてよく(`25:10:00` = 90600 秒)、72 時未満まで受ける。
- 値は運行日の「正午の 12 時間前」からの秒。日本は夏時間が無いので、その日の 0 時(日本時間)からの秒と同じ。
- 表は**どの日に走る便か**を持たない(`calendar.txt` を読まない)。運行日は、リアルタイムの側で TripUpdate の `start_date` から決め、無ければ「日本時間の今日と昨日のうち、今に近い方」とする(`gtfsScheduled`)。

## 5. データの入れ替え

今あるのは「zip を読む関数」だけで、zip を取ってくる・定期的に読み直す・フィードと結ぶ仕組みは無い(`gtfs_realtime.go` の先頭のコメントにも同じことが書いてある)。

作る時の流れの案(推測。決まっていない):

1. 事業者のデータカタログから GTFS-JP の zip を取る(URL とライセンスはフィードと一緒に確かめる)。
2. `gtfsstatic.Read` で読み、エラー(`ErrBad`・`ErrTooLarge`)なら前の表を使い続ける。
3. フィードの id ごとに表を持ち、`newGTFSRealtime` の `static` から返す。
4. 事業者がダイヤを改正した時(GTFS-JP の `feed_info.txt` の有効期間など)に読み直す。

## 6. 置き場

| 物 | 置き場 |
|---|---|
| コード | `backend/internal/gtfsstatic/gtfsstatic.go` |
| テストのデータ | `backend/internal/gtfsstatic/testdata/basic/`・`messy/`(テストの中で zip にする) |
| 本番の zip | 未定 |

## 7. テスト

`cd backend && go test ./internal/gtfsstatic/`。ふつうの表・崩れた CSV・nil の表・壊れた zip・上限・展開すると大きくなる zip・時刻の読み方を確かめる。

## 8. 未定・決めること

- どの事業者の静的データを使うか: 未定。
- 本番の zip の置き場(リポジトリ・Cloud Storage・起動の時に取る など): 未定。
- 読み直す間隔と、ダイヤの改正の見つけ方: 未定。
- `calendar.txt` を読んで、その日に走らない便を外すか: 未定。
- `nextBuses` の停留所を名前で選べるようにするか: 未定。
