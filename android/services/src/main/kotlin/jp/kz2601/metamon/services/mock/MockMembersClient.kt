package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.sharing.MembersClient
import jp.kz2601.metamon.services.sharing.ROLE_MEMBER
import jp.kz2601.metamon.services.sharing.ROLE_OWNER
import jp.kz2601.metamon.services.sharing.SharedMember
import jp.kz2601.metamon.services.sharing.SharedMemberList
import kotlinx.coroutines.delay
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * 端末内の参加者表(iOS の `DebugMockMembers`・#903)。つまみ `METAMON_MOCK_MEMBERS=owner|member` が自分の立場、
 * `METAMON_MOCK_MEMBERS_FAIL=1` で外す・抜ける・管理者にするを失敗させる(楽観的 UI の巻き戻しを見る)。
 */
class MockMembersClient(
    private val role: String,
    private val failsWrites: Boolean = false,
    private val writeDelayMillis: Long = 300,
) : MembersClient {
    private val mutex = Mutex()
    private var items = listOf(
        SharedMember(uid = "uid-owner", role = ROLE_OWNER, you = role == ROLE_OWNER),
        SharedMember(uid = "uid-dad", displayName = "父", role = ROLE_MEMBER, you = role == ROLE_MEMBER),
        SharedMember(uid = "uid-kid", displayName = "たろう", role = ROLE_MEMBER),
    )

    override suspend fun members(appId: String): SharedMemberList = mutex.withLock { SharedMemberList(items, role) }

    override suspend fun setAdmin(appId: String, uid: String, admin: Boolean) = write {
        items = items.map { if (it.uid == uid) it.copy(admin = admin) else it }
    }

    override suspend fun removeMember(appId: String, uid: String) = write { items = items.filterNot { it.uid == uid } }

    /** iOS と同じく、抜けるのは member の時の自分(父) */
    override suspend fun leave(appId: String) = write { items = items.filterNot { it.uid == "uid-dad" } }

    private suspend fun write(change: () -> Unit) {
        delay(writeDelayMillis)
        if (failsWrites) throw ApiError.Server(503, "unavailable", "mock failure")
        mutex.withLock { change() }
    }
}
