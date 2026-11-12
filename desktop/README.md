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

Firebase Auth の REST クライアント(`auth/FirebaseAuthRestClient.kt`・まだ画面にはつながない)の API キーは `TAILORIT_FIREBASE_API_KEY` から読む(Firebase コンソールのプロジェクト設定 > 全般の「ウェブ API キー」。リポジトリ・ファイルには書かない)。`TAILORIT_IDENTITY_TOOLKIT_URL`・`TAILORIT_SECURE_TOKEN_URL` で接続先を差し替えられる(Auth エミュレーターなど)。

## 今できること

- `docs/dsl/examples` の DSL を 1 本読み、画面を 6 列のグリッド(`area`)に並べる。area の無い部品は下に全幅で積み、フォーム画面は段(band)で積む。広いウィンドウでは幅 720dp で止めて中央に寄せる。最小 360×560、大きさと位置は次の起動で戻す。
- 描ける部品: 並び・見出し・text・label・badge・markdown(書式付き)・button・link・list(LazyColumn・チェック・右クリック / Delete キーで削除)・table(列の幅を引いて変える・見出しで並べ替え)・timeline・gridTable・searchBar・feed / matchList(見出しと案内だけ)・入力 20 種(textField〜qrScanner)・fileImport・image・avatar・icon(`docs/dsl/icon-map.json` をビルド時に同梱し、Material の絵に写す。表に無い名前は細い円)・stat・chart(棒・線・扇)・progressBar・gauge・calendar(今月の表)。
- openUrlButton(`OpenUrlButton.kt`): link と同じ openUrl(許可した scheme だけ)で開く。`app` があればそれを先に使い、`to` の `{{fieldId}}` はフォームの値で埋める(Android の `OpenApp`・`FormTemplate` をビルド時に取り込む)。app の候補は先頭から試し、開けなければ次(slack:// が無ければ Web)。ラベル・アイコンの既定は Android と同じ(`renderer_open_<app>`)。style は primary / secondary(既定)/ plain。知らない app・許可しない URL はボタンを出さず一言だけ出す。
- 出さない部品: hidden の項目につながった部品は、入力に限らず枠ごと描かない。フォームの外の入力(field の無い入力も)は「この入力はフォームの画面で使えます」の一言にする。判定は `DesktopLeaves.state`(iOS の `RenderTree.state` と同じ順: hidden の項目 → 描けない type → 出さない削除・保存ボタン → フォームの外の入力)で、画面・操作・適合テストが同じ関数を見る。
- 記録を入れて開く: `./gradlew :desktop:run --args="kakeibo /path/records.json stats"`(2 つ目は `{"<entity>": [{"id", "data"}]}`、3 つ目は最初の画面)。
- 入力: 日付・時刻は文字で入れて「今日」「‹ ›」で動かす。写真・動画はファイルを選ぶ・落とす・Ctrl+V、録音はファイルを選ぶ(撮影・録音・位置はこのパソコンでは使えない旨の枠)。手書きはマウス・ペンで描いて PNG に。QR は画像ファイルかクリップボードの画像から読む(zxing)。
- 遷移: push はウィンドウの中で積み、sheet はウィンドウ内のダイアログ、fullScreen はウィンドウ全体に重ねる。タブは下に並べる。confirmMessage は確認ダイアログ。規則は Android と同じ `NavigationResolver`・`FormDraft`(`android/renderer` のソースをビルド時に取り込む。`android/` は変えない)。
- アクション: 保存・削除は端末に残す(下の「端末の保存」)。resetForm は下書きを戻す。openUrl はブラウザ、addToCalendar は .ics を OS の一時フォルダ(`tailorit-calendar/`)に書いて既定のカレンダーで開く(ダウンロードには残さない)、export は CSV か PDF(format)をダウンロードに書き出し、表を TSV でクリップボードにも入れる。書き出しは同じ名前のファイルを上書きせず `名前 (2).csv`・`名前 (3).csv`… にし、Windows で使えない名前(CON・PRN・AUX・NUL・COM1・LPT1 など)は頭に `_` を付ける(`_CON.csv`)。PDF は裏で作り、その間は「書き出しています…」、終わったら書いた場所を出す。scheduleNotification はトレイの通知(起動中だけ)、lookupBook はモックの本(`TAILORIT_API_BASE` を付けると `POST /v1/lookup/book`)。openVoice・startActivity・endActivity・follow は「使えない」旨を出す。
- トレイ / メニューバーに常駐する(`Tray.kt`): 開く・すぐ記録(最初の記録できるフォーム画面を DSL と同じ開き方で開く)・音声(まだ使えない旨)・走っている timerDisplay の残り / 経過(メニューとツールチップ)・終了。ウィンドウの × はトレイに隠す(メニューの「ウィンドウを閉じてもトレイに残す」で切り替え・次の起動にも残る)。トレイが無い環境では × で終わる。通知(scheduleNotification・タイマーの終わり)もこのアイコンから出す。Linux は GNOME だと拡張(AppIndicator)が要る。メニューの一番上に、前にあるウィンドウのアプリの集計(`stat`)を最大 3 個「ラベル 値」で出す(並びは initialScreen → タブ順 → 残りで Mac のメニューバーと同じ。値はアプリ内の stat と同じ・記録を読み終えるまでは「—」・stat の無いアプリでは出さない)。メニューの「ログイン時に開く」(既定 off)で、サインインした時に自動で開く: Windows は `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` の `TailorIt`、Linux は `$XDG_CONFIG_HOME/autostart/tailorit.desktop`(無ければ `~/.config/autostart`)に書く・消す。配布した形(msi・deb など)で入れた時だけ出し、`gradlew run` と macOS(Mac 版が受け持つ)では出さない。
- 複数のウィンドウ(`Windows.kt`): アプリごとに 1 枚。コマンドパレットでアプリを選ぶ・トレイの「新しいウィンドウ」で開く(開いていれば前に出す)。一覧の行をダブルクリック / 右クリックの「別のウィンドウで開く」、またはパレットの「この記録を別のウィンドウで開く」で記録の詳細を別の 1 枚に(同じ記録は 1 枚)。遷移・下書き・ショートカット・Ctrl+F はウィンドウごと、記録はアプリごとに共有するので、片方で保存すると他方の一覧にすぐ出る。同じ記録を 2 枚で直した時は後に保存した方が勝ち、上書きしたことを知らせる。詳細のウィンドウは保存・閉じる・Esc で閉じる。× は他にウィンドウがあればその 1 枚だけ閉じ、最後の 1 枚はトレイに隠す。大きさ・位置はアプリ / 詳細の種類ごとに覚え、2 枚目からは少しずらして画面の中に収める。
- 端末の保存(`store/SessionPersistence.kt`): 記録は端末の置き場(`store/`)に書き、再起動しても残る。最後に開いたアプリから開く。既定は通信しない。`TAILORIT_API=stg` と `TAILORIT_DEV_TOKEN` が両方ある時だけ、保存・削除を送信待ちに積んで stg へ送り、起動時に前回送れなかった分を送り直す(送り待ちがあれば画面の下に件数)。記録の JSON を引数で渡した時は端末に残さない。起動時は端末の記録を裏で読み、読み終えるまでウィンドウにスケルトン(形だけの画面)を出す。サインアウトの画面(#1749)ができるまでは、コマンドパレットの「この端末のデータを消す」で確認のあと開いている全アプリの記録・送り待ち・設定を消す(`AppHub.wipeLocalData()`)。
- 起動は 1 つだけ: 同じ保存先ではアプリを 1 つしか起動しない。もう一度起動すると、起動中の方が前に出る(トレイに隠れていても出す。`--args=kakeibo` のようにアプリを渡すとそのアプリを開く)。起動中の方に渡せない時は「Tailor It はもう起動しています」と出して終わる。保存先を分ければ同時に起動できる(`-Dtailorit.dataDir=<フォルダ>`。`gradlew run` では `JAVA_TOOL_OPTIONS=-Dtailorit.dataDir=/tmp/tailorit-b ./gradlew :desktop:run` のように渡す)。記録の JSON を渡して開く時(上の「記録を入れて開く」)は端末に残さないので、この制限は無く何本でも起動できる。
- サインインはまだ無い。
- 言語: OS の言語が日本語なら日本語、それ以外は英語。`TAILORIT_LANG=en`(または `-Dtailorit.lang=en`)で替えられる。文言は Android の `strings*.xml` をビルド時にそのまま同梱し、同じキーで引く(引けなければ英語 → 日本語)。
- 遊び・データの部品の文言(精算・トーナメント・カード・QR・表の集計の見出し・「全員」「参加者」)も Android と同じキーで引き、英語の時は英語で出す。Android に無い文言だけ `desktop_` のキーで足す。適合テストの描画の木の値(精算の題・回戦の名前)は Android と同じく日本語のまま。画面の文字の直書きは `HardcodedJapaneseTest` が見つけ、残してよい物は理由付きで一覧にする。
- 明暗: OS のダークモードに従う(Windows はレジストリ、Linux は freedesktop の color-scheme か GNOME の設定、起動中の切り替えも 5 秒ごとに追う)。`TAILORIT_THEME=light / dark` で固定できる。
- 電池(`power/`): 電池の残り・充電の状態・省電力を読む口(iOS の `DevicePowerSource` と同じ形)。Linux は `/sys/class/power_supply`(マウスなどの電池は数えない・複数の電池は容量で重みを付けた平均)と `/sys/firmware/acpi/platform_profile` を 60 秒ごとに読み、変わった時だけ流す。Windows と macOS は今は「不明」。`METAMON_MOCK=1` の時は `METAMON_BATTERY_LEVEL`・`METAMON_BATTERY_STATE`・`METAMON_LOW_POWER` の偽物。式と画面にはまだ出さず、値は送らない。
- 読み上げ: 見出し・タブ(選択中)・ダイアログの名前・知らせ(live region)・絵で描く部品(グラフ・進み具合・メーター・星・画像)の説明を付けている。Windows はナレーターで読む(Java Access Bridge を通すので `jabswitch -enable` が要る)。Linux の読み上げ(Orca)は対象外にする。Compose Desktop が Linux の読み上げに対応していないため([JetBrains のドキュメント](https://kotlinlang.org/docs/multiplatform/compose-desktop-accessibility.html))。
- ファイル: `fileImport` で CSV(取り込みの確認を出す)・テキスト・PDF(本文)を取り込む。大きすぎるファイルは読む前に断る(CSV・テキストは 5 MB、PDF は 30 MB まで。一覧に落とした CSV も同じ)。
- ドラッグ & ドロップ: 画面に写真・動画・CSV・テキスト・PDF を落とすと、photoField / videoField / fileImport / list に振り分ける。外へ引き出すこともできる(`io/DragOut.kt`): list・table の行は、選んだ行を引くと選んだ行全部、選んでいない行ならその 1 行を、文字はコピーと同じ TSV(見出し付き)、ファイルを受けるアプリ(エクスプローラーなど)には `<一覧名>.csv` で渡す。写真・image・一覧の行のサムネイルは PNG のファイルで渡す。ファイルは落とした時に OS の一時フォルダ(`tailorit-drag/`)へ書き、アプリを閉じると消える(落ちて残った分は次の起動で、1 日より古い物を消す)。
- 写真を大きく見る(`PhotoPreview.kt`): 記録・フォームの写真(data URL と手元の画像ファイル)を押す(フォーカスして Space でも)か、何も選んでいない時に Space で、ウィンドウ内のダイアログに大きく出す。コマンドパレットの「写真を大きく見る」からも開ける。← → で送り(端で止まる)、Esc / Space / 暗幕のクリックで閉じる。並びは DSL の項目順で、隠す項目・算出項目・https の写真は出さない。開いている間は下の画面に Tab で入らず、画面が移ったら閉じる。読み上げの名前は「写真のプレビュー: <項目名>」。「既定のアプリで開く」は PNG にしてこの人だけの一時フォルダ(Windows は `%LOCALAPPDATA%\TailorIt\tmp\open`、macOS は `~/Library/Caches/TailorIt/tmp/open`、Linux は `$XDG_RUNTIME_DIR/tailorit/open`、無ければ端末の保存先の `tmp/open`)へ書き、OS のビューアに渡す(元のファイルの場所は渡さない)。書いた物はアプリを閉じる時・「この端末のデータを消す」で消し、残った分は次の起動で 1 日より古い物を消す。
- クリップボード: 一覧の「表をコピー」で表計算に貼れる TSV、「表を貼り付け」で表計算の行を取り込む。list・table の行はクリックで選び、Shift+クリックで範囲、Ctrl+クリック(macOS は ⌘)で足し引き、Ctrl+A で全部。選んでいる時は Ctrl+C と「表をコピー」が選んだ行だけを同じ TSV(見出し付き)でコピーする(table は見えている列)。詳細へ移る行(onItemTap)は修飾キー無しのクリックで今までどおり開く(`RowSelection.kt`)。画像は Ctrl+V で photoField へ。
- 印刷: Ctrl+P(または見出しの「印刷」)で一番上の画面をプレビューし、OS の印刷ダイアログから A4 縦で印刷する。頁は export の PDF と同じ(`io/PdfIo.pages`)。一覧・table・gridTable・chart は表ごとに頁を改めて全行を複数頁に割り、stat などの値とフォームの入力は「項目 / 値」の表にする(`Print.kt`)。プレビューの頁は裏で描き、描き終えるまでは回る印を出して「印刷…」を押せない。画面の無い DSL では Ctrl+P は効かない。
- キーボード: Ctrl+K(または Ctrl+Shift+P)でコマンドパレット(画面・アクション・同梱のアプリを名前で選ぶ)、Ctrl+N 新規・Ctrl+S 保存・Ctrl+F 検索・Delete 削除(確認あり)・Esc 閉じる / 戻る・F1(または Ctrl+/)で一覧。macOS は Ctrl の代わりに ⌘、削除は ⌘⌫。Tab はグリッドの行 → 列の順(`Keyboard.kt`・`CommandPalette.kt`)。
- ライセンス: F1 の一覧の「サードパーティのライセンス」かコマンドパレットで、このアプリ・依存・同梱物・Java 実行環境のライセンスを出す(`Licenses.kt`)。一覧はビルド時に `app/third-party-licenses.gradle.kts` が runtimeClasspath の POM と jar の NOTICE から作る(本文の正本は `android/scripts/license-texts`)。Maven を通らない同梱物は `app/licenses/vendored/*.properties` に 1 件 1 ファイルで足す。Java 実行環境の分は起動中の実行環境の `legal/` から読む。
- まだ描けない部品は「デスクトップ版ではまだ使えない部品」の枠で残す。

## 構成

| 場所 | 中身 |
|---|---|
| `settings.gradle.kts` | `:dsl`(`../android/dsl`)と `:desktop`(`app/`)。版は `../android/gradle/libs.versions.toml` を共有 |
| `app/src/main/kotlin/.../desktop/` | ウィンドウ・画面の描画・モックのレコード |
| `app/src/main/kotlin/.../desktop/Session.kt` | 画面の状態と操作(遷移・下書き・保存・削除)。画面と適合テストが同じ物を使う |
| `app/src/main/kotlin/.../desktop/InputLeaves.kt`・`Actions.kt` | 入力の部品 20 種・OS に頼むアクション |
| `app/src/main/kotlin/.../desktop/DesktopSupport.kt` | 描ける type の一覧と部品の state(`DesktopLeaves.state`: rendered・hidden・needsForm・unavailable・unknown)。画面・操作・適合テストが同じ物を使う |
| `services/` | `android/services` の純 Kotlin の部分(API クライアント・記録の保存・送信待ち・同意)を同じソースのまま取り込む。取り込むファイルは `services/build.gradle.kts` で名指し。宣言にだけ出る Android の型は `services/src/shim` の置き換え |
| `app/src/main/kotlin/.../desktop/net/` | 接続先の設定(既定はモック)・`X-Client-Platform: windows / linux / macos`・User-Agent |
| `app/src/main/kotlin/.../desktop/store/` | 端末の保存。`%APPDATA%\TailorIt`・`$XDG_DATA_HOME/tailorit`(無ければ `~/.local/share/tailorit`)・`~/Library/Application Support/TailorIt` に設定・同意・記録・送信待ち・DSL を JSON で置く。サインアウトで消す。書き込みは同じフォルダの一時ファイル(毎回別の名前の `*.tmp`)に書いてから置き換え、落ちて残った `*.tmp` は開く時・全部消す時に消す |
| `app/src/main/kotlin/.../desktop/SingleInstance.kt` | 起動は保存先ごとに 1 つ。保存先の `.lock` をロックし(プロセスが終われば OS が外す)、127.0.0.1 の空いた口と合言葉を `.instance` に書いて 2 つ目の起動を待つ。2 つ目は合言葉と引数を送って前に出させて終わる。ロックを作れない時(保存先に書けないなど)は起動を止めない。記録の JSON を渡した起動はロックを取らない |
| `app/src/main/kotlin/.../desktop/io/` | 取り込み・書き出し・ドラッグ & ドロップ・クリップボード(PDF は Apache PDFBox)。書き出しの名前は `io/FileNames.kt`(上書きしない・Windows の予約名)。外へ渡すファイルは OS の一時フォルダの `tailorit-drag/`(引き出し)・`tailorit-calendar/`(.ics)に置き、起動時に 1 日より古い物を消す |
| `app/src/main/kotlin/.../desktop/PhotoPreview.kt` | 写真を大きく見るダイアログ・見せる写真の集め方(Mac の `MacQuickLook.sources` と同じ規則)・「既定のアプリで開く」の一時ファイル |
| `app/src/main/kotlin/.../desktop/i18n/`・`app/src/main/resources/i18n/desktop/` | 文言。Android の `renderer`・`app` の `values`(日本語)・`values-en`(英語)をビルド時に `i18n/` へ同梱して読む。デスクトップだけの文言は同じ形の XML(キーは `desktop_`) |
| `app/src/main/kotlin/.../desktop/theme/`・`A11y.kt` | OS に従う明暗・読み上げの説明 |
| `app/src/main/kotlin/.../desktop/power/` | 電池の残り・充電・省電力を読む口(Linux は sysfs、Windows・macOS は今は不明、`METAMON_MOCK=1` で偽物)。まだどこからも使っていない |
| `app/src/test/kotlin/.../desktop/DesktopConformanceTest.kt` | 全 OS 共通の適合テストのうち、今比べられる部分(画面・部品の state(hidden・needsForm を含む)・入力の値とラベル・操作の steps・openUrlButton の label / url・stat / gauge / progressBar / chart の DSL の値と計算した値)。違ってよい所(描けない type・まだ比べない props)は `ConformanceAllowlist.kt` に理由付きで書き、無い違いは落ちる |
