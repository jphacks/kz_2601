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
- 描ける部品: 並び・見出し・text・label・badge・markdown(書式付き)・button・link・list(LazyColumn・チェック・右クリック / Delete キーで削除)・table(列の幅を引いて変える・見出しで並べ替え)・timeline・gridTable・searchBar・feed / matchList(見出しと案内だけ)・入力 20 種(textField〜qrScanner)・fileImport・image・avatar・stat・chart(棒・線・扇)・progressBar・gauge・calendar(今月の表)。
- 記録を入れて開く: `./gradlew :desktop:run --args="kakeibo /path/records.json stats"`(2 つ目は `{"<entity>": [{"id", "data"}]}`、3 つ目は最初の画面)。
- 入力: 日付・時刻は文字で入れて「今日」「‹ ›」で動かす。写真・動画はファイルを選ぶ・落とす・Ctrl+V、録音はファイルを選ぶ(撮影・録音・位置はこのパソコンでは使えない旨の枠)。手書きはマウス・ペンで描いて PNG に。QR は画像ファイルかクリップボードの画像から読む(zxing)。
- 遷移: push はウィンドウの中で積み、sheet はウィンドウ内のダイアログ、fullScreen はウィンドウ全体に重ねる。タブは下に並べる。confirmMessage は確認ダイアログ。規則は Android と同じ `NavigationResolver`・`FormDraft`(`android/renderer` のソースをビルド時に取り込む。`android/` は変えない)。
- アクション: 保存・削除・resetForm はメモリの中のモック。openUrl はブラウザ、addToCalendar は .ics を書き出して既定のカレンダーで開く、export は CSV か PDF(format)をダウンロードに書き出し、表を TSV でクリップボードにも入れる、scheduleNotification はトレイの通知(起動中だけ)、lookupBook はモックの本(`TAILORIT_API_BASE` を付けると `POST /v1/lookup/book`)。openVoice・startActivity・endActivity・follow は「使えない」旨を出す。
- サインイン・通信はまだ無い。
- 言語: OS の言語が日本語なら日本語、それ以外は英語。`TAILORIT_LANG=en`(または `-Dtailorit.lang=en`)で替えられる。文言は Android の `strings*.xml` をビルド時にそのまま同梱し、同じキーで引く(引けなければ英語 → 日本語)。
- 明暗: OS のダークモードに従う(Windows はレジストリ、Linux は freedesktop の color-scheme か GNOME の設定、起動中の切り替えも 5 秒ごとに追う)。`TAILORIT_THEME=light / dark` で固定できる。
- 読み上げ: 見出し・タブ(選択中)・ダイアログの名前・知らせ(live region)・絵で描く部品(グラフ・進み具合・メーター・星・画像)の説明を付けている。Windows は Java Access Bridge を通す(`jabswitch -enable` が要る)。Linux の Orca は JDK に橋渡しが無く、まだ読めない。
- ファイル: `fileImport` で CSV(取り込みの確認を出す)・テキスト・PDF(本文)を取り込む。
- ドラッグ & ドロップ: 画面に写真・動画・CSV・テキスト・PDF を落とすと、photoField / videoField / fileImport / list に振り分ける。写真は外へ引き出せる。
- クリップボード: 一覧の「表をコピー」で表計算に貼れる TSV、「表を貼り付け」で表計算の行を取り込む。画像は Ctrl+V で photoField へ。
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
| `services/` | `android/services` の純 Kotlin の部分(API クライアント・記録の保存・送信待ち・同意)を同じソースのまま取り込む。取り込むファイルは `services/build.gradle.kts` で名指し。宣言にだけ出る Android の型は `services/src/shim` の置き換え |
| `app/src/main/kotlin/.../desktop/net/` | 接続先の設定(既定はモック)・`X-Client-Platform: windows / linux / macos`・User-Agent |
| `app/src/main/kotlin/.../desktop/store/` | 端末の保存。`%APPDATA%\TailorIt`・`$XDG_DATA_HOME/tailorit`(無ければ `~/.local/share/tailorit`)・`~/Library/Application Support/TailorIt` に設定・同意・記録・送信待ち・DSL を JSON で置く。サインアウトで消す |
| `app/src/main/kotlin/.../desktop/io/` | 取り込み・書き出し・ドラッグ & ドロップ・クリップボード(PDF は Apache PDFBox) |
| `app/src/main/kotlin/.../desktop/i18n/`・`app/src/main/resources/i18n/desktop/` | 文言。Android の `renderer`・`app` の `values`(日本語)・`values-en`(英語)をビルド時に `i18n/` へ同梱して読む。デスクトップだけの文言は同じ形の XML(キーは `desktop_`) |
| `app/src/main/kotlin/.../desktop/theme/`・`A11y.kt` | OS に従う明暗・読み上げの説明 |
| `app/src/test/kotlin/.../desktop/DesktopConformanceTest.kt` | 全 OS 共通の適合テストのうち、今比べられる部分(画面・入力の値とラベル・操作の steps) |
