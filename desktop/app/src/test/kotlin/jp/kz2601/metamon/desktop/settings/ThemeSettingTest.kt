package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.theme.OsTheme
import jp.kz2601.metamon.desktop.theme.ThemeMode
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 明暗を決める順(環境変数 > 設定 > OS)と保存(#3018)。 */
class ThemeSettingTest {
    @Test
    fun envBeatsSetting() {
        assertEquals(ThemeMode.DARK, ThemeSetting.resolve("dark", ThemeMode.LIGHT))
        assertEquals(ThemeMode.LIGHT, ThemeSetting.resolve(" Light ", ThemeMode.DARK))
        assertEquals(ThemeMode.LIGHT, ThemeSetting.resolve("light", ThemeMode.SYSTEM))
    }

    @Test
    fun settingUsedWhenEnvIsAbsentOrNotFixed() {
        assertEquals(ThemeMode.DARK, ThemeSetting.resolve(null, ThemeMode.DARK))
        assertEquals(ThemeMode.LIGHT, ThemeSetting.resolve("", ThemeMode.LIGHT))
        assertEquals(ThemeMode.DARK, ThemeSetting.resolve("system", ThemeMode.DARK))
        assertEquals(ThemeMode.DARK, ThemeSetting.resolve("purple", ThemeMode.DARK))
        assertEquals(ThemeMode.SYSTEM, ThemeSetting.resolve(null, ThemeMode.SYSTEM))
    }

    @Test
    fun fullOrderEnvThenSettingThenOs() {
        fun dark(env: String?, setting: ThemeMode, os: Boolean?, fallback: Boolean) =
            OsTheme.isDark(ThemeSetting.resolve(env, setting), os, fallback)
        assertTrue(dark("dark", ThemeMode.LIGHT, os = false, fallback = false))
        assertFalse(dark(null, ThemeMode.LIGHT, os = true, fallback = true))
        assertTrue(dark(null, ThemeMode.DARK, os = false, fallback = false))
        assertTrue(dark(null, ThemeMode.SYSTEM, os = true, fallback = false))
        assertTrue(dark(null, ThemeMode.SYSTEM, os = null, fallback = true))
    }

    @Test
    fun savesAndReadsBack() {
        val store = InMemoryKeyValueStore()
        val setting = ThemeSetting(store)
        assertEquals(ThemeMode.SYSTEM, setting.choice)
        setting.choice = ThemeMode.DARK
        assertEquals(ThemeMode.DARK, ThemeSetting(store).choice)
        assertEquals(ThemeMode.DARK, ThemeSetting(store).resolve(null))
        setting.choice = ThemeMode.LIGHT
        assertEquals(ThemeMode.LIGHT, ThemeSetting(store).choice)
        setting.choice = ThemeMode.SYSTEM
        assertNull(store.getString(ThemeSetting.KEY))
        assertEquals(ThemeMode.SYSTEM, ThemeSetting(store).choice)
    }

    @Test
    fun unknownSavedValueIsSystem() {
        val store = InMemoryKeyValueStore()
        store.putString(ThemeSetting.KEY, "purple")
        assertEquals(ThemeMode.SYSTEM, ThemeSetting(store).choice)
    }
}
