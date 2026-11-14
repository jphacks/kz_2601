package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.records.InterestState
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.social.ReportReasonOption
import jp.kz2601.metamon.renderer.components.social.SocialNotice
import jp.kz2601.metamon.renderer.components.social.SocialPrompt
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import jp.kz2601.metamon.services.social.BlockedUser
import jp.kz2601.metamon.services.social.MatchingAgeGate
import jp.kz2601.metamon.services.social.MatchingClient
import jp.kz2601.metamon.services.social.ModerationClient
import jp.kz2601.metamon.services.social.ReportReason
import jp.kz2601.metamon.services.social.ReportTarget
import kotlinx.coroutines.ExperimentalCoroutinesApi
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

/** 通報・ブロック・仲間探し(#877・iOS の AppRuntime+Moderation / AppRuntime+Matching)を AppRuntime 越しに試す。 */
@OptIn(ExperimentalCoroutinesApi::class)
class SocialRuntimeTest {
    private val appId = "test-match"

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"仲間探し"},"initialScreen":"home",
         "entities":[
           {"id":"profiles","name":"プロフィール","scope":"public","match":"profile","fields":[{"id":"name","name":"名前","type":"string"}]},
           {"id":"contacts","name":"連絡先","scope":"public","match":"matched","fields":[{"id":"line","name":"LINE","type":"string"}]},
           {"id":"memo","name":"メモ","fields":[{"id":"text","name":"本文","type":"string"}]}
         ],
         "screens":[{"id":"home","title":"仲間","components":[
           {"type":"matchList","id":"in","area":"A1:F6","entity":"profiles","show":"incoming"}
         ]}]}
        """.trimIndent(),
    )

    private class FakeModeration : ModerationClient {
        val reports = mutableListOf<Pair<ReportTarget, ReportReason>>()
        val blocks = mutableListOf<Pair<String, String?>>()
        var fail: Exception? = null
        var blocked = listOf<BlockedUser>()

        override suspend fun report(target: ReportTarget, reason: ReportReason) {
            fail?.let { throw it }
            reports += target to reason
        }

        override suspend fun blockedUsers(): List<BlockedUser> = blocked

        override suspend fun block(uid: String, name: String?): BlockedUser {
            fail?.let { throw it }
            blocks += uid to name
            return BlockedUser(uid, name ?: "匿名")
        }

        override suspend fun blockListing(listingId: String): BlockedUser = error("unused")

        override suspend fun unblock(uid: String) {
            fail?.let { throw it }
        }
    }

    private class FakeMatching : MatchingClient {
        val sent = mutableListOf<Triple<String, String, Boolean>>()
        var fail: Exception? = null
        var incoming = listOf<MatchInterest>()

        override suspend fun sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Boolean): MatchInterest {
            fail?.let { throw it }
            sent += Triple(recordId, fromRecordId, ageConfirmed)
            return MatchInterest("other", recordId)
        }

        override suspend fun list(appId: String, box: MatchBox): List<MatchInterest> = if (box == MatchBox.INCOMING) incoming else emptyList()

        override suspend fun accept(appId: String, uid: String, ageConfirmed: Boolean): MatchInterest {
            fail?.let { throw it }
            return MatchInterest(uid, status = MatchInterest.MATCHED)
        }

        override suspend fun decline(appId: String, uid: String) = Unit

        override suspend fun unmatch(appId: String, uid: String) = Unit
    }

    private val moderation = FakeModeration()
    private val matching = FakeMatching()
    private val ageGate = MatchingAgeGate(InMemoryKeyValueStore())

    private suspend fun TestScope.rt(
        services: SocialServices? = SocialServices(matching, moderation, ageGate),
        uid: () -> String? = { "me" },
    ): AppRuntime {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        // 他の人(other)の投稿と自分(me)のプロフィール
        engine.write(appId, "profiles", null, JsonObject(mapOf("name" to JsonPrimitive("はなこ"))), "other", "はなこ", shared = true)
        engine.write(appId, "profiles", null, JsonObject(mapOf("name" to JsonPrimitive("自分"))), "me", "自分", shared = true)
        val rt = AppRuntime(AppDocument(appId, 1, dsl), engine, backgroundScope, currentUserId = uid, socialServices = services)
        rt.load(setOf("profiles"))
        return rt
    }

    private fun AppRuntime.other(): Record = records("profiles").first { it.author == "other" }

    private fun AppRuntime.shown(): List<String?> = recordsFlow.value["profiles"].orEmpty().map { it.author }

    @Test
    fun reportOnlyOthersSharedPostsAndSendTheChosenReason() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt()
        val mine = rt.records("profiles").first { it.author == "me" }
        assertFalse(rt.canReport("profiles", mine))
        assertTrue(rt.canReport("profiles", rt.other()))

        rt.reportRecord("profiles", rt.other())
        assertEquals(SocialPrompt.Report("はなこ"), rt.socialPrompt.value)
        rt.submitReport(ReportReasonOption.HARASSMENT)
        assertEquals(SocialPrompt.Notice(SocialNotice.REPORTED), rt.socialPrompt.value)
        assertEquals(listOf(ReportTarget.RecordTarget(appId, "profiles", rt.other().id) to ReportReason.HARASSMENT), moderation.reports)
    }

    @Test
    fun reportLimitIsShownApart() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt()
        moderation.fail = ApiError.Server(429, "rate_limited", "")
        rt.reportRecord("profiles", rt.other())
        rt.submitReport(ReportReasonOption.SPAM)
        assertEquals(SocialPrompt.Notice(SocialNotice.REPORT_LIMITED), rt.socialPrompt.value)
    }

    @Test
    fun withoutModerationThereIsNoMenu() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt(services = null)
        assertFalse(rt.canReport("profiles", rt.other()))
        assertNull(rt.blockedUsers)
        assertNull(rt.interestState("profiles", rt.other()))
    }

    @Test
    fun blockHidesThePostsAndTheCandidateThenUnblockShowsThem() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt()
        assertEquals(setOf("me", "other"), rt.shown().toSet())
        rt.blockAuthor("profiles", rt.other())
        assertEquals(SocialPrompt.Block("はなこ"), rt.socialPrompt.value)
        rt.confirmBlock()
        assertEquals(listOf("other" to "はなこ"), moderation.blocks)
        assertEquals(SocialPrompt.Notice(SocialNotice.BLOCKED, "はなこ"), rt.socialPrompt.value)
        assertEquals(listOf("me"), rt.shown())
        assertNull(rt.interestState("profiles", rt.other())) // 候補にも出さない
        assertEquals(mapOf("other" to "はなこ"), rt.blockedUsers!!.value)

        rt.unblock("other")
        assertEquals(setOf("me", "other"), rt.shown().toSet())
    }

    @Test
    fun failedBlockShowsThePostsAgain() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt()
        moderation.fail = IOException("offline")
        rt.blockAuthor("profiles", rt.other())
        rt.confirmBlock()
        assertEquals(SocialPrompt.Notice(SocialNotice.BLOCK_FAILED), rt.socialPrompt.value)
        assertEquals(setOf("me", "other"), rt.shown().toSet())
    }

    @Test
    fun blockedListFromTheServerHidesOnStart() = runTest(UnconfinedTestDispatcher()) {
        moderation.blocked = listOf(BlockedUser("other", "はなこ"))
        val rt = rt()
        rt.refreshMatches(setOf(MatchBox.INCOMING)) // ブロック一覧も読む
        assertEquals(listOf("me"), rt.shown())
    }

    @Test
    fun anotherPersonIsNotHiddenByThePreviousBlocksAndSignOutForgetsThem() = runTest(UnconfinedTestDispatcher()) {
        moderation.blocked = listOf(BlockedUser("other", "はなこ"))
        var uid: String? = "me"
        val rt = rt(uid = { uid })
        rt.refreshMatches(setOf(MatchBox.INCOMING))
        assertEquals(listOf("me"), rt.shown())
        assertNull(rt.interestState("profiles", rt.other()))

        // 別の人になった直後(一覧を読み直す前)も、前の人のブロックで候補を隠さない
        uid = "someone"
        assertEquals(InterestState.NONE, rt.interestState("profiles", rt.other()))

        // 後片付けでブロック一覧を捨て、次の人の分は読み直す
        rt.resetForAccountChange()
        assertTrue(rt.blockedUsers!!.value.isEmpty())
        moderation.blocked = listOf(BlockedUser("x", "たろう"))
        uid = "me"
        rt.refreshMatches(setOf(MatchBox.INCOMING))
        assertEquals(mapOf("x" to "たろう"), rt.blockedUsers!!.value)
    }

    @Test
    fun firstInterestAsksAgeThenSends() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt()
        assertEquals(InterestState.NONE, rt.interestState("profiles", rt.other()))
        rt.sendInterest("profiles", rt.other())
        assertEquals(SocialPrompt.AdultConfirm, rt.socialPrompt.value)
        assertTrue(matching.sent.isEmpty())

        rt.confirmAdult()
        assertNull(rt.socialPrompt.value)
        assertTrue(ageGate.isConfirmed)
        val mine = rt.records("profiles").first { it.author == "me" }
        assertEquals(listOf(Triple(rt.other().id, mine.id, true)), matching.sent)
        assertEquals(InterestState.SENT, rt.interestState("profiles", rt.other()))

        // 2 回目からは聞かない
        rt.sendInterest("profiles", rt.other())
        assertNull(rt.socialPrompt.value)
    }

    @Test
    fun under18SendsNothingAndRemembersNothing() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt()
        rt.sendInterest("profiles", rt.other())
        rt.declineAdult()
        assertFalse(ageGate.isConfirmed)
        assertTrue(matching.sent.isEmpty())
        assertEquals(SocialCopy.UNDER_18, rt.toast)
    }

    @Test
    fun serverAskingForAgeAgainResetsTheGate() = runTest(UnconfinedTestDispatcher()) {
        ageGate.confirm()
        val rt = rt()
        matching.fail = ApiError.Server(400, "bad_request", "", JsonObject(mapOf("reason" to JsonPrimitive("age_unconfirmed"))))
        rt.sendInterest("profiles", rt.other())
        assertFalse(ageGate.isConfirmed)
        assertEquals(SocialPrompt.AdultConfirm, rt.socialPrompt.value)
        assertEquals(InterestState.NONE, rt.interestState("profiles", rt.other())) // 戻っている
    }

    @Test
    fun acceptConnectsAndToasts() = runTest(UnconfinedTestDispatcher()) {
        ageGate.confirm()
        matching.incoming = listOf(MatchInterest("other", "p"))
        val rt = rt()
        rt.refreshMatches(setOf(MatchBox.INCOMING))
        assertEquals(InterestState.INCOMING, rt.interestState("profiles", rt.other()))
        rt.acceptInterest(rt.matchItems(MatchBox.INCOMING)!!.single())
        assertEquals(InterestState.MATCHED, rt.interestState("profiles", rt.other()))
        assertEquals(SocialCopy.CONNECTED, rt.toast)
    }

    @Test
    fun hidingBlockedKeepsPrivateEntitiesAndUnsentRows() {
        val rows = mapOf(
            "profiles" to listOf(Record("1", author = "x"), Record("2", author = "y"), Record("3")),
            "memo" to listOf(Record("4", author = "x")),
        )
        val out = SocialRuntime.hidingBlocked(rows, mapOf("x" to "")) { it == "profiles" }
        assertEquals(listOf("2", "3"), out["profiles"]!!.map { it.id })
        assertEquals(listOf("4"), out["memo"]!!.map { it.id })
    }

    @Test
    fun errorTextFollowsIos() {
        assertEquals(SocialCopy.OFFLINE, SocialRuntime.errorText(IOException("x")))
        assertEquals(SocialCopy.DAILY_LIMIT, SocialRuntime.errorText(ApiError.Server(429, "rate_limited", "")))
        assertEquals(SocialCopy.NOT_FOUND, SocialRuntime.errorText(ApiError.Server(404, "not_found", "")))
        assertEquals(
            SocialCopy.UNBLOCK_TO_SEND,
            SocialRuntime.errorText(ApiError.Server(400, "bad_request", "", JsonObject(mapOf("reason" to JsonPrimitive("blocked"))))),
        )
        assertEquals(
            SocialCopy.PROFILE_NOT_SENT,
            SocialRuntime.errorText(ApiError.Server(400, "bad_request", "", JsonObject(mapOf("field" to JsonPrimitive("fromRecordId"))))),
        )
    }

    @Test
    fun reasonOptionsMatchTheApiReasons() {
        assertEquals(ReportReason.entries.map { it.wire }, ReportReasonOption.entries.map { it.wire })
        assertEquals(ReportReason.entries.map { it.name }, ReportReasonOption.entries.map { it.name })
    }
}
