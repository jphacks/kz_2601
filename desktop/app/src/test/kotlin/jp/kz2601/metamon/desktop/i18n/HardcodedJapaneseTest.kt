package jp.kz2601.metamon.desktop.i18n

import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * 画面に出す日本語を Kotlin に直書きしていないか(#1759・Android の HardcodedJapaneseTest と同じ形)。main の .kt の文字列リテラルを見る。
 * コメント・ログ・開発者向けの例外の文は見ない。残してよいものは [allowed] に理由つきで並べる。
 */
class HardcodedJapaneseTest {
    private val src = File("src/main/kotlin")

    private val japanese = Regex("""[\p{IsHiragana}\p{IsKatakana}\p{IsHan}]""")

    /** ログ・開発者向けの失敗の文(人には出さない)。 */
    private val developerOnly = Regex("""\bSystem\.err\.println\(|\bthrow\b|\brequire\(|\bcheck\(|\berror\(""")

    private val pkg = "jp/kz2601/metamon/desktop"

    /** 残してよい直書き(src/main/kotlin からのパス → 文字列)。 */
    private val allowed: Map<String, Set<String>> = mapOf(
        // 適合テストの描画の木の値(iOS の日本語と突き合わせる。Android の PlayNodes・BracketRules と同じ。画面は strings で出す)
        "$pkg/PlayLeaves.kt" to setOf("精算", "決勝", "準決勝", "準々決勝", "\$round 回戦"),
        "$pkg/LeafValues.kt" to setOf("件"),
        // 追加のキーの聞き取りの辞書(表示しない)
        "$pkg/Keyboard.kt" to setOf("追加", "新規", "作る"),
        // METAMON_MOCK の本の検索の見本(相手のデータ)
        "$pkg/Actions.kt" to setOf("見本の本", "見本 太郎", "見本書房", "モック"),
        // 既定値はテスト用。画面は strings から渡す
        "$pkg/io/PdfIo.kt" to setOf("記録がありません"),
        // まだ移していない(取り込みの確認の文。取り込みの文言と一緒に直す)
        "$pkg/io/RecordIo.kt" to setOf(
            "\${plan.rows.size} 件を追加します", "当てた列: ", "使わない列: ", "読めない行 \${plan.failures.size} 件(飛ばします)",
            "・\${it.line} 行目 \${it.fieldName}: \${failureText(it.kind)}\${if (it.cell.isEmpty()) \"\" else \"「\${it.cell}」\"}",
            "上限を超える \${plan.overLimit} 件は入りません", "読めない値", "必須が空", "長すぎる",
        ),
    )

    @Test
    fun noHardcodedJapaneseInMainSources() {
        assertTrue("ソースが無い: ${src.absolutePath}", src.isDirectory)
        val found = mutableListOf<String>()
        src.walkTopDown().filter { it.extension == "kt" }.sortedBy { it.path }.forEach { file ->
            val path = file.relativeTo(src).invariantSeparatorsPath
            val lines = file.readLines()
            for ((line, literal) in literals(file.readText())) {
                if (!japanese.containsMatchIn(literal)) continue
                if (developerOnly.containsMatchIn(lines[line - 1])) continue
                if (literal in allowed[path].orEmpty()) continue
                found += "$path:$line \"$literal\""
            }
        }
        assertTrue("日本語の直書きがあります。strings_desktop.xml(日英)か Android のキーへ移してください:\n" + found.joinToString("\n"), found.isEmpty())
    }

    @Test
    fun scannerSkipsCommentsAndReadsTemplates() {
        val text = "// \"コメント\"\nval a = \"image/*\" /* \"ブロック\" */\nval b = \"\${x.joinToString(\"・\")} を入力\"\n"
        val got = literals(text).map { it.second }
        assertTrue(got.toString(), got == listOf("image/*", "\${x.joinToString(\"・\")} を入力"))
    }

    /** 文字列リテラル(行, 中身)。コメントは飛ばす。テンプレートの中の文字列は外側の中身に含める(Android と同じ読み方)。 */
    private fun literals(text: String): List<Pair<Int, String>> {
        val out = mutableListOf<Pair<Int, String>>()
        var i = 0
        var line = 1
        fun advance(n: Int) {
            repeat(n) { if (i < text.length && text[i] == '\n') line++; i++ }
        }
        while (i < text.length) {
            when {
                text.startsWith("//", i) -> while (i < text.length && text[i] != '\n') advance(1)
                text.startsWith("/*", i) -> {
                    val end = text.indexOf("*/", i + 2).let { if (it < 0) text.length else it + 2 }
                    advance(end - i)
                }
                text.startsWith("\"\"\"", i) -> {
                    val start = line
                    val end = text.indexOf("\"\"\"", i + 3).let { if (it < 0) text.length else it }
                    out += start to text.substring(i + 3, end)
                    advance(end + 3 - i)
                }
                text[i] == '"' -> {
                    val start = line
                    advance(1)
                    val sb = StringBuilder()
                    while (i < text.length && text[i] != '"' && text[i] != '\n') {
                        if (text[i] == '\\') {
                            sb.append(text, i, minOf(i + 2, text.length)); advance(2)
                        } else if (text.startsWith("\${", i)) {
                            sb.append("\${"); advance(2)
                            var depth = 1
                            while (depth > 0 && i < text.length) {
                                val c = text[i]
                                if (c == '{') depth++ else if (c == '}') depth--
                                if (c == '"') {
                                    sb.append(c); advance(1)
                                    while (i < text.length && text[i] != '"') { sb.append(text[i]); advance(1) }
                                }
                                sb.append(text[i]); advance(1)
                            }
                        } else {
                            sb.append(text[i]); advance(1)
                        }
                    }
                    advance(1)
                    out += start to sb.toString()
                }
                text[i] == '\'' && i + 2 < text.length && (text[i + 1] == '"' || text[i + 1] == '\\') -> {
                    val end = text.indexOf('\'', i + 2)
                    advance(if (end < 0) 1 else end + 1 - i)
                }
                else -> advance(1)
            }
        }
        return out
    }
}
