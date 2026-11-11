package jp.kz2601.metamon.services.local

import jp.kz2601.metamon.services.MarkState
import jp.kz2601.metamon.services.Record
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import java.io.File
import kotlin.coroutines.CoroutineContext

/** ローカル行の未送信状態。`NONE` 以外は [SyncQueue] に対応する操作が積まれている。 */
@Serializable
enum class PendingOp { NONE, CREATE, UPDATE, DELETE }

/**
 * 記録のローカル DB 1 行(iOS の `LocalRecord`・#48 の汎用スキーマ)。
 * `data` は DSL の Field に関係なく丸ごと持つ(DSL から消えた項目も残し、表示側が無視するだけ)。
 */
@Serializable
data class LocalRecord(
    val appId: String,
    val entity: String,
    val recordId: String,
    val data: JsonObject = JsonObject(emptyMap()),
    val createdAt: String? = null,
    val updatedAt: String? = null,
    val pendingOp: PendingOp = PendingOp.NONE,
    /** 作成の冪等キー(= 未送信の作成の仮 id の元)。サーバー採番後も参照用に残す */
    val clientId: String? = null,
    /** 作った人の uid(shared / public の entity だけ・#249) */
    val author: String? = null,
    /** 投稿者の表示名(#250)。自分の未送信の作成は送る予定の名前 */
    val authorName: String? = null,
    /** 印の状態(#255)。サーバーの値か、押した直後の楽観的な値 */
    val marks: Map<String, MarkState>? = null,
) {
    /** 描画器に渡す形。 */
    val record: Record
        get() = Record(
            id = recordId, entity = entity, data = data, author = author, authorName = authorName,
            marks = marks, createdAt = createdAt, updatedAt = updatedAt,
        )

    /** まだサーバーに無い自分の作成(一覧で「未送信」と出す) */
    val unsent: Boolean get() = pendingOp == PendingOp.CREATE

    val isTemporary: Boolean get() = recordId.startsWith(TEMP_PREFIX)

    /** 今の id か、作成を送る前の仮 id なら true。 */
    fun isKnownAs(id: String): Boolean = recordId == id || clientId?.let(::temporaryId) == id

    companion object {
        const val TEMP_PREFIX = "local-"

        /** 未送信の作成の仮 id(`local-<clientId>`)。サーバー採番後に差し替わる。 */
        fun temporaryId(clientId: String): String = "$TEMP_PREFIX$clientId"

        fun fromServer(appId: String, entity: String, record: Record): LocalRecord = LocalRecord(
            appId = appId, entity = entity, recordId = record.id, data = record.data,
            createdAt = record.createdAt, updatedAt = record.updatedAt,
            author = record.author, authorName = record.authorName, marks = record.marks,
        )
    }
}

/**
 * 記録のローカル DB(iOS の `RecordStore`)。
 *
 * 選定: Room ではなく **ファイル + JSON(appId / entity ごとに 1 ファイル)**。iOS と同じ形。
 * - 1 entity ≤ 1000 件・読みは entity 単位の全件なので、ファイル = 索引 (appId, entity) で足りる
 * - 新しい依存(Room + KSP)とスキーマ移行を抱えず、JVM の単体テストでそのまま動く
 * 行数が万単位になったら Room に置き換える。公開の関数は変えない。
 *
 * @param directory 置き場。null ならメモリだけ(テスト用)
 * @param io ファイル読み書きの文脈(テストでは仮想時間を崩さないよう差し替える)
 */
