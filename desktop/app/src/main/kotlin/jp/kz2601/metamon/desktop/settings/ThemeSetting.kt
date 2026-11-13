package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.theme.ThemeMode
import jp.kz2601.metamon.services.consent.KeyValueStore

/**
 * 明暗の設定(#3018)。OS に従う / 明るい / 暗いの 3 択([ThemeMode])を端末に保存する。
 * 決める順は 環境変数(`-Dtailorit.theme`・`TAILORIT_THEME` の light・dark) > 設定 > OS。
 * 起動中の切り替えは、[choice] を替えたあと [resolve] をもう一度呼んで `TailorItTheme(mode)` に渡す。
 */
class ThemeSetting(private val store: KeyValueStore) {
    var choice: ThemeMode
        get() = when (store.getString(KEY)?.trim()?.lowercase()) {
            "light" -> ThemeMode.LIGHT
            "dark" -> ThemeMode.DARK
            else -> ThemeMode.SYSTEM
        }
        set(value) = when (value) {
            ThemeMode.SYSTEM -> store.remove(KEY)
            ThemeMode.LIGHT -> store.putString(KEY, "light")
            ThemeMode.DARK -> store.putString(KEY, "dark")
        }

    fun resolve(env: String? = envValue()): ThemeMode = resolve(env, choice)

    companion object {
        const val KEY = "tailorit.theme"

        /** 環境変数・システムプロパティの値(未設定は null)。 */
        fun envValue(): String? = System.getProperty("tailorit.theme") ?: System.getenv("TAILORIT_THEME")

        /** 環境で light・dark が固定されていればそれ、そうでなければ設定。 */
        fun resolve(env: String?, setting: ThemeMode): ThemeMode {
            val fixed = ThemeMode.parse(env)
            return if (fixed != ThemeMode.SYSTEM) fixed else setting
        }
    }
}
