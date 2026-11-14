package jp.kz2601.metamon.services.follows

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.IOException

/** iOS の FollowStoreTests を写したもの(楽観的に反映・失敗で戻す・送信中の二度押し)。 */
class FollowStoreTest {
    private class FakeFollows : FollowsClient {
        var uids = listOf<String>()
        var fail: Exception? = null
        var gate: CompletableDeferred<Unit>? = null
        val calls = mutableListOf<Pair<String, Boolean>>()

        /** サーバーの答えを変える(null は頼まれた通り) */
        var answer: Boolean? = null

        override suspend fun list(appId: String): List<String> {
            fail?.let { throw it }
            return uids
        }

        override suspend fun setFollow(appId: String, uid: String, on: Boolean): Boolean {
            calls += uid to on
            gate?.await()
            fail?.let { throw it }
            return answer ?: on
        }
    }

    private val client = FakeFollows()
    private val store = FollowStore("app-1", client)

    @Test
    fun loadReplacesTheListAndKeepsTheOldOneOnFailure() = runTest {
        assertNull(store.uids)
        client.uids = listOf("a", "b")
        store.load()
        assertEquals(setOf("a", "b"), store.uids)

        client.fail = IOException("offline")
        try {
            store.load()
            fail("throws")
        } catch (_: IOException) {
        }
        assertEquals(setOf("a", "b"), store.uids)
    }

    @Test
    fun followShowsBeforeTheServerAnswers() = runTest(UnconfinedTestDispatcher()) {
        val gate = CompletableDeferred<Unit>().also { client.gate = it }
        val job = async { store.set("a", on = true) }
        assertTrue("先に画面に出す", store.isFollowing("a"))
        assertEquals(setOf("a"), store.snapshot.value.busy)

        store.set("a", on = true) // 送信中の二度押しは送らない
        assertEquals(1, client.calls.size)

        gate.complete(Unit)
        job.await()
        assertTrue(store.isFollowing("a"))
        assertTrue(store.snapshot.value.busy.isEmpty())
    }

    @Test
    fun failureRestoresTheListBeforeTheTapAndThrows() = runTest {
        client.uids = listOf("a")
        store.load()
        val before = store.uids
        client.fail = IOException("offline")
        try {
            store.set("a", on = false)
            fail("throws")
        } catch (_: IOException) {
        }
        assertSame(before, store.uids)
        assertTrue(store.snapshot.value.busy.isEmpty())
    }

    @Test
    fun failureBeforeTheFirstLoadGoesBackToUnread() = runTest {
        client.fail = IOException("offline")
        runCatching { store.set("a", on = true) }
        assertNull("読めていない状態に戻す", store.uids)
    }

    @Test
    fun theServerAnswerWins() = runTest {
        client.answer = false // 例: 参加者でなくなっていた
        store.set("a", on = true)
        assertFalse(store.isFollowing("a"))
        assertEquals(emptySet<String>(), store.uids)
    }
}
