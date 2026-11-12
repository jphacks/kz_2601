package jp.kz2601.metamon.desktop.power

import com.sun.jna.Native
import com.sun.jna.Structure
import com.sun.jna.win32.StdCallLibrary

/**
 * Windows: kernel32 の GetSystemPowerStatus を JNA で読む。
 * JNA は Windows の上で初めて読む時に載せる。載らない・呼べない時は不明を返す。
 */
object WindowsPowerReader : DevicePowerReader {
    private val isWindows = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    private val kernel32: Kernel32? by lazy {
        if (!isWindows) null else runCatching { Native.load("kernel32", Kernel32::class.java) }.getOrNull()
    }

    override fun read(): DevicePowerState {
        val k = kernel32 ?: return DevicePowerState.Unknown
        return runCatching {
            val s = SystemPowerStatus()
            if (!k.GetSystemPowerStatus(s)) return DevicePowerState.Unknown
            map(
                acLineStatus = s.ACLineStatus.toUByteInt(),
                batteryFlag = s.BatteryFlag.toUByteInt(),
                batteryLifePercent = s.BatteryLifePercent.toUByteInt(),
                systemStatusFlag = s.SystemStatusFlag.toUByteInt(),
            )
        }.getOrDefault(DevicePowerState.Unknown)
    }

    private const val UNKNOWN = 255
    private const val AC_OFFLINE = 0
    private const val AC_ONLINE = 1
    private const val FLAG_CHARGING = 8
    private const val FLAG_NO_BATTERY = 128
    private const val SAVER_ON = 1
    private const val FULL_LEVEL = 0.99

    /**
     * SYSTEM_POWER_STATUS の値(0〜255)を状態にする。
     * - BatteryFlag に 128(電池が無い)が立てば残量・充電は不明。255 は「分からない」で、電池が無いとはみなさない
     * - BatteryLifePercent が 255 なら残量は不明
     * - ACLineStatus が 0 なら電池駆動、1 なら 99% 以上で満充電・未満で充電中。255 なら BatteryFlag の充電中(8)だけ見る
     * - SystemStatusFlag が 1 なら省電力(バッテリー節約機能)
     */
    internal fun map(acLineStatus: Int, batteryFlag: Int, batteryLifePercent: Int, systemStatusFlag: Int): DevicePowerState {
        val saver = systemStatusFlag == SAVER_ON
        val flagKnown = batteryFlag != UNKNOWN
        if (flagKnown && batteryFlag and FLAG_NO_BATTERY != 0) return DevicePowerState.Unknown.copy(isPowerSaver = saver)
        val level = batteryLifePercent.takeIf { it != UNKNOWN }?.let { it / 100.0 }
        val charge = when (acLineStatus) {
            AC_OFFLINE -> DevicePowerState.Charge.UNPLUGGED
            AC_ONLINE -> if (level != null && level >= FULL_LEVEL) DevicePowerState.Charge.FULL else DevicePowerState.Charge.CHARGING
            else -> if (flagKnown && batteryFlag and FLAG_CHARGING != 0) DevicePowerState.Charge.CHARGING else DevicePowerState.Charge.UNKNOWN
        }
        return DevicePowerState.of(level, charge, saver)
    }

    private fun Byte.toUByteInt(): Int = toInt() and 0xFF

    @Suppress("FunctionName")
    internal interface Kernel32 : StdCallLibrary {
        fun GetSystemPowerStatus(status: SystemPowerStatus): Boolean
    }

    /** winbase.h の SYSTEM_POWER_STATUS。JNA が反射で触るので public。 */
    @Structure.FieldOrder("ACLineStatus", "BatteryFlag", "BatteryLifePercent", "SystemStatusFlag", "BatteryLifeTime", "BatteryFullLifeTime")
    class SystemPowerStatus : Structure() {
        @JvmField var ACLineStatus: Byte = 0
        @JvmField var BatteryFlag: Byte = 0
        @JvmField var BatteryLifePercent: Byte = 0
        @JvmField var SystemStatusFlag: Byte = 0
        @JvmField var BatteryLifeTime: Int = 0
        @JvmField var BatteryFullLifeTime: Int = 0
    }
}
