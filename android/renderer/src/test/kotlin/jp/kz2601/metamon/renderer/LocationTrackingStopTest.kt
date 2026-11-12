package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Track
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.location.LocationTracking
import jp.kz2601.metamon.renderer.components.location.TrackingTarget
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** #869: 止めて消す(サインアウト・アプリの削除)前に、保存の途中の 1 点を待つ。消した後に点が残らない。 */
class LocationTrackingStopTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"散歩","voiceEntry":"none"},"initialScreen":"s",
         "entities":[{"id":"point","name":"地点","fields":[
           {"id":"loc","name":"場所","type":"location"},{"id":"at","name":"時刻","type":"datetime"}]}],
         "screens":[{"id":"s","title":"t","components":[
           {"type":"locationTracker","id":"tr","entity":"point","locationField":"loc","dateField":"at","label":"散歩を記録する"}]}]}
        """.trimIndent(),
    )
    private val point = dsl.entityModels().first { it.id == "point" }
    private val target = TrackingTarget.of(dsl.screens.first().components[0], "散歩", "app-1")!!
    private val t0 = Instant.parse("2026-11-11T01:00:00Z")

    @After
    fun reset() = runBlocking { LocationTracking.stopAndAwait(null) }

    @Test
    fun stopAndAwaitWaitsForTheInFlightPointSoAWipeAfterItLeavesNoRow() = runBlocking {
        val rows = mutableListOf<Map<String, JSONValue>>()
        val entered = CompletableDeferred<Unit>()
        val gate = CompletableDeferred<Unit>()
        LocationTracking.begin(target, 600, entity = { point }, save = { data ->
            entered.complete(Unit)
            gate.await() // RecordSyncEngine.write の途中
            rows += data
            true
        }, currentAppId = { "app-1" })
        LocationTracking.began(0)

        val inFlight = launch { LocationTracking.savePoint(Track.Point(35.0, 139.0, t0), 5.0) }
        entered.await()
        var stopped = false
        val stopper = launch {
            LocationTracking.stopAndAwait(null)
            stopped = true
        }
        repeat(3) { yield() }
        assertFalse("記録は止まる", LocationTracking.state.value.isActive)
        assertFalse("保存の途中の点が書き終わるまで返らない", stopped)

        gate.complete(Unit)
        stopper.join()
        inFlight.join()
        assertTrue(stopped)
        assertEquals("途中の点は止める前に書き終わる", 1, rows.size)

        rows.clear() // wipe
        LocationTracking.savePoint(Track.Point(35.01, 139.01, t0.plusSeconds(30)), 5.0) // 遅れて届いた点
        assertEquals("消した後に点は足さない", 0, rows.size)
    }

    @Test
    fun stopAndAwaitForAnotherAppKeepsRecording() = runBlocking {
        LocationTracking.begin(target, 600, entity = { point }, save = { true }, currentAppId = { "app-1" })
        LocationTracking.began(0)
        LocationTracking.stopAndAwait("app-2")
        assertTrue("別のアプリの削除では止めない", LocationTracking.state.value.isActive)
        LocationTracking.stopAndAwait("app-1")
        assertFalse(LocationTracking.state.value.isActive)
    }
}
