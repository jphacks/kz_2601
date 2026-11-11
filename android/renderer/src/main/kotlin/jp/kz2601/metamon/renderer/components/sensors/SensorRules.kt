package jp.kz2601.metamon.renderer.components.sensors

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Sensor
import jp.kz2601.metamon.dsl.SensorThreshold
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import java.time.Instant
import java.util.Locale
import kotlin.math.abs
import kotlin.math.acos
import kotlin.math.asin
import kotlin.math.atan2
import kotlin.math.pow
import kotlin.math.round
import kotlin.math.sqrt

// sensorValue(#871)の計算。iOS の MetamonDSL/Records/SensorReading.swift の写し(SensorManager に依らない・JVM テストする)。

/** センサーの種類(iOS の `SensorKind`)。単位・桁・gauge の既定範囲は iOS と同じ。 */
enum class SensorKind {
    TILT, PITCH, ROLL, ACCELERATION, ROTATION, PRESSURE, ALTITUDE;

    /** 気圧計を使うか。それ以外は加速度計・ジャイロ。 */
    val usesBarometer: Boolean get() = this == PRESSURE || this == ALTITUDE

    val unit: String
        get() = when (this) {
            TILT, PITCH, ROLL -> "°"
            ACCELERATION -> "G"
            ROTATION -> "°/s"
            PRESSURE -> "hPa"
            ALTITUDE -> "m"
        }

    /** 小数の桁(表示と保存で同じ)。 */
    val fractionDigits: Int
        get() = when (this) {
            TILT, PITCH, ROLL, ROTATION -> 0
            ACCELERATION -> 2
            PRESSURE, ALTITUDE -> 1
        }

    /** gauge の既定の範囲。 */
    val defaultRange: ClosedFloatingPointRange<Double>
        get() = when (this) {
            TILT -> 0.0..90.0
            PITCH -> -90.0..90.0
            ROLL -> -180.0..180.0
            ACCELERATION -> 0.0..2.0
            ROTATION -> 0.0..360.0
            PRESSURE -> 950.0..1050.0
            ALTITUDE -> -20.0..20.0
        }

    /** 表示値。気圧計の種類は null。 */
    fun value(m: MotionSample): Double? = when (this) {
        TILT -> SensorMath.tiltDegrees(m.gravity)
        PITCH -> SensorMath.degrees(SensorMath.pitch(m.gravity))
        ROLL -> SensorMath.degrees(SensorMath.roll(m.gravity))
        ACCELERATION -> m.userAcceleration.magnitude
        ROTATION -> SensorMath.degrees(m.rotationRate.magnitude)
        PRESSURE, ALTITUDE -> null
    }

    fun value(a: AltitudeSample): Double? = when (this) {
        PRESSURE -> a.pressureHPa
        ALTITUDE -> a.relativeAltitude
        else -> null
    }

    /** 「12」「0.35」「1013.2」(単位は別に出す)。-0 は 0。 */
    fun format(v: Double): String = String.format(Locale.ROOT, "%.${fractionDigits}f", rounded(v))

    /** 保存する値(表示と同じ桁に丸める)。 */
    fun rounded(v: Double): Double {
        val p = 10.0.pow(fractionDigits)
        val r = round(v * p) / p
        return if (r == 0.0) 0.0 else r
    }

    companion object {
        fun of(s: Sensor?): SensorKind? = when (s) {
            Sensor.TILT -> TILT
            Sensor.PITCH -> PITCH
            Sensor.ROLL -> ROLL
            Sensor.ACCELERATION -> ACCELERATION
            Sensor.ROTATION -> ROTATION
            Sensor.PRESSURE -> PRESSURE
            Sensor.ALTITUDE -> ALTITUDE
            Sensor.UNKNOWN, null -> null
        }
    }
}

data class Vector3(val x: Double, val y: Double, val z: Double) {
    val magnitude: Double get() = sqrt(x * x + y * y + z * z)
    operator fun minus(o: Vector3) = Vector3(x - o.x, y - o.y, z - o.z)
    operator fun times(k: Double) = Vector3(x * k, y * k, z * k)
}

