package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import jp.kz2601.metamon.services.social.AuthorProfile
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.coroutines.EmptyCoroutineContext

/** 共有アプリへの初めての投稿で名前を聞き、答えた名前を送る行に付ける(iOS の AppRuntime+Chat・#875)。 */
class AuthorNameTest {
    private val appId = "test-chat"

    private fun TestScope.rt(profile: AuthorProfile?): AppRuntime {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        return AppRuntime(
            AppDocument(appId, 1, RuntimeFixture.dsl("chat")),
            engine,
            backgroundScope,
            currentUserId = { "me" },
            authorProfile = profile,
        )
    }

    @Test
    fun asksOnceForSharedEntityAndAttachesTheName() = runTest {
        val profile = AuthorProfile(InMemoryKeyValueStore())
        val rt = rt(profile)
        assertTrue(rt.needsAuthorName("messages"))
        assertFalse(rt.needsAuthorName("nope"))

        rt.answerAuthorName("はなこ")
        assertFalse(rt.needsAuthorName("messages"))
        assertTrue(rt.addRecord("messages", mapOf("text" to JSONValue.Str("やあ"))))

        val row = rt.recordSync.cached(appId, "messages").single()
        assertEquals("はなこ", row.authorName)
        assertEquals("me", row.author)
    }

    @Test
    fun withoutProfileNeverAsks() = runTest {
        assertFalse(rt(null).needsAuthorName("messages"))
    }
}
