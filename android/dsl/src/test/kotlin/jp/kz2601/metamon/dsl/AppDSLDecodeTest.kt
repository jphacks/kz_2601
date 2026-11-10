package jp.kz2601.metamon.dsl

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class AppDSLDecodeTest {
    private val examplesDir = File(
        requireNotNull(System.getProperty("metamon.dslExamplesDir")) { "metamon.dslExamplesDir が未設定(Gradle から実行する)" },
    )

    private fun example(name: String) = File(examplesDir, "$name.json").readText()

    @Test
    fun decodesInitialExample() {
        val dsl = AppDSL.decode(example("initial"))

        assertEquals(1, dsl.schemaVersion)
        assertEquals("はじめてのアプリ", dsl.app.name)
        assertEquals(VoiceEntry.FULLSCREEN, dsl.app.voiceEntry)
        assertTrue(dsl.isVoiceOnly)
        assertEquals("home", dsl.initialScreen)

        val home = requireNotNull(dsl.screen("home"))
        val mic = home.components.single()
        assertEquals(ComponentType.VOICE_INPUT, mic.type)
        assertEquals("mic", mic.id)
        assertEquals("A1:F12", mic.area)
        assertEquals("どんなアプリが欲しい？", mic.prompt)
        assertEquals(Size.LARGE, mic.size)
    }

    @Test
    fun decodesEveryExampleWithoutUnknownTypes() {
        val files = examplesDir.listFiles { f -> f.extension == "json" }.orEmpty().sortedBy { it.name }
        assertTrue("例が見つからない: $examplesDir", files.isNotEmpty())
        for (file in files) {
            val dsl = AppDSL.decode(file.readText())
            assertTrue("${file.name}: 画面が無い", dsl.screens.isNotEmpty())
            val unknown = dsl.screens.flatMap { it.components }.flatMap { listOf(it) + it.children }
                .filter { it.type == ComponentType.UNKNOWN }
            assertTrue("${file.name}: 知らない type がある $unknown", unknown.isEmpty())
        }
    }

    @Test
    fun unknownValuesFallBackInsteadOfFailing() {
        val dsl = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"x","voiceEntry":"sideways","newKey":1},"initialScreen":"home",
             "screens":[
               {"id":"home","components":[
                 {"type":"hologram","id":"h"},
                 {"type":"text","text":"ok","children":"not-an-array"},
                 42
               ]},
               {"title":"id が無いので捨てる"}
             ]}
            """.trimIndent(),
        )

        assertEquals(VoiceEntry.FLOATING, dsl.app.voiceEntry)
        assertEquals(1, dsl.screens.size)
        val components = dsl.screens.single().components
        assertEquals(listOf(ComponentType.UNKNOWN, ComponentType.TEXT), components.map { it.type })
        assertEquals(emptyList<Component>(), components[1].children)
    }

    @Test
    fun decodesEnvelope() {
        val doc = AppDocument.decode("""{"appId":"a1","version":3,"dsl":${example("initial")}}""")
        assertEquals("a1", doc.appId)
        assertEquals(3, doc.version)
        assertEquals("home", doc.dsl.initialScreen)
    }

    @Test
    fun fallbackDecodes() {
        assertEquals(ComponentType.TEXT, AppDSL.fallback.screens.single().components.single().type)
    }
}