/** 動きの 1 回分(iOS の device motion と同じ向き・単位: 重力と加速度は G・角速度は rad/s)。 */
data class MotionSample(
    val gravity: Vector3 = Vector3(0.0, 0.0, -1.0),
    val userAcceleration: Vector3 = Vector3(0.0, 0.0, 0.0),
    val rotationRate: Vector3 = Vector3(0.0, 0.0, 0.0),
)

/** 気圧の 1 回分(hPa)と開始時からの高度差(m)。 */
data class AltitudeSample(val pressureHPa: Double, val relativeAltitude: Double)

object SensorMath {
    /** 標準重力(m/s²)。Android の加速度は m/s²、iOS は G。 */
    const val STANDARD_GRAVITY = 9.80665

    fun degrees(radians: Double): Double = radians * 180 / Math.PI

    fun hectopascals(kilopascals: Double): Double = kilopascals * 10

    /** 水平からの傾き(0 = 平置き・90 = 立てた状態)。表裏は区別しない。 */
    fun tiltDegrees(g: Vector3): Double {
        val m = g.magnitude
        if (m <= 0) return 0.0
        return degrees(acos(minOf(1.0, abs(g.z) / m)))
    }

    /** 前後の傾き(ラジアン)。立てると +π/2(iOS の attitude.pitch に合わせる)。 */
    fun pitch(g: Vector3): Double = atan2(-g.y, sqrt(g.x * g.x + g.z * g.z))

    /** 左右の傾き(ラジアン)。右の辺を下げると +(iOS の attitude.roll に合わせる)。 */
    fun roll(g: Vector3): Double = atan2(g.x, -g.z)

    /** 水平器の泡(-1..1)。泡は高い方へ動く=重力の逆向き。 */
    fun bubble(g: Vector3, maxDegrees: Double = 45.0): Pair<Double, Double> {
        val m = g.magnitude
        if (m <= 0) return 0.0 to 0.0
        fun clamp(v: Double) = v.coerceIn(-1.0, 1.0)
        val x = degrees(asin(clamp(g.x / m))) / maxDegrees
        val y = degrees(asin(clamp(g.y / m))) / maxDegrees
        return clamp(-x) to clamp(y)
    }

    /**
     * 気圧 [p] の、基準 [p0] からの高さ(m)。国際標準大気の式(Android の `SensorManager.getAltitude` と同じ)。
     * p0 に開始時の気圧を入れると「開始時を 0 とした高度差」になる(iOS の CMAltimeter.relativeAltitude)。
     */
    fun altitude(p0: Double, p: Double): Double {
        if (p0 <= 0 || p <= 0) return 0.0
        return 44330.0 * (1 - (p / p0).pow(1 / 5.255))
    }

    /** Android の加速度(m/s²・端末が受ける力)→ iOS の向きの重力(G)。平置きで (0,0,-1)。 */
    fun gravityFromAndroid(accel: Vector3): Vector3 = accel * (-1 / STANDARD_GRAVITY)
}

/** 指数平滑(時間で重みを決める・間隔が揺れても同じ効き)。最初の値はそのまま。 */
class SensorSmoother(private val timeConstantMillis: Double) {
    private var last: Double? = null
    private var lastAt: Long? = null

    fun update(v: Double, atMillis: Long): Double {
        val prev = last
        val prevAt = lastAt
        val out = if (prev == null || prevAt == null || timeConstantMillis <= 0) {
            v
        } else {
            val dt = (atMillis - prevAt).coerceAtLeast(0).toDouble()
            val a = dt / (timeConstantMillis + dt)
            prev + a * (v - prev)
        }
        last = out
        lastAt = atMillis
        return out
    }

    fun reset() {
        last = null
        lastAt = null
    }
}

/**
 * 加速度計の生の値 → [MotionSample]。重力は低域通過で取り出し、残りを動きの加速度にする(iOS の gravity / userAcceleration)。
 * 角速度は別のセンサー(ジャイロ)から [rotation] で入れる。
 */
