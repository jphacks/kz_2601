package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.MarkState
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordPermission
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.ExternalUrlPolicy
import jp.kz2601.metamon.renderer.NavigationHost
import jp.kz2601.metamon.renderer.components.ai.AiHost
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Presentation
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.services.http.RecordWriteDenial
import jp.kz2601.metamon.services.local.DeletionTicket
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncEvent
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.JsonObject
import jp.kz2601.metamon.services.Record as StoredRecord

/** 確認ダイアログ(confirmMessage 付きのアクション)。主ボタンで [perform]。 */
class Confirm(
    val message: String,
    val primaryLabel: String,
    val isDestructive: Boolean,
    internal val perform: suspend () -> Unit,
)

/** 画面が読む状態(iOS の AppRuntime の @Observable なプロパティ)。遷移は [AppRuntime.navigation] が別に持つ。 */
data class RuntimeState(
    val document: AppDocument,
    val records: Map<String, List<Record>> = emptyMap(),
    val loading: Set<String> = emptySet(),
    val failed: Set<String> = emptySet(),
    val toast: String? = null,
    val confirm: Confirm? = null,
    val showVoiceSheet: Boolean = false,
    /** 「削除しました・元に戻す」(M11) */
    val recordUndo: DeletionTicket? = null,
) {
    val dsl: AppDSL get() = document.dsl
}

/**
 * アプリ全体の単一ランタイム(iOS の `AppRuntime`)。DSL・記録の表示用キャッシュ・遷移・アクションの実行を持ち、
 * 描画器には [DSLRuntime] として見せる。Android の部品(Context など)には触らない(端末の機能は [RuntimeHost])。
 *
 * - 記録: [RecordSyncEngine](ローカル DB に即反映・送信は列)
 * - 遷移: [NavigationResolver]
 * - 起動: キャッシュ → サーバーから復元([restore])→ 同期の開始
 */
