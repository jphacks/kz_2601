package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.screen
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.File

/**
 * docs/dsl/conformance/cases(全 OS 共通の期待値・iOS が書く)のうち、骨格のデスクトップ版が今比べられる部分を回す。
 * - 全 case の DSL が共有の :dsl で読めて、画面の id・見出しが expected.json の screens と同じ。
 * - 各画面の部品の木(id・type・子)が同じ(並びは grid の配置を入れるまで見ない)。
 * - デスクトップ版が描ける部品(DesktopLeaves)は、expected が rendered の時に rendered。text の text・button の label が同じ。
 * 値の解決(stat・list の行など)と操作(steps)は、描画の規則を純 Kotlin に移してから足す。
 */
class DesktopConformanceTest {
    private val docsDir = File(requireNotNull(System.getProperty("metamon.dslDocsDir")) { "metamon.dslDocsDir が未設定(Gradle から実行する)" })
    private val casesDir = docsDir.resolve("conformance/cases")

    @Test
    fun conformanceCases() {
        val cases = casesDir.listFiles { f -> f.resolve("expected.json").isFile }.orEmpty().sortedBy { it.name }
        assertTrue("case が無い: $casesDir", cases.isNotEmpty())
        val failures = mutableListOf<String>()
        var screens = 0
        for (dir in cases) {
            val case = Json.parseToJsonElement(dir.resolve("case.json").readText()) as JsonObject
            val expected = Json.parseToJsonElement(dir.resolve("expected.json").readText()) as JsonObject
            val dsl = try {
                decodeDsl(case["dsl"]!!)
            } catch (e: Exception) {
                failures += "${dir.name}: DSL が読めない $e"
                continue
            }
            for (s in expected["screens"] as JsonArray) {
                s as JsonObject
                screens++
                val id = s.str("id")!!
                val screen = dsl.screen(id)
                if (screen == null) {
                    failures += "${dir.name}/$id: 画面が無い"
                    continue
                }
                if (screen.title != s.str("title").orEmpty()) failures += "${dir.name}/$id: 見出し ${screen.title} != ${s.str("title")}"
                compare("${dir.name}/$id", screen.components, s["nodes"] as JsonArray, failures)
            }
        }
        if (failures.isNotEmpty()) fail("${failures.size} 件が違う:\n" + failures.joinToString("\n"))
        println("desktop conformance: ${cases.size} cases / $screens screens OK")
    }

    @Test
    fun bundledExampleLoads() {
        val dsl = Examples.load("shopping")
        assertEquals("home", dsl.initialScreen)
        assertTrue(dsl.screens.isNotEmpty())
    }

    @Test
    fun mockStoreSavesAndValidates() {
        val dsl = Examples.load("shopping")
        val entity = dsl.entities.first { it.id == "groceries" }
        assertEquals(listOf("品名"), entity.missingRequired(entity.defaultDraft()))
        val store = MockStore().save("groceries", null, entity.defaultDraft() + ("name" to "牛乳"))
        assertEquals("new-1", store.list("groceries").single().id)
        assertEquals("牛乳", store.list("groceries").single().data["name"])
        assertTrue(store.delete("groceries", "new-1").list("groceries").isEmpty())
    }

    private fun decodeDsl(e: JsonElement): AppDSL =
        if (e is JsonPrimitive && e.isString) AppDSL.decode(docsDir.resolve(e.content).readText()) else AppDSL.decode(e.toString())

    /** 部品の木を比べる。並びは (id, type) で揃えてから比べる(配置の順は後で)。 */
    private fun compare(at: String, components: List<Component>, nodes: JsonArray, failures: MutableList<String>) {
        val want = nodes.map { it as JsonObject }.sortedBy { key(it.str("id"), it.str("type")) }
        val got = components.sortedBy { key(it.id, DesktopLeaves.typeName(it)) }
        val wantKeys = want.map { key(it.str("id"), it.str("type")) }
        val gotKeys = got.map { key(it.id, DesktopLeaves.typeName(it)) }
        if (wantKeys != gotKeys) {
            failures += "$at: 部品 $gotKeys != $wantKeys"
            return
        }
        for ((c, n) in got.zip(want)) {
            val here = "$at/${key(c.id, DesktopLeaves.typeName(c))}"
            val state = n.str("state")
            val mine = DesktopLeaves.state(c)
            // デスクトップ版で描けない部品は unavailable(iOS は rendered)でよい。知らない type の判定は同じであること
            if ((state == "unknown") != (mine == "unknown")) failures += "$here: state $mine != $state"
            if (mine == "rendered" && state == "rendered") {
                val props = n["props"] as? JsonObject
                when (c.type.wire) {
                    "text" -> props?.str("text")?.let { if (it != c.text) failures += "$here: text ${c.text} != $it" }
                    "button" -> props?.str("label")?.let { if (it != c.label) failures += "$here: label ${c.label} != $it" }
                }
            }
            val children = n["children"] as? JsonArray
            if (children != null) compare(here, c.children, children, failures)
        }
    }

    private fun key(id: String?, type: String?) = "${id ?: "-"}:${type.orEmpty()}"

    private fun JsonObject.str(k: String): String? = (this[k] as? JsonPrimitive)?.contentOrNull
}
