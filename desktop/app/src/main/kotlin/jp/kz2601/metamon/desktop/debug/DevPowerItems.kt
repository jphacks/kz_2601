package jp.kz2601.metamon.desktop.debug

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.power.DevPowerOverride
import jp.kz2601.metamon.desktop.power.DevPowerPreset

/** 電池の状態を作り物にする(#3212)。「15%・省電力」「充電中」「不明」「元に戻す」。 */
object DevPowerItems {
    fun items(host: DevMenuHost): List<DevMenuItem> =
        DevPowerPreset.entries.map { preset ->
            val title = Strings.format("desktop_dev_power", label(preset))
            DevMenuItem("power:${preset.name.lowercase()}", title, listOf("battery", "power")) {
                DevPowerOverride.select(preset)
                host.message(title)
            }
        }

    fun label(preset: DevPowerPreset): String = when (preset) {
        DevPowerPreset.LOW_POWER_SAVER -> Strings["desktop_dev_power_low"]
        DevPowerPreset.CHARGING -> Strings["desktop_dev_power_charging"]
        DevPowerPreset.UNKNOWN -> Strings["desktop_dev_power_unknown"]
        DevPowerPreset.OFF -> Strings["desktop_dev_power_off"]
    }
}
