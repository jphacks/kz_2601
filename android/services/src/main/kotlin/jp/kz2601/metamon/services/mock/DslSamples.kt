package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.services.FetchedDocument
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

/**
 * 同梱のサンプル DSL(docs/dsl/examples・iOS の `DSLExamples`)。読み方は外から渡す(アプリは assets、テストはファイル)。
 * [read] は名前(拡張子なし)→ JSON 本文。無ければ null。
 */
class DslSamples(private val read: (String) -> String?) {
    /** 本文の JSON。無い名前・読めない本文は null。 */
    fun json(name: String): JsonObject? =
        read(name)?.let { runCatching { DslJson.parseToJsonElement(it).jsonObject }.getOrNull() }

    fun has(name: String): Boolean = json(name) != null

    /** 封筒(`DSLEnvelope`)にして返す。無い名前は例外。 */
    fun document(name: String, appId: String, version: Int): FetchedDocument {
        val dsl = json(name) ?: throw IllegalArgumentException("sample DSL not found: $name")
        return envelope(appId, version, dsl)
    }

    companion object {
        const val INITIAL = "initial"

        fun envelope(appId: String, version: Int, dsl: JsonObject): FetchedDocument {
            val document = AppDocument(appId, version, DslJson.decodeFromJsonElement(AppDSL.serializer(), dsl))
            val json = buildJsonObject {
                put("appId", appId)
                put("version", version)
                put("dsl", dsl)
            }.toString()
            return FetchedDocument(document, json)
        }
    }
}
