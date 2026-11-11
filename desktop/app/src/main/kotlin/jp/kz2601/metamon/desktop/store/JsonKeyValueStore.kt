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
import java.io.FileOutputStream
import java.nio.file.AccessDeniedException
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption

/**
 * 小さな設定の置き場(Android の SharedPreferences・iOS の UserDefaults に当たる)。1 ファイルの JSON。
 * 同意・クラッシュ報告の設定は android/services の `AiConsentStore` などがそのまま使う(鍵も iOS・Android と同じ)。
 * 書くたびに丸ごと置き換える(小さいので)。壊れたファイルは `<名前>.corrupt` に退けて空から始める(上書きで消さない)。
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
        val root = runCatching { json.parseToJsonElement(f.readText()) as? JsonObject }.getOrNull()
        if (root == null) {
            setAside(f)
            return mutableMapOf()
        }
        return root.mapNotNull { (k, v: JsonElement) -> (v as? JsonPrimitive)?.let { k to it } }.toMap(mutableMapOf())
    }

    /** 読めないファイルは次の保存で上書きせず `<名前>.corrupt` に退ける(後で調べられる・前の .corrupt は置き換える)。 */
    private fun setAside(f: File) {
        val aside = File(f.parentFile, "${f.name}.corrupt")
        runCatching { Files.move(f.toPath(), aside.toPath(), StandardCopyOption.REPLACE_EXISTING) }
            .onSuccess { System.err.println("[TailorIt] ${f.name} を読めないので ${aside.name} に退けました") }
            .onFailure { System.err.println("[TailorIt] ${f.name} を退けられませんでした: $it") }
    }

    private companion object {
        val json = Json { prettyPrint = true }
    }
}

/**
 * 一時ファイルに書いてから置き換える(書く途中で落ちても前の中身が残る)。
 * - 置き換える前に一時ファイルを disk まで書き切る(`force`。電源断で中身が空のファイルに置き換わらない)
 * - Windows はウイルス対策・検索の索引が開いている間 [AccessDeniedException] で置き換えられないので、少し待って [MOVE_RETRIES] 回まで試し直す
 */
internal fun writeAtomically(
    file: File,
    text: String,
    move: (Path, Path) -> Unit = { from, to -> Files.move(from, to, StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE) },
    pause: (Long) -> Unit = Thread::sleep,
) {
    file.parentFile?.mkdirs()
    val tmp = File(file.parentFile, "${file.name}.tmp")
    FileOutputStream(tmp).use { out ->
        out.write(text.toByteArray(Charsets.UTF_8))
        out.channel.force(true)
    }
    var attempt = 0
    while (true) {
        try {
            move(tmp.toPath(), file.toPath())
            return
        } catch (e: AccessDeniedException) {
            if (attempt >= MOVE_RETRIES) throw e
            attempt++
            pause(MOVE_BACKOFF_MILLIS * attempt)
        }
    }
}

/** 置き換えを試し直す回数(初回の後)。 */
internal const val MOVE_RETRIES = 3

private const val MOVE_BACKOFF_MILLIS = 20L
