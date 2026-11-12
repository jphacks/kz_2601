package jp.kz2601.metamon.desktop.power

import java.io.IOException
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import kotlin.io.path.isDirectory
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.name
import kotlin.io.path.readText

/**
 * Linux: `/sys/class/power_supply/<名前>/` を読む。`sysRoot` はテストでフィクスチャの道に替える。
 * - type=Battery で scope=Device でない物(マウス・ヘッドセットの電池を除く)を端末の電池とみなす
 * - 電池が複数(ThinkPad の BAT0 + BAT1)なら energy_full(無ければ charge_full)で重みを付けた平均。重みが無ければ単純な平均
 * - 省電力は `/sys/firmware/acpi/platform_profile` が low-power の時(power-profiles-daemon の「省電力」がこれを書く)
 */
class LinuxSysfsPowerReader(private val sysRoot: Path = Paths.get("/sys")) : DevicePowerReader {
    override fun read(): DevicePowerState {
        val supplies = sysRoot.resolve("class/power_supply")
        val entries = if (supplies.isDirectory()) supplies.listDirectoryEntries().sortedBy { it.name } else emptyList()
        val batteries = entries.mapNotNull(::readBattery)
        val mainsOnline = entries.filter { attr(it, "type") in MAINS_TYPES }.mapNotNull { attr(it, "online") }
        return combine(batteries, mainsOnline.map { it == "1" }, isPowerSaver())
    }

    internal data class Battery(val capacity: Int?, val status: String?, val fullWeight: Long?)

    private fun readBattery(dir: Path): Battery? {
        if (attr(dir, "type") != "Battery" || attr(dir, "scope") == "Device") return null
        if (attr(dir, "present") == "0") return null
        return Battery(
            capacity = attr(dir, "capacity")?.toIntOrNull(),
            status = attr(dir, "status"),
            fullWeight = (attr(dir, "energy_full") ?: attr(dir, "charge_full"))?.toLongOrNull()?.takeIf { it > 0 },
        )
    }

    private fun isPowerSaver(): Boolean = attr(sysRoot.resolve("firmware/acpi"), "platform_profile") == "low-power"

    internal companion object {
        private val MAINS_TYPES = setOf("Mains", "USB", "USB_C", "USB_PD")

        /** 1 行のファイルを読む。無い・読めない(権限)なら null。 */
        fun attr(dir: Path, name: String): String? = try {
            val f = dir.resolve(name)
            if (Files.isRegularFile(f)) f.readText().trim().ifEmpty { null } else null
        } catch (_: IOException) {
            null
        } catch (_: SecurityException) {
            null
        }

        fun combine(batteries: List<Battery>, mainsOnline: List<Boolean>, isPowerSaver: Boolean): DevicePowerState {
            if (batteries.isEmpty()) return DevicePowerState.Unknown.copy(isPowerSaver = isPowerSaver)
            val withCapacity = batteries.filter { it.capacity != null }
            val level = when {
                withCapacity.isEmpty() -> null
                withCapacity.all { it.fullWeight != null } -> {
                    val total = withCapacity.sumOf { it.fullWeight!! }.toDouble()
                    withCapacity.sumOf { it.capacity!! * it.fullWeight!!.toDouble() } / total / 100.0
                }
                else -> withCapacity.sumOf { it.capacity!! }.toDouble() / withCapacity.size / 100.0
            }
            return DevicePowerState.of(level, charge(batteries.map { it.status }, mainsOnline, level), isPowerSaver)
        }

        /**
         * 電池ごとの status をまとめる。どれかが充電中なら充電中。全部が満充電なら満充電。
         * "Not charging" は電源につながっているが止めている(しきい値で止める設定・満杯近く)ので満充電に寄せる。
         * status が読めない時だけ AC の online で決める。
         */
        fun charge(statuses: List<String?>, mainsOnline: List<Boolean>, level: Double?): DevicePowerState.Charge {
            val known = statuses.mapNotNull { s -> s?.let(::parseStatus) }
            return when {
                DevicePowerState.Charge.CHARGING in known -> DevicePowerState.Charge.CHARGING
                DevicePowerState.Charge.UNPLUGGED in known -> DevicePowerState.Charge.UNPLUGGED
                // 残りは満充電だけ(Unknown の電池が混ざっていても、読めた物が全部満杯なら満杯)
                known.isNotEmpty() -> DevicePowerState.Charge.FULL
                mainsOnline.any { it } -> if (level != null && level >= 1.0) DevicePowerState.Charge.FULL else DevicePowerState.Charge.CHARGING
                mainsOnline.isNotEmpty() -> DevicePowerState.Charge.UNPLUGGED
                else -> DevicePowerState.Charge.UNKNOWN
            }
        }

        /** カーネルの status の文字(Unknown・Charging・Discharging・Not charging・Full)。Unknown は null。 */
        fun parseStatus(s: String): DevicePowerState.Charge? = when (s) {
            "Charging" -> DevicePowerState.Charge.CHARGING
            "Discharging" -> DevicePowerState.Charge.UNPLUGGED
            "Full", "Not charging" -> DevicePowerState.Charge.FULL
            else -> null
        }
    }
}
