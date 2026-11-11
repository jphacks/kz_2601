package jp.kz2601.metamon.account

import android.app.Activity
import jp.kz2601.metamon.auth.AuthSession
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.runtime.AppDirectory
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.Record
import jp.kz2601.metamon.services.account.AccountClient
import jp.kz2601.metamon.services.account.SignOutFlow
import jp.kz2601.metamon.services.local.AccountOwnerStore
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.ConnectivityMonitor
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
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.IOException
import kotlin.coroutines.EmptyCoroutineContext

/**
 * 端末のデータの後片付け(#841)。サインアウト(外からも)・uid が変わった時に前の人のデータが全部消え、
 * 前の人の送信待ちが次の人のトークンで送られず、サインイン前は同期を始めない。
 */
class AccountGuardTest {
    private val voice = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"fullscreen"},"initialScreen":"voice",
           "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}""",
    )

    private class FakeAuth(initial: AuthUser?) : AuthService {
        override val kind = "fake"
        val user = MutableStateFlow(initial)
        override val currentUser: StateFlow<AuthUser?> = user
        var deleted = false

        override suspend fun signInAnonymously(): AuthUser = error("unused")
        override suspend fun signInWithGoogle(googleIdToken: String): AuthUser = error("unused")
        override suspend fun signInWithApple(activity: Activity): AuthUser = error("unused")
        override suspend fun idToken(forceRefresh: Boolean): String? = user.value?.let { "id-${it.uid}" }
        override suspend fun signOut() {
            user.value = null
        }

        override suspend fun deleteCurrentUser() {
            deleted = true
            user.value = null
        }
    }

    private class Net(online: Boolean = false) : ConnectivityMonitor {
        override val online = MutableStateFlow(online)
    }

    /** 記録の作成のたびに、その時のトークンを覚える(送れたことにはしない)。 */
    private class TokenSpy(private val tokens: OwnerGatedTokens) : AppConfigClient by RuntimeFixture.offlineClient {
        val sentWith = mutableListOf<String?>()

        override suspend fun createRecord(appId: String, entity: String, data: JsonObject, clientId: String, authorName: String?): Record {
            sentWith += tokens.idToken(false)
            throw IOException("offline")
        }
    }

    private class Fixture(
        val auth: FakeAuth,
        val guard: AccountGuard,
        val engine: RecordSyncEngine,
        val net: Net,
        val spy: TokenSpy,
        val runtime: AppRuntime,
        val appId: CurrentAppStore,
        val apps: AppListCache,
        val documents: DocumentCache,
        val saved: AccountOwnerStore,
        val tokens: OwnerGatedTokens,
    ) {
        var syncStarts = 0
        var forgotPermissions = 0
        var signedOut = 0
    }

    private fun TestScope.fixture(user: AuthUser?, saved: String? = null, online: Boolean = false): Fixture {
        val auth = FakeAuth(user)
        val owner = ActiveOwner()
        val tokens = OwnerGatedTokens(auth, owner)
        val spy = TokenSpy(tokens)
        val net = Net(online)
        val store = RecordStore(null, EmptyCoroutineContext)
        val queue = SyncQueue(null, store, spy, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, spy, net, backgroundScope)
        val appId = CurrentAppStore(null).apply { set("app-of-a") }
        val apps = AppListCache(null)
        val documents = DocumentCache(null, EmptyCoroutineContext)
        val runtime = AppRuntime(
            AppDocument("app-of-a", 0, voice), engine, backgroundScope,
            directory = AppDirectory(appId, { throw IOException("offline") }, apps, fresh = { AppDocument(it, 0, voice) }),
        )
        val ownerStore = AccountOwnerStore(null).apply { uid = saved }
        lateinit var f: Fixture
        val wipe = LocalDataWipe(engine, documents, apps, resetRuntime = runtime::resetForAccountChange, forgetDeviceState = { f.forgotPermissions++ })
        val guard = AccountGuard(
            auth, ownerStore, owner,
            startSync = {
                f.syncStarts++
                engine.start()
            },
            stopSync = engine::stop,
            wipe = { wipe() },
            onSignedOut = { f.signedOut++ },
        )
        f = Fixture(auth, guard, engine, net, spy, runtime, appId, apps, documents, ownerStore, tokens)
        return f
    }

    private val a = AuthUser("uid-a", isAnonymous = true)
    private val b = AuthUser("uid-b", isAnonymous = false, provider = AuthUser.GOOGLE)
    private val note = JsonObject(mapOf("text" to JsonPrimitive("A のメモ")))

    /** A のデータを端末に残す(未送信の記録・保存した DSL・一覧)。 */
    private suspend fun Fixture.leaveDataOfA() {
        engine.save("app-of-a", "notes", null, note)
        documents.store("app-of-a", """{"appId":"app-of-a","version":3,"dsl":{}}""")
        apps.store(listOf(AppSummary(id = "app-of-a", name = "A のアプリ")))
        runtime.appsList() // 一覧を読み込んだ状態(サーバーに届かず保存した一覧)
    }

    @Test
    fun signedOutAtLaunchDoesNotStartSync() = runTest {
        val f = fixture(user = null)
        f.guard.start(backgroundScope)
        runCurrent()

        assertEquals(0, f.syncStarts)
        assertNull(f.guard.active.value)
        assertNull(f.tokens.idToken(false))
    }

    @Test
    fun signedOutHookRunsOnlyWhenSomeoneSignsOut() = runTest {
        val f = fixture(user = null)
        f.guard.start(backgroundScope)
        runCurrent()
        assertEquals("起動時のサインインしていない状態では呼ばない(サインイン画面で受けたリンクを残す)", 0, f.signedOut)

        f.auth.user.value = a
        runCurrent()
        assertEquals(0, f.signedOut)

        f.auth.user.value = null // 外からのサインアウト
        runCurrent()
        assertEquals(1, f.signedOut)
    }

    @Test
    fun signingInAdoptsTheOwnerAndStartsSync() = runTest {
        val f = fixture(user = null)
        f.guard.start(backgroundScope)
        runCurrent()

        f.auth.user.value = a
        runCurrent()

        assertEquals(1, f.syncStarts)
        assertEquals(a, f.guard.active.value)
        assertEquals("uid-a", f.saved.uid)
        assertEquals("id-uid-a", f.tokens.idToken(false))
        assertEquals(0, f.guard.generation.value) // 初めての人は何も消さない
    }

    @Test
    fun changingUidWipesEverythingOfThePreviousPerson() = runTest {
        val f = fixture(user = a)
        f.guard.start(backgroundScope)
        runCurrent()
        f.leaveDataOfA()
        assertEquals(1, f.engine.queue.count())
        assertEquals(1, f.runtime.apps.value.size)

        f.auth.user.value = b
        runCurrent()

        assertEquals(0, f.engine.queue.count())
        assertTrue(f.engine.cached("app-of-a", "notes").isEmpty())
        assertNull(f.documents.loadJson("app-of-a"))
        assertTrue(f.apps.load().isEmpty())
        assertTrue(f.runtime.apps.value.isEmpty())
        assertNotEquals("app-of-a", f.appId.current)
        assertEquals(f.appId.current, f.runtime.document.appId)
        assertEquals(1, f.forgotPermissions)
        assertEquals(1, f.guard.generation.value)
        assertEquals(b, f.guard.active.value)
        assertEquals("uid-b", f.saved.uid)
    }

    @Test
    fun pendingRecordsOfThePreviousPersonAreNeverSentWithTheNextToken() = runTest {
        val f = fixture(user = a)
        f.guard.start(backgroundScope)
        runCurrent()
        f.leaveDataOfA()

        // B に変わった直後(見張りが後片付けする前)に電波が戻って列が流れても、B のトークンは付かない
        assertEquals("uid-a", f.guard.owner.uid)
        f.auth.user.value = b
        assertNull(f.tokens.idToken(false))
        f.engine.queue.resume() // 見張りより先に列が流れた

        assertTrue(f.spy.sentWith.isNotEmpty())
        assertTrue(f.spy.sentWith.all { it == null })
        runCurrent()
        assertEquals(0, f.engine.queue.count())
        f.net.online.value = true
        runCurrent()
        assertFalse(f.spy.sentWith.contains("id-uid-b"))
        // 受け入れた後は B のトークンが付く
        assertEquals("id-uid-b", f.tokens.idToken(false))
    }

    @Test
    fun externalSignOutWipesAndStopsSync() = runTest {
        val f = fixture(user = a)
        f.guard.start(backgroundScope)
        runCurrent()
        f.leaveDataOfA()

        f.auth.user.value = null // 外から(トークンの失効・他の画面)サインアウトされた
        runCurrent()

        assertNull(f.guard.active.value)
        assertEquals(0, f.engine.queue.count())
        assertTrue(f.apps.load().isEmpty())
        assertNull(f.saved.uid)
        assertEquals(1, f.guard.generation.value)
        assertNull(f.tokens.idToken(false))
    }

    @Test
    fun relaunchWithAnotherPersonWipesBeforeStartingSync() = runTest {
        // A の未送信が残ったままアプリが落ち、その間に B が入った
        val f = fixture(user = b, saved = "uid-a")
        f.engine.save("app-of-a", "notes", null, note)

        f.guard.start(backgroundScope)
        runCurrent()

        assertEquals(0, f.engine.queue.count())
        assertEquals(1, f.guard.generation.value)
        assertEquals(1, f.syncStarts)
        assertEquals("uid-b", f.saved.uid)
    }

    @Test
    fun relaunchWithTheSamePersonKeepsTheData() = runTest {
        val f = fixture(user = a, saved = "uid-a")
        f.engine.save("app-of-a", "notes", null, note)

        f.guard.start(backgroundScope)
        runCurrent()

        assertEquals(1, f.engine.queue.count())
        assertEquals(0, f.guard.generation.value)
        assertEquals("app-of-a", f.appId.current)
    }

    @Test
    fun linkingKeepsTheSameUidAndTheData() = runTest {
        val f = fixture(user = a)
        f.guard.start(backgroundScope)
        runCurrent()
        f.leaveDataOfA()

        val linked = a.copy(isAnonymous = false, provider = AuthUser.GOOGLE)
        f.auth.user.value = linked
        runCurrent()

        assertEquals(linked, f.guard.active.value)
        assertEquals(1, f.engine.queue.count())
        assertEquals(0, f.guard.generation.value)
    }

    @Test
    fun signOutWipesOnceAndSignsOut() = runTest {
        val f = fixture(user = a)
        f.guard.start(backgroundScope)
        runCurrent()
        f.leaveDataOfA()
        val session = AuthSession(f.auth)

        assertEquals(SignOutFlow.Prompt.Unsent(1, canSend = false), session.signOutPrompt(f.engine))
        val outcome = session.signOutAndWipe(SignOutFlow.Choice.DISCARD_AND_WIPE, f.engine, f.guard::wipeForSignOut)
        runCurrent()

        assertEquals(SignOutFlow.Outcome.SignedOut, outcome)
        assertNull(f.auth.user.value)
        assertNull(f.guard.active.value)
        assertEquals(0, f.engine.queue.count())
        assertTrue(f.apps.load().isEmpty())
        assertEquals(1, f.guard.generation.value) // 続くサインアウトでもう一度は消さない
    }

    @Test
    fun sendThenSignOutKeepsEverythingWhenRecordsCannotBeSent() = runTest {
        val f = fixture(user = a)
        f.guard.start(backgroundScope)
        runCurrent()
        f.leaveDataOfA()
        val session = AuthSession(f.auth)

        val outcome = session.signOutAndWipe(SignOutFlow.Choice.SEND_THEN_WIPE, f.engine, f.guard::wipeForSignOut)

        assertEquals(SignOutFlow.Outcome.Unsent(1), outcome)
        assertEquals(a, f.auth.user.value)
        assertEquals(1, f.engine.queue.count())
        assertEquals(0, f.guard.generation.value)
    }

    @Test
    fun deletingTheAccountCallsTheServerThenWipesAndSignsOut() = runTest {
        val f = fixture(user = a)
        f.guard.start(backgroundScope)
        runCurrent()
        f.leaveDataOfA()
        var serverDeleted = false
        val account = object : AccountClient {
            override suspend fun deleteAccount() {
                assertEquals("id-uid-a", f.tokens.idToken(false)) // 削除の口には本人のトークンで送る
                serverDeleted = true
            }
        }

        AuthSession(f.auth).deleteAccount(account, reauthenticate = { false }, wipe = f.guard::wipeForSignOut)
        runCurrent()

        assertTrue(serverDeleted)
        assertTrue(f.auth.deleted)
        assertNull(f.guard.active.value)
        assertEquals(0, f.engine.queue.count())
        assertTrue(f.apps.load().isEmpty())
        assertNull(f.saved.uid)
    }

    @Test
    fun failedServerDeletionKeepsTheAccountAndTheData() = runTest {
        val f = fixture(user = a)
        f.guard.start(backgroundScope)
        runCurrent()
        f.leaveDataOfA()
        val account = object : AccountClient {
            override suspend fun deleteAccount() = throw IOException("offline")
        }

        try {
            AuthSession(f.auth).deleteAccount(account, reauthenticate = { false }, wipe = f.guard::wipeForSignOut)
            fail("サーバーに届かなければ throw する")
        } catch (_: IOException) {
        }
        runCurrent()

        assertFalse(f.auth.deleted)
        assertEquals(a, f.guard.active.value)
        assertEquals(1, f.engine.queue.count())
        assertEquals(0, f.guard.generation.value)
    }
}
