package jp.kz2601.metamon.language

import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * 画面に出す日本語を Kotlin に直書きしていないか(#843・#1062)。:app と :renderer の main の .kt の文字列リテラルを見る。
 * コメント・ログ・開発者向けの例外の文は見ない。残してよいものは [allowed] に理由つきで並べる。
 */
class HardcodedJapaneseTest {
    private val androidRoot: File = File(System.getProperty("user.dir")).absoluteFile.parentFile

    private val japanese = Regex("""[\p{IsHiragana}\p{IsKatakana}\p{IsHan}]""")

    /** ログ・開発者向けの失敗の文(人には出さない)。 */
    private val developerOnly = Regex("""\bLog\.[a-z]\(|\bthrow\b|\brequire\(|\bcheck\(|\berror\(""")

    /** 残してよい直書き(パス → 文字列)。 */
    private val allowed: Map<String, Set<String>> = mapOf(
        // 言語の名前はその言語で書く
        "app/src/main/kotlin/jp/kz2601/metamon/language/AppLanguage.kt" to setOf("日本語"),
        // 話した言葉の聞き取りの辞書(表示しない)
        "app/src/main/kotlin/jp/kz2601/metamon/voice/stage/BuildChoreography.kt" to setOf(
            "グラフ", "推移", "チャート", "チェック", "やること", "完了", "一覧", "リスト", "履歴", "記録", "入力", "メモ", "日記",
            "合計", "集計", "平均", "残り", "回数", "カレンダー", "予定", "スケジュール", "地図", "場所", "散歩", "ルート", "写真", "画像", "カメラ",
        ),
        "app/src/main/kotlin/jp/kz2601/metamon/voice/stage/GeneratingStage.kt" to setOf(
            "別のアプリ", "別アプリ", "新しいアプリ", "新規アプリ", "違うアプリ", "次のアプリ", "べつのアプリ", "あたらしいアプリ",
        ),
        // 発話の意図の辞書(#884・iOS の VoiceIntent と同じ語。表示しない)
        "app/src/main/kotlin/jp/kz2601/metamon/voice/VoiceIntent.kt" to setOf(
            "元に戻", "もとに戻", "取り消", "とりけし", "アンドゥ", "あんどぅ", "ひとつ前", "一つ前", "さっきの状態", "前の状態",
            "戻して", "戻す", "もどして", "もどす",
            "別のアプリ", "別アプリ", "新しいアプリ", "新規アプリ", "違うアプリ", "次のアプリ", "べつのアプリ", "あたらしいアプリ",
            "アプリ", "変身", "へんしん", "一覧", "いちらん", "リスト", "の",
            "に切り替え", "に切替", "にきりかえ", "を切り替え", "を切替", "をきりかえ", "へ切り替え", "へ切替",
            "を開い", "をひらい", "を開け", "をひらけ", "に変身", "にへんしん", "のアプリ", "の方", "のほう",
            "別", "別の", "べつの", "他", "他の", "ほか", "ほかの", "違う", "ちがう", "次", "次の", "つぎの", "前", "前の", "まえの",
            "どれか", "何か", "なにか", "新しい", "あたらしい", "新規", "新しく",
            "じゃあ", "じゃ", "では", "次は", "つぎは", "今度は", "こんどは", "やっぱり", "やっぱ",
        ),
        "app/src/main/kotlin/jp/kz2601/metamon/voice/stage/PositionHint.kt" to setOf(
            "右下", "左下", "右上", "左上", "一番下", "一番上", "下部", "上部", "中央", "右", "左",
        ),
        "renderer/src/main/kotlin/jp/kz2601/metamon/renderer/form/FormInputs.kt" to setOf("本文", "日記", "メモ", "詳細", "説明", "感想"),
        // 日付の書式(言語で分けている)
        "app/src/main/kotlin/jp/kz2601/metamon/voice/VoiceScreen.kt" to setOf("M月d日 H:mm"),
        "renderer/src/main/kotlin/jp/kz2601/metamon/renderer/components/lists/CalendarLeaf.kt" to setOf("yyyy年M月", "M月d日(E)"),
        // 画面は strings から渡す。既定値はテスト・プレビュー用
        "app/src/main/kotlin/jp/kz2601/metamon/voice/VoiceViewModel.kt" to setOf("このまま作って", "(これまでに反映した変更)"),
        // AI へ送る指示(人に出さない)
        "renderer/src/main/kotlin/jp/kz2601/metamon/renderer/components/ai/AiRequests.kt" to setOf(
            "\\n\\n候補: ",
            "入力と同じ言語で、結果だけを返してください。前置き・引用符・説明は不要です。",
            "あなたは要約アシスタントです。与えられた文章の要点を短くまとめてください。\$common",
            "あなたは分類アシスタントです。与えられた文章に最も当てはまる候補を 1 つだけ選んでください。",
            "あなたは文章の清書アシスタントです。意味を変えずに、読みやすく自然な文章に書き直してください。\$common",
            "あなたは情報抽出アシスタントです。与えられた文章から重要な点や値を短い箇条書きで抜き出してください。\$common",
            "あなたは前向きなアシスタントです。与えられた内容を踏まえて、短い一言や提案を 1 つだけ返してください。\$common",
            "下の材料",
        ),
        // AI へ送る材料の「外部データ」の節(人に出さない。言語は端末でなく request.locale で分ける。iOS の AIFeedInput と同じ文)
        "renderer/src/main/kotlin/jp/kz2601/metamon/renderer/components/ai/AiFeedInput.kt" to setOf(
            "気圧が大きく下がる", "最高 \${f(it)}°", "最低 \${f(it)}°", "降水確率 \$it%",
            "外部データ: なし(添えられるデータがありません。外部のデータを推測・作成しないでください)",
            "外部データ(アプリが表示している公開データ):", "- (取得できませんでした)", "- (今は表示するものがありません)",
        ),
        // 適合テストの描画の木の値(iOS の日本語と突き合わせる。画面は strings で出す)
        "renderer/src/main/kotlin/jp/kz2601/metamon/renderer/conformance/PlayNodes.kt" to setOf("精算"),
        "renderer/src/main/kotlin/jp/kz2601/metamon/renderer/components/Containers.kt" to setOf("詳細"),
        "renderer/src/main/kotlin/jp/kz2601/metamon/renderer/components/play/BracketLeaf.kt" to setOf("決勝", "準決勝", "準々決勝", "\$round 回戦"),
        "renderer/src/main/kotlin/jp/kz2601/metamon/renderer/components/lists/ValueLabels.kt" to setOf(
            "まだありません", "件数", "%sの合計", "%sの平均", "%sの最小", "%sの最大", "件", "データがありません",
            "%d 項目。%s の %s", "%d 項目。最大は %s の %s、最小は %s の %s", "全員", "参加者",
        ),
        // METAMON_MOCK のすれ違いの相手が送ってくる記録(相手のデータ。iOS の MockPassbyTransport と同じ値)
        "app/src/main/kotlin/jp/kz2601/metamon/nearby/MockPassbyTransport.kt" to setOf("テスト花子", "よろしくお願いします"),
        // まだ移していない(サインインの失敗。OS の文をそのまま出す所と一緒に直す)
        "app/src/main/kotlin/jp/kz2601/metamon/auth/AuthSession.kt" to setOf("サインインできませんでした。もう一度お試しください"),
    )

    @Test
    fun noHardcodedJapaneseInMainSources() {
        val found = mutableListOf<String>()
        for (module in listOf("app", "renderer")) {
            File(androidRoot, "$module/src/main").walkTopDown().filter { it.extension == "kt" }.sortedBy { it.path }.forEach { file ->
                val path = file.relativeTo(androidRoot).invariantSeparatorsPath
                val lines = file.readLines()
                for ((line, literal) in literals(file.readText())) {
                    if (!japanese.containsMatchIn(literal)) continue
                    if (developerOnly.containsMatchIn(lines[line - 1])) continue
                    if (literal in allowed[path].orEmpty()) continue
                    found += "$path:$line \"$literal\""
                }
            }
        }
        assertTrue("日本語の直書きがあります。strings_<機能>.xml(日英)へ移してください:\n" + found.joinToString("\n"), found.isEmpty())
    }

    @Test
    fun scannerSkipsCommentsAndReadsTemplates() {
        val src = "// \"コメント\"\nval a = \"image/*\" /* \"ブロック\" */\nval b = \"\${x.joinToString(\"・\")} を入力\"\n"
        val got = literals(src).map { it.second }
        assertTrue(got.toString(), got == listOf("image/*", "\${x.joinToString(\"・\")} を入力"))
    }

    /** 文字列リテラル(行, 中身)。コメントは飛ばす。テンプレートの中の文字列は外側の中身に含める。 */
    private fun literals(src: String): List<Pair<Int, String>> {
        val out = mutableListOf<Pair<Int, String>>()
        var i = 0
        var line = 1
        fun advance(n: Int) {
            repeat(n) { if (i < src.length && src[i] == '\n') line++; i++ }
        }
        while (i < src.length) {
            when {
                src.startsWith("//", i) -> while (i < src.length && src[i] != '\n') advance(1)
                src.startsWith("/*", i) -> {
                    val end = src.indexOf("*/", i + 2).let { if (it < 0) src.length else it + 2 }
                    advance(end - i)
                }
                src.startsWith("\"\"\"", i) -> {
                    val start = line
                    val end = src.indexOf("\"\"\"", i + 3).let { if (it < 0) src.length else it }
                    out += start to src.substring(i + 3, end)
                    advance(end + 3 - i)
                }
                src[i] == '"' -> {
                    val start = line
                    advance(1)
                    val sb = StringBuilder()
                    while (i < src.length && src[i] != '"' && src[i] != '\n') {
                        if (src[i] == '\\') {
                            sb.append(src, i, minOf(i + 2, src.length)); advance(2)
                        } else if (src.startsWith("\${", i)) {
                            sb.append("\${"); advance(2)
                            var depth = 1
                            while (depth > 0 && i < src.length) {
                                val c = src[i]
                                if (c == '{') depth++ else if (c == '}') depth--
                                if (c == '"') {
                                    sb.append(c); advance(1)
                                    while (i < src.length && src[i] != '"') { sb.append(src[i]); advance(1) }
                                }
                                sb.append(src[i]); advance(1)
                            }
                        } else {
                            sb.append(src[i]); advance(1)
                        }
                    }
                    advance(1)
                    out += start to sb.toString()
                }
                src[i] == '\'' && i + 2 < src.length && (src[i + 1] == '"' || src[i + 1] == '\\') -> {
                    // '"' や '\'' などの文字リテラル
                    val end = src.indexOf('\'', i + 2)
                    advance(if (end < 0) 1 else end + 1 - i)
                }
                else -> advance(1)
            }
        }
        return out
    }
}