class RecordStore(
    private val directory: File?,
    private val io: CoroutineContext = Dispatchers.IO,
) {
    private val lock = Mutex()

    /** "appId/entity" → 行(メモリキャッシュ。ファイルは書くたびに置き換える) */
    private val tables = mutableMapOf<String, List<LocalRecord>>()

    // 読み

    /** 表示用: 削除待ちを除き、createdAt 降順(サーバー既定と同じ)。未送信の作成は今の時刻なので先頭に来る。 */
    suspend fun records(appId: String, entity: String): List<Record> =
        rows(appId, entity).filter { it.pendingOp != PendingOp.DELETE }.map { it.record }

    suspend fun rows(appId: String, entity: String): List<LocalRecord> =
        lock.withLock { table(appId, entity) }.sortedByDescending { it.createdAt ?: "" }

    suspend fun row(appId: String, entity: String, recordId: String): LocalRecord? =
        lock.withLock { table(appId, entity).firstOrNull { it.recordId == recordId } }

    /** [row] と同じだが、作成の送信で id が替わった後でも仮 id で引ける。 */
    suspend fun current(appId: String, entity: String, recordId: String): LocalRecord? = lock.withLock {
        val t = table(appId, entity)
        t.firstOrNull { it.recordId == recordId } ?: t.firstOrNull { it.isKnownAs(recordId) }
    }

    /** 端末内の全行(読み込んだ表 + まだ読んでいない entity のファイル)。消してよい動画・録音のファイルを決める(#1098)。 */
    suspend fun allRows(): List<LocalRecord> = lock.withLock {
        val loaded = tables.values.flatten()
        val dir = directory ?: return@withLock loaded
        val onDisk = withContext(io) {
            dir.listFiles()?.filter { it.isDirectory }.orEmpty().flatMap { appDir ->
                appDir.listFiles()?.filter { it.extension == "json" }.orEmpty().flatMap { f ->
                    runCatching { json.decodeFromString(rowsSerializer, f.readText()) }.getOrElse { emptyList() }
                }
            }
        }
        loaded + onDisk.filter { "${it.appId}/${it.entity}" !in tables }
    }

    /** 未送信の行数(appId 単位)。読み込んだ entity だけ数える。 */
    suspend fun pendingCount(appId: String): Int = lock.withLock {
        tables.filterKeys { it.startsWith("$appId/") }.values.flatten().count { it.pendingOp != PendingOp.NONE }
    }

    // 書き

    suspend fun upsert(row: LocalRecord) = edit(row.appId, row.entity) { t ->
        val i = t.indexOfFirst { it.recordId == row.recordId }
        if (i >= 0) t[i] = row else t += row
    }

    suspend fun remove(appId: String, entity: String, recordId: String) = edit(appId, entity) { t ->
        t.removeAll { it.recordId == recordId }
    }

    /** 行を削除待ちにする(仮 id でも引ける)。 */
    suspend fun markDeleting(appId: String, entity: String, recordId: String) = edit(appId, entity) { t ->
        val i = t.indexOfFirst { it.isKnownAs(recordId) }
        if (i >= 0) t[i] = t[i].copy(pendingOp = PendingOp.DELETE)
    }

    /**
     * サーバーの全件で置き換える(サーバーが正)。未送信の行だけは残す。
     * - 作成待ちの仮行: そのまま残す
     * - 更新・削除待ちの行: ローカルを優先(送った後の次の取得でサーバー版になる)
     */
    suspend fun replace(appId: String, entity: String, server: List<Record>) = edit(appId, entity) { t ->
        val pending = t.filter { it.pendingOp != PendingOp.NONE }
        t.clear()
        t += server.map { LocalRecord.fromServer(appId, entity, it) }
        for (p in pending) {
            val i = t.indexOfFirst { it.recordId == p.recordId }
            if (i >= 0) t[i] = p else t += p
        }
    }

    /**
     * 送れた: 仮 id の行をサーバーの記録に差し替える(作成)/ 未送信の印を落とす(更新)。
     * 送る間に消された行は削除待ちのまま残す(列の削除が送られるまで一覧に戻さない)。
     */
    suspend fun markSynced(appId: String, entity: String, localId: String, server: Record) = edit(appId, entity) { t ->
        val local = t.firstOrNull { it.recordId == localId }
        t.removeAll { it.recordId == localId || it.recordId == server.id }
        var row = LocalRecord.fromServer(appId, entity, server).copy(
            clientId = localId.removePrefix(LocalRecord.TEMP_PREFIX).takeIf { localId.startsWith(LocalRecord.TEMP_PREFIX) },
        )
        if (row.marks == null) row = row.copy(marks = local?.marks) // 作成の応答に marks が無くても印を消さない
        if (local?.pendingOp == PendingOp.DELETE) row = row.copy(pendingOp = PendingOp.DELETE)
        t += row
    }

    /** 印だけを書き換える(#255)。行が無ければ false。 */
    suspend fun setMark(appId: String, entity: String, recordId: String, mark: String, state: MarkState): Boolean {
        var found = false
        edit(appId, entity) { t ->
            val i = t.indexOfFirst { it.recordId == recordId }
            if (i >= 0) {
                found = true
                t[i] = t[i].copy(marks = (t[i].marks ?: emptyMap()) + (mark to state))
            }
        }
        return found
    }

    /** 送るのを諦めた(サーバーが断った): 作成は行ごと消し、更新・削除は未送信の印を戻す。 */
    suspend fun revertPending(appId: String, entity: String, recordId: String) = edit(appId, entity) { t ->
        val i = t.indexOfFirst { it.recordId == recordId }
        if (i < 0) return@edit
        if (t[i].pendingOp == PendingOp.CREATE) t.removeAt(i) else t[i] = t[i].copy(pendingOp = PendingOp.NONE)
    }

    /** アプリ削除(#180): その appId の行とファイルを消す。 */
    suspend fun removeAll(appId: String) {
        lock.withLock {
            tables.keys.filter { it.startsWith("$appId/") }.forEach(tables::remove)
            val dir = directory?.resolve(safe(appId)) ?: return
            withContext(io) { dir.deleteRecursively() }
        }
    }

    /** アカウント削除(#119): 全部消す。 */
    suspend fun removeEverything() {
        lock.withLock {
            tables.clear()
            val dir = directory ?: return
            withContext(io) {
                dir.listFiles()?.filter { it.isDirectory }?.forEach { it.deleteRecursively() }
            }
        }
    }

    // ファイル

    private suspend fun edit(appId: String, entity: String, block: (MutableList<LocalRecord>) -> Unit) {
        lock.withLock {
            val t = table(appId, entity).toMutableList()
            block(t)
            save(appId, entity, t)
        }
    }

    private suspend fun table(appId: String, entity: String): List<LocalRecord> {
        val key = "$appId/$entity"
        tables[key]?.let { return it }
        val file = file(appId, entity)
        val loaded = if (file == null) emptyList() else withContext(io) {
            runCatching { if (file.exists()) json.decodeFromString(rowsSerializer, file.readText()) else emptyList() }
                .getOrElse { emptyList() } // 壊れたファイルは読み捨てる(サーバーから引き直せる)
        }
        tables[key] = loaded
        return loaded
    }

    private suspend fun save(appId: String, entity: String, rows: List<LocalRecord>) {
        tables["$appId/$entity"] = rows
        val file = file(appId, entity) ?: return
        withContext(io) {
            runCatching { AtomicFile.write(file, json.encodeToString(rowsSerializer, rows)) }
                .onFailure { println("[Metamon] RecordStore save failed: $it") }
        }
    }

    private fun file(appId: String, entity: String): File? =
        directory?.resolve(safe(appId))?.resolve("${safe(entity)}.json")

    companion object {
        internal val json = Json { ignoreUnknownKeys = true; encodeDefaults = false }
        private val rowsSerializer = ListSerializer(LocalRecord.serializer())

        internal fun safe(s: String): String = s.map { if (it.isLetterOrDigit() || it == '-' || it == '_') it else '_' }.joinToString("")
    }
}

/** 一時ファイルに書いてから置き換える(書く途中で落ちても前の中身が残る)。 */
internal object AtomicFile {
    fun write(file: File, text: String) {
        file.parentFile?.mkdirs()
        val tmp = File(file.parentFile, "${file.name}.tmp")
        tmp.writeText(text)
        java.nio.file.Files.move(
            tmp.toPath(), file.toPath(),
            java.nio.file.StandardCopyOption.REPLACE_EXISTING, java.nio.file.StandardCopyOption.ATOMIC_MOVE,
        )
    }
}
