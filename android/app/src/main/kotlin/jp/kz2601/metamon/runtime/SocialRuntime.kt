package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.InterestState
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.social.ReportReasonOption
import jp.kz2601.metamon.renderer.components.social.SocialNotice
import jp.kz2601.metamon.renderer.components.social.SocialPrompt
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.social.MatchingAgeGate
import jp.kz2601.metamon.services.social.MatchingClient
import jp.kz2601.metamon.services.social.MatchingReason
import jp.kz2601.metamon.services.social.MatchingStore
import jp.kz2601.metamon.services.social.ModerationClient
import jp.kz2601.metamon.services.social.ReportReason
import jp.kz2601.metamon.services.social.ReportTarget
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** 仲間探し(#507)と通報・ブロック(#321)の API。live の appconfig がある時だけ AppContainer が渡す。 */
class SocialServices(
    val matching: MatchingClient?,
    val moderation: ModerationClient?,
    val ageGate: MatchingAgeGate,
)

/**
 * AppRuntime の仲間探し・通報・ブロック(iOS の AppRuntime+Matching / AppRuntime+Moderation)。
 * AppRuntime は [jp.kz2601.metamon.renderer.components.social.ModerationHost] などをここへ流すだけ。
 * ブロックした人の共有の投稿は [hidingBlocked] で描画に渡す記録から除き、仲間探しの一覧からも除く。
 */
class SocialRuntime(
    private val services: SocialServices?,
    private val scope: CoroutineScope,
    private val env: Env,
) {
    /** AppRuntime から借りるもの。 */
    interface Env {
        val appId: String

        fun currentUid(): String?

        val entities: List<Entity>

        fun entity(id: String): Entity?

        /** ブロックで隠す前の記録。 */
        fun records(entity: String): List<Record>

        suspend fun reload(entities: Set<String>)

        fun toast(message: String)

        /** ブロック中の人が変わった(描画に渡す記録を作り直す)。 */
        fun blockedChanged()
    }

    private val _prompt = MutableStateFlow<SocialPrompt?>(null)
    val prompt: StateFlow<SocialPrompt?> = _prompt.asStateFlow()

    private val _blocked = MutableStateFlow<Map<String, String>>(emptyMap())

    /** ブロック中の uid → 表示名。 */
    val blocked: StateFlow<Map<String, String>> = _blocked.asStateFlow()
    private var blockedLoadedFor: String? = null

    private val _version = MutableStateFlow(0)

    /** 一覧・送信中・ブロックが変わるたびに進む(MatchingHost.matchingVersion)。 */
    val version: StateFlow<Int> = _version.asStateFlow()

    /** 通報 / ブロックの確認を出している相手。 */
    private var moderating: Request? = null

    /** 18 歳以上の確認のあとに続ける操作。 */
    private var pendingInterest: PendingInterest? = null
    private val stores = mutableMapOf<String, MatchingStore>()

    private data class Request(val target: ReportTarget, val authorUid: String?, val authorName: String?)

    sealed interface PendingInterest {
        data class Send(val entity: String, val record: Record) : PendingInterest

        data class Accept(val item: MatchInterest) : PendingInterest
    }

    val moderationAvailable: Boolean get() = services?.moderation != null

    // 通報・ブロック

    /** 他人の共有レコードだけ(自分の投稿・private の entity・作者の分からない行は出さない)。 */
    fun canReport(entity: String, record: Record): Boolean {
        val me = env.currentUid() ?: return false
        val author = record.author?.takeIf { it.isNotEmpty() } ?: return false
        return moderationAvailable && env.entity(entity)?.isShared == true && author != me
    }

    fun reportRecord(entity: String, record: Record) {
        if (!canReport(entity, record)) return
        moderating = request(entity, record)
        _prompt.value = SocialPrompt.Report(record.authorName)
    }

    fun blockAuthor(entity: String, record: Record) {
        if (!canReport(entity, record)) return
        moderating = request(entity, record)
        _prompt.value = SocialPrompt.Block(record.authorName)
    }

    private fun request(entity: String, record: Record) =
        Request(ReportTarget.RecordTarget(env.appId, entity, record.id), record.author, record.authorName)

    fun submitReport(reason: ReportReasonOption) {
        val req = moderating ?: return dismiss()
        val client = services?.moderation ?: return dismiss()
        moderating = null
        _prompt.value = null
        scope.launch {
            _prompt.value = try {
                client.report(req.target, ReportReason.valueOf(reason.name))
                SocialPrompt.Notice(SocialNotice.REPORTED)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                SocialPrompt.Notice(if ((e as? ApiError)?.status == 429) SocialNotice.REPORT_LIMITED else SocialNotice.REPORT_FAILED)
            }
        }
    }

    /** 先に端末で隠してから送る(失敗したら戻す)。 */
    fun confirmBlock() {
        val req = moderating ?: return dismiss()
        val client = services?.moderation ?: return dismiss()
        val uid = req.authorUid ?: return dismiss()
        moderating = null
        _prompt.value = null
        setBlocked(_blocked.value + (uid to (req.authorName ?: "")))
        scope.launch {
            _prompt.value = try {
                val user = client.block(uid, req.authorName)
                setBlocked(_blocked.value - uid + (user.uid to user.name))
                stores.values.forEach { it.forget(user.uid) }
                SocialPrompt.Notice(SocialNotice.BLOCKED, user.name.ifEmpty { req.authorName })
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                setBlocked(_blocked.value - uid)
                SocialPrompt.Notice(SocialNotice.BLOCK_FAILED)
            }
        }
    }

    /** 設定の「ブロック中のユーザー」から解除する。サーバーが除いていた投稿を取り直す。 */
    suspend fun unblock(uid: String) {
        val client = services?.moderation ?: return
        val name = _blocked.value[uid] ?: return
        setBlocked(_blocked.value - uid)
        try {
            client.unblock(uid)
            env.reload(env.entities.filter { it.isShared }.map { it.id }.toSet())
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            setBlocked(_blocked.value + (uid to name))
            env.toast(SocialCopy.UNBLOCK_FAILED)
        }
    }

    /** サインイン中の uid のブロック一覧を 1 回読む(失敗しても投稿はサーバーが除くので、次の起動で読み直す)。 */
    suspend fun loadBlockedUsers() {
        val client = services?.moderation ?: return
        val me = env.currentUid() ?: return
        if (blockedLoadedFor == me) return
        val users = try {
            client.blockedUsers()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            return
        }
        setBlocked(users.associate { it.uid to it.name })
        blockedLoadedFor = me
    }

    /** 共有の entity から、ブロック中の人の投稿を除く(描画に渡す記録)。 */
    fun hidingBlocked(records: Map<String, List<Record>>): Map<String, List<Record>> =
        hidingBlocked(records, _blocked.value) { env.entity(it)?.isShared == true }

    private fun setBlocked(next: Map<String, String>) {
        _blocked.value = next
        _version.update { it + 1 }
        env.blockedChanged()
    }

    // 仲間探し

    /** このアプリの一覧。live で、プロフィールの entity があり、サインインしている時だけ。 */
    private val store: MatchingStore?
        get() {
            val client = services?.matching ?: return null
            val me = env.currentUid()?.takeIf { it.isNotEmpty() } ?: return null
            if (env.entities.none { it.isMatchProfile }) return null
            val appId = env.appId
            return stores.getOrPut("$appId|$me") {
                MatchingStore(appId, client).also { s -> scope.launch { s.snapshot.collect { _version.update { it + 1 } } } }
            }
        }

    fun interestState(entity: String, record: Record): InterestState? {
        val store = store ?: return null
        if (env.entity(entity)?.isMatchProfile != true) return null
        val author = record.author?.takeIf { it.isNotEmpty() } ?: return null
        if (author == env.currentUid() || author in _blocked.value) return null
        return store.state(author)
    }

    fun interestBusy(record: Record): Boolean = record.author?.let { store?.isBusy(it) } ?: false

    suspend fun sendInterest(entity: String, record: Record) {
        val store = store ?: return
        val me = env.currentUid()
        val author = record.author?.takeIf { it.isNotEmpty() && it != me } ?: return
        val mine = env.records(entity).firstOrNull { it.author == me }
        if (mine == null) return env.toast(SocialCopy.PROFILE_FIRST)
        if (!services!!.ageGate.isConfirmed) return askAge(PendingInterest.Send(entity, record))
        try {
            if (store.send(author, record.id, mine.id).isMatched) connected()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            failed(e, PendingInterest.Send(entity, record))
        }
    }

    fun matchItems(box: MatchBox): List<MatchInterest>? {
        val items = store?.items(box) ?: return null
        val blocked = _blocked.value
        return if (blocked.isEmpty()) items else items.filter { it.uid !in blocked }
    }

    fun matchLoadFailed(box: MatchBox): Boolean = store?.snapshot?.value?.failed?.contains(box) ?: false

    suspend fun refreshMatches(boxes: Set<MatchBox>) {
        val store = store ?: return
        for (box in MatchBox.entries) {
            if (box !in boxes) continue
            try {
                store.load(box)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // 一覧の「読み込めませんでした」で出す
            }
        }
    }

    suspend fun acceptInterest(item: MatchInterest) {
        val store = store ?: return
        if (!services!!.ageGate.isConfirmed) return askAge(PendingInterest.Accept(item))
        try {
            store.accept(item.uid)
            connected()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            failed(e, PendingInterest.Accept(item))
        }
    }

    suspend fun declineInterest(item: MatchInterest, box: MatchBox) {
        val store = store ?: return
        try {
            if (box == MatchBox.OUTGOING) store.withdraw(item.uid) else store.decline(item.uid)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            failed(e, null)
        }
    }

    suspend fun unmatch(item: MatchInterest) {
        val store = store ?: return
        try {
            store.unmatch(item.uid)
            env.toast(SocialCopy.UNMATCHED)
            reloadMatched()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            failed(e, null)
        }
    }

    // 18 歳以上の確認

    private fun askAge(pending: PendingInterest) {
        pendingInterest = pending
        _prompt.value = SocialPrompt.AdultConfirm
    }

    fun confirmAdult() {
        val pending = pendingInterest
        services?.ageGate?.confirm()
        pendingInterest = null
        _prompt.value = null
        when (pending) {
            is PendingInterest.Send -> scope.launch { sendInterest(pending.entity, pending.record) }
            is PendingInterest.Accept -> scope.launch { acceptInterest(pending.item) }
            null -> Unit
        }
    }

    fun declineAdult() {
        pendingInterest = null
        _prompt.value = null
        env.toast(SocialCopy.UNDER_18)
    }

    /** 閉じた・キャンセル: 何もしない(送らない・覚えない)。 */
    fun dismiss() {
        moderating = null
        pendingInterest = null
        _prompt.value = null
    }

    /** つながった: 連絡先(match "matched")が見えるようになるので読み直す。 */
    private suspend fun connected() {
        env.toast(SocialCopy.CONNECTED)
        reloadMatched()
    }

    private suspend fun reloadMatched() {
        val ids = env.entities.filter { it.isMatchMatched }.map { it.id }.toSet()
        if (ids.isNotEmpty()) env.reload(ids)
    }

    /** 失敗: 一覧は MatchingStore が戻している。理由を出す。サーバーが確認を求めたら聞き直す。 */
    private fun failed(error: Exception, retry: PendingInterest?) {
        if ((error as? ApiError)?.reason == MatchingReason.AGE_UNCONFIRMED) {
            services?.ageGate?.reset()
            if (retry != null) askAge(retry)
            return
        }
        env.toast(errorText(error))
    }

    companion object {
        /** shared / public の entity から、[blocked] の人の投稿を除く。作者の無い行(未送信の自分の分)は残す。 */
        fun hidingBlocked(
            records: Map<String, List<Record>>,
            blocked: Map<String, String>,
            isShared: (String) -> Boolean,
        ): Map<String, List<Record>> {
            if (blocked.isEmpty()) return records
            return records.mapValues { (entity, rows) ->
                if (!isShared(entity)) rows else rows.filter { r -> r.author == null || r.author !in blocked }
            }
        }

        /** 仲間探しの失敗の一言(iOS の `matchingErrorText`)。 */
        fun errorText(error: Throwable): String {
            val api = error as? ApiError
            if (api?.reason == MatchingReason.BLOCKED) return SocialCopy.UNBLOCK_TO_SEND
            if (api?.field == "fromRecordId") return SocialCopy.PROFILE_NOT_SENT
            when (FailureKind.classify(error)) {
                FailureKind.Offline -> return SocialCopy.OFFLINE
                FailureKind.Cancelled -> return SocialCopy.TRY_AGAIN
                else -> Unit
            }
            return when (api?.status) {
                429 -> SocialCopy.DAILY_LIMIT
                403 -> SocialCopy.MATCHING_UNAVAILABLE
                404 -> SocialCopy.NOT_FOUND
                else -> SocialCopy.TRY_AGAIN
            }
        }
    }
}

/** 仲間探し・ブロックのトースト(iOS の L10n キーと同じ日本語。AppRuntime の他のトーストと同じく日本語だけ)。 */
object SocialCopy {
    const val PROFILE_FIRST = "先に自分のプロフィールを書いてください"
    const val UNMATCHED = "つながりを解除しました"
    const val UNDER_18 = "18 歳未満の方は「つながりたい」を使えません"
    const val CONNECTED = "つながりました。連絡先が見られます"
    const val UNBLOCK_TO_SEND = "ブロックを解除してから送れます"
    const val PROFILE_NOT_SENT = "プロフィールの送信が終わってから送れます"
    const val OFFLINE = "オフラインのあいだは送れません"
    const val TRY_AGAIN = "うまくいきませんでした。もう一度お試しください"
    const val DAILY_LIMIT = "今日はこれ以上送れません。明日またお試しください"
    const val MATCHING_UNAVAILABLE = "このアプリでは今は仲間探しを使えません"
    const val NOT_FOUND = "相手が見つかりませんでした。一覧を更新してください"
    const val UNBLOCK_FAILED = "解除できませんでした"
}
