package jp.kz2601.metamon.runtime

import androidx.annotation.StringRes
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.InterestState
import jp.kz2601.metamon.dsl.records.MarkState
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordPermission
import jp.kz2601.metamon.nearby.NearbyShareController
import jp.kz2601.metamon.notifications.TimerActivityUpdate
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.components.media.PhotoReader
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.form.FormDraftRestore
import jp.kz2601.metamon.renderer.form.FormDraftStorage
import jp.kz2601.metamon.renderer.EntityLoadState
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.FormMember
import jp.kz2601.metamon.renderer.components.audio.DictationHost
import jp.kz2601.metamon.renderer.components.importing.AudioFileTranscriptionHost
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.renderer.NavigationHost
import jp.kz2601.metamon.renderer.components.nearby.NearbyHost
import jp.kz2601.metamon.renderer.components.nearby.NearbyPassbyController
import jp.kz2601.metamon.renderer.components.play.RecordWriter
import jp.kz2601.metamon.renderer.components.play.TimerHost
import jp.kz2601.metamon.renderer.components.play.TimerRun
import jp.kz2601.metamon.renderer.components.ai.AiChatPairSaving
import jp.kz2601.metamon.renderer.components.ai.AiHost
import jp.kz2601.metamon.renderer.components.social.AuthorNameHost
import jp.kz2601.metamon.renderer.components.social.BlockedUsersHost
import jp.kz2601.metamon.renderer.components.social.FollowsHost
import jp.kz2601.metamon.renderer.components.social.MarksHost
import jp.kz2601.metamon.renderer.components.social.MatchingHost
import jp.kz2601.metamon.renderer.components.social.ModerationHost
import jp.kz2601.metamon.renderer.components.social.ReportReasonOption
import jp.kz2601.metamon.renderer.components.social.SocialPrompt
import jp.kz2601.metamon.renderer.components.social.SocialPromptHost
import jp.kz2601.metamon.renderer.form.DiscardGuard
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Presentation
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.sharing.AppMembersStore
import jp.kz2601.metamon.services.http.RecordWriteDenial
import jp.kz2601.metamon.services.local.DeletionTicket
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncEvent
import jp.kz2601.metamon.services.social.AuthorProfile
import jp.kz2601.metamon.services.sync.HotReload
import jp.kz2601.metamon.services.sync.HotReloadStatus
import jp.kz2601.metamon.services.sync.RecordsChange
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.JsonObject
import jp.kz2601.metamon.services.Record as StoredRecord

/** 確認ダイアログ(confirmMessage 付きのアクション)。主ボタンで [perform]。 */
class Confirm(
    val message: String,
    @StringRes val primaryLabel: Int,
    val isDestructive: Boolean,
    internal val perform: suspend () -> Unit,
)

/** 画面が読む状態(iOS の AppRuntime の @Observable なプロパティ)。遷移は [AppRuntime.navigation] が別に持つ。 */
data class RuntimeState(
    val document: AppDocument,
    val records: Map<String, List<Record>> = emptyMap(),
    val loading: Set<String> = emptySet(),
    val failed: Set<String> = emptySet(),
    val toast: UiText? = null,
    val confirm: Confirm? = null,
    val showVoiceSheet: Boolean = false,
    /** 「削除しました・元に戻す」(M11) */
    val recordUndo: DeletionTicket? = null,
    /** 起動時にサーバーへ届かなかった(手元・保存済みの DSL を表示中)。つながって取り直せたら false */
    val offline: Boolean = false,
    /**
     * [offline] のうち保存した DSL も無かった(iOS の unreachable の状態カード・#1107)。画面に消えないカードを出す。
     * つながって取り直せたら false
     */
    val unreachable: Boolean = false,
    /** 上部のバナー(iOS の summaryBanner)。ホットリロードの「最新の内容に更新しました」(#1108) */
    val banner: RuntimeBanner? = null,
) {
    val dsl: AppDSL get() = document.dsl
}

/** 上部のバナーの 1 回分。同じ文でも [id] が変われば出し直す・読み直す。 */
data class RuntimeBanner(val id: Long, val text: UiText)

