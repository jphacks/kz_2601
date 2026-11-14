package jp.kz2601.metamon.services.local

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.PlanLimit
import jp.kz2601.metamon.services.http.ProRequired
import jp.kz2601.metamon.services.http.RecordWriteDenial
import jp.kz2601.metamon.services.http.SharedAccessDenial
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.io.File
import java.util.UUID
import kotlin.coroutines.CoroutineContext
import kotlin.math.pow

/** appconfig へ送る 1 操作(iOS の `SyncOperation`)。`id` が作成の `clientId`(冪等キー)。ファイルに残し再起動後も送る。 */
@Serializable
data class SyncOperation(
    val id: String = UUID.randomUUID().toString().lowercase(),
    val appId: String,
    val entity: String,
    /** 作成は仮 id(`local-<clientId>`)。送れた後は [SyncQueue] が仮 id → サーバー id を覚える */
    val recordId: String,
    val kind: PendingOp,
    val data: JsonObject = JsonObject(emptyMap()),
    val attempts: Int = 0,
    val enqueuedAtMillis: Long = System.currentTimeMillis(),
    /** 作成で送る投稿者の表示名(#250)。null なら送らない(backend が「匿名」を付ける) */
    val authorName: String? = null,
)

/** 列から画面への知らせ(iOS の `SyncEvent`)。断られた操作の `recordId` は積んだ時の id(作成なら仮 id)。 */
sealed interface SyncEvent {
    /** 1 件送れた。ローカル DB は更新済みなので entity を読み直せばよい */
    data class Sent(val appId: String, val entity: String) : SyncEvent

    /** 競合(404 / 409 / 412)。サーバー優先で未送信の印を落とした。entity をサーバーから引き直す */
    data class Conflict(val appId: String, val entity: String, val message: String) : SyncEvent

    /** 内容の拒否(その他の 4xx)。作成はローカルの行も消した。`field` は弾かれた Field.id */
    data class Rejected(
        val appId: String, val entity: String, val message: String,
        val field: String? = null, val recordId: String? = null,
    ) : SyncEvent

    /** 共有の記録を作った人以外が変えようとした(403・#249)。entity をサーバーから引き直す */
    data class Forbidden(val appId: String, val entity: String, val recordId: String? = null) : SyncEvent

    /** 共有のデータに参加していない(403 `not_member`・#254) */
    data class NotMember(val appId: String, val entity: String, val recordId: String? = null) : SyncEvent

    /** 共有の記録の書き込みに Pro が要る(402・#316) */
    data class ProRequiredEvent(val appId: String, val entity: String, val pro: ProRequired, val recordId: String? = null) : SyncEvent

    /** アプリの決まりで断られた(#272 管理者だけ・#273 同じ内容) */
    data class Denied(val appId: String, val entity: String, val denial: RecordWriteDenial, val recordId: String? = null) : SyncEvent

    /** プランの上限(403 `plan_limit_reached`・#317) */
    data class PlanLimitEvent(val appId: String, val entity: String, val limit: PlanLimit, val recordId: String? = null) : SyncEvent

    /** 運営がアプリを止めている(403 `app_stopped`・#346) */
    data class AppStopped(val appId: String, val entity: String, val recordId: String? = null) : SyncEvent

    /** 他の人に見える文字に表示できない言葉がある(422 `text_not_allowed`・#321) */
    data class TextNotAllowed(val appId: String, val entity: String, val field: String?, val recordId: String? = null) : SyncEvent

    /** 続けて失敗して上限に達した。次のきっかけ(オンライン復帰・前面復帰・今すぐ同期)まで止める */
    data class Paused(val pending: Int) : SyncEvent

    /** 列が空になった */
    data object Drained : SyncEvent
}

/** 指数バックオフ: base × multiplier^(attempt-1)。attempt は 1 始まり。 */
data class BackoffPolicy(val baseMillis: Long = 500, val multiplier: Double = 2.0, val maxAttempts: Int = 5) {
    fun delayMillis(attempt: Int): Long = (baseMillis * multiplier.pow((attempt - 1).coerceAtLeast(0))).toLong()

