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
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
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
    /** 止まった送り待ちを起こし直す間隔(ミリ秒)。null なら起こさない。 */
    private val retryEveryMillis: Long? = RETRY_EVERY_MILLIS,
) {
    val queue: SyncQueue? = client?.let { store.queue(it, policy) }
    val isLive: Boolean get() = queue != null

    /** 送り待ちの件数(このアプリの分だけ・画面に小さく出す)。 */
    var pending: Int by mutableStateOf(0)
        private set

    /**
     * 断られて端末の行が戻った時に、戻った行だけを画面の記録に混ぜる口(SharedRecords が付け、画面のスレッドで混ぜる)。
     * 書き込みの列の上で呼ぶ(この時点の端末は、画面から積んだ書き込みを全部終えている)。
     */
    @Volatile
    var onReload: ((Reload) -> Unit)? = null

    @Volatile
    private var models: List<Entity> = emptyList()
    private val writes = Channel<suspend () -> Unit>(Channel.UNLIMITED)

    /** 端末に書き終えた画面の記録(書き込みの列の上でだけ書き換える)。断られた分の差はこれと端末を比べて出す。 */
    @Volatile
    private var written: MockStore = MockStore()

    init {
        scope.launch {
            for (w in writes) runCatching { w() }.onFailure { System.err.println("[TailorIt] 記録の保存に失敗: ${it::class.simpleName}") }
        }
        queue?.let { q ->
            // 送り始める前に受け口を付ける(SharedFlow は後から来た人に過去を渡さない)
            scope.launch(start = CoroutineStart.UNDISPATCHED) {
                q.events.collect { ev ->
                    // 列は端末に 1 本でアプリをまたぐ。他のアプリの知らせは件数を数え直すだけ
                    refreshPending()
                    if (ev.appIdOrNull == appId && ev.needsReload()) submit { reload(ev) }
                }
            }
            // 続けて失敗して止まった列を起こす。取りこぼした起こし(SyncQueue.drain の lost wake-up)もここで拾う
            retryEveryMillis?.let { every ->
                scope.launch {
                    while (true) {
                        delay(every)
                        runCatching { if (q.count() > 0) q.resume() }
                        refreshPending()
                    }
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
        val rows = screenRows().mapValues { (_, list) -> list.map { it.second } }.filterValues { it.isNotEmpty() }
        val next = rows.values.flatten().mapNotNull { NEW_ID.matchEntire(it.id)?.groupValues?.get(1)?.toIntOrNull() }.maxOrNull()?.plus(1) ?: 1
        refreshPending()
        return MockStore(rows, next).also { written = it }
    }

    /**
     * 端末の行を画面の行にする(entity ごと・作った順・削除待ちは出さない)。
     * 送れた作成はサーバーの id に替わっているが、画面には作った時の仮 id(`local-<clientId>`)のまま出す(開いているウィンドウ・遷移が迷子にならない)。
     * 他の記録を指す値(ref)も同じ仮 id に揃える。
     */
    private suspend fun screenRows(): Map<String, List<Pair<LocalRecord, Row>>> {
        val local = models.associate { m -> m.id to store.records.rows(appId, m.id).filter { it.pendingOp != PendingOp.DELETE }.reversed() }
        val alias = local.values.flatten().mapNotNull { r -> screenId(r).takeIf { it != r.recordId }?.let { r.recordId to it } }.toMap()
        fun remap(v: JSONValue): JSONValue = when (v) {
            is JSONValue.Str -> alias[v.value]?.let { JSONValue.Str(it) } ?: v
            is JSONValue.Arr -> JSONValue.Arr(v.items.map(::remap))
            else -> v
        }
        return local.mapValues { (_, rows) ->
            rows.map { r -> r to Row(screenId(r), r.data.mapValues { MockStore.cell(remap(JSONValue.from(it.value))) }, author = r.author) }
        }
    }

    /**
     * 断られた(書き込みの列の上): 知らせが指す行だけ、端末に書き終えた画面([written])と今の端末の差を出して画面に渡す。
     * recordId の無い知らせ(競合)はその entity の差を全部。
     */
    private suspend fun reload(ev: SyncEvent) {
        val entity = ev.entityOrNull ?: return
        val target = ev.recordIdOrNull
        val disk = screenRows()[entity].orEmpty()
        fun named(r: LocalRecord?, id: String) = target == null || id == target || r?.isKnownAs(target) == true
        val before = written.list(entity).associateBy { it.id }
        val upserts = disk.filter { (r, row) -> named(r, row.id) && before[row.id]?.data != row.data }.map { it.second }
        val onDisk = disk.mapTo(HashSet()) { it.second.id }
        val removed = before.keys.filter { it !in onDisk && named(null, it) }.toSet()
        val delta = Reload(entity, upserts, removed)
        if (delta.isEmpty) return
        written = delta.applyTo(written)
        onReload?.invoke(delta)
    }

    /** 新しい記録の仮 id(送れるとサーバーの id に替わる。替わる前の id でも引ける)。 */
    fun newRecordId(): String = LocalRecord.temporaryId(UUID.randomUUID().toString().lowercase())

    /** 止まった送り待ちを起こして送る(ウィンドウが前に出た時。送り先がある時だけ)。 */
    fun resume(): Job? {
        val q = queue ?: return null
        return scope.launch {
            runCatching { q.resume() }
            refreshPending()
        }
    }

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
            written = written.applying(changes)
            refreshPending()
            // 続けて失敗して止まっていても、画面で書き換えたら起こして送る
            queue?.let { q -> scope.launch { runCatching { q.resume() }; refreshPending() } }
        }
    }

    /** サインアウト・端末のデータを消す: [DesktopStore.wipe]。前に積んだ書き込みを流し終えてから消す。 */
    suspend fun wipe() {
        idle()
        store.wipe()
        written = MockStore()
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

    /** そのアプリのウィンドウが全部閉じた: 書き終えるのを待って列を閉じ、裏の仕事(送り待ちの受け口・起こし直し)を止める。 */
    suspend fun shutdown() {
        idle()
        close()
        scope.cancel()
    }

    /** 裏の仕事が止まったか(テスト用)。 */
    internal val isShutDown: Boolean get() = !scope.coroutineContext[Job]!!.isActive

    private fun submit(w: suspend () -> Unit) {
        writes.trySend(w)
    }

    internal sealed interface Change {
        val entity: String

        data class Put(override val entity: String, val row: Row, val created: Boolean) : Change
        data class Delete(override val entity: String, val id: String) : Change
    }

    private fun MockStore.applying(changes: List<Change>): MockStore = changes.fold(this) { s, c ->
        when (c) {
            is Change.Put -> {
                val l = s.list(c.entity)
                s.copy(rows = s.rows + (c.entity to if (l.any { it.id == c.row.id }) l.map { if (it.id == c.row.id) c.row else it } else l + c.row))
            }
            is Change.Delete -> s.delete(c.entity, c.id)
        }
    }

    internal fun diff(old: MockStore, new: MockStore): List<Change> = buildList {
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
                    // 送れた作成も画面の仮 id で積む(列が仮 id → サーバー id を引く。送れた後も行に clientId が残り、読み直しても同じ id で出る)
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
        pending = queue?.operations()?.count { it.appId == appId } ?: 0
    }

    internal fun SyncEvent.needsReload(): Boolean = when (this) {
        is SyncEvent.Sent, is SyncEvent.Drained, is SyncEvent.Paused -> false
        else -> true // 断られた: 端末の行を戻した・消したので読み直す
    }

    /** 断られて戻った 1 つの entity の行: [upserts] は入れる・差し替える行、[removed] は消す行の id(どちらも画面の id)。 */
    data class Reload(val entity: String, val upserts: List<Row>, val removed: Set<String>) {
        val isEmpty: Boolean get() = upserts.isEmpty() && removed.isEmpty()

        /** 画面の記録に混ぜる。他の行(断られていない・後から書き換えた)はそのまま。 */
        fun applyTo(s: MockStore): MockStore {
            val byId = upserts.associateBy { it.id }
            val kept = s.list(entity).filterNot { it.id in removed }.map { byId[it.id] ?: it }
            val added = upserts.filter { u -> kept.none { it.id == u.id } }
            return s.copy(rows = s.rows + (entity to kept + added))
        }
    }

    companion object {
        private val NEW_ID = Regex("new-(\\d+)")

        /** 止まった送り待ちを起こし直す間隔。 */
        const val RETRY_EVERY_MILLIS = 30_000L

        /** 画面に出す id(送れた作成も仮 id のまま)。 */
        fun screenId(r: LocalRecord): String = r.clientId?.let(LocalRecord::temporaryId) ?: r.recordId

        /** 知らせのアプリ(止まった・空になったは列全体なので null)。 */
        val SyncEvent.appIdOrNull: String?
            get() = when (this) {
                is SyncEvent.Sent -> appId
                is SyncEvent.Conflict -> appId
                is SyncEvent.Rejected -> appId
                is SyncEvent.Forbidden -> appId
                is SyncEvent.NotMember -> appId
                is SyncEvent.ProRequiredEvent -> appId
                is SyncEvent.Denied -> appId
                is SyncEvent.PlanLimitEvent -> appId
                is SyncEvent.AppStopped -> appId
                is SyncEvent.TextNotAllowed -> appId
                is SyncEvent.Paused, SyncEvent.Drained -> null
            }

        val SyncEvent.entityOrNull: String?
            get() = when (this) {
                is SyncEvent.Sent -> entity
                is SyncEvent.Conflict -> entity
                is SyncEvent.Rejected -> entity
                is SyncEvent.Forbidden -> entity
                is SyncEvent.NotMember -> entity
                is SyncEvent.ProRequiredEvent -> entity
                is SyncEvent.Denied -> entity
                is SyncEvent.PlanLimitEvent -> entity
                is SyncEvent.AppStopped -> entity
                is SyncEvent.TextNotAllowed -> entity
                is SyncEvent.Paused, SyncEvent.Drained -> null
            }

        /** 断られた操作の記録(積んだ時の id)。競合は持たない。 */
        val SyncEvent.recordIdOrNull: String?
            get() = when (this) {
                is SyncEvent.Rejected -> recordId
                is SyncEvent.Forbidden -> recordId
                is SyncEvent.NotMember -> recordId
                is SyncEvent.ProRequiredEvent -> recordId
                is SyncEvent.Denied -> recordId
                is SyncEvent.PlanLimitEvent -> recordId
                is SyncEvent.AppStopped -> recordId
                is SyncEvent.TextNotAllowed -> recordId
                else -> null
            }

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
