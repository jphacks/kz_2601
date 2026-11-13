package jp.kz2601.metamon.renderer.components.health

import android.content.Context
import androidx.compose.runtime.staticCompositionLocalOf
import java.time.LocalDate

/** 1 日分の値(その日の歩数・その日の最新の体重 kg)。 */
data class HealthDailyValue(val day: LocalDate, val value: Double)

/** ヘルスの読み書きが使えるか(iOS の `HealthDataError.unavailable` に当たる端末側の事情)。 */
enum class HealthAvailability {
    /** 使える(Health Connect が入っている・34+ は OS が持つ) */
    AVAILABLE,

    /** この端末では使えない */
    UNAVAILABLE,

    /** Health Connect の更新・導入が要る(13 以前で入っていない時を含む) */
    UPDATE_REQUIRED,
}

/** 読み書きの失敗(iOS の `HealthDataError`)。 */
enum class HealthFailure { UNAVAILABLE, UPDATE_REQUIRED, DENIED, FAILED }

class HealthDataException(val failure: HealthFailure, cause: Throwable? = null) : Exception(failure.name, cause)

/**
 * `healthStat` と `Field.health` が読み書きするヘルスのデータ(iOS の `HealthDataSource`)。
 * 実体は [HealthConnectSource]。確認用は [MockHealthSource]。
 */
interface HealthSource {
    fun availability(): HealthAvailability

    /** その metric に要る Health Connect の権限名(読み取り)。 */
    fun readPermissions(weight: Boolean): Set<String>

    /** 体重を書くのに要る権限名。 */
    fun writePermissions(): Set<String>

    /** 今許可されている権限名。 */
    suspend fun granted(): Set<String>

    /** 今日を最後とする直近 [days] 日の歩数(古い順・1 日 1 件・記録が無い日は 0)。 */
    suspend fun steps(days: Int): List<HealthDailyValue>

    /** 直近 [days] 日の体重 kg(古い順・記録のある日だけ・その日の最新値)。 */
    suspend fun weights(days: Int): List<HealthDailyValue>

    /** 体重 kg を 1 件書き込む。 */
    suspend fun saveWeight(kg: Double)
}

/**
 * 部品が使うヘルスのデータ。null なら端末の Health Connect([HealthConnectSource])。
 * `METAMON_MOCK=1` の時は :app が [MockHealthSource] を入れる。
 */
val LocalHealthSource = staticCompositionLocalOf<HealthSource?> { null }

/** 端末の Health Connect(モックでなければこれ)。 */
fun defaultHealthSource(context: Context): HealthSource = HealthConnectSource(context)

/** 確認用の固定値(iOS の `MockHealthData` と同じ数)。 */
object MockHealthSource : HealthSource {
    val stepSeries: List<Double> = listOf(7120.0, 9340.0, 4210.0, 8800.0, 10230.0, 5600.0, 6240.0)
    val weightSeries: List<Double> = listOf(62.4, 62.3, 62.3, 62.1, 62.0, 61.9, 61.8)

    override fun availability() = HealthAvailability.AVAILABLE

    override fun readPermissions(weight: Boolean): Set<String> = emptySet()

    override fun writePermissions(): Set<String> = emptySet()

    override suspend fun granted(): Set<String> = emptySet()

    override suspend fun steps(days: Int): List<HealthDailyValue> = series(stepSeries, days)

    override suspend fun weights(days: Int): List<HealthDailyValue> = series(weightSeries, days)

    override suspend fun saveWeight(kg: Double) = Unit

    /** 日数が系列より長い時は古い日を詰めない(iOS の `MockHealthData.series`)。 */
    fun series(values: List<Double>, days: Int, today: LocalDate = LocalDate.now()): List<HealthDailyValue> {
        val starts = HealthStatRules.dayStarts(days, today)
        val tail = values.takeLast(starts.size)
        val pad = starts.size - tail.size
        return starts.drop(pad).mapIndexed { i, d -> HealthDailyValue(d, tail[i]) }
    }
}