class AppRuntime(
    document: AppDocument,
    val recordSync: RecordSyncEngine,
    private val scope: CoroutineScope,
    private val host: RuntimeHost = UnavailableHost,
    /** サインイン中の uid(共有の記録の作者判定 #249) */
    private val currentUserId: () -> String? = { null },
    /** 共有アプリに投稿する時の表示名(#250・未設定は null) */
    private val authorName: () -> String? = { null },
    /** サーバーの最新 DSL。投げたら手元の DSL のまま(オフライン) */
    private val restore: (suspend (appId: String) -> AppDocument)? = null,
    /** 削除を取り消せる時間(M11) */
    private val undoGraceMillis: Long = 5_000,
    /** aiButton のオンライン版・端末内 AI(#873)。null なら AI ボタンは理由つきで無効 */
    private val aiHost: AiHost? = null,
) : DSLRuntime, NavigationHost {
    private val _state = MutableStateFlow(RuntimeState(document))
    val state: StateFlow<RuntimeState> = _state.asStateFlow()

    private val _navigation = MutableStateFlow(NavigationState.initial(document.dsl))
    override val navigation: StateFlow<NavigationState> = _navigation.asStateFlow()
    override val navigationHost: NavigationHost get() = this

    /** 描画器のフォームが読む記録([RuntimeState.records] と同じもの)。 */
    private val _records = MutableStateFlow<Map<String, List<Record>>>(emptyMap())
    override val recordsFlow: StateFlow<Map<String, List<Record>>> = _records.asStateFlow()

    val document: AppDocument get() = _state.value.document
    val dsl: AppDSL get() = document.dsl
    val toast: String? get() = _state.value.toast

    private var entityCache: Pair<AppDSL, Map<String, Entity>>? = null
    private val savingForms = mutableSetOf<FormDraft>()
    private val savingLock = Mutex()
    private var eventsJob: Job? = null

    /** 最後に保存したフォーム。サーバーが断ったら「保存しました」を取り消す */
    private var lastSavedForm: SavedForm? = null

    private data class SavedForm(val appId: String, val entity: String, val recordId: String, val form: FormDraft)

    // 起動

    /** 同期を始め、サーバーの最新 DSL に合わせ、画面の entity の記録を読む(ローカル先出し → サーバー)。 */
    suspend fun bootstrap() {
        recordSync.start()
        if (eventsJob == null) eventsJob = scope.launch { recordSync.events.collect { handle(it) } }
        restore?.let { fetch ->
            try {
                val remote = fetch(document.appId)
                if (remote.version > document.version || (remote.version == document.version && document.version == 0)) apply(remote)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // 手元の DSL のまま(通信断はトーストしない。画面内の表示に任せる)
            }
        }
        load(dsl.entities.map { it.id }.toSet())
    }

    /** DSL を差し替える(別アプリ・生成の結果)。表示中の記録・遷移・モーダルは捨てる。 */
    fun apply(doc: AppDocument) {
        _state.value = RuntimeState(doc)
        _records.value = _state.value.records
        _navigation.value = NavigationState.initial(doc.dsl)
    }

    /** [apply] して、その DSL の entity の記録を読む(生成・差し替えの結果はここを通す。読まないと一覧が空のまま)。 */
    suspend fun show(doc: AppDocument) {
        apply(doc)
        load(doc.dsl.entities.map { it.id }.toSet())
    }

    // DSLRuntime

    override fun onVoiceInput(componentId: String?) {
        _state.update { it.copy(showVoiceSheet = true) }
    }

    override fun onAction(action: Action, context: ActionContext) {
        scope.launch { run(action, context) }
    }

    override fun setNavigation(next: NavigationState) {
        _navigation.value = next
    }

    // 画面の操作

    fun dismissToast() = _state.update { it.copy(toast = null) }

    fun dismissVoiceSheet() = _state.update { it.copy(showVoiceSheet = false) }

    fun showToast(message: String) = _state.update { it.copy(toast = message) }

    suspend fun confirmAccepted() {
        val confirm = _state.value.confirm ?: return
        _state.update { it.copy(confirm = null) }
        confirm.perform()
    }

    fun confirmDismissed() = _state.update { it.copy(confirm = null) }

    // 記録

    fun records(entity: String): List<Record> = _state.value.records[entity].orEmpty()

    /** DSL の entity(集計・権限・フォームが読む形)。 */
    override val entityModels: List<Entity> get() = dsl.entities.mapNotNull { entityModel(it.id) }

    override fun currentUid(): String? = currentUserId()

    override fun entityModel(id: String): Entity? {
        val current = dsl
        val cache = entityCache?.takeIf { it.first === current }?.second ?: current.entityModels()
            .associateBy { it.id }
            .also { entityCache = current to it }
        return cache[id]
    }

    override fun canEdit(entity: String, record: Record): Boolean = RecordPermission.canEdit(record, entityModel(entity), currentUserId())

    override fun canDelete(entity: String, record: Record): Boolean = RecordPermission.canDelete(record, entityModel(entity), currentUserId())

    suspend fun load(entities: Set<String>, force: Boolean = false) {
        for (e in entities) if (force || _state.value.records[e] == null) refresh(e)
    }

    /** ローカル先出し → サーバー全件で置換。届かなければローカル表示のまま(ローカルも空なら「読み込めませんでした」)。 */
    suspend fun refresh(entity: String) {
        val appId = document.appId
        if (_state.value.records[entity] == null) {
            val cached = recordSync.cached(appId, entity)
            if (cached.isNotEmpty()) setRecords(appId, entity, cached)
        }
        _state.update { it.copy(loading = it.loading + entity) }
        try {
            val fresh = recordSync.pull(appId, entity)
            if (document.appId != appId) return
            _state.update { it.copy(records = it.records + (entity to fresh.map { r -> r.toDsl() }), failed = it.failed - entity) }
            _records.value = _state.value.records
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            if (document.appId != appId) return
            if (_state.value.records[entity].isNullOrEmpty()) {
                _state.update { it.copy(records = it.records + (entity to emptyList()), failed = it.failed + entity) }
                _records.value = _state.value.records
            }
        } finally {
            _state.update { it.copy(loading = it.loading - entity) }
        }
    }

    /** 一覧のワンタップ更新(list.item.toggle)。表示中に無い記録は保存しない。 */
    suspend fun updateRecord(entity: String, recordId: String, data: Map<String, JSONValue>): Boolean {
        val current = record(entity, recordId) ?: return false
        if (!canEdit(entity, current)) {
            showToast(RuntimeCopy.READ_ONLY)
            return false
        }
        setRecords(document.appId, entity, recordSync.save(document.appId, entity, recordId, data.toJson()))
        return true
    }

    /** フォームを通さず 1 件足す。作った行の id(仮 id)。entity が無ければ null。 */
    suspend fun createRecord(entity: String, data: Map<String, JSONValue>): String? {
        if (dsl.entities.none { it.id == entity }) return null
        val (list, id) = write(entity, null, data)
        setRecords(document.appId, entity, list)
        return id
    }

    // AI ボタン(#873)

    override val ai: AiHost? get() = aiHost

    override suspend fun loadRecords(entities: Set<String>) = load(entities)

    override suspend fun addRecord(entity: String, data: Map<String, JSONValue>): Boolean = createRecord(entity, data) != null

    /** 一覧から消して「元に戻す」を出す。前の取り消し待ちは時間が来たら送られる。 */
    suspend fun deleteUndoably(entity: String, recordId: String) {
        val appId = document.appId
        val (list, ticket) = recordSync.deleteUndoably(appId, entity, recordId, undoGraceMillis)
        setRecords(appId, entity, list)
        _state.update { it.copy(recordUndo = ticket) }
    }

    suspend fun undoRecordDelete() {
        val ticket = _state.value.recordUndo ?: return
        _state.update { it.copy(recordUndo = null) }
        val back = recordSync.undoDelete(ticket) ?: return
        setRecords(ticket.appId, ticket.entity, back)
    }

    fun dismissRecordUndo() = _state.update { it.copy(recordUndo = null) }

    // 遷移

    private fun navigate(screen: String, presentation: Presentation, recordId: String?, from: Route) {
        _navigation.update { NavigationResolver.navigate(it, screen, presentation, recordId, from, dsl) }
    }

    fun close(route: Route) {
        _navigation.update { NavigationResolver.close(it, route) }
    }

    // アクション

    /** confirmMessage があれば先に確認を出す。 */
    suspend fun run(action: Action, context: ActionContext) {
        val message = action.confirmMessage
        if (message != null) {
            val (label, destructive) = RuntimeCopy.confirmButton(action.type)
            _state.update { it.copy(confirm = Confirm(message, label, destructive) { perform(action, context) }) }
            return
        }
        perform(action, context)
    }

    private suspend fun perform(action: Action, ctx: ActionContext) {
        when (action.type) {
            ActionType.NAVIGATE -> {
                val screen = action.screen ?: return
                val presentation = Presentation.of(action.presentation?.wire)
                val ref = action.prefillRef
                if (ref != null) {
                    // 親から子を作る(#256): 今の記録を子フォームの ref に積み、子は新規で開く
                    val parentId = ctx.currentRecordId ?: return showToast(unsavedParentToast(ctx))
                    FormPrefill.set(mapOf(ref to JSONValue.Str(parentId)))
                    navigate(screen, presentation, null, ctx.route)
                    return
                }
                navigate(screen, presentation, ctx.recordId, ctx.route)
            }

            ActionType.BACK -> close(ctx.route)

            ActionType.SAVE_RECORD -> saveRecord(action, ctx)

            ActionType.DELETE_RECORD -> {
                val entity = action.entity ?: ctx.form?.entity?.id ?: return
                val id = ctx.recordId ?: ctx.form?.recordId ?: return
                if (entityModel(entity)?.single == true) return // single の 1 件は消さない(#267)
                val current = record(entity, id)
                if (current != null && !canDelete(entity, current)) return showToast(RuntimeCopy.READ_ONLY)
                deleteUndoably(entity, id)
                if (ctx.recordId == null) close(ctx.route) // フォーム上からの削除は画面を閉じる
            }

            ActionType.OPEN_VOICE -> _state.update { it.copy(showVoiceSheet = true) }

            ActionType.OPEN_URL -> {
                val raw = action.url ?: return
                val url = ExternalUrlPolicy.openable(raw) ?: return showToast(RuntimeCopy.OPEN_URL_REJECTED)
                attempt(action.type) { host.openUrl(url) }
            }

            ActionType.RESET_FORM -> ctx.form?.reset()

            ActionType.FOLLOW, ActionType.UNFOLLOW ->
                attempt(action.type) { host.follow(action, ctx, on = action.type == ActionType.FOLLOW) }

            ActionType.SCHEDULE_NOTIFICATION -> {
                val spec = action.notification ?: return
                attempt(action.type) {
                    val next = host.scheduleNotification(spec, document.appId)
                    showToast(next?.let(RuntimeCopy::notificationScheduled) ?: RuntimeCopy.NOTIFICATION_FAILED)
                }
            }

            ActionType.CANCEL_NOTIFICATION -> {
                val id = action.notificationId ?: return
                attempt(action.type) { host.cancelNotification(id, document.appId) }
            }

            ActionType.EXPORT -> attempt(action.type) { host.export(action, ctx, this) }

            ActionType.START_ACTIVITY -> {
                val spec = action.activity ?: return
                attempt(action.type) { host.startActivity(spec, document.appId) }
            }

            ActionType.END_ACTIVITY -> attempt(action.type) { host.endActivity(document.appId) }

            ActionType.ADD_TO_CALENDAR -> attempt(action.type) { host.addToCalendar(action, ctx, this) }

            ActionType.LOOKUP_BOOK -> attempt(action.type) { host.lookupBook(action, ctx, this) }

            ActionType.UNKNOWN -> showToast(RuntimeCopy.unknownAction(action.type.wire))
        }
    }

    private suspend fun saveRecord(action: Action, ctx: ActionContext) {
        val form = ctx.form
        val entity = action.entity ?: form?.entity?.id
        if (form == null || entity == null) return showToast(RuntimeCopy.NO_FORM)
        val missing = form.flagMissingRequired()
        if (missing.isNotEmpty()) return showToast(RuntimeCopy.missingRequired(missing))
        // single(#267): 既存の 1 件があれば常にそれを上書き
        val recordId = form.recordId ?: if (entityModel(entity)?.single == true) records(entity).firstOrNull()?.id else null
        if (recordId != null) {
            val current = record(entity, recordId)
            if (current != null && !canEdit(entity, current)) return showToast(RuntimeCopy.READ_ONLY)
        }
        // 既存の記録を何も変えずに保存: 書かずに画面だけ進める(M10)
        if (!form.needsWrite) return leaveAfterSave(action, ctx)
        // 連打: 1 回目のローカル保存を待つ間の 2 回目は捨てる
        if (!savingLock.withLock { savingForms.add(form) }) return
        try {
            val appId = document.appId
            val (list, written) = write(entity, recordId, form.payload)
            setRecords(appId, entity, list)
            form.markSaved(queuedOffline = !recordSync.isOnline)
            lastSavedForm = SavedForm(appId, entity, written, form)
        } finally {
            savingLock.withLock { savingForms.remove(form) }
        }
        leaveAfterSave(action, ctx)
    }

    /** 保存の後: screen があればそこへ、無ければ閉じる。 */
    private fun leaveAfterSave(action: Action, ctx: ActionContext) {
        val screen = action.screen
        if (screen != null) navigate(screen, Presentation.of(action.presentation?.wire), null, ctx.route) else close(ctx.route)
    }

    /** prefillRef の親が未保存。 */
    fun unsavedParentToast(ctx: ActionContext): String = RuntimeCopy.saveParentFirst(ctx.form?.entity?.name.orEmpty())

    private suspend fun attempt(type: ActionType, body: suspend () -> Unit) {
        try {
            body()
        } catch (e: CancellationException) {
            throw e
        } catch (_: HostUnavailableException) {
            showToast(RuntimeCopy.UNAVAILABLE_ON_ANDROID)
        } catch (e: Exception) {
            showToast(message(e, null))
        }
    }

    // 書き込みの拒否

    /** 同期で保存が弾かれた時。項目が分かれば DSL の名前で出す(英語の field id・サーバーの文は出さない)。 */
    fun rejectedToast(entity: String, field: String?, message: String): String {
        field ?: return RuntimeCopy.REJECTED_GENERIC
        return RuntimeCopy.rejected(entityModel(entity)?.fields?.firstOrNull { it.id == field }?.name ?: field)
    }

    fun textNotAllowedToast(entity: String?, field: String?): String {
        field ?: return RuntimeCopy.TEXT_NOT_ALLOWED
        if (field == "authorName") return RuntimeCopy.textNotAllowed("表示名")
        val name = entity?.let { entityModel(it) }?.fields?.firstOrNull { it.id == field }?.name ?: return RuntimeCopy.TEXT_NOT_ALLOWED
        return RuntimeCopy.textNotAllowed(name)
    }

    fun denialMessage(denial: RecordWriteDenial, entity: String): String {
        val ent = entityModel(entity)
        fun name(id: String) = ent?.fields?.firstOrNull { it.id == id }?.name
        return when (denial) {
            is RecordWriteDenial.AdminsOnly -> denial.field?.let(::name)?.let(RuntimeCopy::adminsOnly) ?: RuntimeCopy.ADMINS_ONLY
            is RecordWriteDenial.Duplicate -> {
                if (denial.fields == listOf("\$author")) return RuntimeCopy.ONE_PER_PERSON
                val names = denial.fields.filter { it != "\$author" }.mapNotNull(::name)
                if (names.isEmpty()) RuntimeCopy.DUPLICATE else RuntimeCopy.duplicate(names)
            }
        }
    }

    /** 人に出す 1 文。OS・サーバーの文は出さない。 */
    fun message(error: Throwable, entity: String?): String {
        RecordWriteDenial.from(error)?.let { d -> entity?.let { return denialMessage(d, it) } }
        return RuntimeCopy.FAILED
    }

    /** 同期の列からの知らせ。送れたらローカルを読み直し、競合・拒否はトーストと読み直し。 */
    suspend fun handle(event: SyncEvent) {
        revokeSaveFeedback(event)
        when (event) {
            is SyncEvent.Sent -> reloadCached(event.appId, event.entity)
            is SyncEvent.Conflict -> ifCurrent(event.appId) {
                showToast(RuntimeCopy.CONFLICT)
                refresh(event.entity)
            }
            is SyncEvent.Forbidden -> ifCurrent(event.appId) {
                showToast(RuntimeCopy.READ_ONLY)
                refresh(event.entity)
            }
            is SyncEvent.Rejected -> ifCurrent(event.appId) {
                showToast(rejectedToast(event.entity, event.field, event.message))
                reloadCached(event.appId, event.entity)
            }
            is SyncEvent.Paused -> showToast(RuntimeCopy.paused(event.pending))
            SyncEvent.Drained -> Unit
            is SyncEvent.NotMember -> ifCurrent(event.appId) {
                showToast(RuntimeCopy.NOT_MEMBER)
                reloadCached(event.appId, event.entity)
            }
            is SyncEvent.Denied -> ifCurrent(event.appId) {
                showToast(denialMessage(event.denial, event.entity))
                refresh(event.entity)
            }
            is SyncEvent.AppStopped -> ifCurrent(event.appId) {
                showToast(RuntimeCopy.APP_STOPPED)
                reloadCached(event.appId, event.entity)
            }
            is SyncEvent.TextNotAllowed -> ifCurrent(event.appId) {
                showToast(textNotAllowedToast(event.entity, event.field))
                reloadCached(event.appId, event.entity)
            }
            // 課金画面・上限のカードは #316 #317 の Issue で出す。ここでは書いた分を戻すだけ
            is SyncEvent.ProRequiredEvent -> ifCurrent(event.appId) {
                showToast(RuntimeCopy.FAILED)
                reloadCached(event.appId, event.entity)
            }
            is SyncEvent.PlanLimitEvent -> ifCurrent(event.appId) {
                showToast(RuntimeCopy.FAILED)
                reloadCached(event.appId, event.entity)
            }
        }
    }

    /** 断られた書き込みが最後に保存したフォームそのものなら「保存しました」を取り消す。 */
    private fun revokeSaveFeedback(event: SyncEvent) {
        val (appId, entity, recordId) = refusedWrite(event) ?: return
        val saved = lastSavedForm ?: return
        if (recordId == null || saved.appId != appId || saved.entity != entity || saved.recordId != recordId) return
        saved.form.markSaveRefused()
        lastSavedForm = null
    }

    // 内部

    private fun record(entity: String, id: String): Record? = records(entity).firstOrNull { it.id == id }

    private suspend fun write(entity: String, recordId: String?, data: Map<String, JSONValue>): Pair<List<StoredRecord>, String> {
        val shared = entityModel(entity)?.isShared == true
        return recordSync.write(
            document.appId, entity, recordId, data.toJson(),
            authorUid = if (shared) currentUserId() else null,
            authorName = if (shared) authorName() else null,
            shared = shared,
        )
    }

    private suspend fun reloadCached(appId: String, entity: String) {
        if (appId != document.appId) return
        setRecords(appId, entity, recordSync.cached(appId, entity))
    }

    private fun setRecords(appId: String, entity: String, list: List<StoredRecord>) {
        if (appId != document.appId) return // 読む間に別のアプリへ切り替わった
        _state.update { it.copy(records = it.records + (entity to list.map { r -> r.toDsl() })) }
        _records.value = _state.value.records
    }

    private suspend fun ifCurrent(appId: String, body: suspend () -> Unit) {
        if (appId == document.appId) body()
    }

    companion object {
        fun refusedWrite(event: SyncEvent): Triple<String, String, String?>? = when (event) {
            is SyncEvent.Forbidden -> Triple(event.appId, event.entity, event.recordId)
            is SyncEvent.Rejected -> Triple(event.appId, event.entity, event.recordId)
            is SyncEvent.NotMember -> Triple(event.appId, event.entity, event.recordId)
            is SyncEvent.Denied -> Triple(event.appId, event.entity, event.recordId)
            is SyncEvent.ProRequiredEvent -> Triple(event.appId, event.entity, event.recordId)
            is SyncEvent.PlanLimitEvent -> Triple(event.appId, event.entity, event.recordId)
            is SyncEvent.AppStopped -> Triple(event.appId, event.entity, event.recordId)
            is SyncEvent.TextNotAllowed -> Triple(event.appId, event.entity, event.recordId)
            is SyncEvent.Sent, is SyncEvent.Conflict, is SyncEvent.Paused, SyncEvent.Drained -> null
        }
    }
}

/** :services の記録(JSON のまま)→ 描画・集計が読む記録。 */
internal fun StoredRecord.toDsl(): Record = Record(
    id = id,
    data = data.mapValues { JSONValue.from(it.value) },
    createdAt = createdAt,
    updatedAt = updatedAt,
    author = author,
    authorName = authorName,
    marks = marks?.mapValues { MarkState(it.value.count, it.value.mine) },
    unsent = id.startsWith("local-"),
)

internal fun Map<String, JSONValue>.toJson(): JsonObject = JsonObject(mapValues { it.value.toJsonElement() })
