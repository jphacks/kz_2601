package jp.kz2601.metamon.services.local

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.MarkState
import jp.kz2601.metamon.services.Record
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import java.io.File
import java.time.Instant
import java.time.temporal.ChronoUnit
import java.util.UUID

/** 取り消せる削除の 1 件(M11)。[RecordSyncEngine.deleteUndoably] が返し、[RecordSyncEngine.undoDelete] に渡す。 */
data class DeletionTicket(val id: UUID, val appId: String, val entity: String, val recordId: String)

/** 印を付けられない理由(#255)。 */
class MarkException(val reason: Reason) : Exception(reason.name) {
    enum class Reason {
        /** オフライン(印は送信待ちに積まない) */
        OFFLINE,

        /** 自分の投稿がまだ送られていない */
        NOT_SYNCED,

        /** ローカルに無い(削除済み) */
        NOT_FOUND,
    }
}

/**
 * ローカル DB([RecordStore])+ 送信待ちの列([SyncQueue])+ 到達性([ConnectivityMonitor])をまとめる(iOS の `RecordSyncEngine`)。
 *
 * 読み: [cached] を即返し、続けて [pull](サーバーの全件 → ローカルを置き換え)。
 * 書き: [save] / [delete] はローカルにすぐ反映して一覧を返し、裏で列が appconfig へ送る。
 * 取り消せる削除: [deleteUndoably] は送るのを `graceMillis` だけ待ち、その間は一覧から隠すだけ。戻せばサーバーには何も送らない。
 *
 * 裏で流すのはアプリが動いている間だけ(到達性の復帰・前面復帰・書き込みがきっかけ)。WorkManager は使わない(iOS と同じ)。
 */