    companion object {
        val immediate = BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 5)
    }
}

/**
 * 未送信の操作の直列の列(iOS の `SyncQueue`)。先頭から 1 件ずつ・失敗は指数バックオフで同じ操作を再試行(最大 `maxAttempts`)。
 * 同じ記録への操作は積む時に畳む(作成+更新 → 作成 / 作成+削除 → 消す / 更新+更新 → 1 つ)。
 *
 * @param directory `queue.json` の置き場。null ならメモリだけ(テスト用)
 * @param sleep バックオフ待ちの差し替え口
 */
class SyncQueue(
    directory: File?,
    private val store: RecordStore,
    private val client: AppConfigClient,
    private val policy: BackoffPolicy = BackoffPolicy(),
    private val sleep: suspend (Long) -> Unit = { delay(it) },
    private val io: CoroutineContext = Dispatchers.IO,
) {
    enum class EnqueueResult { APPENDED, MERGED, CANCELLED_CREATE }

    private val file: File? = directory?.resolve("queue.json")
    private val lock = Mutex()
    private var ops = mutableListOf<SyncOperation>()

    /** 仮 id → サーバー id(作成を送った後に同じ記録への更新・削除が来た時に引き直す) */
    private val aliases = mutableMapOf<String, String>()
    private var inFlight: String? = null
    private var loaded = false

    @Volatile
    var isDraining = false
        private set

    /** 列が空と見て抜ける直前に呼ぶ割り込み口。テストが「抜けぎわの enqueue」の順序を作るために使う。 */
    internal var onIdle: (suspend () -> Unit)? = null

    @Volatile
    var isPaused = false
        private set

    private val _events = MutableSharedFlow<SyncEvent>(extraBufferCapacity = 64)
    val events: SharedFlow<SyncEvent> = _events.asSharedFlow()

    @Serializable
    private data class Persisted(val ops: List<SyncOperation> = emptyList(), val aliases: Map<String, String> = emptyMap())

    suspend fun count(): Int = lock.withLock { load(); ops.size }

    suspend fun operations(): List<SyncOperation> = lock.withLock { load(); ops.toList() }

    // 積む

    suspend fun enqueue(op: SyncOperation): EnqueueResult = lock.withLock {
        load()
        fun same(o: SyncOperation) = o.appId == op.appId && o.entity == op.entity && o.recordId == op.recordId && o.id != inFlight
        val result = when (op.kind) {
            PendingOp.UPDATE -> {
                val i = ops.indexOfFirst { same(it) && (it.kind == PendingOp.CREATE || it.kind == PendingOp.UPDATE) }
                if (i >= 0) {
                    ops[i] = ops[i].copy(data = JsonObject(ops[i].data + op.data))
                    EnqueueResult.MERGED
                } else null
            }
            PendingOp.DELETE -> {
                val i = ops.indexOfFirst { same(it) && it.kind == PendingOp.CREATE }
                if (i >= 0) {
                    ops.removeAt(i)
                    ops.removeAll { same(it) && it.kind == PendingOp.UPDATE }
                    EnqueueResult.CANCELLED_CREATE
                } else {
                    ops.removeAll { same(it) && it.kind == PendingOp.UPDATE }
                    null
                }
            }
            PendingOp.CREATE, PendingOp.NONE -> null
        }
        if (result == null) ops += op
        persist()
        result ?: EnqueueResult.APPENDED
    }

    /** appId の未送信を捨てる(アプリ削除 #180)。送っている 1 件は止められないので残す。 */
    suspend fun discard(appId: String): Int = lock.withLock {
        load()
        val before = ops.size
        ops.removeAll { it.appId == appId && it.id != inFlight }
        persist()
        before - ops.size
    }

    /** 1 件の記録の未送信を捨てる(wipe を跨いだ書き込みを取り消す)。送っている 1 件は止められないので残す。 */
    suspend fun discard(appId: String, entity: String, recordId: String): Int = lock.withLock {
        load()
        val before = ops.size
        ops.removeAll { it.appId == appId && it.entity == entity && it.recordId == recordId && it.id != inFlight }
        persist()
        before - ops.size
    }

    /** アカウント削除(#119): 未送信を全部捨てる。 */
    suspend fun discardAll() = lock.withLock {
        load()
        ops.clear()
        aliases.clear()
        file?.let { f -> withContext(io) { f.delete() } }
    }

    // 流す

    /** 停止を解いて流す。オンライン復帰・前面復帰・「今すぐ同期」から呼ぶ。 */
    suspend fun resume() {
        lock.withLock {
            load()
            isPaused = false
            if (ops.isNotEmpty()) ops[0] = ops[0].copy(attempts = 0)
        }
        drain()
    }

    /** 先頭から順に送る。空になるか、続けて失敗して止まるまで戻らない。同時に 2 本は走らない。 */
    suspend fun drain() {
        lock.withLock {
            load()
            if (isDraining || isPaused) return
            isDraining = true
        }
        var draining = true
        try {
            while (true) {
                // 「空なら isDraining を落として抜ける」を 1 つの lock で行い、抜けぎわに積まれた操作を取りこぼさない
                val op = lock.withLock {
                    val next = ops.firstOrNull()
                    if (next == null) {
                        isDraining = false
                        draining = false
                    } else {
                        inFlight = next.id
                    }
                    next
                } ?: break
                try {
                    perform(op)
                    finish(op)
                    _events.emit(SyncEvent.Sent(op.appId, op.entity))
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Throwable) {
                    val refusal = refusal(e, op, rowId(op))
                    if (refusal != null) {
                        store.revertPending(op.appId, op.entity, refusal.second)
                        finish(op)
                        println("[Metamon] sync ${op.kind} ${op.entity}/${op.recordId} refused: ${refusal.first}")
                        _events.emit(refusal.first)
                        continue
                    }
                    val attempt = lock.withLock {
                        val i = ops.indexOfFirst { it.id == op.id }
                        if (i < 0) return@withLock 0
                        ops[i] = ops[i].copy(attempts = ops[i].attempts + 1)
                        persist()
                        ops[i].attempts
                    }
                    println("[Metamon] sync ${op.kind} ${op.entity}/${op.recordId} failed (attempt $attempt/${policy.maxAttempts}): $e")
                    if (attempt >= policy.maxAttempts) {
                        // 止めると同時に isDraining も落とす。Paused を受けてすぐ resume された時の起こしを取りこぼさない
                        lock.withLock {
                            isPaused = true
                            isDraining = false
                            draining = false
                            inFlight = null
                        }
                        _events.emit(SyncEvent.Paused(count()))
                        return
                    }
                    sleep(policy.delayMillis(attempt)) // 止められたら次のきっかけで続き
                } finally {
                    // 止めた時は上で戻し済み(次の drain がもう持っているかもしれない)
                    if (draining) lock.withLock { inFlight = null }
                }
            }
            onIdle?.invoke()
            _events.emit(SyncEvent.Drained)
        } finally {
            if (draining) lock.withLock { isDraining = false }
        }
    }

    /** 断られた時の知らせと、未送信の印を戻す行の id。再試行すべき失敗なら null。 */
    private suspend fun refusal(e: Throwable, op: SyncOperation, row: String): Pair<SyncEvent, String>? {
        val (a, en, rid) = Triple(op.appId, op.entity, op.recordId)
        if (SharedAccessDenial.from(e) == SharedAccessDenial.NOT_MEMBER) return SyncEvent.NotMember(a, en, rid) to row
        (e as? ApiError)?.let { api ->
            api.planLimit?.let { return SyncEvent.PlanLimitEvent(a, en, it, rid) to row }
            if (api.isAppStopped) return SyncEvent.AppStopped(a, en, rid) to row
            if (api.isTextNotAllowed) return SyncEvent.TextNotAllowed(a, en, api.field, rid) to row
        }
        RecordWriteDenial.from(e)?.let { return SyncEvent.Denied(a, en, it, rid) to op.recordId }
        ProRequired.from(e)?.let { return SyncEvent.ProRequiredEvent(a, en, it, rid) to row }
        return when (val o = classify(e)) {
            Outcome.Transient -> null
            Outcome.Conflict -> SyncEvent.Conflict(a, en, e.message ?: e.toString()) to row
            Outcome.Forbidden -> SyncEvent.Forbidden(a, en, rid) to row
            is Outcome.Rejected -> SyncEvent.Rejected(a, en, o.message, (e as? ApiError)?.field, rid) to row
        }
    }

    private suspend fun finish(op: SyncOperation) = lock.withLock {
        ops.removeAll { it.id == op.id }
        persist()
    }

    private suspend fun perform(op: SyncOperation) {
        val (serverId, data) = lock.withLock { (aliases[op.recordId] ?: op.recordId) to resolvingAliases(op.data) }
        when (op.kind) {
            PendingOp.CREATE -> {
                val rec = client.createRecord(op.appId, op.entity, data, clientId = op.id, authorName = op.authorName)
                lock.withLock { aliases[op.recordId] = rec.id; persist() }
                store.markSynced(op.appId, op.entity, op.recordId, rec)
            }
            PendingOp.UPDATE -> {
                val rec = client.updateRecord(op.appId, op.entity, serverId, data)
                store.markSynced(op.appId, op.entity, op.recordId, rec)
            }
            PendingOp.DELETE -> {
                try {
                    client.deleteRecord(op.appId, op.entity, serverId)
                } catch (e: ApiError) {
                    if (e.status != 404) throw e // もう無い = 目的は達した
                }
                store.remove(op.appId, op.entity, serverId)
                lock.withLock { aliases.remove(op.recordId); persist() }
            }
            PendingOp.NONE -> Unit
        }
    }

    /** 操作が指すローカル行の id。積んだ後に作成が届くと、行は仮 id からサーバー id に替わっている。 */
    private suspend fun rowId(op: SyncOperation): String = lock.withLock { aliases[op.recordId] ?: op.recordId }

    /** 値に残った仮 id を送れたサーバー id に差し替える(未送信のメンバーを選んだ ref など)。lock の中で呼ぶ。 */
    internal fun resolvingAliases(data: JsonObject): JsonObject = JsonObject(
        data.mapValues { (_, v) ->
            val s = (v as? JsonPrimitive)?.takeIf { it.isString }?.content
            val real = s?.takeIf { it.startsWith(LocalRecord.TEMP_PREFIX) }?.let(aliases::get)
            if (real != null) JsonPrimitive(real) else v
        },
    )

    // 永続化(lock の中で呼ぶ)

    private suspend fun load() {
        if (loaded) return
        loaded = true
        val f = file ?: return
        val saved = withContext(io) {
            runCatching { if (f.exists()) RecordStore.json.decodeFromString(Persisted.serializer(), f.readText()) else null }.getOrNull()
        } ?: return
        ops = saved.ops.toMutableList()
        aliases.putAll(saved.aliases)
    }

    private suspend fun persist() {
        val f = file ?: return
        val text = RecordStore.json.encodeToString(Persisted.serializer(), Persisted(ops.toList(), aliases.toMap()))
        withContext(io) {
            runCatching { AtomicFile.write(f, text) }.onFailure { println("[Metamon] SyncQueue persist failed: $it") }
        }
    }

    sealed interface Outcome {
        data object Transient : Outcome
        data object Conflict : Outcome
        data object Forbidden : Outcome
        data class Rejected(val message: String) : Outcome
    }

    companion object {
        /** 通信断・5xx・401(トークン更新待ち)・408・425・429 は再試行。404/409/412 は競合(サーバー優先)。403 は作った人以外。他の 4xx は内容の拒否。 */
        fun classify(error: Throwable): Outcome {
            val api = error as? ApiError ?: return Outcome.Transient
            return when (val s = api.status) {
                404, 409, 412 -> Outcome.Conflict
                403 -> Outcome.Forbidden
                401, 408, 425, 429 -> Outcome.Transient
                in 400..499 -> Outcome.Rejected(api.message ?: "HTTP $s")
                else -> Outcome.Transient
            }
        }
    }
}
