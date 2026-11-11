package jp.kz2601.metamon.account

import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.feeds.FeedCache
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

/** 後片付けで、会話の履歴・投稿に出す名前・feed の覚えた行・端末内の動画 / 録音も消す(#841 + #893 / #875 / #876 / #863 / #1084)。 */
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
        val feedDir = tmp.newFolder("metamon-feeds").apply { resolve("x.json").writeText("{}") }
        val feeds = FeedCache(feedDir, io = EmptyCoroutineContext)
        var resets = 0
        var consentForgotten = 0

        LocalDataWipe(
            engine, DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = { resets++ },
            forgetDeviceState = {},
            chatHistory = history,
            authorProfile = profile,
            removeLocalMedia = { video.parentFile?.deleteRecursively() },
            feedCache = feeds,
            forgetConsent = { consentForgotten++ },
        )()

        assertTrue(history.load("app-of-a").isEmpty())
        assertNull(profile.name)
        assertFalse(profile.asked)
        assertFalse(video.exists())
        assertFalse(feedDir.exists())
        assertEquals(1, resets)
        assertEquals(1, consentForgotten)
    }

    @Test
    fun removesLeftoverCaptureAndRecordingTempFiles() {
        val cache = tmp.newFolder("cache")
        val photo = cache.resolve("metamon_photos").apply { mkdirs() }.resolve("capture-1.jpg").apply { writeText("x") }
        val video = cache.resolve("metamon_videos").apply { mkdirs() }.resolve("capture-1.mp4").apply { writeText("x") }
        val rec = cache.resolve("metamon-rec-abc.m4a").apply { writeText("x") }
        val other = cache.resolve("autonav-step").apply { writeText("x") }

        MediaTempFiles.removeAll(cache)

        assertFalse(photo.exists())
        assertFalse(video.exists())
        assertFalse(rec.exists())
        assertTrue("関係ない cache は残す", other.exists())
    }
}
