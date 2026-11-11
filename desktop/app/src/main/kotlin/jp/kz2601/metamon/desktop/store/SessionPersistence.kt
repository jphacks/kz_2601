package jp.kz2601.metamon.desktop.store

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.Row
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.desktop.typedValue
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.LocalRecord
import jp.kz2601.metamon.services.local.PendingOp
import jp.kz2601.metamon.services.local.SyncEvent
import jp.kz2601.metamon.services.local.SyncOperation
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import java.time.Instant
import java.util.UUID

/**
 * 画面(DesktopSession)の記録を端末に残し、送り先があれば SyncQueue で送る(#1887)。
 *
 * - 画面はこれまでどおり [MockStore] を見る。変わるたびに前後の差を [RecordStore] に書く(作成・更新・削除)
 * - 送り先([client])がある時だけ、同じ差を [SyncQueue] に積んで送る。無ければ(既定のモック)端末に残すだけで通信しない
 * - 書き込みは 1 本の列で順に流す(画面を止めない)。[idle] で書き終わりを待てる
 *
 * @param client null ならモック(通信しない)
 */
class SessionPersistence(
    val store: DesktopStore,
    val appId: String,
    client: AppConfigClient?,
    private val uid: String? = "me",
    policy: BackoffPolicy = BackoffPolicy(),
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
) {
    val queue: SyncQueue? = client?.let { store.queue(it, policy) }
    val isLive: Boolean get() = queue != null

    /** 送り待ちの件数(画面に小さく出す)。 */
    var pending: Int by mutableStateOf(0)
        private set

    /** 断られて端末の行が戻った時に、画面の記録を読み直す口(DesktopSession が付ける)。 */
    @Volatile
    var onReload: ((MockStore) -> Unit)? = null

    private var models: List<Entity> = emptyList()
    private val writes = Channel<suspend () -> Unit>(Channel.UNLIMITED)

    init {
        scope.launch {
            for (w in writes) runCatching { w() }.onFailure { System.err.println("[TailorIt] 記録の保存に失敗: ${it::class.simpleName}") }
        }
        queue?.let { q ->
            // 送り始める前に受け口を付ける(SharedFlow は後から来た人に過去を渡さない)
            scope.launch(start = CoroutineStart.UNDISPATCHED) {
                q.events.collect { ev ->
                    pending = q.count()
                    if (ev.needsReload()) onReload?.let { cb -> submit { cb(load(models)) } }
                }
            }
        }
    }

    /**
     * 端末に残っている記録を読む(DSL の entity ごと・作った順)。
     * createdAt は画面には渡さない(その場で作った記録と同じく並び順で比べる。混ぜると新しい記録が古い側に並ぶ)。
     */
    suspend fun load(models: List<Entity>): MockStore {
        this.models = models
        val rows = models.associate { m ->
            m.id to store.records.rows(appId, m.id).filter { it.pendingOp != PendingOp.DELETE }.reversed().map { r ->
                Row(r.recordId, r.data.mapValues { MockStore.cell(JSONValue.from(it.value)) }, author = r.author)
            }
        }.filterValues { it.isNotEmpty() }
        val next = rows.values.flatten().mapNotNull { NEW_ID.matchEntire(it.id)?.groupValues?.get(1)?.toIntOrNull() }.maxOrNull()?.plus(1) ?: 1
        refreshPending()
        return MockStore(rows, next)
    }

    /** 新しい記録の仮 id(送れるとサーバーの id に替わる。替わる前の id でも引ける)。 */
    fun newRecordId(): String = LocalRecord.temporaryId(UUID.randomUUID().toString().lowercase())

    /** 起動時: 前回送れずに残った送り待ちを送り直す(送り先がある時だけ)。 */
    fun resendPending(): Job? {
        val q = queue ?: return null
        return scope.launch {
            refreshPending()
            q.drain()
            refreshPending()
        }
    }

    /** 画面の記録が変わった。前後の差を端末に書き、送り先があれば積んで送る。 */
    fun changed(models: List<Entity>, old: MockStore, new: MockStore) {
        if (old == new) return
        this.models = models
        val changes = diff(old, new)
        if (changes.isEmpty()) return
        submit {
            changes.forEach { apply(it) }
            refreshPending()
            queue?.let { q -> scope.launch { q.drain(); refreshPending() } }
        }
    }

    /** サインアウト・端末のデータを消す: [DesktopStore.wipe]。前に積んだ書き込みを流し終えてから消す。 */
    suspend fun wipe() {
        idle()
        store.wipe()
        pending = 0
    }

    /** ここまでに積んだ書き込みが終わるのを待つ。 */
    suspend fun idle() {
        val done = CompletableDeferred<Unit>()
        if (writes.trySend { done.complete(Unit) }.isFailure) return
        done.await()
    }

    /** 書き込みの列を閉じる(積んだ分は書き終える)。アプリを替えた時。 */
    fun close() {
        writes.close()
    }

    private fun submit(w: suspend () -> Unit) {
        writes.trySend(w)
    }

    private sealed interface Change {
        val entity: String

        data class Put(override val entity: String, val row: Row, val created: Boolean) : Change
        data class Delete(override val entity: String, val id: String) : Change
    }

    private fun diff(old: MockStore, new: MockStore): List<Change> = buildList {
        for (entity in old.rows.keys + new.rows.keys) {
            val before = old.list(entity).associateBy { it.id }
            val after = new.list(entity)
            for (row in after) {
                val prev = before[row.id]
                if (prev == null) add(Change.Put(entity, row, created = true))
                else if (prev.data != row.data) add(Change.Put(entity, row, created = false))
            }
            val kept = after.mapTo(HashSet()) { it.id }
            before.keys.filterNot { it in kept }.forEach { add(Change.Delete(entity, it)) }
        }
    }

    private fun json(entity: String, row: Row): JsonObject {
        val model = models.firstOrNull { it.id == entity }
        return JsonObject(row.data.mapValues { (k, v) -> typedValue(v, model?.field(k)?.type).toJsonElement() })
    }

    private suspend fun apply(c: Change) {
        val records = store.records
        val now = Instant.now().toString()
        when (c) {
            is Change.Put -> {
                val data = json(c.entity, c.row)
                val current = if (c.created) null else records.current(appId, c.entity, c.row.id)
                if (current == null) {
                    val clientId = c.row.id.takeIf { it.startsWith(LocalRecord.TEMP_PREFIX) }?.removePrefix(LocalRecord.TEMP_PREFIX)
                        ?: UUID.randomUUID().toString().lowercase()
                    records.upsert(
                        LocalRecord(
                            appId = appId, entity = c.entity, recordId = c.row.id, data = data,
                            createdAt = c.row.createdAt?.takeIf { it.isNotEmpty() } ?: now, updatedAt = now,
                            pendingOp = if (isLive) PendingOp.CREATE else PendingOp.NONE,
                            clientId = clientId.takeIf { isLive }, author = c.row.author ?: uid,
                        ),
                    )
                    queue?.enqueue(SyncOperation(id = clientId, appId = appId, entity = c.entity, recordId = c.row.id, kind = PendingOp.CREATE, data = data))
                } else {
                    val op = when {
                        !isLive -> PendingOp.NONE
                        current.pendingOp == PendingOp.CREATE -> PendingOp.CREATE
                        else -> PendingOp.UPDATE
                    }
                    records.upsert(current.copy(data = data, updatedAt = now, pendingOp = op))
                    queue?.enqueue(SyncOperation(appId = appId, entity = c.entity, recordId = c.row.id, kind = PendingOp.UPDATE, data = data))
                }
            }
            is Change.Delete -> {
                val current = records.current(appId, c.entity, c.id) ?: return
                val q = queue
                if (q == null) {
                    records.remove(appId, c.entity, current.recordId)
                } else if (q.enqueue(SyncOperation(appId = appId, entity = c.entity, recordId = c.id, kind = PendingOp.DELETE)) == SyncQueue.EnqueueResult.CANCELLED_CREATE) {
                    records.remove(appId, c.entity, current.recordId) // まだ送っていない作成は送らずに消す
                } else {
                    records.markDeleting(appId, c.entity, c.id)
                }
            }
        }
    }

    private suspend fun refreshPending() {
        pending = queue?.count() ?: 0
    }

    private fun SyncEvent.needsReload(): Boolean = when (this) {
        is SyncEvent.Sent, is SyncEvent.Drained, is SyncEvent.Paused -> false
        else -> true // 断られた: 端末の行を戻した・消したので読み直す
    }

    companion object {
        private val NEW_ID = Regex("new-(\\d+)")

        /** 送るのは `TAILORIT_API=stg` と `TAILORIT_DEV_TOKEN` が両方ある時だけ(#1860)。それ以外はモック(通信しない)。 */
        fun client(config: DesktopApiConfig, store: DesktopStore): AppConfigClient? =
            if (config.isLive && !config.devToken.isNullOrEmpty()) DesktopApi.appConfig(config, store.documents) else null

        /**
         * 起動時に作る(#1887)。記録・送り待ちはまだ読まない(読むのは [load])ので画面を止めない。
         * 読み終えた後の送り直し([resendPending])まで含めた起動の流れは SharedRecords.opening。
         */
        fun create(
            store: DesktopStore,
            appId: String,
            config: DesktopApiConfig = DesktopApiConfig.fromEnvironment(),
        ): SessionPersistence {
            store.lastAppId = appId
            return SessionPersistence(store, appId, client(config, store))
        }
    }
}
