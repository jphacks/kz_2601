package jp.kz2601.metamon.release

import jp.kz2601.metamon.services.release.ReleaseRequest
import jp.kz2601.metamon.services.release.ReleaseRequestClient
import jp.kz2601.metamon.services.release.ReleaseStatus
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * リリース申請の状態(#1254・iOS の `ReleaseUI`)。変身一覧のカードのバッジと申請のシートで共有する。
 * 後片付け(#841)で [reset] する。
 */
class ReleaseRequestsModel(private val client: ReleaseRequestClient, private val log: (String) -> Unit = {}) {
    private val _byApp = MutableStateFlow<Map<String, ReleaseRequest>>(emptyMap())

    /** appId → いま見せる申請(申請中があればそれ) */
    val byApp: StateFlow<Map<String, ReleaseRequest>> = _byApp.asStateFlow()

    /** 自分の申請を読み直す。読めなければ前のまま */
    suspend fun refresh() {
        val items = attempt { client.myRequests() } ?: return
        _byApp.value = ReleaseRequest.latestByApp(items)
    }

    /** 申請中(見送り以外)。あればシートはフォームの代わりに状況を出す */
    fun open(appId: String): ReleaseRequest? = _byApp.value[appId]?.takeIf { it.kind != ReleaseStatus.DECLINED }

    /** 送る。受け付けられた申請(申請中が既にあればそれ)。失敗は null */
    suspend fun send(appId: String, purpose: String, contact: String): ReleaseRequest? {
        val p = purpose.trim()
        if (p.isEmpty()) return null
        val c = contact.trim().ifEmpty { null }
        val r = attempt { client.request(appId, p, c) } ?: return null
        _byApp.update { it + (appId to r) }
        log("release request: status=${r.status}")
        return r
    }

    fun reset() {
        _byApp.value = emptyMap()
    }

    private inline fun <T> attempt(block: () -> T): T? = try {
        block()
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        log("release request failed: ${e.javaClass.simpleName}")
        null
    }
}
