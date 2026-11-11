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

## 今できること

- `docs/dsl/examples` の DSL を 1 本読み、画面を積んで描く(並び・見出し・text・label・button・link・list・textField などの入力・toggle・picker・stepper)。
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
| `app/src/main/kotlin/.../desktop/io/` | 取り込み・書き出し・ドラッグ & ドロップ・クリップボード(PDF は Apache PDFBox) |
| `app/src/test/kotlin/.../desktop/DesktopConformanceTest.kt` | 全 OS 共通の適合テストのうち、今比べられる部分 |
