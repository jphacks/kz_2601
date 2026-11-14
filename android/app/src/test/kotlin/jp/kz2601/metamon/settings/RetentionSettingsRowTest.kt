package jp.kz2601.metamon.settings

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.retention.RetentionStrings
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.KeyValueRetentionConsent
import jp.kz2601.metamon.runtime.RecordRetentionRunner
import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.LocalRecord
import jp.kz2601.metamon.services.local.PendingOp
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.time.Duration
import java.time.Instant
import kotlin.coroutines.EmptyCoroutineContext

/** #482 設定の「記録の自動削除」: 行の状態と、「あとで」にした期間を設定から確認し直す流れ。 */
class RetentionSettingsRowTest {
    private val now: Instant = Instant.parse("2026-11-11T00:00:00Z")
    private val appId = "retention-settings"

    /** training の entity に保存期間(と scope)を付けた DSL。[days] に無い entity は期間なし。 */
    private fun dsl(days: Map<String, Int>, scope: String? = null): AppDSL {
        val root = Json.parseToJsonElement(File(examplesDir, "training.json").readText()).jsonObject
        val entities = root["entities"]!!.jsonArray.map { e ->
            val o = e.jsonObject.toMutableMap()
            days[o["id"]!!.jsonPrimitive.content]?.let { o["retentionDays"] = JsonPrimitive(it) }
            if (scope != null) o["scope"] = JsonPrimitive(scope)
            JsonObject(o)
        }
        return AppDSL.decode(JsonObject(root + ("entities" to JsonArray(entities))).toString())
    }

    @Test
    fun rowsOnlyForEntitiesWithPeriodAndShowConsentState() = runTest {
        val consent = KeyValueRetentionConsent(InMemoryKeyValueStore())
        val rt = runtime(dsl(mapOf("workout" to 30)), appId)
        fun rows() = RetentionSettingsRow.of(rt.entityModels) { consent.days(appId, it) }
        assertEquals("期間のある entity だけ", listOf("workout"), rows().map { it.entity })
        assertEquals(30, rows().single().days)
        assertFalse("未確認は確認の入口を出す", rows().single().active)

        consent.set(appId, "workout", 30)
        assertTrue(rows().single().active)
        consent.set(appId, "workout", 60)
        assertFalse("確認した期間より短い設定は確認し直す", rows().single().active)
    }

    @Test
    fun noRowsWithoutPeriod() = runTest {
        val rt = runtime(dsl(emptyMap()), appId)
        assertTrue(RetentionSettingsRow.of(rt.entityModels) { null }.isEmpty())
    }

    @Test
    fun sharedFlagForFooter() = runTest {
        val rt = runtime(dsl(mapOf("workout" to 7), scope = "shared"), appId)
        assertTrue(RetentionSettingsRow.of(rt.entityModels) { null }.single().isShared)
    }

    @Test
    fun deferredPeriodCanBeConfirmedFromSettings() = runTest {
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        val runner = RecordRetentionRunner(engine, KeyValueRetentionConsent(InMemoryKeyValueStore()), currentUserId = { "me" }, now = { now })
        store.upsert(
            LocalRecord(
                appId = appId, entity = "workout", recordId = "old",
                createdAt = now.minus(Duration.ofDays(8)).toString(), pendingOp = PendingOp.NONE,
            ),
        )
        val rt = AppRuntime(AppDocument(appId, 1, dsl(mapOf("workout" to 7))), engine, backgroundScope, retention = runner, currentUserId = { "me" })
        rt.bootstrap()
        runCurrent()
        assertNotNull("起動で確認を出す", runner.prompt.value)

        runner.defer() // 「あとで」
        val deferred = RetentionSettingsRow.of(rt.entityModels) { runner.consentedDays(appId, it) }.single()
        assertFalse("あとでにした分は設定で確認の入口を出す", deferred.active)
        assertEquals("確認するまで消さない", listOf("old"), engine.cached(appId, "workout").map { it.id })

        // 設定の「確認して自動で消す」(RetentionSettingsSection と同じ呼び方)
        runner.confirm(appId, rt.entityModel("workout")!!)
        rt.sweepRetention("workout")
        runCurrent()
        assertNull(runner.prompt.value)
        assertTrue(RetentionSettingsRow.of(rt.entityModels) { runner.consentedDays(appId, it) }.single().active)
        assertTrue("確認したらすぐ消す", engine.cached(appId, "workout").isEmpty())
        assertEquals(listOf("old"), queue.operations().filter { it.kind == PendingOp.DELETE }.map { it.recordId })
    }

    @Test
    fun stopInstructionNamesTheEntity() {
        // iOS の `removeRetention(entityName:)` と同じ文
        assertEquals("「筋トレ」の記録を自動で消すのをやめて", RetentionStrings.text(R.string.retention_stop_instruction_entity, false, "筋トレ"))
        assertEquals("Stop deleting Workout entries automatically", RetentionStrings.text(R.string.retention_stop_instruction_entity, true, "Workout"))
        assertEquals("記録を自動で消すのをやめて", RetentionStrings.text(R.string.retention_stop_instruction, false))
        assertEquals("Stop deleting entries automatically", RetentionStrings.text(R.string.retention_stop_instruction, true))
    }

    private companion object {
        val examplesDir = File(System.getProperty("metamon.examplesDir") ?: "../../docs/dsl/examples")
    }
}
