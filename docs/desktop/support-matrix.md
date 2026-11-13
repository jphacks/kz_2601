# デスクトップ版(Windows・Linux)の部品の対応表

DSL の全部の type(Container 7・Leaf 70 = 77。`docs/dsl/dsl.schema.json` と同じ数)とアクション 16 種を、デスクトップ版でどう出すかを 1 行ずつ書く。親は #1746。

- 事実の元: `origin/develop` の `748767fd6`(openUrlButton の行と数は `origin/epic/god` の `abe167e0f`)。
- 描ける type: `desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/DesktopSupport.kt` の `DesktopLeaves`(`containers`・`leaves`)と `io/DesktopIo.kt` の `IoLeaves.types`。
- 描けない type: 上に無い type。適合テストの `state` は `unavailable` になり、画面には「デスクトップ版ではまだ使えない部品」の一言を出す。
- 適合テストで違ってよい所: `desktop/app/src/test/kotlin/jp/kz2601/metamon/desktop/ConformanceAllowlist.kt` の `unavailableTypes`。この表の state が `unavailable` の行と一致する(placeSearch・callButton は適合テストの case に出てこない(`docs/dsl/conformance/coverage.json` の `notYetCovered`)ので一覧に無い)。
- 正: iOS(`ComponentRegistry` に 77 全部を登録)と Android(`ComponentRegistry.standard` に 77 全部。`DeviceCapabilitiesTest` が抜けを落とす)はどれも描ける。Mac は `ClientCapabilities.macUnavailableComponents` の 4 つ(healthStat・sensorValue・locationTracker・dashcam)だけ「この Mac では使えません」の枠。
- 決まっていない部品(状態が「未定」)の推奨は **推奨・未確定**。決めるのは #1746 でオーナー。
- 正典(`docs/00_decisions.md`)は変えない。ここは今のコードの写し。

## 状態の意味

| 状態 | 意味 |
|---|---|
| 描ける | iOS と同じ役目を果たす(見た目はデスクトップの形) |
| 代わり | 描くが、パソコンに無い機能の所は別の形で受ける(ファイルを選ぶ・キーボードで入れる・縦に並べる など) |
| 使えません | 今は描かない。パソコンに無い機能か、まだつないでいない(つなぐ Issue があれば書く) |
| 未定 | 今は描かない。作り方を #1746 で決める(候補と推奨を書く) |

「適合 state」は `DesktopLeaves.state` が返す値(`rendered` / `unavailable`)。

## Container(7)

