package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** docs/desktop/support-matrix.md の「適合 state」の列が DesktopLeaves.state と同じか(#1746)。 */
class SupportMatrixDocTest {
    private val doc = File(requireNotNull(System.getProperty("metamon.dslDocsDir")) { "metamon.dslDocsDir が未設定(Gradle から実行する)" })
        .resolve("../desktop/support-matrix.md")

    /** Container・Leaf の表の (type, 適合 state)。 */
    private fun rows(): Map<String, String> {
        var inTable = false
        val out = linkedMapOf<String, String>()
        doc.readLines().forEach { line ->
            if (line.startsWith("## ")) inTable = line.startsWith("## Container") || line.startsWith("## Leaf")
            if (!inTable || !line.startsWith("| ")) return@forEach
            val cells = line.trim().trim('|').split('|').map { it.trim() }
            if (cells.size < 3 || cells[0] == "type" || cells[0].startsWith("---")) return@forEach
            out[cells[0]] = cells[2]
        }
        return out
    }

    /** 適合 state は rendered / unavailable の 2 つ(needsForm はフォームが無いだけで描ける)。 */
    private fun desktopState(t: ComponentType): String =
        if (DesktopLeaves.state(Component(type = t)) == "unavailable") "unavailable" else "rendered"

    @Test
    fun supportMatrixMatchesDesktopLeaves() {
        val rows = rows()
        val types = ComponentType.entries.filter { it != ComponentType.UNKNOWN }
        assertEquals("表の行と DSL の type が同じ", types.map { it.wire }.sorted(), rows.keys.sorted())
        val wrong = types.mapNotNull { t ->
            val want = desktopState(t)
            val got = rows.getValue(t.wire)
            "${t.wire}: 表 $got・DesktopLeaves $want".takeIf { got != want }
        }
        assertTrue("食い違う行: $wrong", wrong.isEmpty())
    }

    @Test
    fun unavailableRowsMatchAllowlist() {
        val doc = rows().filterValues { it == "unavailable" }.keys
        val listed = ConformanceAllowlist.unavailableTypes.keys
        assertTrue("allowlist は表の unavailable の中だけ: ${listed - doc}", doc.containsAll(listed))
    }
}
