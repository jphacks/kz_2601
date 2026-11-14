package jp.kz2601.metamon.settings

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject

/** ライセンスの一覧の 1 件(iOS の `LicenseEntry` と同じ形)。 */
data class LicenseEntry(
    val id: String,
    val kind: Kind,
    val name: String,
    val version: String?,
    val url: String?,
    val license: String,
    val copyright: List<String>,
    val text: String,
    val notices: List<Notice>,
) {
    enum class Kind { APP, DEPENDENCY }

    data class Notice(val title: String, val text: String)
}

/**
 * 設定の「ライセンス」(iOS の `ThirdPartyLicenses`・#306 / #898)。中身は `res/raw/third_party_licenses.json`
 * (`android/scripts/third-party-licenses.py` が配布版の依存と Gradle のキャッシュの POM から生成)。
 */
object ThirdPartyLicenses {
    /** 一覧を読む。形が違えば null(画面は「読み込めませんでした」) */
    fun parse(json: String): List<LicenseEntry>? = runCatching {
        Json.parseToJsonElement(json).jsonObject.getValue("entries").jsonArray.map { e ->
            val o = e.jsonObject
            LicenseEntry(
                id = o.string("id")!!,
                kind = if (o.string("kind") == "app") LicenseEntry.Kind.APP else LicenseEntry.Kind.DEPENDENCY,
                name = o.string("name")!!,
                version = o.string("version"),
                url = o.string("url"),
                license = o.string("license")!!,
                copyright = (o["copyright"] as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.contentOrNull },
                text = o.string("text")!!,
                notices = (o["notices"] as? JsonArray).orEmpty().map { n ->
                    LicenseEntry.Notice(n.jsonObject.string("title")!!, n.jsonObject.string("text")!!)
                },
            )
        }
    }.getOrNull()

    /** 本文を段落に分ける(空行区切り・段落内の改行と字下げは詰める。iOS の `paragraphs` と同じ) */
    fun paragraphs(text: String): List<String> {
        val out = mutableListOf<String>()
        var current = mutableListOf<String>()
        for (line in text.split("\n")) {
            val trimmed = line.trim()
            if (trimmed.isEmpty()) {
                if (current.isNotEmpty()) out += current.joinToString(" ")
                current = mutableListOf()
            } else {
                current += trimmed
            }
        }
        if (current.isNotEmpty()) out += current.joinToString(" ")
        return out
    }

    private fun JsonObject.string(key: String): String? = (this[key] as? JsonPrimitive)?.contentOrNull
}