| type | 状態 | 適合 state | 理由・代わりの形 | Mac | 関係する Issue |
|---|---|---|---|---|---|
| card | 描ける | rendered | カードの枠に子を並べる(子に area があれば枠の中も 6 列のグリッド) | 描ける | #1741 |
| carousel | 代わり | rendered | 横に送らず、子を縦に並べる | 描ける | #1741 |
| disclosure | 描ける | rendered | `expanded`(無ければ閉じる)で始め、見出しを押す・Enter / Space で開け閉めする。開け閉めはウィンドウの間だけ覚える。読み上げは「開いています / 閉じています」(#3214) | 描ける | #1741 |
| grid | 代わり | rendered | 子に area があれば 6 列のグリッド、無ければ縦に並べる(columns はまだ見ない) | 描ける | #1741 |
| hstack | 描ける | rendered | 子を同じ幅で横に並べる | 描ける | #1741 |
| section | 描ける | rendered | 見出しと子 | 描ける | #1741 |
| vstack | 描ける | rendered | 子を縦に並べる | 描ける | #1741 |

## Leaf(70)

| type | 状態 | 適合 state | 理由・代わりの形 | Mac | 関係する Issue |
|---|---|---|---|---|---|
| aiButton | 使えません | unavailable | AI の呼び出し(/v1/ai)をまだつないでいない | 描ける | #2165 |
| aiChat | 使えません | unavailable | AI の呼び出し(/v1/ai)をまだつないでいない | 描ける | #2166 |
| audioRecorder | 代わり | rendered | 録音はしない。音声ファイルを選ぶ | 描ける | #1743 |
| avatar | 描ける | rendered | | 描ける | #1742 |
| badge | 描ける | rendered | | 描ける | #1742 |
| bracket | 描ける | rendered | | 描ける | #1745 |
| button | 描ける | rendered | | 描ける | #1747 |
| calendar | 描ける | rendered | 今月の表 | 描ける | #1744 |
| callButton | 未定 | unavailable | パソコンに電話が無い。推奨・未確定: 番号をコピーできる枠にし、`tel:` を OS に渡す(通話のアプリが入っていれば開く) | 描ける | #1746 |
| chart | 描ける | rendered | 棒・線・扇 | 描ける | #1744 |
| chatThread | 使えません | unavailable | 共有レコードの購読(会話の同期)がまだ無い | 描ける | #2214 |
| colorPicker | 描ける | rendered | | 描ける | #1743 |
| dashcam | 使えません | unavailable | カメラの常時録画。パソコンのカメラは使わない | 使えません | #2179 |
| datePicker | 描ける | rendered | 文字で入れ、「今日」「‹ ›」で動かす | 描ける | #1743 |
| divider | 描ける | rendered | | 描ける | #1742 |
| drawingField | 描ける | rendered | マウス・ペンで描いて PNG に | 描ける | #1743 |
| emptyState | 描ける | rendered | | 描ける | #1742 |
| feed | 代わり | rendered | 取得しない。見出しと「デスクトップ版ではまだ取得しません」の案内だけ | 描ける | #1744 |
| fileImport | 描ける | rendered | CSV・テキスト・PDF を選ぶ・落とす | 描ける | #1753 |
| flashcards | 描ける | rendered | | 描ける | #1745 |
| gauge | 描ける | rendered | | 描ける | #1742 |
| gridTable | 描ける | rendered | | 描ける | #1744 |
| healthStat | 使えません | unavailable | ヘルスケアのデータ(HealthKit / Health Connect)がパソコンに無い | 使えません | #2179 |
| icon | 描ける | rendered | `docs/dsl/icon-map.json` を Material の絵に写す | 描ける | #1742 |
| image | 描ける | rendered | | 描ける | #1742 |
| label | 描ける | rendered | | 描ける | #1742 |
| link | 描ける | rendered | 既定のブラウザで開く(許可した scheme だけ) | 描ける | #1742 |
| list | 描ける | rendered | | 描ける | #1744 |
| locationButton | 代わり | rendered | 位置は取れない。「このパソコンでは使えません」の枠と、保存済みの値だけ出す | 描ける | #1743 |
| locationPicker | 代わり | rendered | 地図で選べない。「このパソコンでは使えません」の枠と、保存済みの値だけ出す | 描ける | #1743 |
| locationTracker | 使えません | unavailable | 位置情報(GPS)がパソコンに無い | 使えません | #2179 |
| mapView | 未定 | unavailable | 地図の SDK を入れていない。候補: MapLibre Native の JVM 版・地図タイルを Compose で描く。推奨・未確定: SDK を入れず「地図で開く」の一覧(#2177) | 描ける | #1746・#2177 |
| markdown | 描ける | rendered | | 描ける | #1742 |
| matchList | 代わり | rendered | つながりを読まない。見出しと「サインインした後に出ます」の案内だけ | 描ける | #1744・#1749 |
| musicPlayer | 使えません | unavailable | 音楽のライブラリ(MusicKit など)がパソコンに無い | 描ける | #2179 |
| nearbyShare | 使えません | unavailable | 近くの端末との共有(Nearby / MultipeerConnectivity)がパソコンに無い | 描ける | #2179 |
| numberField | 描ける | rendered | | 描ける | #1743 |
| openUrlButton | 描ける | rendered | `OpenUrlButton.kt`。link と同じ openUrl(許可した scheme だけ)で開く。`app` があれば先に試し、開けなければ次(slack:// が無ければ Web)。知らない app・許可しない URL はボタンを出さず一言だけ出す | 描ける | #1740 |
| photoField | 代わり | rendered | 撮影はしない。ファイルを選ぶ・落とす・Ctrl+V | 描ける | #1743・#1764 |
| picker | 描ける | rendered | | 描ける | #1743 |
| placeSearch | 未定 | unavailable | 場所の検索も地図も入れていない。推奨・未確定: 地図を出さず、検索の結果を一覧にして「地図で開く」(mapView と同じ形) | 描ける | #1746 |
| progressBar | 描ける | rendered | | 描ける | #1742 |
| qrCode | 描ける | rendered | | 描ける | #1745 |
| qrScanner | 代わり | rendered | カメラで読まない。画像ファイルかクリップボードの画像から読む(zxing) | 描ける | #1743 |
| randomPicker | 描ける | rendered | | 描ける | #1745 |
| rating | 描ける | rendered | | 描ける | #1743 |
| routeGuide | 使えません | unavailable | 道案内(地図と位置)がパソコンに無い | 描ける | #2179 |
| searchBar | 描ける | rendered | | 描ける | #1744 |
| secureField | 描ける | rendered | | 描ける | #1743 |
| sensorValue | 使えません | unavailable | 加速度・気圧などのセンサーがパソコンに無い | 使えません | #2179 |
| settlement | 描ける | rendered | | 描ける | #1745 |
| shareButton | 未定 | unavailable | OS の共有をまだつないでいない。推奨・未確定: 「コピー・ファイルに書き出す」で受ける(#2178) | 描ける | #1746・#2178 |
| slider | 描ける | rendered | | 描ける | #1743 |
| spacer | 描ける | rendered | | 描ける | #1742 |
| speakButton | 使えません | unavailable | 読み上げ(音声合成)をまだつないでいない。Windows の SAPI・Linux の speech-dispatcher で鳴らす予定 | 描ける | #2173 |
| stat | 描ける | rendered | | 描ける | #1744 |
| stepper | 描ける | rendered | | 描ける | #1743 |
| table | 描ける | rendered | 列の幅を引いて変える・見出しで並べ替え | 描ける | #1744 |
| text | 描ける | rendered | | 描ける | #1742 |
| textEditor | 描ける | rendered | | 描ける | #1743 |
| textField | 描ける | rendered | | 描ける | #1743 |
| timePicker | 描ける | rendered | | 描ける | #1743 |
| timeline | 描ける | rendered | | 描ける | #1744 |
| timerDisplay | 描ける | rendered | 走っている残り / 経過はトレイにも出す | 描ける | #1745・#1761 |
| toggle | 描ける | rendered | | 描ける | #1743 |
| transcribeField | 代わり | rendered | 話して入れない。キーボードで入れる | 描ける | #1743・#1751 |
| videoField | 代わり | rendered | 撮影はしない。ファイルを選ぶ・落とす | 描ける | #1743・#1764 |
| videoPlayer | 未定 | unavailable | 動画の再生を入れていない。候補: VLCJ(VLC が要る)・JavaFX Media。推奨・未確定: 埋め込まず「既定のアプリで再生」の枠(#2175) | 描ける | #1746・#2175 |
| voiceInput | 使えません | unavailable | 音声入力(音声認識)をまだつないでいない。端末の中の文字起こし(whisper.cpp)で描く予定 | 描ける | #1751・#2172 |
| webView | 未定 | unavailable | Web の埋め込みを入れていない。候補: JCEF(KCEF・Chromium 同梱で +100MB 前後)。推奨・未確定: 埋め込まず「ブラウザで開く」の枠(#2176) | 描ける | #1746・#2176 |

数(Container と Leaf の合計 77): 描ける 48・代わり 11・使えません 12・未定 6。

## アクション(16)

元: `desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/Actions.kt` の `DesktopActionHost` と `Session.kt`。

| type | 状態 | 理由・代わりの形 | 関係する Issue |
|---|---|---|---|
| addToCalendar | 代わり | .ics を OS の一時フォルダ(`tailorit-calendar/`)に書き、既定のカレンダーで開く | #1747 |
| back | 描ける | | #1747 |
| cancelNotification | 代わり | トレイの通知の予約を消す(起動中だけ) | #1752 |
| deleteRecord | 描ける | 端末の保存から消す(stg に向けた時だけ送る) | #1754・#1887 |
| endActivity | 使えません | Live Activity に当たる物がパソコンに無い | — |
| export | 描ける | CSV か PDF をダウンロードに書き出し、表を TSV でクリップボードにも入れる | #1753 |
| follow | 使えません | まだつないでいない(サインインが要る) | #1749 |
| lookupBook | 描ける | 既定はモックの本。`TAILORIT_API_BASE` があれば `POST /v1/lookup/book` | #1747 |
| navigate | 描ける | push はウィンドウの中で積む・sheet はウィンドウ内のダイアログ・fullScreen はウィンドウ全体に重ねる | #1747 |
| openUrl | 描ける | 既定のブラウザ(許可した scheme だけ) | #1747 |
| openVoice | 使えません | 音声入力をまだつないでいない | #1751・#2167 |
| resetForm | 描ける | | #1747 |
| saveRecord | 描ける | 端末の保存に書く(stg に向けた時だけ送る) | #1754・#1887 |
| scheduleNotification | 代わり | トレイの通知(起動中だけ・時刻は次の 1 回) | #1752・#2185・#2186・#2187 |
| startActivity | 使えません | Live Activity に当たる物がパソコンに無い | — |
| unfollow | 使えません | まだつないでいない(サインインが要る) | #1749 |

## 直す時

- `DesktopLeaves` か `ConformanceAllowlist.unavailableTypes` を変えたら、この表の状態と適合 state も同じコミットで直す。
- 「未定」の行は #1746 でオーナーが決めたら、決まった形と子の Issue に書き換える。
- AI に描けない部品を作らせない申告(#2161)は、この表の「使えません」「未定」を元にする。
