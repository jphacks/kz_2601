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
 * - Battery 以外の type(Mains・USB・Wireless など)は全部充電器とみなし、online を見る
 * - 残量は capacity。無ければ energy_now/energy_full、それも無ければ charge_now/charge_full
 * - 電池が複数(ThinkPad の BAT0 + BAT1)なら、全部に energy_full があればそれで、全部に charge_full があればそれで重みを付けた平均。どちらでもなければ単純な平均
 * - 省電力は `/sys/firmware/acpi/platform_profile` が low-power・quiet・cool の時(power-profiles-daemon の「省電力」がこれを書く)
 */
class LinuxSysfsPowerReader(private val sysRoot: Path = Paths.get("/sys")) : DevicePowerReader {
    override fun read(): DevicePowerState {
        val supplies = sysRoot.resolve("class/power_supply")
        val entries = if (supplies.isDirectory()) supplies.listDirectoryEntries().sortedBy { it.name } else emptyList()
        val batteries = entries.mapNotNull(::readBattery)
        val mainsOnline = entries
            .filter { attr(it, "type").let { t -> t != null && t != "Battery" } }
            .mapNotNull { attr(it, "online") }
        return combine(batteries, mainsOnline.map { it == "1" }, isPowerSaver())
    }

    /** [capacity] は 0〜100(%)。[energyFull]・[chargeFull] は重み付けに使う(0 以下は無い扱い)。 */
    internal data class Battery(
        val capacity: Double?,
        val status: String?,
        val energyFull: Long? = null,
        val chargeFull: Long? = null,
    )

    private fun readBattery(dir: Path): Battery? {
        if (attr(dir, "type") != "Battery" || attr(dir, "scope") == "Device") return null
        if (attr(dir, "present") == "0") return null
        val energyFull = longAttr(dir, "energy_full")
        val chargeFull = longAttr(dir, "charge_full")
        val capacity = attr(dir, "capacity")?.toDoubleOrNull()
            ?: ratio(longAttr(dir, "energy_now"), energyFull)
            ?: ratio(longAttr(dir, "charge_now"), chargeFull)
        return Battery(capacity = capacity, status = attr(dir, "status"), energyFull = energyFull, chargeFull = chargeFull)
    }

    // TODO: platform_profile が無い機種は power-profiles-daemon の D-Bus(net.hadess.PowerProfiles の ActiveProfile == "power-saver")も見る
    private fun isPowerSaver(): Boolean = attr(sysRoot.resolve("firmware/acpi"), "platform_profile") in POWER_SAVER_PROFILES

    internal companion object {
        private val POWER_SAVER_PROFILES = setOf("low-power", "quiet", "cool")

        /** 「つないだまま止めている」とみなす残量の下限。これ以上なら満充電。 */
        private const val FULL_LEVEL = 0.99

        /** 1 行のファイルを読む。無い・読めない(権限)なら null。 */
        fun attr(dir: Path, name: String): String? = try {
            val f = dir.resolve(name)
            if (Files.isRegularFile(f)) f.readText().trim().ifEmpty { null } else null
        } catch (_: IOException) {
            null
        } catch (_: SecurityException) {
            null
        }

        fun longAttr(dir: Path, name: String): Long? = attr(dir, name)?.toLongOrNull()?.takeIf { it > 0 }

        private fun ratio(now: Long?, full: Long?): Double? =
            if (now != null && full != null) now.toDouble() / full * 100.0 else null

        fun combine(batteries: List<Battery>, mainsOnline: List<Boolean>, isPowerSaver: Boolean): DevicePowerState {
            if (batteries.isEmpty()) return DevicePowerState.Unknown.copy(isPowerSaver = isPowerSaver)
            val withCapacity = batteries.filter { it.capacity != null }
            val level = when {
                withCapacity.isEmpty() -> null
                withCapacity.all { it.energyFull != null } -> weighted(withCapacity) { it.energyFull!! }
                withCapacity.all { it.chargeFull != null } -> weighted(withCapacity) { it.chargeFull!! }
                else -> withCapacity.sumOf { it.capacity!! } / withCapacity.size / 100.0
            }
            return DevicePowerState.of(level, charge(batteries.map { it.status }, mainsOnline, level), isPowerSaver)
        }

        private fun weighted(batteries: List<Battery>, weight: (Battery) -> Long): Double {
            val total = batteries.sumOf { weight(it) }.toDouble()
            return batteries.sumOf { it.capacity!! * weight(it) } / total / 100.0
        }

        /**
         * 電池ごとの status をまとめる。どれかが充電中なら充電中、どれかが放電中なら電池駆動。
         * 充電器があって全部 offline なら電池駆動(Full・Not charging でも抜いてある)。
         * "Not charging" はつないだまま止めている(しきい値で止める設定など)ので、残量が 99% 以上なら満充電、未満なら充電中。
         * status が読めない時だけ充電器の online で決める。
         */
        fun charge(statuses: List<String?>, mainsOnline: List<Boolean>, level: Double?): DevicePowerState.Charge {
            val known = statuses.mapNotNull { it?.takeIf { s -> s in KNOWN_STATUSES } }
            val pluggedByLevel = if (level != null && level >= FULL_LEVEL) DevicePowerState.Charge.FULL else DevicePowerState.Charge.CHARGING
            return when {
                "Charging" in known -> DevicePowerState.Charge.CHARGING
                "Discharging" in known -> DevicePowerState.Charge.UNPLUGGED
                mainsOnline.isNotEmpty() && mainsOnline.none { it } -> DevicePowerState.Charge.UNPLUGGED
                // 残りは Full と Not charging だけ(Unknown の電池が混ざっていても読めた物で決める)
                "Not charging" in known -> pluggedByLevel
                known.isNotEmpty() -> DevicePowerState.Charge.FULL
                mainsOnline.any { it } -> pluggedByLevel
                else -> DevicePowerState.Charge.UNKNOWN
            }
        }

        /** カーネルの status の文字のうち意味のある物。Unknown などは捨てる。 */
        private val KNOWN_STATUSES = setOf("Charging", "Discharging", "Full", "Not charging")
    }
}
