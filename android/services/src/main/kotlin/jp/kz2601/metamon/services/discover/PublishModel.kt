package jp.kz2601.metamon.services.discover

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.social.AuthorProfile
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** 公開・非公開にできなかった理由。 */
enum class PublishFailure {
    /** サーバーにつながっていない構成(モック) */
    UNAVAILABLE,

    /** 422 text_not_allowed: 表示できない言葉([PublishState.flaggedField] の欄) */
    TEXT_NOT_ALLOWED,

    /** 403: このアプリの作成者ではない(参加したアプリ・入れたアプリ) */
    NOT_OWNER,

    /** 400: まだ中身が無い */
    BLANK_APP,

    /** 圏外・時間切れ */
    OFFLINE,
    OTHER,
}

/** 公開の確認(公開してよいかの注意と、「探す」に出す名前)。 */
data class PublishConfirm(val displayName: String)

/** 一度だけ出すお知らせ。 */
enum class PublishNotice { PUBLISHED, UNPUBLISHED, PUBLISH_FAILED, UNPUBLISH_FAILED }

data class PublishState(
    /** 自分の掲載(null = 公開していない) */
    val listing: PublicApp? = null,
    val loaded: Boolean = false,
    val description: String = "",
    val tags: String = "",
    val busy: Boolean = false,
    /** 公開の確認を出している */
    val confirm: PublishConfirm? = null,
    val failure: PublishFailure? = null,
    /** 表示できない言葉があった欄(`description` / `tags` / `name` / `authorName`) */
    val flaggedField: String? = null,
    val notice: PublishNotice? = null,
)

/**
 * 共有シートの下半分「みんなに公開」(iOS の `ShareAppSheet.publishSection`・#902 / #1239 の残り)。
 * 公開する前に確認(誰でも見つけて入れられる・個人の情報を書かない・表示名)を挟む。
 * @param client null = 「探す」が使えない構成(何も出さない)
 * @param profile 表示名の初期値と覚え先(参加の表示名・投稿の名前と同じ)
 */
class PublishModel(
    private val appId: String,
    private val client: DiscoverClient?,
    private val profile: AuthorProfile?,
) {
    private val _state = MutableStateFlow(PublishState())
    val state: StateFlow<PublishState> = _state.asStateFlow()

    /** 自分の掲載を探す(iOS と同じく新着の一覧から。ページを [MAX_PAGES] まで)。読めなければ「公開していない」で出す。 */
    suspend fun load() {
        val client = client ?: return _state.update { it.copy(loaded = true) }
        val listing = try {
            var token: String? = null
            var found: PublicApp? = null
            for (i in 0 until MAX_PAGES) {
                val page = client.search("", DiscoverSort.NEW, token)
                found = page.items.firstOrNull { it.id == appId && it.mine }
                token = page.nextPageToken
                if (found != null || token.isNullOrEmpty()) break
            }
            found
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
        _state.update { it.copy(listing = listing, loaded = true) }
    }

    fun setDescription(text: String) = _state.update {
        it.copy(description = text.take(MAX_DESCRIPTION), flaggedField = it.flaggedField.takeIf { f -> f != "description" })
    }

    fun setTags(text: String) = _state.update { it.copy(tags = text, flaggedField = it.flaggedField.takeIf { f -> f != "tags" }) }

    /** 「公開する」: 先に確認を出す(表示名の初期値は設定の「投稿に表示する名前」)。 */
    fun requestPublish() {
        if (client == null || _state.value.busy) return
        _state.update { it.copy(confirm = PublishConfirm(profile?.name.orEmpty()), failure = null) }
    }

    fun cancelPublish() = _state.update { it.copy(confirm = null) }

    /** 確認の「公開する」。空欄の表示名は送らない(サーバーが「匿名」にする)。 */
    suspend fun confirmPublish(displayName: String) {
        val client = client ?: return
        if (_state.value.busy) return
        val name = AuthorProfile.normalized(displayName)
        val now = _state.value
        _state.update { it.copy(confirm = null, busy = true, failure = null) }
        try {
            val listing = client.publish(
                appId,
                PublishRequest(description = now.description.trim().ifEmpty { null }, tags = splitTags(now.tags), authorName = name),
            )
            if (name != null) profile?.answer(name)
            _state.update { it.copy(listing = listing, busy = false, flaggedField = null, notice = PublishNotice.PUBLISHED) }
        } catch (e: CancellationException) {
            _state.update { it.copy(busy = false) }
            throw e
        } catch (e: Exception) {
            val api = e as? ApiError
            _state.update {
                it.copy(
                    busy = false,
                    failure = failureOf(e),
                    flaggedField = if (api?.isTextNotAllowed == true) api.field else null,
                    notice = PublishNotice.PUBLISH_FAILED,
                )
            }
        }
    }

    /** 「公開をやめる」(確認は画面で取る)。 */
    suspend fun unpublish() {
        val client = client ?: return
        if (_state.value.busy || _state.value.listing == null) return
        _state.update { it.copy(busy = true, failure = null) }
        try {
            client.unpublish(appId)
            _state.update { it.copy(listing = null, busy = false, notice = PublishNotice.UNPUBLISHED) }
        } catch (e: CancellationException) {
            _state.update { it.copy(busy = false) }
            throw e
        } catch (e: Exception) {
            // 404 = もう掲載が無い(別の端末でやめた)。やめた扱いにする
            if ((e as? ApiError)?.status == 404) {
                _state.update { it.copy(listing = null, busy = false, notice = PublishNotice.UNPUBLISHED) }
                return
            }
            _state.update { it.copy(busy = false, failure = failureOf(e), notice = PublishNotice.UNPUBLISH_FAILED) }
        }
    }

    fun consumeNotice() = _state.update { it.copy(notice = null) }

    companion object {
        const val MAX_PAGES = 3

        /** backend の上限(説明 300・タグ 8 個 × 20 文字)。 */
        const val MAX_DESCRIPTION = 300
        const val MAX_TAGS = 8
        const val MAX_TAG_LENGTH = 20

        /** 「健康 習慣」「#健康、習慣」→ [健康, 習慣](iOS と同じ区切り: 半角・全角の空白・カンマ・読点)。 */
        fun splitTags(raw: String): List<String> =
            raw.split(' ', '　', ',', '、', '\n')
                .map { it.trim().removePrefix("#").take(MAX_TAG_LENGTH) }
                .filter { it.isNotEmpty() }
                .distinct()
                .take(MAX_TAGS)

        fun failureOf(e: Throwable): PublishFailure {
            val api = e as? ApiError
            if (api?.isTextNotAllowed == true) return PublishFailure.TEXT_NOT_ALLOWED
            when (api?.status) {
                403 -> return PublishFailure.NOT_OWNER
                400 -> return PublishFailure.BLANK_APP
            }
            return when (FailureKind.classify(e)) {
                FailureKind.Offline, FailureKind.Timeout -> PublishFailure.OFFLINE
                else -> PublishFailure.OTHER
            }
        }
    }
}
