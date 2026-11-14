package jp.kz2601.metamon.account

import jp.kz2601.metamon.renderer.components.RemoteImages
import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.feeds.FeedCache
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import jp.kz2601.metamon.services.push.InMemoryPushPrefs
import jp.kz2601.metamon.services.push.NotifyConsentStore
import jp.kz2601.metamon.services.push.PushDeviceRegistrar
import jp.kz2601.metamon.services.push.forgetAccount
import jp.kz2601.metamon.services.social.AuthorProfile
import jp.kz2601.metamon.services.update.NoticeDismissals
import jp.kz2601.metamon.switcher.AppThumbnailStore
import jp.kz2601.metamon.voice.ChatHistoryEntry
import jp.kz2601.metamon.voice.ChatHistoryStore
import jp.kz2601.metamon.widget.WidgetSnapshot
import jp.kz2601.metamon.widget.WidgetSnapshotStore
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import kotlin.coroutines.EmptyCoroutineContext

/** 後片付けで、会話の履歴・投稿に出す名前・feed の覚えた行・端末内の動画 / 録音・https の画像のキャッシュ・切り替え一覧のサムネも消す(#841 + #893 / #875 / #876 / #863 / #1084 / #1199)。 */
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
        val images = tmp.newFolder(RemoteImages.DISK_DIR).apply { resolve("journal").writeText("x") }
        val thumbDir = tmp.newFolder("metamon-thumbs").apply { resolve("app-of-a.jpg").writeText("x") }
        var resets = 0
        var consentForgotten = 0

        LocalDataWipe(
            engine, DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = { resets++ },
            forgetDeviceState = {},
            chatHistory = history,
            authorProfile = profile,
            removeLocalMedia = { video.parentFile?.deleteRecursively() },
            clearImageCache = { RemoteImages.wipe(images) },
            feedCache = feeds,
            thumbnails = AppThumbnailStore(thumbDir),
            forgetConsent = { consentForgotten++ },
        )()

        assertTrue(history.load("app-of-a").isEmpty())
        assertNull(profile.name)
        assertFalse(profile.asked)
        assertFalse(video.exists())
        assertFalse(feedDir.exists())
        assertFalse(images.exists())
        assertFalse("切り替え一覧のサムネも消す(#1199)", thumbDir.exists())
        assertEquals(1, resets)
        assertEquals(1, consentForgotten)
    }

    /** ホーム画面のウィジェットの中身も消す(#912)。ランタイムを戻した後に消す(戻す前の値で書き直されない)。 */
    @Test
    fun clearsWidgetSnapshotAfterResettingRuntime() = runTest {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        val widget = WidgetSnapshotStore(InMemoryKeyValueStore()).apply {
            save(WidgetSnapshot("app-of-a", "家計簿", listOf(WidgetSnapshot.Stat("k", "合計", "¥1")), 0))
        }
        val order = mutableListOf<String>()

        LocalDataWipe(
            engine, DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = { order += "reset" },
            forgetDeviceState = {},
            clearWidget = {
                order += "widget"
                widget.clear()
            },
        )()

        assertNull(widget.load())
        assertEquals(listOf("reset", "widget"), order)
    }

    /** 通知の同意(uid + appId ごと)と閉じたお知らせも消す。installationId は端末で 1 つなので残す(iOS `LocalWipe.preservedKeys`)。 */
    @Test
    fun wipesNotifyConsentAndNoticeDismissals() = runTest {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        val push = InMemoryPushPrefs().apply { putString(PushDeviceRegistrar.INSTALLATION_ID_KEY, "install-1") }
        val consent = NotifyConsentStore(push).apply { set(true, "uid-a", "app-1") }
        val notices = NoticeDismissals(InMemoryKeyValueStore()).apply { dismiss("notice-1") }

        LocalDataWipe(
            engine, DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = {},
            forgetDeviceState = {
                push.forgetAccount()
                notices.clear()
            },
        )()

        assertFalse(consent.allowed("uid-a", "app-1"))
        assertFalse(notices.isDismissed("notice-1"))
        assertEquals("install-1", push.getString(PushDeviceRegistrar.INSTALLATION_ID_KEY))
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
