package jp.kz2601.metamon.desktop.power

import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

// 端末の電池・充電・省電力(iOS の DevicePowerSource.swift #2006 と同じ形)。式・UI にはまだ出さない(#1775 の決定待ち)。
// 値は端末の中だけで使い、サーバーには送らない。

/** 端末の電源の状態。 */
data class DevicePowerState(
    /** 残り 0〜1。分からない時(電池が無い・読めない)は null。 */
    val batteryLevel: Double?,
    val charge: Charge,
    /** 省電力。読めない OS では false。 */
    val isPowerSaver: Boolean,
) {
    enum class Charge { UNKNOWN, UNPLUGGED, CHARGING, FULL }

    /** 電源につながっている(満充電も含む)。 */
    val isCharging: Boolean get() = charge == Charge.CHARGING || charge == Charge.FULL
    val isFull: Boolean get() = charge == Charge.FULL

    companion object {
        val Unknown = DevicePowerState(batteryLevel = null, charge = Charge.UNKNOWN, isPowerSaver = false)

        /** 0〜1 に丸めて作る(NaN は不明)。 */
        fun of(batteryLevel: Double?, charge: Charge, isPowerSaver: Boolean) = DevicePowerState(
            batteryLevel = batteryLevel?.takeUnless { it.isNaN() }?.coerceIn(0.0, 1.0),
            charge = charge,
            isPowerSaver = isPowerSaver,
        )
    }
}

/** 端末の電源の状態を読む口。読むだけ。 */
interface DevicePowerSource {
    /** 今の値。ファイルを読むので UI のスレッドで呼ばない。 */
    val current: DevicePowerState

    /** 集めた時に今の値を 1 回、その後は変わった時だけ流す。 */
    val changes: Flow<DevicePowerState>
}

/** OS から 1 回読む部分(OS ごとの実体・テストで差し替える)。 */
fun interface DevicePowerReader {
    fun read(): DevicePowerState
}

/**
 * OS には変化の知らせが無い(Linux の sysfs・Windows の GetSystemPowerStatus)ので一定の間隔で読み直す実体。
 * 読みに失敗したら不明にする(落とさない)。
 */
class PollingDevicePowerSource(
    private val reader: DevicePowerReader,
    private val interval: Duration = 60.seconds,
    private val dispatcher: CoroutineDispatcher = Dispatchers.IO,
) : DevicePowerSource {
    override val current: DevicePowerState get() = safeRead()

    override val changes: Flow<DevicePowerState> = flow {
        while (true) {
            emit(safeRead())
            delay(interval)
        }
    }.distinctUntilChanged().flowOn(dispatcher)

    private fun safeRead(): DevicePowerState = runCatching { reader.read() }.getOrDefault(DevicePowerState.Unknown)
}

/** テスト・`METAMON_MOCK=1` で使う偽物。`state` を書き換えると集めている側に届く。 */
class FakeDevicePowerSource(
    initial: DevicePowerState = DevicePowerState(batteryLevel = 0.8, charge = DevicePowerState.Charge.UNPLUGGED, isPowerSaver = false),
) : DevicePowerSource {
    private val flow = MutableStateFlow(initial)

    var state: DevicePowerState
        get() = flow.value
        set(value) {
            flow.value = value
        }

    override val current: DevicePowerState get() = flow.value
    override val changes: StateFlow<DevicePowerState> get() = flow

    companion object {
        /** `METAMON_BATTERY_LEVEL`(0〜1・無ければ不明)・`METAMON_BATTERY_STATE`(unknown/unplugged/charging/full)・`METAMON_LOW_POWER=1`(iOS と同じ名前)。 */
        fun fromEnvironment(env: Map<String, String>): FakeDevicePowerSource {
            val level = env["METAMON_BATTERY_LEVEL"]?.toDoubleOrNull()
            val charge = env["METAMON_BATTERY_STATE"]?.let { s -> DevicePowerState.Charge.entries.firstOrNull { it.name.equals(s, ignoreCase = true) } }
                ?: if (level == null) DevicePowerState.Charge.UNKNOWN else DevicePowerState.Charge.UNPLUGGED
            return FakeDevicePowerSource(DevicePowerState.of(level, charge, isPowerSaver = env["METAMON_LOW_POWER"] == "1"))
        }
    }
}

object DevicePowerSources {
    /** `METAMON_MOCK=1` なら偽物。それ以外は OS ごとの実体(Linux は sysfs・Windows と Mac は今は不明を返す)。 */
    fun make(
        env: Map<String, String> = System.getenv(),
        osName: String = System.getProperty("os.name").orEmpty(),
    ): DevicePowerSource = when {
        env["METAMON_MOCK"] == "1" -> FakeDevicePowerSource.fromEnvironment(env)
        osName.startsWith("Linux", ignoreCase = true) -> PollingDevicePowerSource(LinuxSysfsPowerReader())
        osName.startsWith("Windows", ignoreCase = true) -> PollingDevicePowerSource(WindowsPowerReader)
        // Mac は Mac 版(Swift)が受け持つ。iOS 版も Mac の電池は読まない
        else -> PollingDevicePowerSource({ DevicePowerState.Unknown })
    }
}

/**
 * Windows: 今は不明を返す。
 * TODO: kernel32 の GetSystemPowerStatus(ACLineStatus・BatteryFlag・BatteryLifePercent・SystemStatusFlag=省電力)で読む。
 *  JNA がまだ依存に無いので足すかの判断と一緒に入れる(powershell・WMI を毎分起こす方法は取らない)。
 */
object WindowsPowerReader : DevicePowerReader {
    override fun read(): DevicePowerState = DevicePowerState.Unknown
}
