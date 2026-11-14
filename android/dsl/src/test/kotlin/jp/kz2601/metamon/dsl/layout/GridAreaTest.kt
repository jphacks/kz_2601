package jp.kz2601.metamon.dsl.layout

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.screen
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** `area` の解析(iOS の GridAreaTests)。 */
class GridAreaTest {
    private val examplesDir = File(
        requireNotNull(System.getProperty("metamon.dslExamplesDir")) { "metamon.dslExamplesDir が未設定(Gradle から実行する)" },
    )

    private fun example(name: String) = AppDSL.decode(File(examplesDir, "$name.json").readText())

    @Test
    fun parsing() {
        assertEquals(listOf(0, 1, 2, 3), GridArea.parse("A1:C3")?.let { listOf(it.col1, it.row1, it.col2, it.row2) })
        assertEquals(3, GridArea.parse("A1:C3")?.colSpan)
        assertEquals(3, GridArea.parse("A1:C3")?.rowSpan)
        assertNull(GridArea.parse("G1:G2"))
        assertNull(GridArea.parse("A0:B1"))
        assertNull(GridArea.parse("C3:A1"))
        assertNull(GridArea.parse("A1"))
    }

    /** 巨大な行は上限(backend の MaxGridRows と同じ 48)に丸める。桁あふれは不正扱い。 */
    @Test
    fun rowsClampToMax() {
        assertEquals(48, GridArea.MAX_ROWS)
        assertEquals(listOf(1, 48), GridArea.parse("A1:F100000000")?.let { listOf(it.row1, it.row2) })
        assertEquals(listOf(48, 48), GridArea.parse("A60:B70")?.let { listOf(it.row1, it.row2) })
        assertEquals(48, GridArea.parse("A1:F48")?.row2)
        assertNull(GridArea.parse("A1:F99999999999999999999999"))
    }

    @Test
    fun overlap() {
        assertTrue(GridArea.parse("A1:C3")!!.overlaps(GridArea.parse("C3:F6")!!))
        assertFalse(GridArea.parse("A1:C3")!!.overlaps(GridArea.parse("D1:F3")!!))
        assertFalse(GridArea.parse("A1:F3")!!.overlaps(GridArea.parse("A4:F6")!!))
    }

    /** 正典サンプルの area は全て有効で、兄弟同士で重ならない(backend の検証と同じ約束)。 */
    @Test
    fun exampleAreasAreValidAndNonOverlapping() {
        val files = examplesDir.listFiles { f -> f.extension == "json" }.orEmpty().sortedBy { it.name }
        assertTrue("例が見つからない: $examplesDir", files.isNotEmpty())
        for (file in files) {
            val name = file.nameWithoutExtension
            for (s in AppDSL.decode(file.readText()).screens) {
                for (c in s.components) {
                    if (c.area != null) assertNotNull("$name/${s.id}: bad area ${c.area}", c.gridArea)
                }
                val overlaps = s.overlappingAreas
                assertTrue(
                    "$name/${s.id}: ${overlaps.map { "${it.first.id}×${it.second.id}" }}",
                    overlaps.isEmpty(),
                )
                assertFalse("$name/${s.id}: examples should use the grid", s.gridPlacement.placed.isEmpty())
                for (c in s.components) {
                    val local = c.localGridPlacement ?: continue
                    for (i in local.indices) {
                        for (j in i + 1 until local.size) {
                            assertFalse(
                                "$name/${s.id}/${c.id}: children overlap",
                                local[i].area.overlaps(local[j].area),
                            )
                        }
                    }
                }
            }
        }
    }

    /** Container 内グリッド(#81): card / section / vstack の子が全員 area を持つ時だけ配置を返す。 */
    @Test
    fun localGridPlacement() {
        val home = requireNotNull(example("water").screen("home"))
        val card = requireNotNull(home.components.firstOrNull { it.type == ComponentType.CARD })
        assertEquals(listOf(0, 3), card.localGridPlacement?.map { it.area.col1 })
        val section = requireNotNull(home.components.firstOrNull { it.id == "quick" })
        assertEquals(2, section.localGridPlacement?.size)
        // area の無い子だけの section は従来どおり積む
        assertNull(home.components.firstOrNull { it.id == "recent" }?.localGridPlacement)
    }
}
