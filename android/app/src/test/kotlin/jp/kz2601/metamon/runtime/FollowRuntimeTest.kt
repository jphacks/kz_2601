package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.usesFollows
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.components.social.followedFor
import jp.kz2601.metamon.renderer.components.social.showsFollowButton
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.follows.FollowsClient
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import jp.kz2601.metamon.services.social.MatchingAgeGate
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import kotlin.coroutines.EmptyCoroutineContext

/** follow / unfollow(#1028・iOS の AppRuntime+Follows)を AppRuntime 越しに試す。 */
@OptIn(ExperimentalCoroutinesApi::class)
class FollowRuntimeTest {
    private val appId = "test-follow"

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"タイムライン"},"initialScreen":"following",
         "entities":[{"id":"posts","name":"投稿","scope":"public","fields":[{"id":"body","name":"本文","type":"string"}]}],
         "screens":[
           {"id":"following","title":"フォロー","components":[
             {"type":"list","id":"tl","area":"A1:F12","entity":"posts","item":{"title":"body"},
              "filter":{"field":"${'$'}author","op":"eq","value":{"myFollows":true}}}]},
           {"id":"post","title":"投稿","entity":"posts","components":[
             {"type":"button","id":"follow","area":"A1:C2","label":"フォローする","action":{"type":"follow","entity":"posts"}},
             {"type":"button","id":"unfollow","area":"D1:F2","label":"フォロー中","action":{"type":"unfollow","entity":"posts"}}]}
         ]}
        """.trimIndent(),
    )

    private class FakeFollows : FollowsClient {
        var uids = listOf<String>()
        var fail: Exception? = null
        var gate: CompletableDeferred<Unit>? = null
        val calls = mutableListOf<Pair<String, Boolean>>()

        override suspend fun list(appId: String): List<String> = uids

        override suspend fun setFollow(appId: String, uid: String, on: Boolean): Boolean {
            calls += uid to on
            gate?.await()
            fail?.let { throw it }
            return on
        }
    }

    private val follows = FakeFollows()
    private val follow = Action(type = ActionType.FOLLOW, entity = "posts")
    private val unfollow = Action(type = ActionType.UNFOLLOW, entity = "posts")
    private val route = Route("post")

    private suspend fun TestScope.rt(
        services: SocialServices? = SocialServices(null, null, MatchingAgeGate(InMemoryKeyValueStore()), follows),
        doc: AppDSL = dsl,
        uid: () -> String? = { "me" },
    ): AppRuntime {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        engine.write(appId, "posts", null, JsonObject(mapOf("body" to JsonPrimitive("こんにちは"))), "other", "はなこ", shared = true)
        engine.write(appId, "posts", null, JsonObject(mapOf("body" to JsonPrimitive("自分の"))), "me", "自分", shared = true)
        val rt = AppRuntime(AppDocument(appId, 1, doc), engine, backgroundScope, currentUserId = uid, socialServices = services)
        rt.load(setOf("posts"))
        return rt
    }

    private fun AppRuntime.post(author: String): Record = records("posts").first { it.author == author }

    private fun AppRuntime.ctx(author: String) = ActionContext(route, recordId = post(author).id)

    @Test
    fun usesFollowsOnlyWithTheFilterOrTheButtons() {
        assertTrue(dsl.usesFollows)
        val plain = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"x"},"initialScreen":"a","screens":[{"id":"a","title":"a","components":[]}]}""",
        )
        assertFalse(plain.usesFollows)
    }

    @Test
    fun followShowsAtOnceAndSwapsTheButton() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt()
        val id = rt.post("other").id
        assertEquals("最初の描画で一覧を読みに行く", emptySet<String>(), rt.followedAuthors())
        assertTrue(rt.showsFollowButton(follow, id))
        assertFalse(rt.showsFollowButton(unfollow, id))

        val gate = CompletableDeferred<Unit>().also { follows.gate = it }
        val job = launch { rt.run(follow, rt.ctx("other")) }
        assertEquals("サーバーの答えより先に出す", true, rt.followState("posts", id))
        assertFalse(rt.showsFollowButton(follow, id))
        assertTrue(rt.showsFollowButton(unfollow, id))
        assertEquals("フォローした人の投稿の一覧", setOf("other"), rt.followedFor(listOf(Filter("\$author", "eq", Filter.myFollowsValue))))

        gate.complete(Unit)
        job.join()
        assertEquals(listOf("other" to true), follows.calls)
        assertEquals(FollowCopy.FOLLOWED, rt.toast)
    }

    @Test
    fun failureRestoresAndToastsTheReason() = runTest(UnconfinedTestDispatcher()) {
        follows.uids = listOf("other")
        val rt = rt()
        rt.followedAuthors()
        val id = rt.post("other").id
        assertEquals(true, rt.followState("posts", id))

        follows.fail = IOException("offline")
        rt.run(unfollow, rt.ctx("other"))
        assertEquals("元に戻す", true, rt.followState("posts", id))
        assertEquals(FollowCopy.OFFLINE, rt.toast)
    }

    @Test
    fun ownPostsAndNoPostHaveNoButton() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt()
        rt.followedAuthors()
        assertNull(rt.followState("posts", rt.post("me").id))
        assertFalse(rt.showsFollowButton(follow, rt.post("me").id))
        assertFalse(rt.showsFollowButton(follow, null))

        rt.run(follow, rt.ctx("me"))
        assertEquals(FollowCopy.OPEN_POST_FIRST, rt.toast)
        rt.run(follow, ActionContext(route))
        assertEquals(FollowCopy.OPEN_POST_FIRST, rt.toast)
        assertTrue(follows.calls.isEmpty())
    }

    @Test
    fun withoutTheApiOrSignInFollowIsUnavailable() = runTest(UnconfinedTestDispatcher()) {
        val mock = rt(services = null)
        assertNull(mock.followedAuthors())
        assertFalse(mock.showsFollowButton(follow, mock.post("other").id))
        mock.run(follow, mock.ctx("other"))
        assertEquals(FollowCopy.UNAVAILABLE, mock.toast)

        val signedOut = rt(uid = { null })
        signedOut.run(follow, ActionContext(route, recordId = signedOut.records("posts").first().id))
        assertEquals(FollowCopy.UNAVAILABLE, signedOut.toast)
    }

    @Test
    fun accountChangeForgetsTheList() = runTest(UnconfinedTestDispatcher()) {
        var me: String? = "me"
        follows.uids = listOf("other")
        val rt = rt(uid = { me })
        rt.followedAuthors()
        assertEquals(setOf("other"), rt.followedAuthors())
        rt.resetForAccountChange()
        me = "someone"
        follows.uids = emptyList()
        rt.followedAuthors()
        assertEquals("次の人には前の人の一覧を見せない", emptySet<String>(), rt.followedAuthors())
    }

    @Test
    fun errorTextFollowsIos() {
        fun reason(r: String) = ApiError.Server(403, "forbidden", "", JsonObject(mapOf("reason" to JsonPrimitive(r))))
        assertEquals(FollowCopy.FULL, FollowRuntime.errorText(reason("follows_full")))
        assertEquals(FollowCopy.NOT_IN_APP, FollowRuntime.errorText(reason("not_in_app")))
        assertEquals(FollowCopy.OFFLINE, FollowRuntime.errorText(IOException("x")))
        assertEquals(FollowCopy.FAILED, FollowRuntime.errorText(ApiError.Server(500, "internal", "")))
    }
}