class MotionFusion(timeConstantMillis: Double = GRAVITY_TIME_CONSTANT_MS) {
    private val gx = SensorSmoother(timeConstantMillis)
    private val gy = SensorSmoother(timeConstantMillis)
    private val gz = SensorSmoother(timeConstantMillis)
    private var gravity = Vector3(0.0, 0.0, -1.0)
    private var user = Vector3(0.0, 0.0, 0.0)
    private var rate = Vector3(0.0, 0.0, 0.0)

    /** [accel] は Android の加速度(m/s²)。 */
    fun accelerometer(accel: Vector3, atMillis: Long): MotionSample {
        val raw = SensorMath.gravityFromAndroid(accel)
        gravity = Vector3(gx.update(raw.x, atMillis), gy.update(raw.y, atMillis), gz.update(raw.z, atMillis))
        // 端末の受ける力 = -(重力 + 動き) なので、動き = -(raw) - (-gravity) を G で
        user = (raw - gravity) * -1.0
        return sample
    }

    /** [radPerSec] は Android のジャイロ(rad/s・iOS と同じ向き)。 */
    fun rotation(radPerSec: Vector3): MotionSample {
        rate = radPerSec
        return sample
    }

    val sample: MotionSample get() = MotionSample(gravity, user, rate)

    companion object {
        const val GRAVITY_TIME_CONSTANT_MS = 200.0
    }
}

/** 気圧(hPa)→ [AltitudeSample]。気圧は平滑化し、最初の値を 0 m にする(CMAltimeter と同じく購読し直すと 0 から)。 */
class AltitudeTracker(timeConstantMillis: Double = PRESSURE_TIME_CONSTANT_MS) {
    private val smoother = SensorSmoother(timeConstantMillis)
    private var baseline: Double? = null

    fun update(hPa: Double, atMillis: Long): AltitudeSample {
        val p = smoother.update(hPa, atMillis)
        val p0 = baseline ?: p.also { baseline = it }
        return AltitudeSample(p, SensorMath.altitude(p0, p))
    }

    fun reset() {
        smoother.reset()
        baseline = null
    }

    companion object {
        /** 気圧は 1 秒ほどで均す(CMAltimeter の更新が約 1 Hz なのに近い揺れ方にする)。 */
        const val PRESSURE_TIME_CONSTANT_MS = 1000.0
    }
}

/** 表示の更新を最大 [hz] 回/秒に間引く。 */
class SensorThrottle(hz: Double = MAX_UI_HZ) {
    val intervalMillis: Double = 1000 / hz.coerceIn(0.1, MAX_UI_HZ)
    private var last: Long? = null

    fun shouldEmit(nowMillis: Long): Boolean {
        val l = last
        if (l != null && nowMillis - l < intervalMillis - 0.5) return false
        last = nowMillis
        return true
    }

    companion object {
        const val MAX_UI_HZ = 10.0
    }
}

/** 一定間隔の記録(画面を開いている間だけ)。開始直後に 1 件、以後 interval 秒ごと、limit 件で終わる。 */
class SensorLogSession(seconds: Int, limit: Int?) {
    val intervalSeconds: Int = seconds.coerceIn(1, MAX_INTERVAL)
    val limit: Int = (limit ?: DEFAULT_LIMIT).coerceIn(1, MAX_LIMIT)
    var saved = 0
        private set
    private var next: Long? = null

    val isFinished: Boolean get() = saved >= limit

    /** [nowMillis] に 1 件保存すべきか。true なら保存済みとして数える(失敗しても数えたまま)。 */
    fun tick(nowMillis: Long): Boolean {
        if (isFinished) return false
        val n = next
        if (n != null && nowMillis < n) return false
        saved += 1
        val step = intervalSeconds * 1000L
        // 予定どおりなら刻みを保ち、大きく遅れたら今から数え直す
        val candidate = (n ?: nowMillis) + step
        next = if (candidate > nowMillis) candidate else nowMillis + step
        return true
    }

    companion object {
        const val DEFAULT_LIMIT = 60
        const val MAX_LIMIT = 1000
        const val MAX_INTERVAL = 3600
    }
}

/** しきい値: 条件に入った瞬間に 1 回だけ発火。出るまで再発火しない・最短 cooldown おき。 */
class SensorThresholdTrigger(val above: Double?, val below: Double?, private val cooldownMillis: Long = DEFAULT_COOLDOWN_MS) {
    var isInside = false
        private set
    private var lastFired: Long? = null

