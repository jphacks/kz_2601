package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.longOrNull
import java.io.File

/**
 * 小さな設定の置き場(Android の SharedPreferences・iOS の UserDefaults に当たる)。1 ファイルの JSON。
 * 同意・クラッシュ報告の設定は android/services の `AiConsentStore` などがそのまま使う(鍵も iOS・Android と同じ)。
 * 書くたびに丸ごと置き換える(小さいので)。壊れたファイルは空として読む。
 */
class JsonKeyValueStore(private val file: File?) : KeyValueStore {
    private val values: MutableMap<String, JsonPrimitive> = load()

    override fun getInt(key: String): Int? = get(key)?.intOrNull

    override fun getLong(key: String): Long? = get(key)?.longOrNull

    override fun getBoolean(key: String): Boolean? = get(key)?.booleanOrNull

    override fun getString(key: String): String? = get(key)?.takeIf { it.isString }?.contentOrNull

    override fun putInt(key: String, value: Int) = set(key, JsonPrimitive(value))

    override fun putLong(key: String, value: Long) = set(key, JsonPrimitive(value))

    override fun putBoolean(key: String, value: Boolean) = set(key, JsonPrimitive(value))

    override fun putString(key: String, value: String) = set(key, JsonPrimitive(value))

    override fun remove(key: String) {
        synchronized(values) {
            if (values.remove(key) != null) persist()
        }
    }

    /** 全部消す(サインアウト)。 */
    fun clear() {
        synchronized(values) {
            values.clear()
            file?.delete()
        }
    }

    fun keys(): Set<String> = synchronized(values) { values.keys.toSet() }

    private fun get(key: String): JsonPrimitive? = synchronized(values) { values[key] }

    private fun set(key: String, value: JsonPrimitive) {
        synchronized(values) {
            values[key] = value
            persist()
        }
    }

    private fun persist() {
        val f = file ?: return
        val text = json.encodeToString(JsonObject.serializer(), JsonObject(values.toSortedMap()))
        runCatching { writeAtomically(f, text) }.onFailure { System.err.println("[TailorIt] ${f.name} の保存に失敗: $it") }
    }

    private fun load(): MutableMap<String, JsonPrimitive> {
        val f = file?.takeIf { it.isFile } ?: return mutableMapOf()
        val root = runCatching { json.parseToJsonElement(f.readText()) as? JsonObject }.getOrNull() ?: return mutableMapOf()
        return root.mapNotNull { (k, v: JsonElement) -> (v as? JsonPrimitive)?.let { k to it } }.toMap(mutableMapOf())
    }

    private companion object {
        val json = Json { prettyPrint = true }
    }
}

/** 一時ファイルに書いてから置き換える(書く途中で落ちても前の中身が残る。services の AtomicFile と同じ)。 */
internal fun writeAtomically(file: File, text: String) {
    file.parentFile?.mkdirs()
    val tmp = File(file.parentFile, "${file.name}.tmp")
    tmp.writeText(text)
    java.nio.file.Files.move(
        tmp.toPath(), file.toPath(),
        java.nio.file.StandardCopyOption.REPLACE_EXISTING, java.nio.file.StandardCopyOption.ATOMIC_MOVE,
    )
}
