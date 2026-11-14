package jp.kz2601.metamon.renderer.components.health

import android.content.Context
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
import androidx.compose.runtime.staticCompositionLocalOf
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.flow
import kotlin.math.max

/**
 * `healthStat` の `live`(#3694)が前面にいる間だけ購読する歩数計(iOS の `HealthLiveSource`)。
 * `TYPE_STEP_COUNTER` は端末の起動からの累計を流す。差分は [LiveStepDelta] で出し、Health Connect の一発読みに足す。
 * 流れの収集をやめると listener を外す。
 */
interface StepCounterSource {
    /** 歩数計の累計(起動からの歩数)。使えない端末は空の流れ。 */
    fun counts(): Flow<Float>
}

/** 部品が使う歩数計。null なら端末の SensorManager([AndroidStepCounterSource])。`METAMON_MOCK=1` の時は :app が [MockStepCounterSource] を入れる。 */
val LocalStepCounterSource = staticCompositionLocalOf<StepCounterSource?> { null }

fun defaultStepCounterSource(context: Context): StepCounterSource = AndroidStepCounterSource(context)

/** 実機: `SensorManager.TYPE_STEP_COUNTER`。`ACTIVITY_RECOGNITION` の許可が要る(聞くのは [HealthStatLeaf])。 */
class AndroidStepCounterSource(context: Context) : StepCounterSource {
    private val manager = context.applicationContext.getSystemService(Context.SENSOR_SERVICE) as? SensorManager

    override fun counts(): Flow<Float> {
        val m = manager ?: return emptyFlow()
        val sensor = m.getDefaultSensor(Sensor.TYPE_STEP_COUNTER) ?: return emptyFlow()
        return callbackFlow {
            val listener = object : SensorEventListener {
                override fun onSensorChanged(e: SensorEvent) {
                    e.values.firstOrNull()?.let { trySend(it) }
                }

                override fun onAccuracyChanged(sensor: Sensor?, accuracy: Int) = Unit
            }
            // 遅延なしで届けてもらう(既定は省電力のためにまとめて届く)
            m.registerListener(listener, sensor, SensorManager.SENSOR_DELAY_UI, 0)
            awaitClose { m.unregisterListener(listener) }
        }
    }
}

/** 歩数計の無い端末(プレビュー・テスト)。 */
object UnavailableStepCounterSource : StepCounterSource {
    override fun counts(): Flow<Float> = emptyFlow()
}

/** 確認用: 1.5 秒ごとに 1 歩ずつ増える(iOS の `MockHealthLiveSource`)。 */
object MockStepCounterSource : StepCounterSource {
    const val INTERVAL_MS = 1_500L

    override fun counts(): Flow<Float> = flow {
        var n = 1000f
        emit(n)
        while (true) {
            delay(INTERVAL_MS)
            n += 1
            emit(n)
        }
    }
}

/**
 * 歩数計の累計 → 購読してからの増分(純粋な計算・JVM テストする)。
 * 最初に届いた値を基準にする(購読した時点の累計)ので、Health Connect の一発読みに足しても二重に数えない。
 * 基準値を取り直した時(前面に戻った時)は新しい [LiveStepDelta] を作る。
 */
class LiveStepDelta {
    private var first: Float? = null

    /** 届いた累計 [counter] に対する、購読してからの増分(0 以上)。 */
    fun delta(counter: Float): Double {
        val f = first ?: counter.also { first = it }
        return max(0.0, (counter - f).toDouble())
    }

    companion object {
        /** 一発読みの並び [baseline] の最後(今日)に増分 [delta] を足した並び。空ならそのまま。 */
        fun apply(baseline: List<HealthDailyValue>, delta: Double): List<HealthDailyValue> {
            if (baseline.isEmpty()) return baseline
            val last = baseline.last()
            return baseline.dropLast(1) + last.copy(value = last.value + delta)
        }
    }
}
