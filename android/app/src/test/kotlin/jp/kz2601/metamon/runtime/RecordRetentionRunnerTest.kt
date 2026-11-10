package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.LocalRecord
import jp.kz2601.metamon.services.local.PendingOp
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.time.Duration
import java.time.Instant
import kotlin.coroutines.EmptyCoroutineContext

/** #482 記録の保存期間: 期限の判定・消す対象・いつ消すか(起動・前面・読んだ後・保存の後)・二重に消さない。 */
class RecordRetentionRunnerTest {
    private val now0: Instant = Instant.parse("2026-11-11T00:00:00Z")
    private val appId = "retention-app"

    private class Fixture(
        val store: RecordStore,
        val queue: SyncQueue,
        val engine: RecordSyncEngine,
        val consent: RetentionConsentStore,
        val runner: RecordRetentionRunner,
        var clock: Instant,
    )

    /** training の workout に保存期間(と scope)を付けた DSL。 */
    private fun dsl(days: Int?, scope: String? = null): AppDSL {
        val root = Json.parseToJsonElement(File(examplesDir, "training.json").readText()).jsonObject
        val entities = root["entities"]!!.jsonArray.map { e ->
            val o = e.jsonObject.toMutableMap()
            if (days != null) o["retentionDays"] = JsonPrimitive(days)
            if (scope != null) o["scope"] = JsonPrimitive(scope)
            JsonObject(o)
        }
        return AppDSL.decode(JsonObject(root + ("entities" to JsonArray(entities))).toString())
    }

    private fun TestScope.fixture(uid: String? = "me"): Fixture = fixture { uid }

    private fun TestScope.fixture(uid: () -> String?): Fixture {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        val consent = KeyValueRetentionConsent(InMemoryKeyValueStore())
        lateinit var f: Fixture
        val runner = RecordRetentionRunner(engine, consent, currentUserId = uid, now = { f.clock })
        f = Fixture(store, queue, engine, consent, runner, now0)
        return f
    }

    private suspend fun Fixture.seed(id: String, ageDays: Long, author: String? = null) = store.upsert(
        LocalRecord(
            appId = appId, entity = "workout", recordId = id,
            createdAt = now0.minus(Duration.ofDays(ageDays)).toString(),
            pendingOp = PendingOp.NONE, author = author,
        ),
    )

    private suspend fun Fixture.deletes(): List<String> =
        queue.operations().filter { it.kind == PendingOp.DELETE }.map { it.recordId }.sorted()

    private fun TestScope.runtime(f: Fixture, dsl: AppDSL): AppRuntime =
        AppRuntime(AppDocument(appId, 1, dsl), f.engine, backgroundScope, retention = f.runner, currentUserId = { "me" })

    private fun workout(dsl: AppDSL) = dsl.entityModels().single { it.id == "workout" }

    @Test
    fun deletesOnlyExpiredAfterConsentAndSendsToServer() = runTest {
        val f = fixture()
        f.seed("old", ageDays = 8)
        f.seed("new", ageDays = 6)
        val entity = workout(dsl(7))

        assertEquals("確認するまで消さない", 0, f.runner.purge(appId, entity))
        f.runner.note(appId, 1, listOf(entity))
        assertNotNull("確認を出す", f.runner.prompt.value)

        f.runner.confirm()
        assertNull(f.runner.prompt.value)
        assertEquals(7, f.consent.days(appId, "workout"))
        assertEquals(1, f.runner.purge(appId, entity))
        assertEquals("期限を過ぎた行だけ", listOf("new"), f.engine.cached(appId, "workout").map { it.id })
        assertEquals("サーバーにも削除を送る(通常の削除と同じ列)", listOf("old"), f.deletes())
    }

