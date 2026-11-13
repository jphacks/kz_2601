package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.util.Locale

/** 言語を決める順(環境変数 > 設定 > OS)と保存(#3016)。 */
class LanguageSettingTest {
    @Test
    fun envBeatsSettingBeatsOs() {
        assertEquals(Lang.EN, LanguageSetting.resolve("en", LanguageChoice.JA, Locale.JAPAN))
        assertEquals(Lang.JA, LanguageSetting.resolve("ja_JP", LanguageChoice.EN, Locale.US))
        assertEquals(Lang.EN, LanguageSetting.resolve(null, LanguageChoice.EN, Locale.JAPAN))
        assertEquals(Lang.JA, LanguageSetting.resolve(null, LanguageChoice.JA, Locale.US))
        assertEquals(Lang.JA, LanguageSetting.resolve(null, LanguageChoice.SYSTEM, Locale.JAPAN))
        assertEquals(Lang.EN, LanguageSetting.resolve(null, LanguageChoice.SYSTEM, Locale.FRANCE))
    }

    @Test
    fun blankEnvFallsThroughToSetting() {
        assertEquals(Lang.EN, LanguageSetting.resolve("  ", LanguageChoice.EN, Locale.JAPAN))
        assertEquals(Lang.JA, LanguageSetting.resolve("", LanguageChoice.SYSTEM, Locale.JAPAN))
    }

    @Test
    fun savesAndReadsBack() {
        val store = InMemoryKeyValueStore()
        val setting = LanguageSetting(store)
        assertEquals(LanguageChoice.SYSTEM, setting.choice)
        setting.choice = LanguageChoice.EN
        assertEquals(LanguageChoice.EN, LanguageSetting(store).choice)
        assertEquals(Lang.EN, LanguageSetting(store).resolve(null, Locale.JAPAN))
        setting.choice = LanguageChoice.SYSTEM
        assertNull(store.getString(LanguageSetting.KEY))
        assertEquals(LanguageChoice.SYSTEM, LanguageSetting(store).choice)
    }

    @Test
    fun unknownSavedValueIsSystem() {
        val store = InMemoryKeyValueStore()
        store.putString(LanguageSetting.KEY, "fr")
        assertEquals(LanguageChoice.SYSTEM, LanguageSetting(store).choice)
        assertEquals(LanguageChoice.JA, LanguageChoice.fromTag(" JA "))
    }

    @Test
    fun restartNoteExistsInBothLanguages() {
        assertEquals("言語の変更は次の起動から反映されます", Strings.table.get(Lang.JA, "desktop_language_restart_note"))
        assertEquals("The language change takes effect the next time you start the app", Strings.table.get(Lang.EN, "desktop_language_restart_note"))
    }
}
