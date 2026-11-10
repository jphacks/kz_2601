package jp.kz2601.metamon.account

import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import jp.kz2601.metamon.services.social.AuthorProfile
import jp.kz2601.metamon.voice.ChatHistoryEntry
import jp.kz2601.metamon.voice.ChatHistoryStore
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import kotlin.coroutines.EmptyCoroutineContext

/** 後片付けで、会話の履歴・投稿に出す名前・端末内の動画 / 録音も消す(#841 + #893 / #875 / #863 / #1084)。 */
class LocalDataWipeTest {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun wipesChatHistoryAuthorNameAndLocalMedia() = runTest {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        val history = ChatHistoryStore(tmp.newFolder("chat_history"))
        history.save(listOf(ChatHistoryEntry(ChatHistoryEntry.Kind.USER, "A の相談")), "app-of-a")
        val profile = AuthorProfile(InMemoryKeyValueStore()).apply { answer("はなこ") }
        val video = tmp.newFolder("metamon-videos").resolve("v1.mp4").apply { writeText("x") }
        var resets = 0

        LocalDataWipe(
            engine, DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = { resets++ },
            forgetDeviceState = {},
            chatHistory = history,
            authorProfile = profile,
            removeLocalMedia = { video.parentFile?.deleteRecursively() },
        )()

        assertTrue(history.load("app-of-a").isEmpty())
        assertNull(profile.name)
        assertFalse(profile.asked)
        assertFalse(video.exists())
        assertEquals(1, resets)
    }
}
