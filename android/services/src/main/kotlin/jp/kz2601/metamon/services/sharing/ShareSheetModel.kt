package jp.kz2601.metamon.services.sharing

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Scope
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.Share
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.SharedAccessDenial
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * シェアリンクの形(iOS の `ShareURL`)。受けるのは 2 つ:
 * - `tailorit://import/<token>`(URL スキーム・QR / 共有ページの「アプリで開く」)
 * - `https://tailor-it.app/s/<token>`(App Links・#1272。iOS の Universal Links と同じ URL)
 */
object ShareLinks {
    const val SCHEME = "tailorit"
    private const val HOST = "import"

    /** App Links のドメイン(assetlinks.json を置くサイト・iOS の `ShareURL.webHost`) */
    const val WEB_HOST = "tailor-it.app"
    private val TOKEN = Regex("^[A-Za-z0-9_.:-]{1,128}$")

    /** 末尾の `/`・クエリ・フラグメントは許す。`/s/<token>/<他>` は扱わない(iOS と同じ) */
    private val WEB = Regex("^https://([^/?#]*)/s/([^/?#]*)/?(?:[?#].*)?$", RegexOption.IGNORE_CASE)

    /** シェアリンクのトークン。それ以外の URL は null。 */
    fun token(url: String): String? {
        val token = appToken(url) ?: webToken(url) ?: return null
        // 点だけのものは除く(発行されるトークンは base64url)
        return token.takeIf { TOKEN.matches(it) && it.any { c -> c != '.' } }
    }

    private fun appToken(url: String): String? {
        val m = Regex("^([A-Za-z][A-Za-z0-9+.-]*)://([^/?#]*)/([^/?#]*)").find(url) ?: return null
        val (scheme, host, token) = m.destructured
        if (!scheme.equals(SCHEME, ignoreCase = true) || !host.equals(HOST, ignoreCase = true)) return null
        return token
    }

    /** host はちょうど [WEB_HOST](ポート・`www.`・userinfo は不可)。`http` は扱わない。 */
    private fun webToken(url: String): String? {
        val (host, token) = WEB.find(url)?.destructured ?: return null
        return token.takeIf { host.equals(WEB_HOST, ignoreCase = true) }
    }

    fun appUrl(token: String): String = "$SCHEME://$HOST/$token"

    /** QR・コピーに使うアプリの URL。サーバーの `url` が形に合わなければトークンから作る。 */
    fun appUrl(share: Share): String = share.url.takeIf { token(it) == share.token } ?: appUrl(share.token)

    /** 送る文(iOS の ShareLink: 文の中にアプリの URL・続けてブラウザの URL)。ブラウザの URL が無ければ文だけ。 */
    fun sendText(message: String, share: Share): String =
        if (share.webUrl.isBlank()) message else "$message\n${share.webUrl}"
}

/** 受け取った人に何が渡るか(DSL の entity の scope で決まる。発行の API は範囲を受け取らない)。 */
enum class ShareScope {
    /** private だけ: 受け取った人は自分のアプリとして使う(記録は共有しない) */
    COPY,

    /** public を含む: 元のアプリに加わり、みんなの投稿が見られる */
    PUBLIC,

    /** scope:shared を含む: 参加者になって記録を一緒に書ける(#254) */
    MEMBERS,
    ;

    companion object {
        fun of(dsl: AppDSL): ShareScope = when {
            dsl.entities.any { it.scope == Scope.SHARED } -> MEMBERS
            dsl.entities.any { it.scope == Scope.PUBLIC } -> PUBLIC
            else -> COPY
        }
    }
}

/** 発行したリンク(appId → Share)。開くたびにトークンを増やさない(iOS の `SharingUI.issued`)。人が変わったら作り直す。 */
class IssuedShares {
    private val map = java.util.concurrent.ConcurrentHashMap<String, Share>()
    operator fun get(appId: String): Share? = map[appId]
    operator fun set(appId: String, share: Share?) {
        if (share == null) map.remove(appId) else map[appId] = share
    }
}

/** リンクを作れなかった理由(画面はこれで文言を選ぶ)。 */
enum class ShareFailure {
    /** 403 not_member: scope:shared のアプリに参加していない */
    NOT_MEMBER,

    /** 400: まだ生成していない(中身が初期のまま) */
    BLANK_APP,

    /** 圏外・時間切れ */
    OFFLINE,

    /** サーバーにつながっていない構成(モック) */
    UNAVAILABLE,
    OTHER,
}

/** 一度だけ出すお知らせ。 */
enum class ShareNotice { COPIED, REVOKED, REVOKE_FAILED }

data class ShareSheetState(
    val share: Share? = null,
    val scope: ShareScope? = null,
    val failure: ShareFailure? = null,
    val busy: Boolean = false,
    val notice: ShareNotice? = null,
) {
    val isLoading: Boolean get() = share == null && failure == null
}

/**
 * アプリの共有シート(iOS の `ShareAppSheet` の上半分: リンクの発行・QR・送る・取り消し)。
 * @param client null = シェアが使えない構成(iOS の `services.sharing == nil`)
 * @param loadDsl 範囲の判定に使う DSL(読めなければ null で範囲を出さない)
 */
class ShareSheetModel(
    private val appId: String,
    private val client: AppConfigClient?,
    private val issued: IssuedShares,
    private val loadDsl: suspend () -> AppDSL?,
) {
    private val _state = MutableStateFlow(ShareSheetState())
    val state: StateFlow<ShareSheetState> = _state.asStateFlow()

    /** 開いた時: 発行済みならそれを、無ければ発行する。範囲も読む。 */
    suspend fun load() {
        val cached = issued[appId]
        if (cached != null) _state.update { it.copy(share = cached, failure = null) } else issue()
        val scope = try {
            loadDsl()?.let(ShareScope::of)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
        _state.update { it.copy(scope = scope) }
    }

    /** 発行(「もう一度」も)。 */
    suspend fun issue() {
        if (client == null) {
            _state.update { it.copy(share = null, failure = ShareFailure.UNAVAILABLE) }
            return
        }
        _state.update { it.copy(failure = null) }
        try {
            val share = client.share(appId)
            issued[appId] = share
            _state.update { it.copy(share = share, failure = null) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            _state.update { it.copy(share = null, failure = failureOf(e)) }
        }
    }

    /** 取り消し(確認は画面で取る)。取り消せたら新しいリンクを用意しておく(iOS と同じ)。 */
    suspend fun revoke() {
        val share = _state.value.share ?: return
        val client = client ?: return
        if (_state.value.busy) return
        _state.update { it.copy(busy = true) }
        try {
            client.revokeShare(share.token)
            issued[appId] = null
            _state.update { it.copy(share = null, busy = false, notice = ShareNotice.REVOKED) }
        } catch (e: CancellationException) {
            _state.update { it.copy(busy = false) }
            throw e
        } catch (_: Exception) {
            _state.update { it.copy(busy = false, notice = ShareNotice.REVOKE_FAILED) }
            return
        }
        issue()
    }

    fun noteCopied() = _state.update { it.copy(notice = ShareNotice.COPIED) }

    fun consumeNotice() = _state.update { it.copy(notice = null) }

    companion object {
        fun failureOf(e: Throwable): ShareFailure {
            if (SharedAccessDenial.from(e) == SharedAccessDenial.NOT_MEMBER) return ShareFailure.NOT_MEMBER
            if ((e as? ApiError)?.status == 400) return ShareFailure.BLANK_APP
            return when (FailureKind.classify(e)) {
                FailureKind.Offline, FailureKind.Timeout -> ShareFailure.OFFLINE
                else -> ShareFailure.OTHER
            }
        }
    }
}
