package jp.kz2601.metamon.renderer.theme

import jp.kz2601.metamon.renderer.theme.icons.materialVector
import jp.kz2601.metamon.renderer.theme.icons.materialVectorNames
import jp.kz2601.metamon.renderer.theme.icons.symbolMaterialNames
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * アイコンの対応表(#842)に名前の漏れが無いこと。DSL の例・schema の説明・backend の AI のカタログ・
 * Android の部品が直に書く名前を集め、全部が [SymbolIcons] で Material の絵に写ることを確かめる。
 */
class SymbolIconTableTest {
    private val dslDir = File(requireNotNull(System.getProperty("metamon.dslDocsDir")) { "metamon.dslDocsDir が未設定(Gradle から実行する)" })
    private val backendDir = File(requireNotNull(System.getProperty("metamon.backendDir")))
    private val rendererSrcDir = File(requireNotNull(System.getProperty("metamon.rendererSrcDir")))

    /** 既定の印そのもの(表に無くてよい名前)。 */
    private val fallback = "circle"

    @Test
    fun kotlinTableMatchesSharedDoc() {
        val doc = Json.parseToJsonElement(File(dslDir, "icon-map.json").readText()).jsonObject
        val shared = doc.getValue("symbols").jsonObject.mapValues { it.value.jsonPrimitive.content }
        assertEquals("icon-map.json と生成した表がずれている(scripts/gen-symbol-icons.py を回す)", shared, symbolMaterialNames)
        assertEquals(fallback, doc.getValue("fallback").jsonPrimitive.content)
        val unresolved = symbolMaterialNames.values.toSet() - materialVectorNames
        assertTrue("絵の無い Material 名: $unresolved", unresolved.isEmpty())
    }

    @Test
    fun everyMaterialNameDraws() {
        for (m in materialVectorNames) assertNotNull(m, materialVector(m))
    }

    @Test
    fun everyIconNameInExamplesSchemaAndBackendIsMapped() {
        val names = sortedMapOf<String, String>()
        fun add(name: String, from: String) {
            val n = name.trim()
            if (n.isNotEmpty() && n != fallback && !n.startsWith("$") && !n.contains("{")) names.putIfAbsent(n, from)
        }

        // 1) DSL の例と適合テストの case: icon / systemImage / emptyIcon と、type icon の name。
        dslDir.walkTopDown().filter { it.isFile && it.extension == "json" && it.name != "dsl.schema.json" && it.name != "icon-map.json" }
            .forEach { f -> collectJson(Json.parseToJsonElement(f.readText())) { add(it, f.relativeTo(dslDir).path) } }
        assertTrue("例からアイコン名が取れない", names.size > 30)

        // 2) schema の説明の例(e.g. flame.fill / 例 flame / figure.run / car / 省略時 timer)。
        val schema = File(dslDir, "dsl.schema.json").readText()
        Regex("""SF Symbols? (?:name, e\.g\.|名\(例) ([a-z0-9. /]+)""").findAll(schema).forEach { m ->
            m.groupValues[1].split("/").forEach { add(it, "dsl.schema.json") }
        }
        Regex("""SF Symbols? [^"]{0,80}省略時 ([a-z][a-z0-9.]*)""").findAll(schema).forEach { add(it.groupValues[1], "dsl.schema.json") }

        // 3) backend(AI のカタログ・ルール・フィード)が書くアイコン名。
        val goIcon = Regex("""(?:[Ii]con|[Ss]ymbol)\w*\\?"?\s*[:=]+\s*\\?["`]([a-z][a-z0-9.]*)""")
        backendDir.walkTopDown().filter { it.isFile && it.extension == "go" && !it.name.endsWith("_test.go") }.forEach { f ->
            goIcon.findAll(f.readText()).forEach { add(it.groupValues[1], f.relativeTo(backendDir).path) }
        }

        // 4) Android の部品が直に書く名前(SymbolIcon("…") / c.icon ?: "…")。
        val ktIcon = Regex("""SymbolIcon\(\s*(?:[\w.]+\s*\?:\s*)?"([a-z][a-z0-9.]*)"""")
        rendererSrcDir.walkTopDown().filter { it.isFile && it.extension == "kt" }.forEach { f ->
            ktIcon.findAll(f.readText()).forEach { add(it.groupValues[1], f.name) }
        }

        val missing = names.filter { (n, _) -> SymbolIcons.vector(n) == null }
        assertTrue("表に無いアイコン名(docs/dsl/icon-map.json に足す): $missing", missing.isEmpty())
        println("SymbolIconTableTest: ${names.size} 語を確認・表 ${symbolMaterialNames.size} 語 → Material ${materialVectorNames.size} 種")
    }

    private fun collectJson(e: JsonElement, onName: (String) -> Unit) {
        when (e) {
            is JsonObject -> {
                for ((k, v) in e) {
                    if (k in setOf("icon", "systemImage", "emptyIcon") && v is JsonPrimitive && v.isString) v.contentOrNull?.let(onName)
                    collectJson(v, onName)
                }
                if ((e["type"] as? JsonPrimitive)?.contentOrNull == "icon") (e["name"] as? JsonPrimitive)?.contentOrNull?.let(onName)
            }
            is JsonArray -> e.forEach { collectJson(it, onName) }
            else -> Unit
        }
    }
}
