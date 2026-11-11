package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Track
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.location.TrackingBarModel
import jp.kz2601.metamon.renderer.components.location.TrackingPhase
import jp.kz2601.metamon.renderer.components.location.TrackingRules
import jp.kz2601.metamon.renderer.components.location.TrackingState
import jp.kz2601.metamon.renderer.components.location.TrackingStatus
import jp.kz2601.metamon.renderer.components.location.TrackingStopReason
import jp.kz2601.metamon.renderer.components.location.TrackingTarget
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** #869: locationTracker の状態の遷移と、1 点 = 1 レコードの組み立て(iOS の LocationTracking / LocationTrackerLeaf)。 */
class LocationTrackerTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"散歩","voiceEntry":"none"},"initialScreen":"s",
         "entities":[{"id":"point","name":"地点","fields":[
           {"id":"loc","name":"場所","type":"location"},{"id":"at","name":"時刻","type":"datetime"},
           {"id":"kind","name":"種類","type":"string","defaultValue":"散歩"},{"id":"memo","name":"メモ","type":"string"}]},
          {"id":"note","name":"メモ","fields":[{"id":"loc","name":"場所","type":"string"}]}],
         "screens":[{"id":"s","title":"t","components":[
           {"type":"locationTracker","id":"tr","entity":"point","locationField":"loc","dateField":"at","label":"散歩を記録する","seconds":60},
           {"type":"locationTracker","id":"bad","entity":"point"}]}]}
        """.trimIndent(),
    )
    private val point = dsl.entityModels().first { it.id == "point" }
    private val note = dsl.entityModels().first { it.id == "note" }
    private val target = TrackingTarget.of(dsl.screens.first().components[0], "散歩")!!
    private val t0 = Instant.parse("2026-11-11T01:00:00Z")
    private fun p(lat: Double, lng: Double, sec: Long) = Track.Point(lat, lng, t0.plusSeconds(sec))

    @Test
    fun registeredAndTargetNeedsEntityAndLocationField() {
        assertNotNull(ComponentRegistry.standard.builder(ComponentType.LOCATION_TRACKER))
        assertEquals(TrackingTarget("散歩", "point", "loc", "at"), target)
        assertNull(TrackingTarget.of(dsl.screens.first().components[1], "散歩"))
    }

    @Test
    fun autoStopIsClamped() {
        assertEquals(3600, Track.autoStopSeconds(null))
        assertEquals(300, Track.autoStopSeconds(60))
        assertEquals(14_400, Track.autoStopSeconds(100_000))
        assertEquals(1800, Track.autoStopSeconds(1800))
    }

    @Test
    fun startBeginStop() {
        var s = TrackingState().start(target, 600)
        assertEquals(TrackingPhase.REQUESTING, s.phase)
        assertTrue(s.isActive)
        assertTrue(s.isRecording(target))
        assertFalse(s.isRecording(target.copy(appName = "別")))
        assertEquals(TrackingStatus(isTracking = true, isRequesting = true, appName = "散歩", entity = "point"), s.status)
        // 記録中にもう一度押しても 2 本目は取らない
        assertEquals(s, s.start(target.copy(entity = "note"), 300))

        s = s.began(1_000)
        assertEquals(TrackingPhase.RECORDING, s.phase)
        assertEquals(601_000L, s.endsAtMillis)
        assertEquals(TrackingStatus(isTracking = true, startedAtMillis = 1_000, appName = "散歩", entity = "point"), s.status)
        assertEquals(754L, s.status.elapsedSeconds(755_500))
        assertTrue(TrackingBarModel.isVisible(s.status))

        assertFalse(s.isExpired(600_999))
        assertTrue(s.isExpired(601_000))

        val stopped = s.stop(TrackingStopReason.AUTO_STOP)
        assertEquals(TrackingPhase.IDLE, stopped.phase)
        assertEquals(TrackingStopReason.AUTO_STOP, stopped.stopReason)
        assertEquals(TrackingStatus.IDLE, stopped.status)
        assertFalse(TrackingBarModel.isVisible(stopped.status))
        // 次の開始で理由と数は消える
        val again = stopped.copy(savedCount = 3).start(target, 600)
        assertNull(again.stopReason)
        assertEquals(0, again.savedCount)
    }

    @Test
    fun deniedAndFailedAreNotActive() {
        val denied = TrackingState().start(target, 600).denied()
        assertEquals(TrackingPhase.DENIED, denied.phase)
        assertFalse(denied.isActive)
        assertEquals(TrackingStatus.IDLE, denied.status)
        val failed = TrackingState().start(target, 600).began(0).stop(TrackingStopReason.SAVE_FAILED)
        assertEquals(TrackingPhase.FAILED, failed.phase)
        assertFalse(failed.isActive)
        // 止まっている時の denied は何もしない
        assertEquals(TrackingState(), TrackingState().denied())
    }

    @Test
    fun keepsOnlyAccuratePointsThatMovedOrAged() {
        var s = TrackingState().start(target, 600)
        assertFalse("許可待ちの間は保存しない", s.shouldKeep(p(35.0, 139.0, 0), 5.0))
        s = s.began(0)
        val a = p(35.681236, 139.767125, 0)
        assertFalse("精度が悪い", s.shouldKeep(a, 80.0))
        assertFalse("精度が無い", s.shouldKeep(a, -1.0))
        assertTrue(s.shouldKeep(a, 10.0))
        s = s.kept(a)
        assertFalse("10 秒で数 m しか動いていない", s.shouldKeep(p(35.681240, 139.767125, 10), 10.0))
        val b = p(35.682100, 139.766300, 30) // 約 120 m
        assertTrue(s.shouldKeep(b, 10.0))
        s = s.kept(b)
        assertTrue("5 分経てば止まっていても 1 点", s.shouldKeep(p(35.682100, 139.766300, 330), 10.0))
        assertEquals(2, s.savedCount)
        assertEquals(Track.distance(a, b), s.distance, 1e-9)
        assertTrue(s.distance in 100.0..140.0)
    }

    @Test
    fun pointRecordData() {
        val at = Instant.parse("2026-11-11T01:02:03Z")
        val data = TrackingRules.pointData(target, point, Track.Point(35.6812361, 139.7671249, at))!!
        assertEquals(JSONValue.Str("35.681236,139.767125"), data["loc"])
        assertEquals(JSONValue.Str("2026-11-11T01:02:03Z"), data["at"])
        assertEquals(JSONValue.Str("散歩"), data["kind"])
        assertFalse("既定値の無い field は入れない", "memo" in data)
        // dateField が無ければ時刻は入れない
        assertFalse("at" in TrackingRules.pointData(target.copy(dateField = null), point, Track.Point(1.0, 2.0, at))!!)
        // 記録先が無い・location でない(アプリが変わった)なら保存しない
        assertNull(TrackingRules.pointData(target, null, Track.Point(1.0, 2.0, at)))
        assertNull(TrackingRules.pointData(target.copy(entity = "note"), note, Track.Point(1.0, 2.0, at)))
        // 保存した点は stat の distance / duration の集計にそのまま乗る
        assertEquals(35.681236 to 139.767125, Track.coordinate(data["loc"]!!))
    }

    @Test
    fun clockAndRemaining() {
        assertEquals("00:00", TrackingRules.clock(-3))
        assertEquals("12:34", TrackingRules.clock(754))
        assertEquals("1:02:03", TrackingRules.clock(3723))
        val s = TrackingState().start(target, 3600).began(0)
        assertEquals(61L, TrackingRules.remainingMinutes(s, 0)) // iOS と同じく +1
        assertEquals(47L, TrackingRules.remainingMinutes(s, (13 * 60 + 30) * 1000L))
    }

    /** 開始した時のアプリの appId を持ち、今のアプリが違えば点を作らない(保存失敗として止まる)。 */
    @Test
    fun pointsGoOnlyToTheAppThatStartedTracking() {
        val c = dsl.screens.first().components[0]
        val scoped = TrackingTarget.of(c, "散歩", "app-1")!!
        assertEquals("app-1", scoped.appId)
        assertFalse("別のアプリの同じ部品は記録中に見せない", TrackingState().start(scoped, 600).isRecording(scoped.copy(appId = "app-2")))
        assertNotNull(TrackingRules.pointData(scoped, point, p(35.0, 139.0, 0), currentAppId = "app-1"))
        assertNull("切り替えた後", TrackingRules.pointData(scoped, point, p(35.0, 139.0, 0), currentAppId = "app-2"))
        assertNull("消した後(新しい appId)", TrackingRules.pointData(scoped, point, p(35.0, 139.0, 0), currentAppId = null))
    }

    /** サインアウト・アカウント削除(null)は必ず止め、アプリの削除はそのアプリの記録だけ止める。 */
    @Test
    fun stopForMatchesSignOutAndTheDeletedApp() {
        val scoped = TrackingTarget.of(dsl.screens.first().components[0], "散歩", "app-1")!!
        val recording = TrackingState().start(scoped, 600).began(0)
        assertTrue(recording.shouldStopFor(null))
        assertTrue(recording.shouldStopFor("app-1"))
        assertFalse(recording.shouldStopFor("app-2"))
        assertFalse("記録していなければ何もしない", TrackingState().shouldStopFor(null))
        assertTrue("許可を待っている間も止める", TrackingState().start(scoped, 600).shouldStopFor("app-1"))
    }
}
