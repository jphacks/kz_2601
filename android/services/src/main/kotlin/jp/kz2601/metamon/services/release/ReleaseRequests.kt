package jp.kz2601.metamon.services.release

import kotlinx.serialization.Serializable

/**
 * リリース申請の 1 件(#1254・iOS の `ReleaseRequest`・openapi-appconfig.yaml `ReleaseRequest`)。
 * [status] は管理者がコンソールで書き換える(received → reviewing → planned / declined)。
 */
@Serializable
data class ReleaseRequest(
    val id: String,
    val appId: String,
    val name: String = "",
    val dslVersion: Int = 0,
    val purpose: String = "",
    val contact: String = "",
    val status: String = ReleaseStatus.RECEIVED.wire,
) {
    /** 未知の値(コンソールの打ち間違いなど)は受付の扱い */
    val kind: ReleaseStatus get() = ReleaseStatus.of(status)

    companion object {
        /** appId → いま見せる申請(申請中があればそれ、無ければ最新)。[items] は新しい順 */
        fun latestByApp(items: List<ReleaseRequest>): Map<String, ReleaseRequest> {
            val out = linkedMapOf<String, ReleaseRequest>()
            for (r in items) {
                val cur = out[r.appId]
                if (cur == null || (cur.kind == ReleaseStatus.DECLINED && r.kind != ReleaseStatus.DECLINED)) out[r.appId] = r
            }
            return out
        }
    }
}

enum class ReleaseStatus(val wire: String) {
    RECEIVED("received"),
    REVIEWING("reviewing"),
    PLANNED("planned"),
    DECLINED("declined"),
    ;

    companion object {
        fun of(wire: String): ReleaseStatus = entries.firstOrNull { it.wire == wire } ?: RECEIVED
    }
}

/** appconfig の release-requests API(iOS の `ReleaseRequestClient`)。 */
interface ReleaseRequestClient {
    /** 申請する。同じアプリの申請中が既にあればそれが返る(冪等) */
    suspend fun request(appId: String, purpose: String, contact: String?): ReleaseRequest

    /** 自分の申請(全アプリ・新しい順) */
    suspend fun myRequests(): List<ReleaseRequest>
}

/** 入力の上限(backend と同じ) */
object ReleaseRequestLimits {
    const val MAX_PURPOSE = 200
    const val MAX_CONTACT = 100
}

/** 端末の中だけで受ける申請(`METAMON_MOCK`)。backend と同じく申請中があればそれを返す。 */
class MockReleaseRequestClient(private val appName: (String) -> String = { "" }) : ReleaseRequestClient {
    private val items = mutableListOf<ReleaseRequest>()

    override suspend fun request(appId: String, purpose: String, contact: String?): ReleaseRequest = synchronized(items) {
        items.firstOrNull { it.appId == appId && it.kind != ReleaseStatus.DECLINED }?.let { return@synchronized it }
        val r = ReleaseRequest(
            id = "rr_mock_${items.size + 1}",
            appId = appId,
            name = appName(appId),
            purpose = purpose.trim(),
            contact = contact?.trim().orEmpty(),
        )
        items.add(0, r)
        r
    }

    override suspend fun myRequests(): List<ReleaseRequest> = synchronized(items) { items.toList() }
}
