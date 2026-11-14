package jp.kz2601.metamon.voice

import jp.kz2601.metamon.services.ChatMessage
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
 * アプリごとの会話の履歴 1 件(iOS の `ChatHistoryEntry`・#210)。
 * USER / ASSISTANT = 吹き出し、APPLIED = 反映の区切り(「ここで反映しました(v3)・〜」)。
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
 * 履歴の保存先(iOS の `ChatHistoryStore`)。appId ごとに直近 [LIMIT] 件の JSON を
 * `filesDir/chat_history/<appId>.json` に置く。[directory] が null ならメモリだけ(テスト・モック起動)。
 * アプリ削除で [remove]、アカウント削除で [removeAll]。
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
        runCatching {
            file.parentFile?.mkdirs()
            val tmp = File(file.parentFile, "${file.name}.tmp")
            tmp.writeText(encode(kept))
            if (!tmp.renameTo(file)) {
                file.delete()
                tmp.renameTo(file)
            }
        }
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
        /** iOS と同じ。30 件では修正を重ねるだけで最初の会話から消えた */
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

/**
 * AI に送る文脈(iOS の `ChatContext`・#210)。backend の契約(`POST /v1/chat` の messages)は変えない。
 * 反映済みの古いやり取りは今の DSL と食い違うので送らない。送るのは
 * 1. 過去の変更の要約(区切りの 1 行ずつ・最大 [MAX_SUMMARIES] 件)を assistant の 1 通にして先頭に
 * 2. 最後の反映より後のやり取り(切り替えで閉じた未反映の会話)
 * 3. 今の会話
 */
object ChatContext {
    const val MAX_SUMMARIES = 5

    /** @param header 要約の見出し(「(これまでに反映した変更)」・UI の言語) */
    fun messages(history: List<ChatHistoryEntry>, live: List<ChatMessage>, header: String): List<ChatMessage> {
        val lastApplied = history.indexOfLast { it.kind == ChatHistoryEntry.Kind.APPLIED }
        val tail = history.drop(lastApplied + 1).mapNotNull {
            when (it.kind) {
                ChatHistoryEntry.Kind.USER -> ChatMessage(ChatMessage.Role.USER, it.text)
                ChatHistoryEntry.Kind.ASSISTANT -> ChatMessage(ChatMessage.Role.ASSISTANT, it.text)
                ChatHistoryEntry.Kind.APPLIED -> null
            }
        }
        val summaries = history.filter { it.kind == ChatHistoryEntry.Kind.APPLIED }.takeLast(MAX_SUMMARIES)
        val head = if (summaries.isEmpty()) {
            emptyList()
        } else {
            val lines = summaries.joinToString("\n") { "- " + summaryLine(it) }
            listOf(ChatMessage(ChatMessage.Role.ASSISTANT, header + "\n" + lines))
        }
        return head + tail + live
    }

    fun summaryLine(entry: ChatHistoryEntry): String {
        val version = entry.version?.let { "v$it" }.orEmpty()
        val text = entry.text.trim()
        return when {
            text.isEmpty() -> version
            version.isEmpty() -> text
            else -> "$version: $text"
        }
    }
}