class RecordSyncEngine(
    val store: RecordStore,
    val queue: SyncQueue,
    private val client: AppConfigClient,
    val connectivity: ConnectivityMonitor,
    private val scope: CoroutineScope,
    private val now: () -> Instant = Instant::now,
) {
    sealed interface Status {
        data object Synced : Status
        data class Pending(val count: Int) : Status
        data class Syncing(val count: Int) : Status
        data class Offline(val pending: Int) : Status
    }

    private val _status = MutableStateFlow<Status>(Status.Synced)
    val status: StateFlow<Status> = _status.asStateFlow()

    private val _pendingCount = MutableStateFlow(0)
    val pendingCount: StateFlow<Int> = _pendingCount.asStateFlow()

    /** 競合・拒否・送信完了の知らせ(画面がトーストと読み直しに使う) */
    private val _events = MutableSharedFlow<SyncEvent>(extraBufferCapacity = 64)
    val events: SharedFlow<SyncEvent> = _events.asSharedFlow()

    val isOnline: Boolean get() = connectivity.isOnline

    private var jobs = emptyList<Job>()
    private val undoLock = Any()
    private val undoable = mutableMapOf<UUID, Pair<DeletionTicket, Job>>()
    private val committing = mutableMapOf<UUID, Job>()

    /** wipe ごとに増やす。積む途中で wipe を跨いだ削除は積まない */
    @Volatile
    private var wipeGeneration = 0

    // 動かす

    /** 到達性と列の知らせを購読し、未送信があれば流し始める。2 回呼んでも 1 回だけ。 */
    fun start() {
        if (jobs.isNotEmpty()) return
        jobs = listOf(
            scope.launch {
                connectivity.online.collect { online ->
                    refreshStatus()
                    if (online) kick()
                }
            },
            scope.launch {
                queue.events.collect { event ->
                    refreshStatus()
                    _events.emit(event)
                }
            },
        )
    }

    fun stop() {
        jobs.forEach { it.cancel() }
        jobs = emptyList()
    }

    /** アカウント削除(#119): 購読を止め、送信待ちとローカル DB を空にする。次の [start] で再開できる。 */
    suspend fun wipe() {
        stop()
        wipeGeneration += 1
        dropPendingDeletions { true }
        settleCommitting()
        queue.discardAll()
        store.removeEverything()
        refreshStatus()
    }

    /**
     * サインアウトの前に未送信を送り切る。オフライン・続けて失敗して止まった・時間切れならそこまで。
     * @return 残った未送信の件数
     */
    suspend fun flush(timeoutMillis: Long = 10_000): Int {
        commitPendingDeletions()
        val deadline = System.nanoTime() + timeoutMillis * 1_000_000
        while (isOnline && System.nanoTime() < deadline) {
            if (!queue.isDraining) {
                queue.resume()
                if (queue.count() == 0 || queue.isPaused) break
            }
            delay(100) // 別の送信が流れている間は待つ
        }
        refreshStatus()
        return queue.count()
    }

    /** 前面に戻ったら流す。背面に回ったら取り消し待ちの削除を待たずに積む(落とされても削除を失わない)。 */
    fun setForeground(active: Boolean) {
        if (active) kick() else scope.launch { commitPendingDeletions() }
    }

    /** 設定の「今すぐ同期」。 */
    fun syncNow() = kick()

    /** オンラインなら列を(止まっていても)流し直す。書き込み・復帰・手動同期のすべてがここを通る。 */
    private fun kick() {
        scope.launch {
            refreshStatus()
            if (!isOnline) return@launch
            queue.resume()
            refreshStatus()
        }
    }

    internal suspend fun refreshStatus() {
        val n = queue.count()
        _pendingCount.value = n
        _status.value = when {
            !isOnline -> Status.Offline(n)
            queue.isDraining -> Status.Syncing(n)
            n > 0 -> Status.Pending(n)
            else -> Status.Synced
        }
    }

    // 読み

    /** ローカル DB の中身(すぐ返す)。削除待ち・取り消し待ちは除く。 */
    suspend fun cached(appId: String, entity: String): List<Record> = shown(appId, entity)

    /** サーバーの全件を取ってローカルを置き換え(未送信の行は残す)、返す。失敗はそのまま投げる(呼ぶ側はローカル表示を続ける)。 */
    suspend fun pull(appId: String, entity: String): List<Record> {
        val server = client.listRecords(appId, entity)
        store.replace(appId, entity, server)
        return shown(appId, entity)
    }

    // 書き(楽観的)

    /** 作成(`recordId == null`)か更新。ローカルにすぐ反映し、一覧を返す。送るのは列。 */
    suspend fun save(
        appId: String, entity: String, recordId: String?, data: JsonObject,
        authorUid: String? = null, authorName: String? = null, shared: Boolean = false,
    ): List<Record> = write(appId, entity, recordId, data, authorUid, authorName, shared).first

    /**
     * [save] と同じ。書いた行の id も返す(作成なら仮 id `local-<clientId>` = 断られた時の [SyncEvent] の recordId)。
     * @param shared shared / public の entity に作る(#250)。投稿者を行に付け、[authorName] を送る
     */
    suspend fun write(
        appId: String, entity: String, recordId: String?, data: JsonObject,
        authorUid: String? = null, authorName: String? = null, shared: Boolean = false,
    ): Pair<List<Record>, String> {
        val stamp = timestamp()
        val existing = recordId?.let { store.row(appId, entity, it) }
        val written = if (recordId != null && existing != null) {
            // PATCH と同じくキー単位でマージ(DSL から消えた項目も残す)
            store.upsert(
                existing.copy(
                    data = JsonObject(existing.data + data),
                    updatedAt = stamp,
                    pendingOp = if (existing.pendingOp == PendingOp.CREATE) PendingOp.CREATE else PendingOp.UPDATE,
                ),
            )
            queue.enqueue(SyncOperation(appId = appId, entity = entity, recordId = recordId, kind = PendingOp.UPDATE, data = data))
            recordId
        } else {
            val clientId = UUID.randomUUID().toString().lowercase()
            val row = LocalRecord(
                appId = appId, entity = entity, recordId = LocalRecord.temporaryId(clientId), data = data,
                createdAt = stamp, updatedAt = stamp, pendingOp = PendingOp.CREATE, clientId = clientId,
                author = if (shared) authorUid else null,
                authorName = if (shared) authorName ?: ANONYMOUS_NAME else null,
            )
            store.upsert(row)
            queue.enqueue(
                SyncOperation(
                    id = clientId, appId = appId, entity = entity, recordId = row.recordId, kind = PendingOp.CREATE,
                    data = data, authorName = if (shared) authorName else null,
                ),
            )
            row.recordId
        }
        kick()
        return shown(appId, entity) to written
    }

    /** 削除。ローカルではすぐ一覧から消え(行は削除待ちで残る)、送れたら行も消える。 */
    suspend fun delete(appId: String, entity: String, recordId: String): List<Record> {
        val result = queue.enqueue(SyncOperation(appId = appId, entity = entity, recordId = recordId, kind = PendingOp.DELETE))
        if (result == SyncQueue.EnqueueResult.CANCELLED_CREATE) {
            store.remove(appId, entity, recordId) // まだサーバーに無い行はその場で消す
        } else {
            store.markDeleting(appId, entity, recordId)
        }
        kick()
        return shown(appId, entity)
    }

    /** アプリ削除(#180): その appId の送信待ちとローカル DB を消す。サーバーの削除が済んでから呼ぶ。 */
    suspend fun purge(appId: String) {
        dropPendingDeletions { it.appId == appId }
        queue.discard(appId)
        store.removeAll(appId)
        refreshStatus()
    }

    /** 印を付ける / 外す(#255)。押した瞬間にローカルを変え、サーバーの値で確定する。失敗したら戻して投げる。 */
    suspend fun toggleMark(appId: String, entity: String, recordId: String, mark: String): MarkState {
        val row = store.row(appId, entity, recordId)?.takeIf { it.pendingOp != PendingOp.DELETE }
            ?: throw MarkException(MarkException.Reason.NOT_FOUND)
        if (row.isTemporary) throw MarkException(MarkException.Reason.NOT_SYNCED)
        if (!isOnline) throw MarkException(MarkException.Reason.OFFLINE)
        val before = row.marks?.get(mark) ?: MarkState()
        val next = MarkState(count = (before.count + if (before.mine) -1 else 1).coerceAtLeast(0), mine = !before.mine)
        store.setMark(appId, entity, recordId, mark, next)
        return try {
            client.setMark(appId, entity, recordId, mark, on = next.mine).also { store.setMark(appId, entity, recordId, mark, it) }
        } catch (e: Throwable) {
            store.setMark(appId, entity, recordId, mark, before)
            throw e
        }
    }

    // 取り消せる削除(M11)

    /**
     * 一覧から隠し、[graceMillis] 後に [delete] と同じ送信をする。待ちはメモリだけに持つ
     * (待ちの間にアプリが落ちたら削除されずに残る = 消えすぎない側)。
     */
    suspend fun deleteUndoably(appId: String, entity: String, recordId: String, graceMillis: Long): Pair<List<Record>, DeletionTicket> {
        val ticket = DeletionTicket(UUID.randomUUID(), appId, entity, recordId)
        synchronized(undoLock) {
            val timer = scope.launch {
                delay(graceMillis)
                commitDeletion(ticket.id)
            }
            undoable[ticket.id] = ticket to timer
        }
        return shown(appId, entity) to ticket
    }

    /** 取り消す。送る前なら隠すのをやめて一覧を返す。もう送った(時間切れ・背面)なら null。 */
    suspend fun undoDelete(ticket: DeletionTicket): List<Record>? {
        val pending = synchronized(undoLock) { undoable.remove(ticket.id) } ?: return null
        pending.second.cancel()
        return shown(ticket.appId, ticket.entity)
    }

    /** 取り消しを待っている削除。 */
    val pendingDeletions: List<DeletionTicket> get() = synchronized(undoLock) { undoable.values.map { it.first } }

    /** 待たずに全部送る(背面に回った・サインアウト前の送り切り)。 */
    suspend fun commitPendingDeletions() {
        synchronized(undoLock) { undoable.keys.toList() }.forEach { commitDeletion(it) }
        settleCommitting()
        refreshStatus()
    }

    private suspend fun commitDeletion(id: UUID) {
        val generation = wipeGeneration
        val self = currentCoroutineContext()[Job]
        val work: Job
        synchronized(undoLock) {
            val pending = undoable.remove(id) ?: return
            if (pending.second != self) pending.second.cancel() // 時間切れのタイマー自身は止めない
            val t = pending.first
            work = scope.launch {
                val current = store.current(t.appId, t.entity, t.recordId)?.recordId ?: t.recordId
                if (wipeGeneration != generation) return@launch // 待つ間に wipe された: 次の人の列に積まない
                delete(t.appId, t.entity, current)
            }
            committing[id] = work
        }
        work.join()
        synchronized(undoLock) { committing.remove(id) }
    }

    /** 列に積む途中の削除が終わるまで待つ。 */
    private suspend fun settleCommitting() {
        while (true) {
            val work = synchronized(undoLock) { committing.values.firstOrNull() } ?: return
            work.join()
            synchronized(undoLock) { committing.values.remove(work) }
        }
    }

    /** 送らずに捨てる(アプリ削除・アカウント削除。ローカルもサーバーも別の経路で消す)。 */
    private fun dropPendingDeletions(match: (DeletionTicket) -> Boolean) = synchronized(undoLock) {
        undoable.entries.filter { match(it.value.first) }.forEach { (id, p) ->
            p.second.cancel()
            undoable.remove(id)
        }
    }

    /** 表示用の一覧: ローカル DB の中身から、取り消し待ちの削除を除く(作成が送られて仮 id が替わっていても同じ行とみなす)。 */
    private suspend fun shown(appId: String, entity: String): List<Record> {
        val hidden = pendingDeletions.filter { it.appId == appId && it.entity == entity }
        if (hidden.isEmpty()) return store.records(appId, entity)
        return store.rows(appId, entity)
            .filter { row -> row.pendingOp != PendingOp.DELETE && hidden.none { row.isKnownAs(it.recordId) } }
            .map { it.record }
    }

    /** ローカル行の時刻。ミリ秒まで持つ(同じ秒に続けて作った行も作った順に並ぶ)。 */
    private fun timestamp(): String = now().truncatedTo(ChronoUnit.MILLIS).toString()

    companion object {
        /** 表示名を送らなかった投稿に backend が付ける名前(appconfig `defaultAuthorName`)。送る前のローカル行にも出す。 */
        const val ANONYMOUS_NAME = "匿名"

        /** 既定の組み立て: `directory` にファイルで残す。 */
        fun create(
            client: AppConfigClient,
            connectivity: ConnectivityMonitor,
            directory: File,
            scope: CoroutineScope,
        ): RecordSyncEngine {
            val store = RecordStore(directory)
            return RecordSyncEngine(store, SyncQueue(directory, store, client), client, connectivity, scope)
        }
    }
}