    @Test
    fun shorterPeriodThanConsentedWaitsForReconfirm() = runTest {
        val f = fixture()
        f.seed("old", ageDays = 5)
        f.consent.set(appId, "workout", 7)
        val shorter = workout(dsl(3))
        assertEquals("確認した期間より短い設定は確認し直すまで消さない", 0, f.runner.purge(appId, shorter))
        f.runner.note(appId, 2, listOf(shorter))
        assertEquals(listOf(3), f.runner.prompt.value?.items?.map { it.days })
        // 延ばした設定は確認し直さない
        val longer = workout(dsl(14))
        f.runner.note(appId, 3, listOf(longer))
        assertNull(f.runner.prompt.value)
    }

    @Test
    fun sharedDeletesOnlyOwnRecords() = runTest {
        val f = fixture(uid = "me")
        f.seed("mine", ageDays = 10, author = "me")
        f.seed("theirs", ageDays = 10, author = "other")
        f.seed("legacy", ageDays = 10, author = null)
        val entity = workout(dsl(7, scope = "shared"))
        f.consent.set(appId, "workout", 7)
        assertEquals(1, f.runner.purge(appId, entity))
        assertEquals("共有は自分が作った記録だけ", listOf("mine"), f.deletes())

        val anonymous = fixture(uid = null)
        anonymous.seed("mine", ageDays = 10, author = "me")
        anonymous.consent.set(appId, "workout", 7)
        assertEquals("uid が分からない時は共有を消さない", 0, anonymous.runner.purge(appId, entity))
    }

    @Test
    fun deferHidesPromptForThisPeriodOnly() = runTest {
        val f = fixture()
        val entity = workout(dsl(7))
        f.runner.note(appId, 1, listOf(entity))
        f.runner.defer()
        f.runner.note(appId, 2, listOf(entity))
        assertNull("あとでにした期間は聞き直さない", f.runner.prompt.value)
        f.runner.note(appId, 3, listOf(workout(dsl(3))))
        assertNotNull("期間が変われば聞く", f.runner.prompt.value)
    }

    @Test
    fun neverDeletesTwice() = runTest {
        val f = fixture()
        f.seed("old", ageDays = 30)
        f.seed("old2", ageDays = 30)
        val entity = workout(dsl(7))
        f.consent.set(appId, "workout", 7)
        // 同時に走っても・続けて走っても 1 行 1 回
        listOf(async { f.runner.purge(appId, entity) }, async { f.runner.purge(appId, entity) }).awaitAll()
        f.runner.purge(appId, entity)
        assertEquals(listOf("old", "old2"), f.deletes())
    }

    @Test
    fun pendingUndoDeletionIsNotDeletedAgain() = runTest {
        val f = fixture()
        f.seed("old", ageDays = 30)
        f.consent.set(appId, "workout", 7)
        f.engine.deleteUndoably(appId, "workout", "old", graceMillis = 60_000)
        assertEquals("取り消し待ちの行は通常の削除に任せる", 0, f.runner.purge(appId, workout(dsl(7))))
        assertTrue(f.deletes().isEmpty())
    }

    @Test
    fun bootstrapPurgesAndUpdatesShownList() = runTest {
        val f = fixture()
        f.seed("old", ageDays = 8)
        f.seed("new", ageDays = 1)
        f.consent.set(appId, "workout", 7)
        val rt = runtime(f, dsl(7))
        rt.bootstrap()
        runCurrent()
        assertEquals("起動時に消す", listOf("old"), f.deletes())
        assertEquals("表示中の一覧も差し替える", listOf("new"), rt.records("workout").map { it.id })
    }

    @Test
    fun bootstrapAsksBeforeFirstPurge() = runTest {
        val f = fixture()
        f.seed("old", ageDays = 8)
        val rt = runtime(f, dsl(7))
        rt.bootstrap()
        runCurrent()
        assertEquals(listOf("workout"), f.runner.prompt.value?.items?.map { it.entity })
        assertTrue("確認前は消さない", f.deletes().isEmpty())
    }

