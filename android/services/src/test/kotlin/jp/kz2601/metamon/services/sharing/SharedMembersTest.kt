package jp.kz2601.metamon.services.sharing

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.concurrent.TimeUnit

/** 参加している人(#903): members API の口・役割ごとに押せる操作・楽観的 UI・抜けた後の後片付け。 */
class SharedMembersTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpMembersClient
    private lateinit var store: AppMembersStore

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpMembersClient(server.metamonHttp())
        store = AppMembersStore(client, CoroutineScope(Dispatchers.Unconfined))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun member(uid: String, role: String = "member", admin: Boolean = false, you: Boolean = false, name: String = "") =
        """{"uid":"$uid","displayName":"$name","role":"$role","admin":$admin,"you":$you}"""

    /** 作成者 o・管理者 a・参加者 m(呼び出した人は [me]) */
    private fun listJson(me: String, role: String, admin: Boolean) =
        """{"role":"$role","admin":$admin,"items":[${member("o", "owner", true, me == "o")},${member("a", admin = true, you = me == "a", name = "母")},${member("m", you = me == "m", name = "たろう")}]}"""

    private fun model(shared: Boolean = true) = MembersModel("app1", client, isShared = { shared }, store = store)

    private suspend fun loaded(me: String, role: String, admin: Boolean): MembersModel {
        server.enqueue(json(200, listJson(me, role, admin)))
        return model().also {
            it.load()
            server.take()
        }
    }

    // MARK: API の口

    @Test
    fun listDecodesMembersAndCallerRole() = runBlocking {
        server.enqueue(json(200, listJson("a", "member", admin = true)))
        val list = client.members("app 1")
        val req = server.take()
        assertEquals("GET", req.method)
        assertEquals("/v1/apps/app%201/members", req.path)
        assertEquals(listOf("o", "a", "m"), list.items.map { it.uid })
        assertTrue(list.items[0].isOwner && list.items[0].isAdmin)
        assertEquals("母", list.items[1].displayName)
        assertFalse(list.isOwner)
        assertTrue(list.isAdmin)
    }

    @Test
    fun ownerIsAdminEvenWithoutAdminKey() = runBlocking {
        server.enqueue(json(200, """{"role":"owner","items":[{"uid":"o","displayName":"","role":"owner","you":true}]}"""))
        val list = client.members("app1")
        assertTrue(list.isAdmin)
        assertTrue(list.items.single().isAdmin)
    }

    @Test
    fun setAdminRemoveAndLeaveSendTheRightRequests() = runBlocking {
        server.enqueue(json(200, """{"uid":"u/1","admin":true}"""))
        server.enqueue(MockResponse().setResponseCode(204))
        server.enqueue(MockResponse().setResponseCode(204))
        client.setAdmin("app1", "u/1", admin = true)
        client.removeMember("app1", "u2")
        client.leave("app1")
        server.take().let {
            assertEquals("PATCH", it.method)
            assertEquals("/v1/apps/app1/members/u%2F1", it.path)
            assertEquals("""{"admin":true}""", it.body.readUtf8())
        }
        server.take().let {
            assertEquals("DELETE", it.method)
            assertEquals("/v1/apps/app1/members/u2", it.path)
        }
        server.take().let {
            assertEquals("DELETE", it.method)
            assertEquals("/v1/apps/app1/members/me", it.path)
        }
    }

    // MARK: 役割ごとに押せる操作

    @Test
    fun ownerCanRemoveAndSetAdminOnOthersButCannotLeave() {
        val list = SharedMemberList(
            items = listOf(SharedMember("o", role = ROLE_OWNER, you = true), SharedMember("a", admin = true), SharedMember("m")),
            role = ROLE_OWNER,
            admin = true,
        )
        assertFalse(MemberActions.canRemove(list, list.items[0]))
        assertFalse(MemberActions.canSetAdmin(list, list.items[0]))
        assertTrue(MemberActions.canRemove(list, list.items[1]))
        assertTrue(MemberActions.canSetAdmin(list, list.items[2]))
        assertFalse(MemberActions.canLeave(list))
    }

    @Test
    fun adminCanSetAdminButNotRemoveAndCanLeave() {
        val list = SharedMemberList(
            items = listOf(SharedMember("o", role = ROLE_OWNER), SharedMember("a", admin = true, you = true), SharedMember("m")),
            role = ROLE_MEMBER,
            admin = true,
        )
        assertFalse(MemberActions.canSetAdmin(list, list.items[0])) // 作成者は外せない
        assertFalse(MemberActions.canSetAdmin(list, list.items[1])) // 自分
        assertTrue(MemberActions.canSetAdmin(list, list.items[2]))
        assertTrue(list.items.none { MemberActions.canRemove(list, it) })
        assertTrue(MemberActions.canLeave(list))
    }

    @Test
    fun plainMemberCanOnlyLeave() {
        val list = SharedMemberList(
            items = listOf(SharedMember("o", role = ROLE_OWNER), SharedMember("m", you = true), SharedMember("x")),
            role = ROLE_MEMBER,
            admin = false,
        )
        assertTrue(list.items.none { MemberActions.canRemove(list, it) || MemberActions.canSetAdmin(list, it) })
        assertTrue(MemberActions.canLeave(list))
    }

    @Test
    fun fullAtHundredPeople() {
        assertFalse(SharedMemberList(items = List(99) { SharedMember("u$it") }).isFull)
        assertTrue(SharedMemberList(items = List(MEMBER_LIMIT) { SharedMember("u$it") }).isFull)
        // 一覧には作成者も 1 行あるが、backend の上限は作成者を除いた参加者の数
        val owner = SharedMember("o", role = ROLE_OWNER)
        assertFalse("作成者 + 99 人なら、まだ 1 人参加できる", SharedMemberList(items = listOf(owner) + List(99) { SharedMember("u$it") }, role = ROLE_OWNER).isFull)
        assertTrue(SharedMemberList(items = listOf(owner) + List(MEMBER_LIMIT) { SharedMember("u$it") }).isFull)
    }

    // MARK: 読み込み

    @Test
    fun appWithoutSharedDataHidesSectionWithoutCallingApi() = runBlocking {
        val m = model(shared = false)
        m.load()
        assertEquals(MembersPhase.HIDDEN, m.state.value.phase)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun noClientHidesSection() = runBlocking {
        val m = MembersModel("app1", null, isShared = { true })
        m.load()
        assertEquals(MembersPhase.HIDDEN, m.state.value.phase)
    }

    @Test
    fun notMemberShowsJoinHintAndStoreKnowsNotAdmin() = runBlocking {
        server.enqueue(json(403, errorBody("forbidden", details = """{"reason":"not_member"}""")))
        val m = model()
        m.load()
        assertEquals(MembersPhase.NOT_MEMBER, m.state.value.phase)
        server.enqueue(json(403, errorBody("forbidden", details = """{"reason":"not_member"}""")))
        store.reload("app1")
        assertEquals(false, store["app1"]?.isAdmin)
    }

    @Test
    fun loadedListGoesToStoreForComponents() = runBlocking {
        val m = loaded("a", ROLE_MEMBER, admin = true)
        assertEquals(MembersPhase.LOADED, m.state.value.phase)
        assertEquals(true, store["app1"]?.isAdmin)
        assertEquals(listOf("o", "a", "m"), store["app1"]?.members?.map { it.uid })
    }

    // MARK: 外す・管理者・抜ける(楽観的 UI)

    @Test
    fun ownerRemoveIsOptimisticAndSuggestsRevokingLink() = runBlocking {
        val m = loaded("o", ROLE_OWNER, admin = true)
        server.enqueue(MockResponse().setResponseCode(204))
        val target = m.state.value.list!!.items.first { it.uid == "m" }
        assertNull(m.remove(target))
        assertEquals(listOf("o", "a"), m.state.value.list?.items?.map { it.uid })
        assertEquals("m", m.state.value.justRemoved?.uid)
        assertEquals("/v1/apps/app1/members/m", server.take().path)
        assertEquals(listOf("o", "a"), store["app1"]?.members?.map { it.uid })
        m.dismissRemoved()
        assertNull(m.state.value.justRemoved)
    }

    @Test
    fun removeFailureRestoresRowAtSamePlace() = runBlocking {
        val m = loaded("o", ROLE_OWNER, admin = true)
        server.enqueue(json(403, errorBody("forbidden")))
        val target = m.state.value.list!!.items.first { it.uid == "a" }
        assertEquals(MembersFailure.REMOVE, m.remove(target))
        assertEquals(listOf("o", "a", "m"), m.state.value.list?.items?.map { it.uid })
        assertNull(m.state.value.justRemoved)
    }

    @Test
    fun removeOfSomeoneAlreadyGoneCountsAsDone() = runBlocking {
        val m = loaded("o", ROLE_OWNER, admin = true)
        server.enqueue(json(404, errorBody("not_found")))
        assertNull(m.remove(m.state.value.list!!.items.first { it.uid == "m" }))
        assertEquals(listOf("o", "a"), m.state.value.list?.items?.map { it.uid })
    }

    @Test
    fun memberCannotRemoveEvenIfCalled() = runBlocking {
        val m = loaded("m", ROLE_MEMBER, admin = false)
        assertNull(m.remove(m.state.value.list!!.items.first { it.uid == "a" }))
        assertEquals(3, m.state.value.list?.items?.size)
        assertEquals(0, server.requestCount - 1)
    }

    @Test
    fun setAdminIsOptimisticAndRevertsOnFailure() = runBlocking {
        val m = loaded("a", ROLE_MEMBER, admin = true)
        val target = m.state.value.list!!.items.first { it.uid == "m" }
        server.enqueue(json(200, """{"uid":"m","admin":true}"""))
        assertNull(m.setAdmin(target, true))
        assertTrue(m.state.value.list!!.items.first { it.uid == "m" }.admin)
        assertEquals("""{"admin":true}""", server.take().body.readUtf8())
        assertEquals(true, store["app1"]?.members?.first { it.uid == "m" }?.admin)

        val now = m.state.value.list!!.items.first { it.uid == "m" }
        server.enqueue(json(403, errorBody("forbidden", details = """{"reason":"admins_only"}""")))
        assertEquals(MembersFailure.SET_ADMIN, m.setAdmin(now, false))
        assertTrue(m.state.value.list!!.items.first { it.uid == "m" }.admin)
    }

    @Test
    fun leaveMarksLeftAndForgetsStore() = runBlocking {
        val m = loaded("m", ROLE_MEMBER, admin = false)
        server.enqueue(MockResponse().setResponseCode(204))
        assertNull(m.leave())
        assertEquals(MembersPhase.LEFT, m.state.value.phase)
        assertEquals("/v1/apps/app1/members/me", server.take().path)
        assertNull(store["app1"])
    }

    @Test
    fun leaveFailureRestoresList() = runBlocking {
        val m = loaded("m", ROLE_MEMBER, admin = false)
        server.enqueue(json(400, errorBody("invalid_argument")))
        assertEquals(MembersFailure.LEAVE, m.leave())
        assertEquals(MembersPhase.LOADED, m.state.value.phase)
        assertFalse(m.state.value.leaving)
    }

    @Test
    fun leaveOfflineSaysOffline() = runBlocking {
        val m = loaded("m", ROLE_MEMBER, admin = false)
        server.shutdown() // 圏外(つながらない)
        assertEquals(MembersFailure.OFFLINE, m.leave())
        assertEquals(MembersPhase.LOADED, m.state.value.phase)
    }

    @Test
    fun ownerCannotLeave() = runBlocking {
        val m = loaded("o", ROLE_OWNER, admin = true)
        assertNull(m.leave())
        assertEquals(MembersPhase.LOADED, m.state.value.phase)
        assertNull(server.takeRequest(100, TimeUnit.MILLISECONDS))
    }

    // MARK: 部品の受け口

    @Test
    fun storeReadsOnlyForAppsThatUseMembers() = runBlocking {
        store.note("app1", dsl("""{"id":"e","name":"E","scope":"shared","fields":[{"id":"f","name":"F","type":"text"}]}"""))
        assertEquals(0, server.requestCount)
        server.enqueue(json(200, listJson("m", ROLE_MEMBER, admin = false)))
        store.note("app1", dsl("""{"id":"e","name":"E","scope":"shared","deleteBy":"admins","fields":[]}"""))
        server.take()
        withTimeout(5_000) { store.entries.first { "app1" in it } }
        assertEquals(false, store["app1"]?.isAdmin)
    }

    @Test
    fun usesMembersSeesMemberFieldsAdminOnlyFieldsAndVisibleTo() {
        assertFalse(dsl("""{"id":"e","name":"E","fields":[{"id":"f","name":"F","type":"text"}]}""").usesMembers())
        assertTrue(dsl("""{"id":"e","name":"E","fields":[{"id":"f","name":"F","type":"member"}]}""").usesMembers())
        assertTrue(dsl("""{"id":"e","name":"E","fields":[{"id":"f","name":"F","type":"text","editableBy":"admins"}]}""").usesMembers())
        val button = """{"type":"button","id":"b","label":"L","visibleTo":"admins"}"""
        assertTrue(dsl("""{"id":"e","name":"E","fields":[]}""", components = """[{"type":"vstack","id":"s","children":[$button]}]""").usesMembers())
    }

    private fun dsl(entity: String, components: String = "[]"): AppDSL = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"A"},"initialScreen":"home","screens":[{"id":"home","title":"H","components":$components}],"entities":[$entity]}""",
    )
}
