package jp.kz2601.metamon.services.local

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.services.FetchedDocument
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull
import java.io.File
import kotlin.coroutines.CoroutineContext

/**
 * 最後に取得した DSL の封筒(`{appId, version, dsl}` の生 JSON)を appId ごとに 1 ファイルで持つ(iOS の `DocumentCache`)。
 * 用途はオフライン起動の復元と即時描画だけ(正はサーバー)。記録は持たない([RecordStore] の担当)。
 * :dsl の型は知らないキーを落とすので、デコード前の文字列をそのまま置く。
 *
 * 置き場は記録のローカル DB と別のフォルダにする(例: `filesDir/metamon-dsl`。記録は `filesDir/metamon-records` など)。
 *
 * @param directory 置き場。null ならメモリだけ(テスト用)
 * @param io ファイル読み書きの文脈
 */
class DocumentCache(
    private val directory: File?,
    private val io: CoroutineContext = Dispatchers.IO,
) {
    private val lock = Mutex()
    private val memory = mutableMapOf<String, String>()

    /**
     * 封筒を保存する。手元により新しい版があれば置き換えない(遅れて届いた古い応答で巻き戻さない)。
     * 版を読めない封筒は保存しない。
     */
    suspend fun store(appId: String, envelopeJson: String) {
        val version = versionOf(envelopeJson) ?: return
        lock.withLock {
            val current = read(appId)?.let(::versionOf)
            if (current != null && current > version) return
            memory[appId] = envelopeJson
            val file = file(appId) ?: return
            withContext(io) {
                runCatching { AtomicFile.write(file, envelopeJson) }
                    .onFailure { println("[Metamon] DocumentCache store failed: $it") }
            }
        }
    }

    suspend fun store(document: FetchedDocument) = store(document.document.appId, document.envelopeJson)

    /** 保存した封筒の生 JSON。無ければ null。 */
    suspend fun loadJson(appId: String): String? = lock.withLock { read(appId) }

    /** 保存した DSL。無いか壊れていれば null(壊れたファイルは消す。サーバーから取り直せる)。 */
    suspend fun load(appId: String): FetchedDocument? {
        val json = loadJson(appId) ?: return null
        return runCatching { FetchedDocument(AppDocument.decode(json), json) }.getOrElse {
            remove(appId)
            null
        }
    }

    /** アプリ削除(#180)。 */
    suspend fun remove(appId: String) {
        lock.withLock {
            memory.remove(appId)
            val file = file(appId) ?: return
            withContext(io) { file.delete() }
        }
    }

    /** アカウント削除(#119): 全 appId の DSL を消す。 */
    suspend fun removeAll() {
        lock.withLock {
            memory.clear()
            val dir = directory ?: return
            withContext(io) { dir.deleteRecursively() }
        }
    }

    private suspend fun read(appId: String): String? {
        memory[appId]?.let { return it }
        val file = file(appId) ?: return null
        val text = withContext(io) { runCatching { if (file.exists()) file.readText() else null }.getOrNull() }
        if (text != null) memory[appId] = text
        return text
    }

    private fun file(appId: String): File? = directory?.resolve("${RecordStore.safe(appId)}.json")

    companion object {
        internal fun versionOf(envelopeJson: String): Int? = runCatching {
            ((RecordStore.json.parseToJsonElement(envelopeJson) as? JsonObject)?.get("version") as? JsonPrimitive)?.intOrNull
        }.getOrNull()
    }
}