    @Test
    fun returningToForegroundPurgesRecordsThatExpiredInBackground() = runTest {
        val f = fixture()
        f.seed("aging", ageDays = 6)
        f.consent.set(appId, "workout", 7)
        val rt = runtime(f, dsl(7))
        rt.bootstrap()
        runCurrent()
        assertTrue(f.deletes().isEmpty())

        rt.setForeground(false)
        f.clock = now0.plus(Duration.ofDays(2))
        rt.setForeground(true)
        runCurrent()
        assertEquals("前面に戻った時に消す", listOf("aging"), f.deletes())
        assertTrue(rt.records("workout").isEmpty())
    }

    @Test
    fun readingRecordsPurgesNewlyArrivedExpired() = runTest {
        val f = fixture()
        f.consent.set(appId, "workout", 7)
        val rt = runtime(f, dsl(7))
        rt.bootstrap()
        runCurrent()
        f.seed("synced", ageDays = 9) // 別の端末で作った自分の記録が届いた
        rt.refresh("workout")
        runCurrent()
        assertEquals("記録を読んだ後に消す", listOf("synced"), f.deletes())
    }

    @Test
    fun noRetentionEntityDoesNothing() = runTest {
        val f = fixture()
        f.seed("old", ageDays = 400)
        val rt = runtime(f, dsl(null))
        rt.bootstrap()
        runCurrent()
        assertNull(f.runner.prompt.value)
        assertTrue(f.deletes().isEmpty())
    }

    @Test
    fun signedOutDeletesNothingEvenPrivate() = runTest {
        val f = fixture(uid = null)
        f.seed("old", ageDays = 30)
        f.consent.set(appId, "workout", 7)
        assertEquals("サインインしていない(後片付けの途中)時は消さない", 0, f.runner.purge(appId, workout(dsl(7))))
        assertTrue(f.deletes().isEmpty())
    }

    @Test
    fun stopsWhenAccountChangesMidPurge() = runTest {
        var calls = 0
        // 1 回目(始め)・2 回目(1 行目の前)は me、その後はサインアウト
        val f = fixture { if (++calls <= 2) "me" else null }
        f.seed("a", ageDays = 30)
        f.seed("b", ageDays = 30)
        f.consent.set(appId, "workout", 7)
        assertEquals("人が変わったら残りは消さない", 1, f.runner.purge(appId, workout(dsl(7))))
        assertEquals(1, f.deletes().size)
    }

    @Test
    fun forgetClearsConsentDeferAndPrompt() = runTest {
        val store = InMemoryKeyValueStore()
        val consent = KeyValueRetentionConsent(store)
        consent.set(appId, "workout", 7)
        consent.set("other-app", "diary", 30)
        consent.clear()
        assertNull("前の人の確認を次の人に使わない", consent.days(appId, "workout"))
        assertNull(consent.days("other-app", "diary"))

        val f = fixture()
        val entity = workout(dsl(7))
        f.runner.note(appId, 1, listOf(entity))
        f.runner.defer()
        f.consent.set(appId, "workout", 7)
        f.runner.forget()
        assertNull(f.consent.days(appId, "workout"))
        f.runner.note(appId, 1, listOf(entity))
        assertNotNull("あとでにした分も聞き直す", f.runner.prompt.value)
        f.runner.forget()
        assertNull("出していた確認も閉じる", f.runner.prompt.value)
    }

    @Test
    fun promptMessageMatchesIos() {
        val p = RetentionPrompt(appId, 1, listOf(RetentionPrompt.Item("e", "日記", 7, isShared = true)))
        assertEquals("「日記」は1週間たった記録を自動で消します。消えた記録は元に戻せません。共有の記録は自分が追加したものだけが消えます。", p.message(english = false))
        assertTrue(p.message(english = true).startsWith("Entries older than the period will be deleted automatically (日記: 1 week)."))
    }

    private companion object {
        val examplesDir = File(System.getProperty("metamon.examplesDir") ?: "../../docs/dsl/examples")
    }
}
