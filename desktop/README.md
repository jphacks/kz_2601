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
- 描ける部品: 並び・見出し・text・label・badge・markdown(書式付き)・button・link・list・入力(textField など・toggle・picker・stepper・rating)・image・avatar・stat・chart(棒・線・扇)・progressBar・gauge・calendar(今月の表)・photoField・videoField・fileImport(グリッドのセルの中に描く)。
- 記録を入れて開く: `./gradlew :desktop:run --args="kakeibo /path/records.json stats"`(2 つ目は `{"<entity>": [{"id", "data"}]}`、3 つ目は最初の画面)。
- 保存・削除・戻るはメモリの中のモック。サインイン・通信はまだ無い。
- ファイル: `fileImport` で CSV(取り込みの確認を出す)・テキスト・PDF(本文)を取り込む。`export` で CSV(iOS と同じ形)・PDF を保存のダイアログへ書き出す。
- ドラッグ & ドロップ: 画面に写真・動画・CSV・テキスト・PDF を落とすと、photoField / videoField / fileImport / list に振り分ける。写真は外へ引き出せる。
- クリップボード: 一覧の「表をコピー」で表計算に貼れる TSV、「表を貼り付け」で表計算の行を取り込む。画像は Ctrl+V で photoField へ。
- まだ描けない部品は「デスクトップ版ではまだ使えない部品」の枠で残す。

## 構成

| 場所 | 中身 |
|---|---|
| `settings.gradle.kts` | `:dsl`(`../android/dsl`)と `:desktop`(`app/`)。版は `../android/gradle/libs.versions.toml` を共有 |
| `app/src/main/kotlin/.../desktop/` | ウィンドウ・画面の描画・モックのレコード |
| `services/` | `android/services` の純 Kotlin の部分(API クライアント・記録の保存・送信待ち・同意)を同じソースのまま取り込む。取り込むファイルは `services/build.gradle.kts` で名指し。宣言にだけ出る Android の型は `services/src/shim` の置き換え |
| `app/src/main/kotlin/.../desktop/net/` | 接続先の設定(既定はモック)・`X-Client-Platform: windows / linux / macos`・User-Agent |
| `app/src/main/kotlin/.../desktop/store/` | 端末の保存。`%APPDATA%\TailorIt`・`$XDG_DATA_HOME/tailorit`(無ければ `~/.local/share/tailorit`)・`~/Library/Application Support/TailorIt` に設定・同意・記録・送信待ち・DSL を JSON で置く。サインアウトで消す |
| `app/src/main/kotlin/.../desktop/io/` | 取り込み・書き出し・ドラッグ & ドロップ・クリップボード(PDF は Apache PDFBox) |
| `app/src/test/kotlin/.../desktop/DesktopConformanceTest.kt` | 全 OS 共通の適合テストのうち、今比べられる部分 |
