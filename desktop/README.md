# Tailor It デスクトップ版(Windows・Linux)

Compose Multiplatform Desktop(JVM)で作る Windows・Linux 版。DSL の読み込み・式・レコードの規則は Android と同じ `android/dsl`(純 Kotlin)をそのまま使う。Android SDK は要らない。

## 動かす

JDK 17 以上が要る。

```
cd desktop
./gradlew :desktop:run                    # 同梱の例 shopping を開く
./gradlew :desktop:run --args=kakeibo     # docs/dsl/examples の別の例
./gradlew :desktop:test :dsl:test         # 適合テスト(docs/dsl/conformance)と :dsl のテスト
```

stg の appconfig を手元で試す(サインインが入るまでの確かめ用。トークンはファイルに書かない):

```
TAILORIT_API=stg TAILORIT_DEV_TOKEN=<Firebase の ID トークン> ./gradlew :desktop:listApps
```

## 今できること

- `docs/dsl/examples` の DSL を 1 本読み、画面を 6 列のグリッド(`area`)に並べる。area の無い部品は下に全幅で積み、フォーム画面は段(band)で積む。広いウィンドウでは幅 720dp で止めて中央に寄せる。最小 360×560、大きさと位置は次の起動で戻す。
- 描ける部品: 並び・見出し・text・label・badge・markdown(書式付き)・button・link・list(LazyColumn・チェック・右クリック / Delete キーで削除)・table(列の幅を引いて変える・見出しで並べ替え)・timeline・gridTable・searchBar・feed / matchList(見出しと案内だけ)・入力 20 種(textField〜qrScanner)・fileImport・image・avatar・icon(`docs/dsl/icon-map.json` をビルド時に同梱し、Material の絵に写す。表に無い名前は細い円)・stat・chart(棒・線・扇)・progressBar・gauge・calendar(今月の表)。
- 記録を入れて開く: `./gradlew :desktop:run --args="kakeibo /path/records.json stats"`(2 つ目は `{"<entity>": [{"id", "data"}]}`、3 つ目は最初の画面)。
- 入力: 日付・時刻は文字で入れて「今日」「‹ ›」で動かす。写真・動画はファイルを選ぶ・落とす・Ctrl+V、録音はファイルを選ぶ(撮影・録音・位置はこのパソコンでは使えない旨の枠)。手書きはマウス・ペンで描いて PNG に。QR は画像ファイルかクリップボードの画像から読む(zxing)。
- 遷移: push はウィンドウの中で積み、sheet はウィンドウ内のダイアログ、fullScreen はウィンドウ全体に重ねる。タブは下に並べる。confirmMessage は確認ダイアログ。規則は Android と同じ `NavigationResolver`・`FormDraft`(`android/renderer` のソースをビルド時に取り込む。`android/` は変えない)。
- アクション: 保存・削除は端末に残す(下の「端末の保存」)。resetForm は下書きを戻す。openUrl はブラウザ、addToCalendar は .ics を書き出して既定のカレンダーで開く、export は CSV か PDF(format)をダウンロードに書き出し、表を TSV でクリップボードにも入れる、scheduleNotification はトレイの通知(起動中だけ)、lookupBook はモックの本(`TAILORIT_API_BASE` を付けると `POST /v1/lookup/book`)。openVoice は下の「音声入力」を有効にした時だけ録音して文字にし、既定は「使えない」旨を出す。startActivity・endActivity・follow は「使えない」旨を出す。
- トレイ / メニューバーに常駐する(`Tray.kt`): 開く・すぐ記録(最初の記録できるフォーム画面を DSL と同じ開き方で開く)・音声(まだ使えない旨)・走っている timerDisplay の残り / 経過(メニューとツールチップ)・終了。ウィンドウの × はトレイに隠す(メニューの「ウィンドウを閉じてもトレイに残す」で切り替え・次の起動にも残る)。トレイが無い環境では × で終わる。通知(scheduleNotification・タイマーの終わり)もこのアイコンから出す。Linux は GNOME だと拡張(AppIndicator)が要る。
- 複数のウィンドウ(`Windows.kt`): アプリごとに 1 枚。コマンドパレットでアプリを選ぶ・トレイの「新しいウィンドウ」で開く(開いていれば前に出す)。一覧の行をダブルクリック / 右クリックの「別のウィンドウで開く」、またはパレットの「この記録を別のウィンドウで開く」で記録の詳細を別の 1 枚に(同じ記録は 1 枚)。遷移・下書き・ショートカット・Ctrl+F はウィンドウごと、記録はアプリごとに共有するので、片方で保存すると他方の一覧にすぐ出る。同じ記録を 2 枚で直した時は後に保存した方が勝ち、上書きしたことを知らせる。詳細のウィンドウは保存・閉じる・Esc で閉じる。× は他にウィンドウがあればその 1 枚だけ閉じ、最後の 1 枚はトレイに隠す。大きさ・位置はアプリ / 詳細の種類ごとに覚え、2 枚目からは少しずらして画面の中に収める。
- 端末の保存(`store/SessionPersistence.kt`): 記録は端末の置き場(`store/`)に書き、再起動しても残る。最後に開いたアプリから開く。既定は通信しない。`TAILORIT_API=stg` と `TAILORIT_DEV_TOKEN` が両方ある時だけ、保存・削除を送信待ちに積んで stg へ送り、起動時に前回送れなかった分を送り直す(送り待ちがあれば画面の下に件数)。記録の JSON を引数で渡した時は端末に残さない。起動時は端末の記録を裏で読み、読み終えるまでウィンドウにスケルトン(形だけの画面)を出す。サインアウトの画面(#1749)ができるまでは、コマンドパレットの「この端末のデータを消す」で確認のあと開いている全アプリの記録・送り待ち・設定を消す(`AppHub.wipeLocalData()`)。
- サインインはまだ無い。
- 音声入力(#1751・既定は off): `TAILORIT_VOICE=1`(または `-Dtailorit.voice=true`)の時だけ、openVoice を 1 回押すとマイクで録り始め、もう 1 回で止めて端末の中で文字にする(whisper.cpp を JNI で。音もモデルも PC の外に出さない)。結果は知らせの文言で出す(声でアプリを変えるチャットの画面はまだ無い)。マイクは Java Sound(16kHz モノラルで開き、開けない装置は 48k / 44.1k で開いて 16kHz に直す)。音が全部 0 の時は OS がマイクを止めているとみて、Windows のプライバシーの設定の場所を出す。
  - ライブラリ: `whisper/native/build.sh`(Linux・macOS)か `whisper\native\build.ps1`(Windows・Visual Studio 2022 の C++)で `whisper/native/out/<os>-<arch>/` に作る。版とハッシュは Android(`android/whisper/src/main/cpp/CMakeLists.txt`)と同じ。作った物は `:desktop:run` と配る形(`appResourcesRootDir`)に入る。git には入れない。無ければ「この PC では用意できていません」。
  - モデル: Android と同じ `ggml-base-q5_1.bin`(約 60MB・版とハッシュを固定)を初回に端末の保存先の `models/` へ落とす。git にも配る形にも入れない。`TAILORIT_WHISPER_MODEL=<ファイル>` で手元のモデルを使う。「この端末のデータを消す」では消さない。
  - 手元の確かめ: `./gradlew :whisper:transcribeWav --args="<wav> [ja|en]"`(読み込みと文字にする時間を出す)。
- 言語: OS の言語が日本語なら日本語、それ以外は英語。`TAILORIT_LANG=en`(または `-Dtailorit.lang=en`)で替えられる。文言は Android の `strings*.xml` をビルド時にそのまま同梱し、同じキーで引く(引けなければ英語 → 日本語)。
- 明暗: OS のダークモードに従う(Windows はレジストリ、Linux は freedesktop の color-scheme か GNOME の設定、起動中の切り替えも 5 秒ごとに追う)。`TAILORIT_THEME=light / dark` で固定できる。
- 読み上げ: 見出し・タブ(選択中)・ダイアログの名前・知らせ(live region)・絵で描く部品(グラフ・進み具合・メーター・星・画像)の説明を付けている。Windows は Java Access Bridge を通す(`jabswitch -enable` が要る)。Linux の Orca は JDK に橋渡しが無く、まだ読めない。
- ファイル: `fileImport` で CSV(取り込みの確認を出す)・テキスト・PDF(本文)を取り込む。
- ドラッグ & ドロップ: 画面に写真・動画・CSV・テキスト・PDF を落とすと、photoField / videoField / fileImport / list に振り分ける。写真は外へ引き出せる。
- クリップボード: 一覧の「表をコピー」で表計算に貼れる TSV、「表を貼り付け」で表計算の行を取り込む。list・table の行はクリックで選び、Shift+クリックで範囲、Ctrl+クリック(macOS は ⌘)で足し引き、Ctrl+A で全部。選んでいる時は Ctrl+C と「表をコピー」が選んだ行だけを同じ TSV(見出し付き)でコピーする(table は見えている列)。詳細へ移る行(onItemTap)は修飾キー無しのクリックで今までどおり開く(`RowSelection.kt`)。画像は Ctrl+V で photoField へ。
- 印刷: Ctrl+P(または見出しの「印刷」)で一番上の画面をプレビューし、OS の印刷ダイアログから A4 縦で印刷する。頁は export の PDF と同じ(`io/PdfIo.pages`)。一覧・table・gridTable・chart は表ごとに頁を改めて全行を複数頁に割り、stat などの値とフォームの入力は「項目 / 値」の表にする(`Print.kt`)。
- キーボード: Ctrl+K(または Ctrl+Shift+P)でコマンドパレット(画面・アクション・同梱のアプリを名前で選ぶ)、Ctrl+N 新規・Ctrl+S 保存・Ctrl+F 検索・Delete 削除(確認あり)・Esc 閉じる / 戻る・F1(または Ctrl+/)で一覧。macOS は Ctrl の代わりに ⌘、削除は ⌘⌫。Tab はグリッドの行 → 列の順(`Keyboard.kt`・`CommandPalette.kt`)。
- まだ描けない部品は「デスクトップ版ではまだ使えない部品」の枠で残す。

## 構成

| 場所 | 中身 |
|---|---|
| `settings.gradle.kts` | `:dsl`(`../android/dsl`)と `:desktop`(`app/`)。版は `../android/gradle/libs.versions.toml` を共有 |
| `app/src/main/kotlin/.../desktop/` | ウィンドウ・画面の描画・モックのレコード |
| `app/src/main/kotlin/.../desktop/Session.kt` | 画面の状態と操作(遷移・下書き・保存・削除)。画面と適合テストが同じ物を使う |
| `app/src/main/kotlin/.../desktop/InputLeaves.kt`・`Actions.kt` | 入力の部品 20 種・OS に頼むアクション |
| `whisper/` | 端末内の文字起こし。`android/whisper` の純 Kotlin の部分(音の区切り・モデルの取得・WhisperEngine)をビルド時に写して使い、`WhisperNative` だけデスクトップの読み方(`compose.application.resources.dir` など)。JNI の C++ と CMake は `whisper/native/` |
| `app/src/main/kotlin/.../desktop/voice/` | マイク(`Microphone`・Java Sound)・文字起こし(`Transcriber`・whisper.cpp)・openVoice の流れ(`VoiceInput`)・モデルの置き場 |
| `services/` | `android/services` の純 Kotlin の部分(API クライアント・記録の保存・送信待ち・同意)を同じソースのまま取り込む。取り込むファイルは `services/build.gradle.kts` で名指し。宣言にだけ出る Android の型は `services/src/shim` の置き換え |
| `app/src/main/kotlin/.../desktop/net/` | 接続先の設定(既定はモック)・`X-Client-Platform: windows / linux / macos`・User-Agent |
| `app/src/main/kotlin/.../desktop/store/` | 端末の保存。`%APPDATA%\TailorIt`・`$XDG_DATA_HOME/tailorit`(無ければ `~/.local/share/tailorit`)・`~/Library/Application Support/TailorIt` に設定・同意・記録・送信待ち・DSL を JSON で置く。サインアウトで消す |
| `app/src/main/kotlin/.../desktop/io/` | 取り込み・書き出し・ドラッグ & ドロップ・クリップボード(PDF は Apache PDFBox) |
| `app/src/main/kotlin/.../desktop/i18n/`・`app/src/main/resources/i18n/desktop/` | 文言。Android の `renderer`・`app` の `values`(日本語)・`values-en`(英語)をビルド時に `i18n/` へ同梱して読む。デスクトップだけの文言は同じ形の XML(キーは `desktop_`) |
| `app/src/main/kotlin/.../desktop/theme/`・`A11y.kt` | OS に従う明暗・読み上げの説明 |
| `app/src/test/kotlin/.../desktop/DesktopConformanceTest.kt` | 全 OS 共通の適合テストのうち、今比べられる部分(画面・入力の値とラベル・操作の steps)。違ってよい所(描けない type・まだ比べない props)は `ConformanceAllowlist.kt` に理由付きで書き、無い違いは落ちる |
