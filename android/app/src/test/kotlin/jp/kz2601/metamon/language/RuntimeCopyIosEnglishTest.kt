package jp.kz2601.metamon.language

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/**
 * ランタイムのトースト・仲間探し・サインインの英語が iOS の訳(Localizable.xcstrings)と同じか(#843)。
 * 日本語の文を iOS のキー(%@ / %lld)に直して引き、iOS に訳がある文だけ比べる。
 */
class RuntimeCopyIosEnglishTest {
    private val androidRoot: File = File(System.getProperty("user.dir")).absoluteFile.parentFile

    private fun strings(file: File): Map<String, String> {
        val nodes = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).getElementsByTagName("string")
        return (0 until nodes.length).associate { i ->
            val e = nodes.item(i) as Element
            e.getAttribute("name") to e.textContent.removeSurrounding("\"").replace("\\'", "'")
        }
    }

    private fun iosEnglish(): Map<String, String> {
        val root = Json.parseToJsonElement(File(androidRoot.parentFile, "ios/Metamon/Resources/Localizable.xcstrings").readText())
        return root.jsonObject.getValue("strings").jsonObject.mapNotNull { (key, v) ->
            val en = v.jsonObject["localizations"]?.jsonObject?.get("en")?.jsonObject?.get("stringUnit")?.jsonObject?.get("value")
            en?.jsonPrimitive?.content?.let { key to it }
        }.toMap()
    }

    private fun iosKey(android: String) = android.replace(Regex("%\\d+\\\$s"), "%@").replace(Regex("%\\d+\\\$d"), "%lld")

    private fun normalized(text: String) = text
        .replace(Regex("%(\\d+\\\$)?@"), "%s").replace(Regex("%(\\d+\\\$)?lld"), "%d").replace(Regex("%\\d+\\\$"), "%")

    @Test
    fun englishMatchesIos() {
        val ios = iosEnglish()
        val problems = mutableListOf<String>()
        for (name in listOf("strings_runtime.xml", "strings_matching.xml", "strings_auth.xml")) {
            val ja = strings(File(androidRoot, "app/src/main/res/values/$name"))
            val en = strings(File(androidRoot, "app/src/main/res/values-en/$name"))
            for ((key, text) in ja) {
                val expected = ios[iosKey(text)] ?: continue
                val actual = en[key].orEmpty()
                if (normalized(expected) != normalized(actual)) problems += "$name $key: iOS \"$expected\" / Android \"$actual\""
            }
        }
        assertEquals(problems.joinToString("\n"), emptyList<String>(), problems)
    }
}
