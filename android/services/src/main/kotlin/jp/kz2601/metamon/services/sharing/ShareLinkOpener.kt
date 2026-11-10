package jp.kz2601.metamon.services.sharing

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.SharePreview
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.SharedAccessDenial
import jp.kz2601.metamon.services.social.AuthorProfile
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.getAndUpdate

/**
 * 受け取ったシェアリンク(iOS の `SharingUI.pendingLink`・#278)。サインイン前・準備の前に届いた分も捨てずに持ち、
 * 準備ができてから 1 回だけ渡す。シェアリンクでない URL は触らない。
 */
class ShareLinkInbox {
    private val _pending = MutableStateFlow<String?>(null)

    /** 保留中のトークン。 */
    val pending: StateFlow<String?> = _pending.asStateFlow()

    /** シェアリンクなら保留して true(後から来た方で上書き)。それ以外は false。 */
    fun receive(url: String?): Boolean {
        val token = url?.let(ShareLinks::token) ?: return false
        _pending.value = token
        return true
    }

    /** 準備済みなら保留中のトークンを渡して空にする。まだなら null(保留は残す)。 */
    fun take(ready: Boolean): String? {
        if (!ready) return null
        return _pending.getAndUpdate { null }
    }

    companion object {
        /**
         * 開いてよいか: サインイン済み・起動の読み込みが済み・AI の同意の画面を出し終えた(出す予定も無い)。
         * 同意の画面と参加の確認を重ねない。
         */
        fun ready(signedIn: Boolean, bootstrapped: Boolean, consentShowing: Boolean, consentPending: Boolean): Boolean =
            signedIn && bootstrapped && !consentShowing && !consentPending
    }
}

/** 記録を共有するアプリのリンクを開いた時の、取り込む前の確認(iOS の `PendingJoin`・#254)。 */
data class PendingJoin(val token: String, val preview: SharePreview)

/** 開けなかった理由(画面はこれで文言を選ぶ)。 */
enum class ShareOpenFailure {
    /** サーバーにつながっていない構成(モック) */
    UNAVAILABLE,

    /** 404: 取り消されたか、無い */
    REVOKED,

    /** 403 inviter_left: リンクを送った人がもう参加していない */
    INVITER_LEFT,

    /** 403 members_full: 参加者が上限 */
    MEMBERS_FULL,

    /** 422 text_not_allowed: 表示名に表示できない言葉(import で見るのは表示名だけ・#321) */
    NAME_REJECTED,

    /** 圏外・時間切れ */
    OFFLINE,
    OTHER,
}

sealed interface ShareOpenResult {
    /** 参加の確認画面を出す(表示名を聞く) */
    data class AskToJoin(val join: PendingJoin) : ShareOpenResult

    /** 取り込んだ・参加した。このアプリに切り替える。[viaJoin] = 確認画面から参加した */
    data class Added(val app: InstallResult, val viaJoin: Boolean) : ShareOpenResult

    data class Failed(val reason: ShareOpenFailure) : ShareOpenResult
}

/**
 * `tailorit://import/<token>` を開く(iOS の `AppRuntime.importShared` / `joinShared`)。
 * 先に `GET /v1/shares/{token}` で確かめ、記録を共有するアプリ(自分のものでない)なら参加の確認へ。
 * それ以外はすぐ取り込む。確かめられなかった時も取り込みに進む(iOS と同じ)。
 * @param client null = シェアが使えない構成
 * @param profile 参加の表示名の初期値と、入れた名前の覚え先(#1152)
 */
class ShareLinkOpener(
    private val client: AppConfigClient?,
    private val profile: AuthorProfile?,
) {
    suspend fun open(token: String): ShareOpenResult {
        val client = client ?: return ShareOpenResult.Failed(ShareOpenFailure.UNAVAILABLE)
        val preview = try {
            client.previewShare(token)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
        if (preview != null && preview.sharedData && !preview.mine) {
            return ShareOpenResult.AskToJoin(PendingJoin(token, preview))
        }
        return importNow(client, token, displayName = null, viaJoin = false)
    }

    /** 確認画面の「参加する」。空欄なら名前なしで参加する。 */
    suspend fun join(join: PendingJoin, displayName: String): ShareOpenResult {
        val client = client ?: return ShareOpenResult.Failed(ShareOpenFailure.UNAVAILABLE)
        val name = AuthorProfile.normalized(displayName)
        if (name != null) profile?.answer(name)
        return importNow(client, join.token, name, viaJoin = true)
    }

    /** 参加の画面の表示名の初期値(設定の「投稿に表示する名前」と同じ)。 */
    fun initialDisplayName(): String = profile?.name.orEmpty()

    private suspend fun importNow(client: AppConfigClient, token: String, displayName: String?, viaJoin: Boolean): ShareOpenResult = try {
        ShareOpenResult.Added(client.importShare(token, displayName), viaJoin)
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        ShareOpenResult.Failed(failureOf(e))
    }

    companion object {
        fun failureOf(e: Throwable): ShareOpenFailure {
            when (SharedAccessDenial.from(e)) {
                SharedAccessDenial.INVITER_LEFT -> return ShareOpenFailure.INVITER_LEFT
                SharedAccessDenial.MEMBERS_FULL -> return ShareOpenFailure.MEMBERS_FULL
                else -> Unit
            }
            val api = e as? ApiError
            if (api?.isTextNotAllowed == true) return ShareOpenFailure.NAME_REJECTED
            if (api?.status == 404) return ShareOpenFailure.REVOKED
            return when (FailureKind.classify(e)) {
                FailureKind.Offline, FailureKind.Timeout -> ShareOpenFailure.OFFLINE
                else -> ShareOpenFailure.OTHER
            }
        }
    }
}
