package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Sensor
import jp.kz2601.metamon.dsl.SensorThreshold
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.sensors.AltitudeTracker
import jp.kz2601.metamon.renderer.components.sensors.MotionFusion
import jp.kz2601.metamon.renderer.components.sensors.SensorAvailability
import jp.kz2601.metamon.renderer.components.sensors.SensorFrames
import jp.kz2601.metamon.renderer.components.sensors.SensorKind
import jp.kz2601.metamon.renderer.components.sensors.SensorLogSession
import jp.kz2601.metamon.renderer.components.sensors.SensorMath
import jp.kz2601.metamon.renderer.components.sensors.SensorPipeline
import jp.kz2601.metamon.renderer.components.sensors.SensorRules
import jp.kz2601.metamon.renderer.components.sensors.SensorSmoother
import jp.kz2601.metamon.renderer.components.sensors.SensorThresholdTrigger
import jp.kz2601.metamon.renderer.components.sensors.SensorThrottle
import jp.kz2601.metamon.renderer.components.sensors.UnavailableSensorSource
import jp.kz2601.metamon.renderer.components.sensors.Vector3
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** sensorValue(#871): 単位の変換・相対高度・平滑化・間引き・記録・しきい値・使えない端末(iOS の SensorReadingTests に合わせる)。 */
class SensorLeavesTest {
    private val g = SensorMath.STANDARD_GRAVITY

    @Test
    fun registered() {
        assertTrue(ComponentType.SENSOR_VALUE.wire in ComponentRegistry.standard.registeredTypes)
    }

    @Test
    fun kindsMatchDslAndIos() {
        assertEquals(SensorKind.ALTITUDE, SensorKind.of(Sensor.ALTITUDE))
        assertNull(SensorKind.of(Sensor.UNKNOWN))
        assertNull(SensorKind.of(null))
        assertEquals(listOf("°", "°", "°", "G", "°/s", "hPa", "m"), SensorKind.entries.map { it.unit })
        assertEquals(listOf(0, 0, 0, 2, 0, 1, 1), SensorKind.entries.map { it.fractionDigits })
        assertTrue(SensorKind.PRESSURE.usesBarometer && SensorKind.ALTITUDE.usesBarometer && !SensorKind.TILT.usesBarometer)
    }

    @Test
    fun formatAndRound() {
        assertEquals("12", SensorKind.TILT.format(12.4))
        assertEquals("0.35", SensorKind.ACCELERATION.format(0.349))
        assertEquals("1013.2", SensorKind.PRESSURE.format(1013.24))
        assertEquals("0", SensorKind.PITCH.format(-0.3)) // -0 にしない
        assertEquals(0.0, SensorKind.ALTITUDE.rounded(-0.04), 0.0)
        assertEquals(1.5, SensorKind.ALTITUDE.rounded(1.46), 1e-9)
    }

    @Test
    fun unitConversion() {
        assertEquals(1013.2, SensorMath.hectopascals(101.32), 1e-9)
        assertEquals(180.0, SensorMath.degrees(Math.PI), 1e-9)
        // 平置き(画面が上): Android は z = +9.8 m/s²、iOS の重力は (0, 0, -1)
        val flat = SensorMath.gravityFromAndroid(Vector3(0.0, 0.0, g))
        assertEquals(-1.0, flat.z, 1e-9)
        assertEquals(0.0, SensorMath.tiltDegrees(flat), 1e-9)
        // 縦に立てる: 重力は (0, -1, 0)・傾き 90°・pitch +90°
        val upright = SensorMath.gravityFromAndroid(Vector3(0.0, g, 0.0))
        assertEquals(90.0, SensorMath.tiltDegrees(upright), 1e-9)
        assertEquals(90.0, SensorMath.degrees(SensorMath.pitch(upright)), 1e-9)
        // 右の辺を下げて 30°: roll +30°
        val r = Math.toRadians(30.0)
        assertEquals(30.0, SensorMath.degrees(SensorMath.roll(Vector3(Math.sin(r), 0.0, -Math.cos(r)))), 1e-9)
        // 角速度 rad/s → °/s
        assertEquals(180.0, SensorKind.ROTATION.value(jp.kz2601.metamon.renderer.components.sensors.MotionSample(rotationRate = Vector3(0.0, 0.0, Math.PI)))!!, 1e-9)
    }

    @Test
    fun motionFusionSeparatesGravityAndUserAcceleration() {
        val fusion = MotionFusion()
        var s = fusion.accelerometer(Vector3(0.0, 0.0, g), 0)
        for (t in 1..50) s = fusion.accelerometer(Vector3(0.0, 0.0, g), t * 100L)
        assertEquals(0.0, SensorKind.ACCELERATION.value(s)!!, 1e-6)
        assertEquals(0.0, SensorKind.TILT.value(s)!!, 1e-6)
        // 急に上へ 0.5 G 加速: 直後は動きとして出る(重力はゆっくり追う)
        s = fusion.accelerometer(Vector3(0.0, 0.0, g * 1.5), 5_100)
        assertTrue(SensorKind.ACCELERATION.value(s)!! in 0.2..0.5)
    }

    @Test
    fun relativeAltitudeStartsAtZero() {
        // 国際標準大気: 海面付近で 1 hPa ≈ 8.4 m
        assertEquals(0.0, SensorMath.altitude(1013.25, 1013.25), 1e-9)
        assertEquals(8.4, SensorMath.altitude(1013.25, 1012.25), 0.1)
        assertTrue(SensorMath.altitude(1013.25, 1014.25) < 0)
        assertEquals(0.0, SensorMath.altitude(0.0, 1000.0), 0.0)

        val tracker = AltitudeTracker(timeConstantMillis = 0.0)
        assertEquals(0.0, tracker.update(1000.0, 0).relativeAltitude, 0.0)
        val up = tracker.update(999.0, 1000)
        assertEquals(999.0, up.pressureHPa, 1e-9)
        assertEquals(SensorMath.altitude(1000.0, 999.0), up.relativeAltitude, 1e-9)
        assertEquals(SensorMath.altitude(1000.0, 999.0), SensorKind.ALTITUDE.value(up)!!, 1e-9)
        // 購読し直すと開始時がまた 0
        tracker.reset()
        assertEquals(0.0, tracker.update(990.0, 2000).relativeAltitude, 0.0)
    }

    @Test
    fun smoothing() {
        val s = SensorSmoother(timeConstantMillis = 1000.0)
        assertEquals(10.0, s.update(10.0, 0), 0.0) // 最初はそのまま
        assertEquals(15.0, s.update(20.0, 1000), 1e-9) // dt = τ で半分寄る
        assertEquals(15.0, s.update(100.0, 1000), 1e-9) // dt = 0 は動かない
        // 雑音のある気圧でも高度は大きく跳ねない
        val tracker = AltitudeTracker()
        tracker.update(1000.0, 0)
        val noisy = tracker.update(1000.5, 100)
        assertTrue(noisy.relativeAltitude > -1.0 && noisy.relativeAltitude < 0)
    }

    @Test
    fun throttleLimitsTo10Hz() {
        val t = SensorThrottle()
        assertTrue(t.shouldEmit(0))
        assertFalse(t.shouldEmit(50))
        assertTrue(t.shouldEmit(100))
        assertEquals(100.0, SensorThrottle(hz = 100.0).intervalMillis, 1e-9)
    }

    @Test
    fun logSession() {
        val s = SensorLogSession(seconds = 60, limit = 3)
        assertTrue(s.tick(0))
        assertFalse(s.tick(59_000))
        assertTrue(s.tick(60_000))
        // 大きく遅れたら今から数え直す
        assertTrue(s.tick(500_000))
        assertTrue(s.isFinished)
        assertFalse(s.tick(1_000_000))
        assertEquals(1, SensorLogSession(0, null).intervalSeconds)
        assertEquals(60, SensorLogSession(5, null).limit)
        assertEquals(1000, SensorLogSession(5, 5000).limit)
        assertEquals(3600, SensorLogSession(99_999, 1).intervalSeconds)
    }

    @Test
    fun thresholdFiresOncePerEntry() {
        val t = SensorThresholdTrigger(above = 30.0, below = null)
        assertFalse(t.update(10.0, 0))
        assertTrue(t.update(31.0, 100))
        assertFalse(t.update(35.0, 200))
        assertFalse(t.update(10.0, 300))
        assertFalse(t.update(40.0, 400)) // cooldown 3 秒
        assertTrue(t.isInside)
        assertFalse(t.update(10.0, 5000))
        assertTrue(t.update(40.0, 5100))
    }

    @Test
    fun pipelineMergesSaves() {
        val p = SensorPipeline(SensorThreshold(above = 30.0, save = true), canSave = true)
        p.startLog(seconds = 60, limit = 2)
        val first = p.ingest(40.0, 0)!!
        assertEquals(listOf(SensorPipeline.Event.Fired(40.0), SensorPipeline.Event.Save(40.0)), first)
        assertTrue(p.isAlerting)
        assertNull(p.ingest(40.0, 50)) // 間引き
        val last = p.ingest(10.0, 60_000)!!
        assertEquals(listOf(SensorPipeline.Event.Save(10.0), SensorPipeline.Event.LogFinished(2)), last)
        assertFalse(p.isLogging)
        assertFalse(p.isAlerting)
        // 保存先が無ければしきい値で保存しない
        assertEquals(listOf(SensorPipeline.Event.Fired(40.0)), SensorPipeline(SensorThreshold(above = 30.0, save = true), canSave = false).ingest(40.0, 0))
    }

    @Test
    fun recordForEntity() {
        val dsl = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"s",
             "entities":[{"id":"reading","name":"気圧","fields":[
               {"id":"hpa","name":"気圧","type":"number"},{"id":"n","name":"回","type":"integer"},
               {"id":"at","name":"時刻","type":"datetime","defaultValue":"now"},{"id":"place","name":"場所","type":"string","defaultValue":"家"},
               {"id":"memo","name":"メモ","type":"string"}]}],
             "screens":[{"id":"s","title":"t","components":[
               {"type":"sensorValue","sensor":"pressure","entity":"reading","field":"hpa","dateField":"at","seconds":60,"limit":120},
               {"type":"sensorValue","sensor":"tilt","entity":"reading","field":"n"}]}]}
            """.trimIndent(),
        )
        val entity = dsl.entityModels().first()
        val (pressure, tilt) = dsl.screens.first().components
        val at = Instant.parse("2026-11-11T01:02:03Z")
        val data = SensorRules.record(SensorKind.PRESSURE, 1013.26, pressure, entity, at)!!
        assertEquals(1013.3, (data["hpa"] as JSONValue.Num).value, 1e-9)
        assertEquals(JSONValue.Str("2026-11-11T01:02:03Z"), data["at"])
        assertEquals(JSONValue.Str("家"), data["place"])
        assertFalse("memo" in data)
        assertEquals(12.0, (SensorRules.record(SensorKind.TILT, 12.4, tilt, entity, at)!!["n"] as JSONValue.Num).value, 0.0)
        assertNull(SensorRules.record(SensorKind.TILT, 1.0, tilt, null, at))
    }

    @Test
    fun gaugeRange() {
        assertEquals(950.0..1050.0, SensorRules.range(SensorKind.PRESSURE, null, null))
        assertEquals(0.0..45.0, SensorRules.range(SensorKind.TILT, null, 45.0))
        assertEquals(-20.0..20.0, SensorRules.range(SensorKind.ALTITUDE, 5.0, 1.0)) // 逆は既定
    }

    @Test
    fun unavailableDevice() {
        for (k in SensorKind.entries) assertEquals(SensorAvailability.UNAVAILABLE, UnavailableSensorSource.availability(k))
    }

    /** 気圧計の SensorEvent.values は 1 要素。3 軸として読まずに気圧・相対高度にする。 */
    @Test
    fun pressureEventHasOneValue() {
        val pressure = SensorFrames(SensorKind.PRESSURE)
        assertEquals(1013.0, pressure.next(android.hardware.Sensor.TYPE_PRESSURE, floatArrayOf(1013f), 0)!!.value, 1e-6)
        val altitude = SensorFrames(SensorKind.ALTITUDE)
        assertEquals(0.0, altitude.next(android.hardware.Sensor.TYPE_PRESSURE, floatArrayOf(1013f), 0)!!.value, 1e-6)
        assertNull(pressure.next(android.hardware.Sensor.TYPE_PRESSURE, floatArrayOf(), 0))
    }

    @Test
    fun motionEventNeedsThreeAxes() {
        val tilt = SensorFrames(SensorKind.TILT)
        assertNull(tilt.next(android.hardware.Sensor.TYPE_ACCELEROMETER, floatArrayOf(0f, 0f), 0))
        val flat = tilt.next(android.hardware.Sensor.TYPE_ACCELEROMETER, floatArrayOf(0f, 0f, g.toFloat()), 0)!!
        assertEquals(0.0, flat.value, 0.5)
        assertTrue(flat.motion != null)
    }
}
