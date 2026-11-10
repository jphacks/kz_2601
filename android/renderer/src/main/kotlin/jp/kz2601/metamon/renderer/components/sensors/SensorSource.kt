package jp.kz2601.metamon.renderer.components.sensors

import android.content.Context
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
import android.os.SystemClock
import androidx.compose.runtime.staticCompositionLocalOf
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.flow
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin

/** センサーが使えるか(iOS の `SensorAvailability`。Android は権限が要らないので denied は無い)。 */
enum class SensorAvailability { AVAILABLE, UNAVAILABLE }

/** 1 回分の値。[motion] は動き由来の時だけ(水平器の泡に使う)。 */
data class SensorFrame(val value: Double, val motion: MotionSample? = null)

/** sensorValue が読むセンサー(iOS の `SensorSource`)。流れの収集をやめるとセンサーを止める。 */
interface SensorSource {
    fun availability(kind: SensorKind): SensorAvailability

    fun frames(kind: SensorKind): Flow<SensorFrame>
}

/** 部品が使うセンサー。null なら端末の SensorManager(テスト・プレビューは差し替える)。 */
val LocalSensorSource = staticCompositionLocalOf<SensorSource?> { null }

/**
 * 実機: SensorManager の加速度計・ジャイロ・気圧計。収集している間だけ listener を登録する。
 * 更新は 10 Hz を頼む(200 Hz 未満なので HIGH_SAMPLING_RATE_SENSORS の権限は要らない)。
 */
class AndroidSensorSource(context: Context) : SensorSource {
    private val manager = context.applicationContext.getSystemService(Context.SENSOR_SERVICE) as? SensorManager

    override fun availability(kind: SensorKind): SensorAvailability =
        if (manager?.getDefaultSensor(sensorType(kind)) != null) SensorAvailability.AVAILABLE else SensorAvailability.UNAVAILABLE

    override fun frames(kind: SensorKind): Flow<SensorFrame> {
        val m = manager ?: return emptyFlow()
        val sensor = m.getDefaultSensor(sensorType(kind)) ?: return emptyFlow()
        return callbackFlow {
            val fusion = MotionFusion()
            val altitude = AltitudeTracker()
            val listener = object : SensorEventListener {
                override fun onSensorChanged(e: SensorEvent) {
                    val at = e.timestamp / 1_000_000 // 起動からの ns → ms(平滑化の間隔にだけ使う)
                    val v = Vector3(e.values[0].toDouble(), e.values[1].toDouble(), e.values[2].toDouble())
                    val frame = when (e.sensor.type) {
                        Sensor.TYPE_PRESSURE -> kind.value(altitude.update(e.values[0].toDouble(), at))?.let { SensorFrame(it) }
                        Sensor.TYPE_GYROSCOPE -> fusion.rotation(v).let { s -> kind.value(s)?.let { SensorFrame(it, s) } }
                        else -> fusion.accelerometer(v, at).let { s -> kind.value(s)?.let { SensorFrame(it, s) } }
                    }
                    frame?.let { trySend(it) }
                }

                override fun onAccuracyChanged(sensor: Sensor?, accuracy: Int) = Unit
            }
            m.registerListener(listener, sensor, SAMPLING_PERIOD_US)
            awaitClose { m.unregisterListener(listener) }
        }
    }

    companion object {
        const val SAMPLING_PERIOD_US = 100_000

        fun sensorType(kind: SensorKind): Int = when (kind) {
            SensorKind.PRESSURE, SensorKind.ALTITUDE -> Sensor.TYPE_PRESSURE
            SensorKind.ROTATION -> Sensor.TYPE_GYROSCOPE
            SensorKind.TILT, SensorKind.PITCH, SensorKind.ROLL, SensorKind.ACCELERATION -> Sensor.TYPE_ACCELEROMETER
        }
    }
}

/** センサーの無い端末と同じ振る舞い(プレビュー・テスト)。 */
object UnavailableSensorSource : SensorSource {
    override fun availability(kind: SensorKind) = SensorAvailability.UNAVAILABLE

    override fun frames(kind: SensorKind): Flow<SensorFrame> = emptyFlow()
}

/** 確認用の決まった波形(iOS の `MockSensorData`): 傾きは 0〜40° を 12 秒で往復してしきい値 30° をまたぐ。 */
object MockSensorSource : SensorSource {
    override fun availability(kind: SensorKind) = SensorAvailability.AVAILABLE

    override fun frames(kind: SensorKind): Flow<SensorFrame> = flow {
        val start = SystemClock.elapsedRealtime()
        while (true) {
            emit(frame(kind, (SystemClock.elapsedRealtime() - start) / 1000.0))
            delay(100)
        }
    }

    fun frame(kind: SensorKind, t: Double): SensorFrame {
        val tilt = 20 + 20 * sin(t * 2 * PI / 12)
        val r = tilt * PI / 180
        val m = MotionSample(
            gravity = Vector3(sin(r) * 0.6, -sin(r) * 0.8, -cos(r)),
            userAcceleration = Vector3(0.05 * sin(t * 3), 0.03 * cos(t * 5), 0.02),
            rotationRate = Vector3(0.2 * sin(t), 0.1, 0.0),
        )
        val a = AltitudeSample(1013.2 - 0.03 * sin(t / 10), 1.5 * sin(t / 20))
        return if (kind.usesBarometer) SensorFrame(kind.value(a) ?: 0.0) else SensorFrame(kind.value(m) ?: 0.0, m)
    }
}