/**
 * アプリ全体の単一ランタイム(iOS の `AppRuntime`)。DSL・記録の表示用キャッシュ・遷移・アクションの実行を持ち、
 * 描画器には [DSLRuntime] として見せる。Android の部品(Context など)には触らない(端末の機能は [RuntimeHost])。
 *
 * - 記録: [RecordSyncEngine](ローカル DB に即反映・送信は列)
 * - 遷移: [NavigationResolver]
 * - 起動: サーバーから復元([restore])。届かなければ保存した DSL([cachedDocument])→ ホットリロードの開始
 * - ホットリロード: [HotReload] の新しい DSL で描き直し、記録の変化で該当 entity を取り直す。前面・背面は [setForeground]
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
    /** 削除を取り消せる時間(M11)。削除のたびに読む(TalkBack の時は長く・H8) */
    private val undoGraceMillis: () -> Long = { 5_000 },
    /** 他端末・共有の相手の変更(#883)。null ならホットリロードしない(モック・テスト) */
    private val hotReloader: HotReload? = null,
    /** 最後に取得した DSL(`DocumentCache`)。[restore] が届かない時の起動に使う(#883) */
    private val cachedDocument: (suspend (appId: String) -> AppDocument?)? = null,
    /** 記録の変化の知らせをまとめる間(連投で取り直しを重ねない) */
    private val recordsRefreshDelayMillis: Long = 250,
    /** 今のアプリの appId の保存・アプリの一覧・新しいアプリ(#881)。null なら appId は固定(モック・テスト) */
    private val directory: AppDirectory? = null,
    /** aiButton のオンライン版・端末内 AI(#873)。null なら AI ボタンは理由つきで無効 */
    private val aiHost: AiHost? = null,
    /** 共有アプリへの投稿に出す名前(#875)。null なら名前を聞かず、[authorName] だけを使う(モック・テスト) */
    private val authorProfile: AuthorProfile? = null,
    /** 仲間探し・通報・ブロック(#877)。null なら出さない(モック・テスト) */
    socialServices: SocialServices? = null,
    /** 記録の保存期間(#482)。null なら自動で消さない(モック・テスト) */
    val retention: RecordRetentionRunner? = null,
    /** 共有アプリの参加者と自分が管理者か(#903)。null なら読まない(モック・テスト) */
    val members: AppMembersStore? = null,
    /** プロセスが落とされた後に戻すフォームの書きかけ(#1185)。既定は端末に残さない(テスト) */
    override val formDrafts: FormDraftRestore = FormDraftRestore(FormDraftStorage.InMemory()),
) : DSLRuntime, NavigationHost, RecordWriter, TimerHost, AiChatPairSaving, AuthorNameHost,
    ModerationHost, MatchingHost, SocialPromptHost, BlockedUsersHost, FollowsHost, NearbyHost, MarksHost {
    private val _state = MutableStateFlow(RuntimeState(document))
    val state: StateFlow<RuntimeState> = _state.asStateFlow()

    private val _navigation = MutableStateFlow(NavigationState.initial(document.dsl))
    override val navigation: StateFlow<NavigationState> = _navigation.asStateFlow()
    override val navigationHost: NavigationHost get() = this

    /** 描画器のフォームが読む記録([RuntimeState.records] と同じもの)。 */
    private val _records = MutableStateFlow<Map<String, List<Record>>>(emptyMap())
    override val recordsFlow: StateFlow<Map<String, List<Record>>> = _records.asStateFlow()

    /** entity ごとの読み込み状態(#1116)。[RuntimeState] の records / loading / failed から作る。 */
    private val _loadStates = MutableStateFlow(loadStatesOf(_state.value))
    override val loadStatesFlow: StateFlow<Map<String, EntityLoadState>> = _loadStates.asStateFlow()

    val document: AppDocument get() = _state.value.document
    val dsl: AppDSL get() = document.dsl
    val toast: UiText? get() = _state.value.toast
    val banner: RuntimeBanner? get() = _state.value.banner

    /** ホットリロードが SSE か・ポーリングか・止まっているか(下部の開発用の表示・#1108)。 */
    val hotReloadStatus: StateFlow<HotReloadStatus> = hotReloader?.status ?: MutableStateFlow(HotReloadStatus.OFF).asStateFlow()
    private var bannerSeq = 0L

    private var entityCache: Pair<AppDSL, Map<String, Entity>>? = null
    private val savingForms = mutableSetOf<FormDraft>()
    private val savingLock = Mutex()
    private var eventsJob: Job? = null
    private var notificationsJob: Job? = null
    private var connectivityJob: Job? = null

    // ホットリロード(#883)
    private var hotReloadJobs: List<Job> = emptyList()
    private var syncingAppId: String? = null
    private var foreground = true

    /** 入力中で差し替えを待たせている DSL(閉じたら当てる・最新の 1 件だけ)。 */
    private var pendingHotReload: AppDocument? = null
    private var pendingHotReloadJob: Job? = null
    private val recordsRefreshJobs = mutableMapOf<String, Job>()

    /** 最後に保存したフォーム。サーバーが断ったら「保存しました」を取り消す */
    private var lastSavedForm: SavedForm? = null

    private data class SavedForm(val appId: String, val entity: String, val recordId: String, val form: FormDraft)

    // 起動

    /**
     * 前に開いていたアプリを開き直し(保存した DSL を先に出す → サーバーの最新)、画面の entity の記録を読む。
     * 記録の同期の start はここではしない(サインインして持ち主を受け入れた後に `AccountGuard` がする・#841)。
     * インストール直後で話しかける前の画面なら、サーバーの最近のアプリを開く(#276・再インストール後)。
     *
     * 1 回だけ走る。画面の作り直し(回転など)で呼び直しても、最初の 1 回の終わりを待つだけ
     * (「+ 新しいアプリ」の後に回転して最近のアプリへ戻されない・同じ取得を重ねない)。
     * 中身はランタイムの scope で回すので、呼んだ画面が途中で消えても止まらない。
     */
    suspend fun bootstrap() {
        val job = bootstrapJob ?: scope.launch { bootstrapOnce() }.also { bootstrapJob = it }
        job.join()
    }

    private var bootstrapJob: Job? = null

    private suspend fun bootstrapOnce() {
        if (eventsJob == null) eventsJob = scope.launch { recordSync.events.collect { handle(it) } }
        if (notificationsJob == null) notificationsJob = scope.launch { watchNotifications() }
        if (connectivityJob == null) connectivityJob = scope.launch { watchConnectivity() }
        if (isIssued(document.appId)) {
            // 再起動しても作ったアプリが残る(iOS の G4)。サーバーの最新は続けて取り直す
            loadCached(document.appId)?.takeIf { it.appId == document.appId && it.version > document.version }?.let(::apply)
            restoreFromServer()
            if (directory?.store?.issuedThisLaunch == true) resumeLatestAppIfEmpty()
        }
        startHotReload()
        scope.launch { social.loadBlockedUsers() }
        sweepRetention(noteDsl = true) // #482 起動時の DSL(オフラインでは apply を通らない)
        pendingNavigationRestore?.let {
            pendingNavigationRestore = null
            applyNavigationRestore(it)
        }
        load(dsl.entities.map { it.id }.toSet())
    }

    /**
     * サーバーの最新 DSL に合わせる(iOS の `restoreFromServer`)。進んでいれば差し替え、遅れていれば今の表示のまま。
     * 届かなければ保存した DSL(手元と同じか新しい版)を出して「オフライン」を知らせる。
     */
    suspend fun restoreFromServer() {
        val fetch = restore ?: return
        try {
            val remote = fetch(document.appId)
            _state.update { it.copy(offline = false, unreachable = false) }
            if (remote.version > document.version || (remote.version == document.version && document.version == 0)) apply(remote)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            val cached = loadCached(document.appId)
            if (cached != null && cached.appId == document.appId && cached.version >= document.version) {
                // 起動時に先に出した版と同じなら描き直さない(見ている画面・遷移を残す)
                if (cached != document) apply(cached)
                _state.update { it.copy(offline = true, unreachable = false, toast = RuntimeCopy.OFFLINE_RESTORED) }
            } else {
                // 手元の DSL のまま。消えない状態カードで次の一手を出し、つながったら読み直す([watchConnectivity])
                _state.update { it.copy(offline = true, unreachable = true) }
            }
        }
    }

    /**
     * 到達性を見張る(iOS の `observeConnectivity`・#1107)。オフラインからつながった時、起動時にサーバーへ届かなかったなら
     * サーバーの最新を取り直す(取れたら状態カードを下げる)。
     */
    private suspend fun watchConnectivity() {
        var previous: Boolean? = null
        recordSync.connectivity.online.collect { online ->
            if (online && previous == false && _state.value.offline) restoreFromServer()
            previous = online
        }
    }

    /**
     * 通知(#886): DSL の読み込み・記録の保存・削除・同期が続いても 400ms まとめて 1 回、予約し直しと条件の評価を host に頼む。
     * 通知の無い DSL では何もしない。
     */
    @OptIn(FlowPreview::class)
    private suspend fun watchNotifications() {
        combine(_state.map { it.document }.distinctUntilChanged(), _records) { doc, recs -> doc to recs }
            .debounce(400)
            .collect { (doc, recs) ->
                if (doc.dsl.app.notifications.isEmpty()) return@collect
                try {
                    host.notificationsChanged(doc.appId, doc.dsl, notificationRecords(doc, recs))
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    // 予約に失敗しても画面は止めない(次の変化で予約し直す)
                }
            }
    }

    /** まだ読み込んでいない entity を端末内 DB で埋める・読めなかった entity は外す(#1124・[NotificationRecords])。 */
    private suspend fun notificationRecords(doc: AppDocument, recs: Map<String, List<Record>>): Map<String, List<Record>> =
        NotificationRecords.fill(doc.dsl.entities.map { it.id }, recs, _state.value.failed) { entity ->
            social.hidingBlocked(mapOf(entity to recordSync.cached(doc.appId, entity).map { r -> r.toDsl() }))[entity].orEmpty()
        }

    private suspend fun loadCached(appId: String): AppDocument? = try {
        cachedDocument?.invoke(appId)
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        null
    }

    /** DSL を差し替える(別アプリ・生成の結果)。表示中の記録・遷移・モーダルは捨てる。appId は次の起動のために保存する。 */
    fun apply(doc: AppDocument) {
        val switched = doc.appId != document.appId
        if (switched) dropPendingHotReload()
        if (isIssued(doc.appId)) directory?.store?.set(doc.appId)
        _state.value = RuntimeState(doc, offline = _state.value.offline, unreachable = _state.value.unreachable)
        publishRecords()
        _navigation.value = NavigationState.initial(doc.dsl)
        noteAppliedVersion(doc, switched)
        sweepRetention(noteDsl = true) // #482 保存期間の確認と期限切れの削除
        members?.note(doc.appId, doc.dsl)
    }

    // アプリの持ち物(iOS の DeviceAppID・変身一覧・#881)

    private val _apps = MutableStateFlow<List<AppSummary>>(emptyList())

    /** 最後に読んだアプリの一覧(`GET /v1/apps` かオフラインの保存)。 */
    val apps: StateFlow<List<AppSummary>> = _apps.asStateFlow()

    /** 一覧に出すアプリ(archived・未生成を除き、今のアプリを先頭に)。 */
    val visibleApps: List<AppSummary> get() = AppSummary.visible(_apps.value, current = document.appId)

    /** 一覧を差し替える(変身一覧の名前の変更・複製・並べ替え・削除・#896)。オフラインの一覧にも残す。 */
    fun setApps(list: List<AppSummary>) {
        _apps.value = list
        directory?.cache?.store(list)
    }

    /**
     * 生成(音声 #995)の結果を受け取る。ai は送った appId で返すので、別の appId なら新しいアプリとして開き、その appId を保存する。
     * 同じアプリの古い版(ホットリロードが先に新しい版を当てた)は捨てる。
     */
    suspend fun adoptGenerated(doc: AppDocument) {
        if (doc == document) return
        if (doc.appId == document.appId && doc.version < document.version) return
        show(doc)
    }

    /** [apply] して、その DSL の entity の記録を読む(生成・切り替えの結果はここを通す。読まないと一覧が空のまま)。 */
    suspend fun show(doc: AppDocument) {
        apply(doc)
        load(doc.dsl.entities.map { it.id }.toSet())
    }

    /** `GET /v1/apps`。届かなければ保存した一覧(とトースト)。結果は [apps] にも入れる。 */
    suspend fun appsList(): List<AppSummary> {
        val dir = directory ?: return _apps.value
        try {
            val list = dir.list()
            dir.cache.store(list)
            _apps.value = list
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            val cached = dir.cache.load()
            if (cached.isNotEmpty()) {
                _apps.value = cached
                showToast(RuntimeCopy.APPS_OFFLINE)
            } else if (_apps.value.isEmpty()) {
                showToast(RuntimeCopy.APPS_FAILED)
            }
        }
        return _apps.value
    }

    /**
     * 別のアプリに切り替える(iOS の `switchApp(to:)`): サーバーの DSL → 届かなければ保存した DSL → [apply]。
     * appId を保存し、ホットリロードを張り直す。読めなければ今のアプリのまま false。
     */
    suspend fun switchApp(appId: String): Boolean {
        if (appId == document.appId) return true
        var offline = false
        val next = try {
            restore?.invoke(appId)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        } ?: loadCached(appId)?.also { offline = true }
        if (next == null) {
            showToast(RuntimeCopy.SWITCH_FAILED)
            return false
        }
        show(next.copy(appId = appId))
        if (document.appId == appId) {
            _state.update { it.copy(offline = offline, toast = RuntimeCopy.switched(next.dsl.app.name, offline)) }
        }
        return true
    }

    /** 「+ 新しいアプリ」: appId を作り直して話しかける前の画面へ(前のアプリはサーバーと一覧に残る)。 */
    fun startNewApp() {
        val dir = directory ?: return
        apply(dir.fresh(dir.store.rotate()))
    }

    /**
     * ログアウト・アカウント削除・別の人のサインイン(#841・iOS の `wipeLocalData` の画面の分)。
     * 前の人のアプリ・記録の表示・一覧・「元に戻す」・ホットリロードを捨て、新しい appId の話しかける前の画面に戻す。
     * 次のサインインで [bootstrap] がもう一度走る。サーバーには何も送らない(記録の DB・送信待ちは `LocalDataWipe` が消す)。
     */
    fun resetForAccountChange() {
        accountGeneration += 1
        bootstrapJob?.cancel()
        bootstrapJob = null
        accountSwitchJob?.cancel()
        accountSwitchJob = null
        catchUpJob?.cancel()
        catchUpJob = null
        pendingNavigationRestore = null
        dropPendingHotReload()
        hotReloader?.stop()
        syncingAppId = null
        lastSavedForm = null
        entityCache = null
        _apps.value = emptyList()
        members?.clear() // 前の人の参加者・管理者を次の人に見せない
        social.forget() // 前の人のブロックで次の人の投稿を隠さない(#877)
        follows.forget()
        val next = directory?.let { it.fresh(it.store.rotate()) } ?: document
        _state.value = RuntimeState(next)
        _records.value = emptyMap()
        publishLoadStates()
        _navigation.value = NavigationState.initial(next.dsl)
    }

    private var accountSwitchJob: Job? = null

    /** [resetForAccountChange] ごとに増やす。前の人の操作で始めた非同期の予約(タイマーのアラーム)を後から入れない */
    @Volatile
    private var accountGeneration = 0

    /**
     * 別のアカウントを受け入れた後(#1317・iOS の `adoptSwitchedAccount`)。[resetForAccountChange] の後に呼ぶ。
     * 話しかける前の画面のままなら一覧の先頭のアプリを開き、起動の流れ(ホットリロードなど)を新しい人で回し直す。
     */
    fun resumeAfterAccountSwitch() {
        accountSwitchJob?.cancel()
        accountSwitchJob = scope.launch {
            resumeLatestAppIfEmpty() // 先に開く(新しい appId の空のアプリをサーバーに取りに行かない)
            bootstrap()
        }
    }

    /** インストール直後のサインイン(#276): 話しかける前の画面なら、サーバーの一覧の先頭のアプリを開く。 */
    private suspend fun resumeLatestAppIfEmpty() {
        val dir = directory ?: return
        if (!dsl.isVoiceOnly) return
        val list = try {
            dir.list()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            return
        }
        dir.cache.store(list)
        _apps.value = list
        val latest = AppSummary.latest(list, excluding = document.appId) ?: return
        switchApp(latest.id)
    }

    // ホットリロード(iOS の startSync / hotReload / setForeground)

    /**
     * 前面・背面(iOS の `setForeground`・#891)。呼ぶのは `AppForeground` だけ(ProcessLifecycleOwner の onStart / onStop)。
     * - 前面: 送信待ちを流す → 購読を張り直す → [catchUpAfterBackground](送信待ちが空いてから DSL → 表示中の記録 → 通知の予約)
     * - 背面: 取り直しを止める → 取り消し待ちの削除を確定する(戻せなくなるので「元に戻す」も消す)→ 購読を止める
     */
    fun setForeground(active: Boolean) {
        if (foreground == active) return
        foreground = active
        catchUpJob?.cancel()
        catchUpJob = null
        recordSync.setForeground(active)
        if (!active) _state.update { it.copy(recordUndo = null) } // 背面では削除を送り切るので戻せない
        hotReloader?.setForeground(active)
        // 起動の前(bootstrap がまだ)は bootstrap が読むので取り直さない
        if (active) sweepRetention() // #482 背面の間に期限を過ぎた記録
        if (active && bootstrapJob?.isCompleted == true) catchUpJob = scope.launch { catchUpAfterBackground() }
    }

    private var catchUpJob: Job? = null

    /**
     * 前面に戻った直後に 1 回(iOS の `catchUpAfterBackground`)。背面の間は購読が止まっているので取りこぼしを埋める。
     * 送信待ち → DSL → 表示中の記録 → 通知の予約し直し の順。どれかが失敗しても残りは続ける。
     */
    internal suspend fun catchUpAfterBackground() {
        val appId = document.appId
        drainQueueForCatchUp()
        if (isIssued(appId) && document.appId == appId) catchUpDocument(appId)
        val entities = _state.value.records.keys.toList()
        for (entity in entities) {
            if (document.appId != appId) return
            // 購読の張り直しが頼んだ取り直しはここでまとめて済ませる(同じ記録を 2 回読まない)
            recordsRefreshJobs.remove(entity)?.cancel()
            refresh(entity)
        }
        rescheduleNotifications()
    }

    /** 送信待ちを流し、流し終わるまで待つ(前面復帰の kick と重なるので最大 [CATCH_UP_DRAIN_WAIT_MILLIS])。 */
    private suspend fun drainQueueForCatchUp() {
        if (!recordSync.isOnline) return
        try {
            recordSync.queue.resume()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            return
        }
        var waited = 0L
        while (recordSync.queue.isDraining && waited < CATCH_UP_DRAIN_WAIT_MILLIS) {
            delay(CATCH_UP_POLL_MILLIS)
            waited += CATCH_UP_POLL_MILLIS
        }
    }

    private suspend fun catchUpDocument(appId: String) {
        val fetch = restore ?: return
        val remote = try {
            fetch(appId)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            return // 届かなければ今の DSL のまま(購読が戻れば追いつく)
        }
        if (document.appId != appId) return
        _state.update { it.copy(offline = false, unreachable = false) }
        hotReload(remote.copy(appId = appId)) // 進んでいれば当てる。操作中なら閉じるまで待つ
    }

    private suspend fun rescheduleNotifications() {
        val doc = document
        if (doc.dsl.app.notifications.isEmpty()) return
        try {
            host.notificationsChanged(doc.appId, doc.dsl, notificationRecords(doc, _records.value))
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // 次の変化で予約し直す
        }
    }

    // プロセスが落とされた後の復元(#891)

    private var pendingNavigationRestore: NavigationSnapshot? = null

    /** 今の画面の位置(Activity の保存状態に入れる)。話しかける前の画面では null。 */
    fun navigationSnapshot(): NavigationSnapshot? {
        val doc = document
        if (doc.dsl.isVoiceOnly) return null
        return NavigationSnapshot(doc.appId, _navigation.value)
    }

    /**
     * 保存しておいた画面の位置を戻す。起動([bootstrap])が終わってから当てる(開き直したアプリと同じ appId で、
     * 今の DSL にその画面がある時だけ。違えばアプリを開いた直後の画面のまま)。
     * 画面の作り直し(回転など)ではランタイムが生きているので、もう動いた遷移を上書きしない。
     */
    fun offerNavigationRestore(snapshot: NavigationSnapshot?) {
        if (snapshot == null) return
        if (bootstrapJob?.isCompleted == true) applyNavigationRestore(snapshot) else pendingNavigationRestore = snapshot
    }

    private fun applyNavigationRestore(snapshot: NavigationSnapshot) {
        val doc = document
        val restored = snapshot.restoreIn(doc.appId, doc.dsl)
        if (_navigation.value == NavigationState.initial(doc.dsl)) restored?.let { _navigation.value = it }
        // 同じ画面に戻れた時だけフォームの書きかけを当てる(#1185)。戻れなければ捨てる
        if (restored != null && _navigation.value == restored) formDrafts.arm(doc.appId) else formDrafts.clear()
    }

    /** 開いているフォームの書きかけを端末に置く(Activity の保存状態を書く時・#1185)。話しかける前の画面では置かない。 */
    fun persistFormDrafts() {
        val doc = document
        if (doc.dsl.isVoiceOnly) return formDrafts.clear()
        formDrafts.persist(doc.appId)
    }

    /** 他端末・ai 直叩きで進んだ DSL を当てる。操作中(シート・push 先・音声シート)なら閉じるまで待ち、最新の 1 件だけ当てる。 */
    fun hotReload(doc: AppDocument) {
        if (doc.appId != document.appId || doc.version <= document.version) return
        if (isInteracting(_navigation.value, _state.value)) {
            if (doc.version > (pendingHotReload?.version ?: 0)) pendingHotReload = doc
            if (pendingHotReloadJob == null) {
                pendingHotReloadJob = scope.launch {
                    combine(_navigation, _state) { n, s -> isInteracting(n, s) }.first { !it }
                    val pending = pendingHotReload
                    pendingHotReload = null
                    pendingHotReloadJob = null
                    pending?.let(::hotReload)
                }
            }
            return
        }
        // 記録・選択中のタブは残す(画面構成だけ描き直す)。新しい entity は読みに行く
        val keep = doc.dsl.entities.map { it.id }.toSet()
        val previous = _state.value
        _state.value = RuntimeState(
            document = doc,
            records = previous.records.filterKeys { it in keep },
            failed = previous.failed.intersect(keep),
            offline = previous.offline,
            unreachable = previous.unreachable,
            banner = RuntimeBanner(++bannerSeq, RuntimeCopy.HOT_RELOADED),
        )
        publishRecords()
        val tab = _navigation.value.selectedTab
        val initial = NavigationState.initial(doc.dsl)
        _navigation.value = if (doc.dsl.app.tabs.any { it.screen == tab }) initial.copy(selectedTab = tab) else initial
        noteAppliedVersion(doc, switched = false)
        sweepRetention(noteDsl = true) // #482
        scope.launch { load(keep) }
    }

    /** 記録の変化。表示中の entity だけ取り直す(未表示は画面が開いた時に読む)。 */
    fun recordsChanged(change: RecordsChange) {
        when (change) {
            is RecordsChange.Entity -> scheduleRecordsRefresh(change.entity)
            RecordsChange.All -> _state.value.records.keys.forEach(::scheduleRecordsRefresh)
        }
    }

    private fun startHotReload() {
        val hr = hotReloader ?: return
        if (hotReloadJobs.isEmpty()) {
            hotReloadJobs = listOf(
                scope.launch { hr.documents.collect { hotReload(it.document) } },
                scope.launch { hr.recordsChange.collect { recordsChanged(it) } },
            )
        }
        val appId = document.appId
        if (!isIssued(appId)) return
        syncingAppId = appId
        hr.start(appId, document.version)
    }

    /** 自分で当てた版を [HotReload] に知らせる。別アプリなら張り直す。 */
    private fun noteAppliedVersion(doc: AppDocument, switched: Boolean) {
        val hr = hotReloader ?: return
        if (hotReloadJobs.isEmpty()) return // 起動前(bootstrap が張る)
        when {
            !isIssued(doc.appId) -> if (syncingAppId != null) {
                syncingAppId = null
                hr.stop()
            }
            switched || syncingAppId != doc.appId -> {
                syncingAppId = doc.appId
                hr.start(doc.appId, doc.version)
            }
            else -> hr.noteVersion(doc.version)
        }
    }

    private fun scheduleRecordsRefresh(entity: String) {
        if (_state.value.records[entity] == null) return
        recordsRefreshJobs.remove(entity)?.cancel()
        recordsRefreshJobs[entity] = scope.launch {
            delay(recordsRefreshDelayMillis)
            recordsRefreshJobs.remove(entity)
            refresh(entity)
        }
    }

    private fun dropPendingHotReload() {
        pendingHotReload = null
        pendingHotReloadJob?.cancel()
        pendingHotReloadJob = null
        recordsRefreshJobs.values.forEach { it.cancel() }
        recordsRefreshJobs.clear()
    }

    private fun isInteracting(n: NavigationState, s: RuntimeState): Boolean =
        n.sheet != null || n.fullScreen != null || s.showVoiceSheet || n.paths.values.any { it.isNotEmpty() }

    // DSLRuntime

    override fun onVoiceInput(componentId: String?) {
        _state.update { it.copy(showVoiceSheet = true) }
    }

    /** transcribeField の話して入力(#864)。AppContainer が本体の認識器で差す。 */
    override var dictation: DictationHost? = null

    /** fileImport の録音ファイルの文字起こし(whisper.cpp・#1180)。AppContainer が差す。 */
    override var audioFileTranscription: AudioFileTranscriptionHost? = null

    /** photoField.analyze の端末内の読み取り(#1119・ML Kit)。AppContainer が渡す。 */
    override var photoReader: PhotoReader? = null

    override fun onAction(action: Action, context: ActionContext) {
        scope.launch { run(action, context) }
    }

    /** 画面が閉じて呼び手が止まっても、書き込みはランタイムの scope で最後までやる。 */
    override suspend fun runAction(action: Action, context: ActionContext) {
        scope.launch { run(action, context) }.join()
    }

    override fun setNavigation(next: NavigationState) {
        _navigation.value = next
    }

    override var discardGuard: DiscardGuard? = null

    // 画面の操作

    fun dismissToast() = _state.update { it.copy(toast = null) }

    /** [banner] がまだ同じ回なら閉じる(閉じる前に次の回が出ていたら残す)。 */
    fun dismissBanner(id: Long) = _state.update { if (it.banner?.id == id) it.copy(banner = null) else it }

    fun dismissVoiceSheet() = _state.update { it.copy(showVoiceSheet = false) }

    fun showToast(message: UiText) = _state.update { it.copy(toast = message) }

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

    override fun canDelete(entity: String, record: Record): Boolean =
        RecordPermission.canDelete(record, entityModel(entity), currentUserId(), isAppAdmin)

    /**
     * 参加者の写し(Compose のスナップショット)。部品は描く時に [formMembers]・[canDelete] を読むだけなので、
     * 届いた後に描き直されるよう StateFlow をここへ写す(写さないと「参加者を読み込めていません」のまま残る)。
     */
    private var memberEntries by mutableStateOf<Map<String, AppMembersStore.Entry>>(emptyMap())

    init {
        members?.let { store -> scope.launch { store.entries.collect { memberEntries = it } } }
    }

    /** 今のアプリで自分が管理者か(#272)。null = 分からない(読む前・読めなかった) */
    val isAppAdmin: Boolean? get() = memberEntries[document.appId]?.isAdmin

    override val viewerIsAdmin: Boolean? get() = isAppAdmin

    override val formMembers: List<FormMember>
        get() = memberEntries[document.appId]?.members.orEmpty().map { FormMember(it.uid, it.displayName) }

    /** 一覧から外す(共有から抜けた後・#903)。保存した一覧はそのまま(次の取得で消える) */
    fun forgetApp(appId: String) = _apps.update { list -> list.filter { it.id != appId } }

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
        publishLoadStates()
        try {
            val fresh = recordSync.pull(appId, entity)
            if (document.appId != appId) return
            _state.update { it.copy(records = it.records + (entity to fresh.map { r -> r.toDsl() }), failed = it.failed - entity) }
            publishRecords()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            if (document.appId != appId) return
            if (_state.value.records[entity].isNullOrEmpty()) {
                _state.update { it.copy(records = it.records + (entity to emptyList()), failed = it.failed + entity) }
                publishRecords()
            }
        } finally {
            _state.update { it.copy(loading = it.loading - entity) }
            publishLoadStates()
        }
        sweepRetention(entity) // #482 サーバーから届いた期限切れ(別の端末で作った自分の記録など)
    }

    /** 一覧のワンタップ更新(list.item.toggle)・トーナメントの勝者・単語カードの結果。表示中に無い記録は保存しない。 */
    override suspend fun updateRecord(entity: String, recordId: String, data: Map<String, JSONValue>): Boolean {
        val current = record(entity, recordId) ?: return false
        if (!canEdit(entity, current)) {
            showToast(RuntimeCopy.READ_ONLY)
            return false
        }
        setRecords(document.appId, entity, recordSync.save(document.appId, entity, recordId, data.toJson()))
        return true
    }

    override fun notifyReadOnly(entity: String) = showToast(RuntimeCopy.READ_ONLY)

    /** timerDisplay の終了通知(端末の予約は [RuntimeHost.timerChanged])。 */
    override fun timerChanged(key: String, name: String, endsAtEpochMillis: Long?) {
        val appId = document.appId
        val appName = dsl.app.name
        val generation = accountGeneration
        scope.launch {
            // 予約する前にサインアウト・アカウントの切り替えがあった: 前の人のアラームを戻さない
            if (accountGeneration != generation) return@launch
            host.timerChanged("$appId/$key", appName, name, endsAtEpochMillis)
        }
    }

    override fun loadTimer(key: String): TimerRun? = host.loadTimer("${document.appId}/$key")

    override fun saveTimer(key: String, run: TimerRun) = host.saveTimer("${document.appId}/$key", run)

    override fun timerVisible(key: String, visible: Boolean) = host.timerVisible("${document.appId}/$key", visible)

    /** timerDisplay.liveActivity の進行中の表示(#913・iOS の `AppRuntime.timerActivity`)。 */
    override fun timerActivity(key: String, name: String, totalSeconds: Int?, run: TimerRun) {
        val appId = document.appId
        val update = TimerActivityUpdate("$appId/$key", appId, dsl.app.name, name, totalSeconds, run)
        val generation = accountGeneration
        scope.launch {
            // サインアウト・アカウントの切り替えがあった: 前の人の表示を出し直さない
            if (accountGeneration != generation) return@launch
            // 出せない(通知の許可が無い)のは始めた時だけ伝える。止めた・消した時は黙る
            if (!host.timerActivity(update) && run.isRunning) showToast(RuntimeCopy.ACTIVITY_NOTIFICATIONS_OFF)
        }
    }

    /** フォームを通さず 1 件足す。作った行の id(仮 id)。entity が無ければ null。 */
    suspend fun createRecord(entity: String, data: Map<String, JSONValue>): String? {
        if (dsl.entities.none { it.id == entity }) return null
        val (list, id) = write(entity, null, data)
        setRecords(document.appId, entity, list)
        return id
    }

    // 近くの人に送る(#1382)

    /** 近距離共有の窓口(AppContainer が作る時に入れる)。null なら送るボタンを押しても何もしない(モック・テスト)。 */
    var nearby: NearbyShareController? = null

    override fun shareNearby(entity: String, records: List<Map<String, JSONValue>>) {
        nearby?.share(entity, records)
    }

    /** すれ違い通信(#1383)。AppContainer が [jp.kz2601.metamon.nearby.PassbyController] を作る時に入れる。null なら表示だけ */
    override var passby: NearbyPassbyController? = null

    /** ローカル DB から読み直す(サーバーには取りに行かない)。すれ違い通信の更新で、仮 id が変わった記録を内容で探す前に呼ぶ。 */
    suspend fun reloadLocal(entity: String) = reloadCached(document.appId, entity)

    // AI ボタン(#873)

    override val ai: AiHost? get() = aiHost

    override suspend fun loadRecords(entities: Set<String>) = load(entities)

    override suspend fun retryRecords(entities: Set<String>) = load(entities, force = true)

    override suspend fun addRecord(entity: String, data: Map<String, JSONValue>): Boolean = createRecord(entity, data) != null

    override val currentAppId: String get() = document.appId

    // 投稿に出す名前(#875・iOS の AppRuntime+Chat)

    /** 共有の entity への初めての投稿で、まだ名前を聞いていなければ聞く。 */
    override fun needsAuthorName(entity: String): Boolean =
        authorProfile != null && entityModel(entity)?.isShared == true && !authorProfile.asked

    override fun answerAuthorName(name: String?) {
        authorProfile?.answer(name)
    }

    // 通報・ブロック・仲間探し(#877・iOS の AppRuntime+Moderation / AppRuntime+Matching)

    private val social = SocialRuntime(socialServices, scope, object : SocialRuntime.Env {
        override val appId: String get() = document.appId
        override fun currentUid(): String? = currentUserId()
        override val entities: List<Entity> get() = entityModels
        override fun entity(id: String): Entity? = entityModel(id)
        override fun records(entity: String): List<Record> = this@AppRuntime.records(entity)
        override suspend fun reload(entities: Set<String>) = load(entities, force = true)
        override fun toast(message: UiText) = showToast(message)
        override fun blockedChanged() = publishRecords()
    })

    /** 描画に渡す記録(ブロックした人の共有の投稿を除く)。 */
    private fun publishRecords() {
        _records.value = social.hidingBlocked(_state.value.records)
        publishLoadStates()
    }

    private fun publishLoadStates() {
        _loadStates.value = loadStatesOf(_state.value)
    }

    private fun loadStatesOf(s: RuntimeState): Map<String, EntityLoadState> =
        EntityLoadState.all(s.records, s.loading, s.failed, s.document.dsl.entities.map { it.id }.toSet())

    /** 印(#255)。ローカルを先に変えてサーバーで確定。失敗はローカルを戻して null(一覧は元の値に戻る)。 */
    override suspend fun toggleMark(entity: String, recordId: String, mark: String): MarkState? {
        val appId = document.appId
        val settled = try {
            recordSync.toggleMark(appId, entity, recordId, mark)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
        if (document.appId == appId) setRecords(appId, entity, recordSync.cached(appId, entity))
        return settled?.let { MarkState(it.count, it.mine) }
    }

    override fun canReport(entity: String, record: Record): Boolean = social.canReport(entity, record)
    override fun reportRecord(entity: String, record: Record) = social.reportRecord(entity, record)
    override fun blockAuthor(entity: String, record: Record) = social.blockAuthor(entity, record)
    override val socialPrompt: StateFlow<SocialPrompt?> get() = social.prompt
    override fun submitReport(reason: ReportReasonOption) = social.submitReport(reason)
    override fun confirmBlock() = social.confirmBlock()
    override fun confirmAdult() = social.confirmAdult()
    override fun declineAdult() = social.declineAdult()
    override fun dismissSocialPrompt() = social.dismiss()
    override val blockedUsers: StateFlow<Map<String, String>>? get() = if (social.moderationAvailable) social.blocked else null
    override suspend fun unblock(uid: String) = social.unblock(uid)
    override val matchingVersion: StateFlow<Int> get() = social.version
    override fun interestState(entity: String, record: Record): InterestState? = social.interestState(entity, record)
    override fun interestBusy(record: Record): Boolean = social.interestBusy(record)
    override suspend fun sendInterest(entity: String, record: Record) = social.sendInterest(entity, record)
    override fun matchItems(box: MatchBox): List<MatchInterest>? = social.matchItems(box)
    override fun matchLoadFailed(box: MatchBox): Boolean = social.matchLoadFailed(box)
    override suspend fun refreshMatches(boxes: Set<MatchBox>) {
        social.loadBlockedUsers()
        social.refreshMatches(boxes)
    }
    override suspend fun acceptInterest(item: MatchInterest) = social.acceptInterest(item)
    override suspend fun declineInterest(item: MatchInterest, box: MatchBox) = social.declineInterest(item, box)
    override suspend fun unmatch(item: MatchInterest) = social.unmatch(item)

    // フォロー(#1028・iOS の AppRuntime+Follows)

    private val follows = FollowRuntime(socialServices?.follows, scope, object : FollowRuntime.Env {
        override val appId: String get() = document.appId
        override val dsl: AppDSL get() = this@AppRuntime.dsl
        override fun currentUid(): String? = currentUserId()
        override fun records(entity: String): List<Record> = this@AppRuntime.records(entity)
        override fun toast(message: UiText) = showToast(message)
    })

    override val followsVersion: StateFlow<Int> get() = follows.version
    override fun followedAuthors(): Set<String>? = follows.followedAuthors()
    override fun followState(entity: String, recordId: String): Boolean? = follows.followState(entity, recordId)

    // AI と話す(#874)

    /**
     * roleField の無い aiChat の 1 往復を対で保存する(iOS の AppRuntime+AIChat)。役は並び順で読むので、
     * 返事を保存できなければ自分の発言も消す(交互を崩さない)。後でサーバーが片方を断った時の取り消しは子 Issue。
     */
    override suspend fun saveAiChatPair(entity: String, user: Map<String, JSONValue>, assistant: Map<String, JSONValue>): Boolean {
        val appId = document.appId
        val userId = createRecord(entity, user) ?: return false
        delay(5) // 作成時刻で並べるので、自分 → AI の順を同じ時刻にしない
        if (appId == document.appId && createRecord(entity, assistant) != null) return true
        setRecords(appId, entity, recordSync.delete(appId, entity, userId))
        return false
    }

    /** 一覧から消して「元に戻す」を出す。前の取り消し待ちは時間が来たら送られる。 */
    suspend fun deleteUndoably(entity: String, recordId: String) {
        val appId = document.appId
        val (list, ticket) = recordSync.deleteUndoably(appId, entity, recordId, undoGraceMillis())
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

            ActionType.FOLLOW, ActionType.UNFOLLOW -> follows.run(action, ctx)

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
                attempt(action.type) {
                    val stat = spec.stat?.let { ActivityStat.read(it, dsl, LeafData(this, _records.value)) }
                    if (!host.startActivity(spec, document.appId, dsl.app.name, stat)) showToast(RuntimeCopy.ACTIVITY_NOTIFICATIONS_OFF)
                }
            }

            ActionType.END_ACTIVITY -> attempt(action.type) { host.endActivity(document.appId) }

            ActionType.ADD_TO_CALENDAR -> attempt(action.type) { host.addToCalendar(action, ctx, this) }

            ActionType.LOOKUP_BOOK -> attempt(action.type) { host.lookupBook(action, ctx, this) }

            ActionType.UNKNOWN -> showToast(RuntimeCopy.unknownAction(action.rawType))
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
            formDrafts.forget(form) // 保存したら端末の書きかけも消す(#1185)
            lastSavedForm = SavedForm(appId, entity, written, form)
            sweepRetention(entity) // #482 保存の後
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
    fun unsavedParentToast(ctx: ActionContext): UiText = RuntimeCopy.saveParentFirst(ctx.form?.entity?.name.orEmpty())

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
    fun rejectedToast(entity: String, field: String?, message: String): UiText {
        field ?: return RuntimeCopy.REJECTED_GENERIC
        return RuntimeCopy.rejected(entityModel(entity)?.fields?.firstOrNull { it.id == field }?.name ?: field)
    }

    fun textNotAllowedToast(entity: String?, field: String?): UiText {
        field ?: return RuntimeCopy.TEXT_NOT_ALLOWED
        if (field == "authorName") return RuntimeCopy.textNotAllowed(RuntimeCopy.AUTHOR_NAME)
        val name = entity?.let { entityModel(it) }?.fields?.firstOrNull { it.id == field }?.name ?: return RuntimeCopy.TEXT_NOT_ALLOWED
        return RuntimeCopy.textNotAllowed(name)
    }

    fun denialMessage(denial: RecordWriteDenial, entity: String): UiText {
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
    fun message(error: Throwable, entity: String?): UiText {
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
            authorName = if (shared) authorName() ?: authorProfile?.name else null,
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
        publishRecords()
    }

    /**
     * 記録の保存期間(#482・[RecordRetentionRunner])。[noteDsl] なら確認が要るかも見直す。
     * 期限切れを消したら表示中の一覧だけ差し替える(未表示の entity は画面が開いた時に読む)。
     */
    fun sweepRetention(entity: String? = null, noteDsl: Boolean = false) {
        val r = retention ?: return
        val doc = document
        val models = entityModels
        if (noteDsl) r.note(doc.appId, doc.version, models)
        val targets = if (entity == null) models else models.filter { it.id == entity }
        if (targets.none { it.retentionDays != null }) return
        scope.launch {
            for (e in r.purgeAll(doc.appId, targets)) if (_state.value.records[e] != null) reloadCached(doc.appId, e)
        }
    }

    private suspend fun ifCurrent(appId: String, body: suspend () -> Unit) {
        if (appId == document.appId) body()
    }

    companion object {
        /** 仮の appId(:dsl の既定値・テスト)。サーバーには取りに行かない。アプリは [AppDirectory] の保存した appId で始める */
        const val LOCAL_APP_ID = "local"

        fun isIssued(appId: String): Boolean = appId != LOCAL_APP_ID

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

/** 前面復帰で送信待ちが流れ終わるのを待つ上限(iOS と同じ 10 秒) */
private const val CATCH_UP_DRAIN_WAIT_MILLIS = 10_000L
private const val CATCH_UP_POLL_MILLIS = 100L
