package jp.kz2601.metamon.account

import android.app.Activity
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.runtime.AppDirectory
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.local.AccountOwnerStore
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.CurrentAppStore
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.coroutines.EmptyCoroutineContext

/**
 * 既存のアカウントへ切り替えた後に、そのアカウントの一番新しいアプリを開く(#1317・iOS の `adoptSwitchedAccount`)。
 * 順は「切り替え → 前の人のデータを消す → 一覧の先頭を開く」。
 */
class AccountSwitchResumeTest {
    private val voice = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"fullscreen"},"initialScreen":"voice",
           "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}""",
    )
    private val built = RuntimeFixture.dsl("warikan")

    private class FakeAuth(initial: AuthUser?) : AuthService {
        override val kind = "fake"
        val user = MutableStateFlow(initial)
        override val currentUser: StateFlow<AuthUser?> = user

        override suspend fun signInAnonymously(): AuthUser = error("unused")
        override suspend fun signInWithGoogle(googleIdToken: String): AuthUser = error("unused")
        override suspend fun signInWithApple(activity: Activity): AuthUser = error("unused")
        override suspend fun idToken(forceRefresh: Boolean): String? = user.value?.let { "id-${it.uid}" }
        override suspend fun signOut() {
            user.value = null
        }

        override suspend fun deleteCurrentUser() {
            user.value = null
        }
    }

    private class Fixture(val auth: FakeAuth, val guard: AccountGuard, val runtime: AppRuntime, val log: MutableList<String>)

    /** サーバーの一覧は今サインインしている人の分を返す。 */
    private fun TestScope.fixture(user: AuthUser?, saved: String?, appsOf: Map<String, List<AppSummary>>): Fixture {
        val auth = FakeAuth(user)
        val log = mutableListOf<String>()
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        val appId = CurrentAppStore(null).apply { set("app-of-a") }
        val apps = AppListCache(null)
        val runtime = AppRuntime(
            AppDocument("app-of-a", 0, voice), engine, backgroundScope,
            restore = { id -> if (id.startsWith("app-of-b")) AppDocument(id, 3, built) else AppDocument(id, 0, voice) },
            directory = AppDirectory(
                appId,
                list = {
                    log += "list"
                    appsOf[auth.user.value?.uid].orEmpty()
                },
                apps,
                fresh = { AppDocument(it, 0, voice) },
            ),
        )
        val wipe = LocalDataWipe(engine, DocumentCache(null, EmptyCoroutineContext), apps, resetRuntime = runtime::resetForAccountChange, forgetDeviceState = {})
        lateinit var guard: AccountGuard
        guard = AccountGuard(
            auth, AccountOwnerStore(null).apply { uid = saved }, ActiveOwner(),
            startSync = engine::start,
            stopSync = engine::stop,
            wipe = {
                wipe()
                log += "wipe"
            },
            onOwnerReplaced = {
                log += "replaced:${guard.generation.value}"
                runtime.resumeAfterAccountSwitch()
            },
        )
        return Fixture(auth, guard, runtime, log)
    }

    private val a = AuthUser("uid-a", isAnonymous = true)
    private val b = AuthUser("uid-b", isAnonymous = false, provider = AuthUser.GOOGLE)
    private val appsOfB = listOf(
        AppSummary(id = "app-of-b-old", name = "古い", currentVersion = 2, updatedAt = "2026-11-01T00:00:00Z"),
        AppSummary(id = "app-of-b-new", name = "新しい", currentVersion = 3, updatedAt = "2026-11-10T00:00:00Z"),
    )

    @Test
    fun switchingWipesThenOpensTheLatestAppOfTheAccount() = runTest {
        val f = fixture(user = a, saved = "uid-a", appsOf = mapOf("uid-b" to appsOfB))
        f.guard.start(backgroundScope)
        runCurrent()
        assertTrue(f.runtime.dsl.isVoiceOnly)

        f.auth.user.value = b // 既存のアカウントへ切り替え
        runCurrent()

        assertEquals(listOf("wipe", "replaced:1", "list"), f.log.take(3))
        assertEquals("app-of-b-new", f.runtime.document.appId)
        assertFalse(f.runtime.dsl.isVoiceOnly)
        assertEquals(b, f.guard.active.value)
    }

    @Test
    fun accountWithoutAppsStaysOnTheVoiceScreen() = runTest {
        val f = fixture(user = a, saved = "uid-a", appsOf = emptyMap())
        f.guard.start(backgroundScope)
        runCurrent()
        val before = f.runtime.document.appId

        f.auth.user.value = b
        runCurrent()

        assertTrue(f.runtime.dsl.isVoiceOnly)
        assertTrue("後片付けで新しい appId になる", f.runtime.document.appId != before)
    }

    @Test
    fun signingInAfterSignOutDoesNotRunTheSwitchFlow() = runTest {
        val f = fixture(user = a, saved = "uid-a", appsOf = mapOf("uid-b" to appsOfB))
        f.guard.start(backgroundScope)
        runCurrent()

        f.auth.user.value = null // サインアウトで消して持ち主を忘れる
        runCurrent()
        f.auth.user.value = b
        runCurrent()

        assertEquals(listOf("wipe"), f.log)
    }

    @Test
    fun sameAccountAtLaunchDoesNotRunTheSwitchFlow() = runTest {
        val f = fixture(user = a, saved = "uid-a", appsOf = mapOf("uid-a" to appsOfB))
        f.guard.start(backgroundScope)
        runCurrent()

        assertTrue(f.log.isEmpty())
        assertEquals("app-of-a", f.runtime.document.appId)
    }
}
