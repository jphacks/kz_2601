package jp.kz2601.metamon.desktop.store

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.put
import java.io.File
import java.util.UUID

/**
 * アプリごとの会話の履歴 1 件(Android の `voice/ChatHistory.kt`・iOS の `ChatHistoryEntry`・#210)。
 * USER / ASSISTANT = 吹き出し、APPLIED = 反映の区切り。
 */
data class ChatHistoryEntry(
    val kind: Kind,
    val text: String,
    /** APPLIED の時だけ。反映後の version */
    val version: Int? = null,
    val id: String = UUID.randomUUID().toString(),
) {
    enum class Kind(val wire: String) { USER("user"), ASSISTANT("assistant"), APPLIED("applied") }
}

/**
 * 履歴の保存先(Android の `ChatHistoryStore` を写す)。appId ごとに直近 [LIMIT] 件を
 * `<AppDataDir>/chat_history/<appId>.json` に置く(形は Android と同じ)。[directory] が null ならメモリだけ。
 * アプリの削除で [remove]、「この端末のデータを消す」・サインアウトで [removeAll]([DesktopStore.wipe] が呼ぶ)。描画はしない。
 */
class ChatHistoryStore(private val directory: File?) {
    private val memory = mutableMapOf<String, List<ChatHistoryEntry>>()

    @Synchronized
    fun load(appId: String): List<ChatHistoryEntry> {
        memory[appId]?.let { return it }
        val file = file(appId) ?: return emptyList()
        val text = runCatching { if (file.exists()) file.readText() else null }.getOrNull() ?: return emptyList()
        val entries = decode(text) ?: run {
            file.delete() // 壊れていたら捨てる(会話が消えるだけ)
            return emptyList()
        }
        return trimmed(entries).also { memory[appId] = it }
    }

    @Synchronized
    fun save(entries: List<ChatHistoryEntry>, appId: String) {
        val kept = trimmed(entries)
        if (kept.isEmpty()) return remove(appId)
        memory[appId] = kept
        val file = file(appId) ?: return
        runCatching { writeAtomically(file, encode(kept)) }
            .onFailure { System.err.println("[TailorIt] chat history: save failed: ${it::class.simpleName}") }
    }

    @Synchronized
    fun remove(appId: String) {
        memory.remove(appId)
        file(appId)?.delete()
    }

    @Synchronized
    fun removeAll() {
        memory.clear()
        directory?.deleteRecursively()
    }

    private fun file(appId: String): File? = directory?.resolve("${safe(appId)}.json")

    companion object {
        /** iOS・Android と同じ */
        const val LIMIT = 200

        /** 直近 [LIMIT] 件(区切りも 1 件と数える) */
        fun trimmed(entries: List<ChatHistoryEntry>): List<ChatHistoryEntry> =
            if (entries.size > LIMIT) entries.takeLast(LIMIT) else entries

        internal fun safe(appId: String): String = appId.replace(Regex("[^A-Za-z0-9._-]"), "_")

        internal fun encode(entries: List<ChatHistoryEntry>): String = buildJsonArray {
            entries.forEach { e ->
                add(
                    buildJsonObject {
                        put("id", e.id)
                        put("kind", e.kind.wire)
                        put("text", e.text)
                        e.version?.let { put("version", it) }
                    },
                )
            }
        }.toString()

        /** 読めない形なら null。知らない kind の 1 件は飛ばす */
        internal fun decode(text: String): List<ChatHistoryEntry>? = runCatching {
            (Json.parseToJsonElement(text) as JsonArray).mapNotNull { element ->
                val o = element as? JsonObject ?: return@mapNotNull null
                val kind = ChatHistoryEntry.Kind.entries.firstOrNull { it.wire == o.string("kind") } ?: return@mapNotNull null
                ChatHistoryEntry(
                    kind = kind,
                    text = o.string("text").orEmpty(),
                    version = (o["version"] as? JsonPrimitive)?.intOrNull,
                    id = o.string("id") ?: UUID.randomUUID().toString(),
                )
            }
        }.getOrNull()

        private fun JsonObject.string(key: String): String? = (this[key] as? JsonPrimitive)?.contentOrNull
    }
}
