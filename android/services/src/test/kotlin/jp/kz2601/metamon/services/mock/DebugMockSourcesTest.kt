package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.UsageMeter
import jp.kz2601.metamon.services.http.ApiError
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.time.Instant
import java.time.ZoneOffset

/** #1023 / #1024 のつまみが使うモック(iOS の `DebugMockUsage`・`DebugMockMembers`・`DummyRecords`)。 */
class DebugMockSourcesTest {
    @Test
    fun usageSpecLikeIos() = runTest {
        val src = MockPlanUsageSource("apps:9/10, records:4200/5000,photos:10/500,pro,participant,broken:x/1,bad")
        assertEquals(UsageMeter(9, 10), src.myUsage().apps)
        assertEquals("pro", src.myUsage().plan)
        val app = src.appUsage("a1")
        assertEquals(UsageMeter(4200, 5000), app.records)
        assertEquals(UsageMeter(10, 500), app.photos)
        assertEquals("creator", app.payer)
        assertFalse(app.upgrade)
        assertFalse("broken" in src.meters)

        val free = MockPlanUsageSource("records:1/2")
        assertEquals("free", free.myUsage().plan)
        assertNull(free.myUsage().apps)
        assertTrue(free.appUsage("a").isMine)
        assertTrue(free.appUsage("a").upgrade)
    }

    @Test
    fun membersListAndWrites() = runTest {
        val owner = MockMembersClient("owner", writeDelayMillis = 0)
        val list = owner.members("a")
        assertTrue(list.isOwner)
        assertEquals(listOf("uid-owner", "uid-dad", "uid-kid"), list.items.map { it.uid })
        assertTrue(list.items.first().you)
        owner.setAdmin("a", "uid-kid", true)
        assertTrue(owner.members("a").items.last().admin)
        owner.removeMember("a", "uid-kid")
        assertEquals(2, owner.members("a").items.size)

        val member = MockMembersClient("member", writeDelayMillis = 0)
        assertTrue(member.members("a").items.single { it.you }.uid == "uid-dad")
        member.leave("a")
        assertEquals(listOf("uid-owner", "uid-kid"), member.members("a").items.map { it.uid })
    }

    @Test
    fun membersFailKeepsTheList() = runTest {
        val m = MockMembersClient("owner", failsWrites = true, writeDelayMillis = 0)
        try {
            m.removeMember("a", "uid-kid")
            fail("失敗するはず")
        } catch (e: ApiError.Server) {
            assertEquals(503, e.status)
        }
        assertEquals(3, m.members("a").items.size)
    }

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","screens":[{"id":"home","components":[]}],
         "entities":[
          {"id":"post","name":"投稿","scope":"shared","fields":[
            {"id":"title","name":"題","type":"string"},{"id":"body","name":"本文","type":"text"},
            {"id":"amount","name":"額","type":"currency"},{"id":"count","name":"数","type":"integer"},
            {"id":"ok","name":"済","type":"boolean"},{"id":"day","name":"日","type":"date"},
            {"id":"at","name":"時","type":"datetime"},{"id":"kind","name":"種","type":"enum","options":["a","b"]},
            {"id":"tag","name":"タグ","type":"ref","refEntity":"tag"},{"id":"tags","name":"タグ達","type":"refs","refEntity":"tag"},
            {"id":"pic","name":"写真","type":"photo"},{"id":"ghost","name":"無","type":"ref","refEntity":"missing"}]},
          {"id":"tag","name":"タグ","fields":[{"id":"name","name":"名前","type":"string"}]}]}
        """.trimIndent(),
    )

    @Test
    fun dummyRecordsFollowFieldTypes() {
        val now = Instant.parse("2026-11-11T03:00:00Z")
        val seed = DummyRecords.make(dsl, count = 3, now = now, stepHours = 24.0, zone = ZoneOffset.UTC)
        val posts = seed.getValue("post")
        assertEquals(listOf("post-1", "post-2", "post-3"), posts.map { it.id })
        val first = posts[0].data
        assertEquals(JsonPrimitive("題 1"), first["title"])
        assertEquals(JsonPrimitive(980), first["amount"])
        assertEquals(JsonPrimitive(3), first["count"])
        assertEquals(JsonPrimitive(true), first["ok"])
        assertEquals(JsonPrimitive("2026-11-11"), first["day"])
        assertEquals(JsonPrimitive("2026-11-11T03:00:00Z"), first["at"])
        assertEquals(JsonPrimitive("a"), first["kind"])
        assertEquals(JsonPrimitive("tag-1"), first["tag"])
        assertEquals(JsonArray(emptyList()), first["tags"])
        assertNull(first["pic"]) // 写真・位置は作らない
        assertNull(first["ghost"]) // 無い entity への参照は作らない
        // 3 件目の見出しは長い文字列(崩れを見る)・2 件目は日付が 2 日前
        assertEquals(JsonPrimitive("駅前の新しくできたイタリアンレストランで友達と夕食"), posts[2].data["title"])
        assertEquals(JsonPrimitive("2026-11-09"), posts[1].data["day"])
        // 作成日時は stepHours ずつ前
        assertEquals(listOf("2026-11-11T03:00:00Z", "2026-11-10T03:00:00Z", "2026-11-09T03:00:00Z"), posts.map { it.createdAt })
        // 共有 entity は自分 / ほかの人が交互
        assertEquals(listOf(MockAuthService.FIXED_UID, "dummy-member-1", MockAuthService.FIXED_UID), posts.map { it.author })
        // 自分だけの entity は投稿者なし
        assertNull(seed.getValue("tag").first().author)

        val mixed = DummyRecords.alternatingAuthors(seed).getValue("tag")
        assertEquals(listOf(MockAuthService.FIXED_UID, DummyRecords.OTHER_UID, MockAuthService.FIXED_UID), mixed.map { it.author })
    }
}