    fun matches(v: Double): Boolean = (above?.let { v > it } ?: false) || (below?.let { v < it } ?: false)

    fun update(v: Double, nowMillis: Long): Boolean {
        val inside = matches(v)
        val was = isInside
        isInside = inside
        if (!inside || was) return false
        val l = lastFired
        if (l != null && nowMillis - l < cooldownMillis) return false
        lastFired = nowMillis
        return true
    }

    companion object {
        const val DEFAULT_COOLDOWN_MS = 3000L
    }
}

/** 1 サンプルごとの処理(間引き → しきい値 → 一定間隔の記録)。画面はイベントを反映するだけ。 */
class SensorPipeline(threshold: SensorThreshold?, canSave: Boolean, hz: Double = SensorThrottle.MAX_UI_HZ) {
    sealed interface Event {
        data class Fired(val value: Double) : Event
        data class Save(val value: Double) : Event
        data class LogFinished(val count: Int) : Event
    }

    private val throttle = SensorThrottle(hz)
    private val trigger = threshold?.takeIf { it.above != null || it.below != null }?.let { SensorThresholdTrigger(it.above, it.below) }
    private val saveOnFire = canSave && threshold?.save == true
    var log: SensorLogSession? = null
        private set

    val isLogging: Boolean get() = log != null
    val isAlerting: Boolean get() = trigger?.isInside ?: false

    fun startLog(seconds: Int, limit: Int?) {
        log = SensorLogSession(seconds, limit)
    }

    fun stopLog() {
        log = null
    }

    /** 間引かれたら null(表示も更新しない)。 */
    fun ingest(v: Double, nowMillis: Long): List<Event>? {
        if (!throttle.shouldEmit(nowMillis)) return null
        val out = mutableListOf<Event>()
        if (trigger?.update(v, nowMillis) == true) {
            out += Event.Fired(v)
            if (saveOnFire) out += Event.Save(v)
        }
        log?.let { s ->
            // しきい値の保存と同じサンプルなら 1 件にまとめる
            if (s.tick(nowMillis) && Event.Save(v) !in out) out += Event.Save(v)
            if (s.isFinished) {
                out += Event.LogFinished(s.saved)
                log = null
            }
        }
        return out
    }
}

/** 部品の値の解決(iOS の SensorValueLeaf の static 関数)。 */
object SensorRules {
    /** 保存する 1 件: field = 表示と同じ桁の値、dateField = 時刻、他は defaultValue のある field だけ。 */
    fun record(kind: SensorKind, value: Double, c: Component, entity: Entity?, at: Instant): Map<String, JSONValue>? {
        val field = c.field ?: return null
        if (entity == null) return null
        val f = entity.field(field) ?: return null
        if (f.isComputed) return null
        val skipping = listOfNotNull(field, c.dateField)
        val data = entity.fields
            .filter { !it.isComputed && it.defaultValue != null && it.id !in skipping }
            .associateTo(LinkedHashMap()) { it.id to FieldCoding.defaultValue(it, at) }
        val v = kind.rounded(value)
        data[field] = JSONValue.Num(if (f.type == FieldType.INTEGER) round(v) else v)
        c.dateField?.let { data[it] = JSONValue.Str(FieldCoding.formatDatetime(at)) }
        return data
    }

    fun range(kind: SensorKind, min: Double?, max: Double?): ClosedFloatingPointRange<Double> {
        val d = kind.defaultRange
        val lo = min ?: d.start
        val hi = max ?: d.endInclusive
        return if (hi > lo) lo..hi else d
    }

    /** 見出しの印(icon-map にある名前だけ)。 */
    fun icon(kind: SensorKind): String = when (kind) {
        SensorKind.TILT, SensorKind.PITCH, SensorKind.ROLL -> "level"
        SensorKind.ACCELERATION -> "waveform"
        SensorKind.ROTATION -> "arrow.triangle.2.circlepath"
        SensorKind.PRESSURE -> "gauge.with.dots.needle.33percent"
        SensorKind.ALTITUDE -> "arrow.up.right"
    }
}
