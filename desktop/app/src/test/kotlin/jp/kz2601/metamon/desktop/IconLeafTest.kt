package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** icon の名前 → 絵(#1742)。 */
class IconLeafTest {
    @Test
    fun bundledMapIsDocsIconMap() {
        val docs = File(requireNotNull(System.getProperty("metamon.dslDocsDir"))).resolve("icon-map.json")
        val symbols = (Json.parseToJsonElement(docs.readText()) as JsonObject)["symbols"] as JsonObject
        assertEquals(symbols.keys, DesktopIcons.symbols.keys)
    }

    @Test
    fun everyMappedNameHasAVector() {
        val missing = DesktopIcons.symbols.filterKeys { DesktopIcons.vector(it) == null }
        assertTrue("絵が無い名前(android/scripts/gen-symbol-icons.py を回す): $missing", missing.isEmpty())
    }

    @Test
    fun suffixesFallBackAndUnknownIsNull() {
        assertEquals("local_fire_department", DesktopIcons.materialName("flame.fill"))
        assertEquals("favorite", DesktopIcons.materialName("heart.fill"))
        assertEquals("check_circle", DesktopIcons.materialName("checkmark.circle.fill"))
        assertNotNull(DesktopIcons.vector("chart.bar.fill"))
        assertNull(DesktopIcons.vector("no.such.symbol"))
        assertNull(DesktopIcons.vector(null))
    }

    @Test
    fun iconIsDecorativeUnlessLabelled() {
        val data = LeafData(emptyList(), { emptyList() })
        assertEquals("rendered", DesktopLeaves.state(Component(type = ComponentType.ICON, name = "flame.fill")))
        assertNull(A11y.describe(Component(type = ComponentType.ICON, name = "flame.fill"), data))
        assertEquals("予算", A11y.describe(Component(type = ComponentType.ICON, name = "yensign", label = "予算"), data))
    }
}
