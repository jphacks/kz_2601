package jp.kz2601.metamon.services.local

import kotlinx.serialization.Serializable
import java.io.File
import java.util.UUID

/**
 * 今開いているアプリの appId(iOS の `DeviceAppID`)。初回起動で UUID を 1 つ作って保存し、次の起動でそれを開き直す。
 * appId は端末が決めてサーバーに送る(`GET /dsl` は知らない appId を version 1 で作り、ai の生成も同じ appId で返す)。
 *
 * 置き場は `filesDir/metamon-app/current.json`。起動の組み立てで同期的に読むので、読み書きは小さく同期にする。
 *
 * @param directory 置き場。null ならメモリだけ(テスト用)
 * @param mint 新しい appId を作る(テストで固定する)
 */
class CurrentAppStore(
    private val directory: File?,
    private val mint: () -> String = { UUID.randomUUID().toString().lowercase() },
) {
    @Serializable
    private data class Saved(val appId: String)

    private val lock = Any()
    private var memory: String? = null

    /** この起動で初めて appId を作ったか(インストール直後・#276)。サーバーの最近のアプリを開く判定に使う。 */
    @Volatile
    var issuedThisLaunch: Boolean = false
        private set

    /** 保存した appId。無ければ作って保存する。 */
    val current: String
        get() = synchronized(lock) {
            read() ?: mint().also {
                write(it)
                issuedThisLaunch = true
            }
        }

    /** 一覧からの切り替え・生成の結果: 既存の appId を今のアプリにする。 */
    fun set(appId: String) {
        synchronized(lock) { if (appId.isNotEmpty() && appId != read()) write(appId) }
    }

    /** 「新しいアプリ」「初期状態に戻す」: 新しい appId を作って保存する(前のアプリはサーバーに残る)。 */
    fun rotate(): String = synchronized(lock) { mint().also(::write) }

    private fun read(): String? {
        memory?.let { return it }
        val file = file() ?: return null
        val saved = runCatching {
            if (file.exists()) RecordStore.json.decodeFromString(Saved.serializer(), file.readText()).appId else null
        }.getOrNull()?.takeIf { it.isNotEmpty() }
        memory = saved
        return saved
    }

    private fun write(appId: String) {
        memory = appId
        val file = file() ?: return
        runCatching { AtomicFile.write(file, RecordStore.json.encodeToString(Saved.serializer(), Saved(appId))) }
            .onFailure { println("[Metamon] CurrentAppStore write failed: $it") }
    }

    private fun file(): File? = directory?.resolve("current.json")
}
