package jp.kz2601.metamon.desktop

/**
 * 適合テストでデスクトップ版が iOS の期待値(docs/dsl/conformance)と違ってよい所と、その理由(#1740)。
 * ここに無い違いは DesktopConformanceTest が落とす。どの case にも出てこない項目も落とす(描けた・比べたら外す)。
 */
object ConformanceAllowlist {
    /** デスクトップ版で描けない type(expected は rendered・デスクトップは unavailable)→ 理由。 */
    val unavailableTypes: Map<String, String> = mapOf(
        "aiButton" to "AI の呼び出し(/v1/ai)をデスクトップにまだつないでいない",
        "aiChat" to "AI の呼び出し(/v1/ai)をデスクトップにまだつないでいない",
        "chatThread" to "共有レコードの購読(会話の同期)がまだ無い",
        "dashcam" to "カメラの常時録画。パソコンのカメラは使わない",
        "healthStat" to "ヘルスケアのデータ(HealthKit / Health Connect)がパソコンに無い",
        "locationTracker" to "位置情報(GPS)がパソコンに無い",
        "mapView" to "地図の描画(地図の SDK)をまだ入れていない",
        "musicPlayer" to "音楽のライブラリ(MusicKit など)がパソコンに無い",
        "nearbyShare" to "近くの端末との共有(Nearby / MultipeerConnectivity)がパソコンに無い",
        "routeGuide" to "道案内(地図と位置)がパソコンに無い",
        "sensorValue" to "加速度・気圧などのセンサーがパソコンに無い",
        "shareButton" to "OS の共有をまだつないでいない",
        "speakButton" to "読み上げ(音声合成)をまだつないでいない",
        "videoPlayer" to "動画の再生をまだ入れていない",
        "voiceInput" to "音声入力(音声認識)をまだつないでいない",
        "webView" to "Web の埋め込みをまだ入れていない",
    )

    /** DSL に書いた値をそのまま写すだけの props。読み込みは :dsl(iOS・Android と同じ)のテストで見るので、ここではまだ比べない。 */
    internal const val DSL_ECHO = "DSL に書いた値をそのまま写すだけ(読み込みは :dsl のテストで見る)"

    private fun echo(vararg keys: String): Map<String, String> = keys.associateWith { DSL_ECHO }

    /** まだ比べない props(type → key → 理由)。 */
    val propsNotCompared: Map<String, Map<String, String>> = mapOf(
        "audioRecorder" to echo("field"),
        "bracket" to echo("entity"),
        "card" to echo("title"),
        "colorPicker" to echo("field"),
        "datePicker" to echo("field"),
        "disclosure" to echo("expanded", "title"),
        "drawingField" to echo("field"),
        "fileImport" to echo("entity", "label"),
        "emptyState" to echo("message", "title"),
        "flashcards" to echo("entity"),
        "grid" to echo("columns"),
        "image" to echo("field") + ("hasImage" to "画像が有るか(読み込みの結果)をまだ出していない"),
        "link" to mapOf("opens" to "押した先(アプリ内 / ブラウザ)の判定をまだ出していない"),
        "locationButton" to echo("field"),
        "locationPicker" to echo("field"),
        "numberField" to echo("field", "placeholder") + ("unit" to "単位は entity の field から引く。まだ比べない"),
        "photoField" to echo("field"),
        "picker" to echo("field", "style") + mapOf(
            "options" to "選択肢の解決(entity の options・ref)をまだ出していない",
            "input" to "フォームの外の picker(input)はまだ比べない",
            "label" to "フォームの外の picker(input)はラベルをまだ比べない",
            "value" to "フォームの外の picker(input)は値をまだ比べない",
        ),
        "qrScanner" to echo("field"),
        "randomPicker" to echo("entity"),
        "rating" to echo("field", "max"),
        "section" to echo("title"),
        "secureField" to echo("field", "placeholder"),
        "settlement" to echo("entity"),
        "slider" to echo("field", "max", "min", "step"),
        "stepper" to echo("field", "max", "min", "step"),
        "textEditor" to echo("field", "placeholder"),
        "textField" to echo("field", "placeholder"),
        "timePicker" to echo("field"),
        "timerDisplay" to echo("field"),
        "toggle" to echo("field"),
        "transcribeField" to echo("field", "placeholder"),
        "videoField" to echo("field"),
    )
}
