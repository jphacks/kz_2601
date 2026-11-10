package jp.kz2601.metamon.language

import android.app.LocaleManager
import android.content.Context
import android.content.res.Configuration
import android.content.res.Resources
import android.os.Build
import android.os.LocaleList
import androidx.core.content.edit
import java.util.Locale

/**
 * アプリごとの言語の読み書き(#843)。
 * - Android 13+: `LocaleManager`(端末の設定 > アプリ > 言語 と同じ値。切り替えるとシステムが画面を作り直す)
 * - Android 12 / 12L: 自前で保存し、各 Activity の [wrap] でリソースの言語を差し替える(切り替え後は画面を作り直す)
 * 端末の言語そのもの(システムの画面・権限のダイアログ)は変えない。
 */
object AppLocales {
    private const val PREFS = "metamon.language"
    private const val KEY = "appLanguage"

    private val usesLocaleManager: Boolean get() = Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU

    fun setting(context: Context): AppLanguage =
        if (usesLocaleManager) {
            val locales = localeManager(context).applicationLocales
            if (locales.isEmpty) AppLanguage.SYSTEM else AppLanguage.fromTag(locales[0].toLanguageTag())
        } else {
            AppLanguage.fromTag(prefs(context).getString(KEY, null))
        }

    /** 実際に使う言語("ja" | "en")。`Accept-Language`・生成の `locale`・音声認識の言語にこれを使う。 */
    fun languageCode(context: Context): String = setting(context).resolve(systemLocales(context))

    /**
     * 言語を変える。Android 13+ はシステムが開いている画面を作り直す。
     * 12 / 12L は戻り値が true の時、呼び出し側が画面を作り直す。
     */
    fun set(context: Context, language: AppLanguage): Boolean {
        if (usesLocaleManager) {
            localeManager(context).applicationLocales =
                language.tag?.let(LocaleList::forLanguageTags) ?: LocaleList.getEmptyLocaleList()
            return false
        }
        prefs(context).edit(commit = true) { putString(KEY, language.tag) }
        return true
    }

    /** Android 12 / 12L で Activity の言語を差し替える(`attachBaseContext` から呼ぶ)。13+ は何もしない。 */
    fun wrap(base: Context): Context {
        if (usesLocaleManager) return base
        val tag = setting(base).tag
        val locales = if (tag == null) systemLocaleList() else LocaleList.forLanguageTags(tag)
        // 書式(数値・日付)と Accept-Language の既定も揃える
        LocaleList.setDefault(locales)
        if (tag == null) return base
        val config = Configuration(base.resources.configuration).apply { setLocales(locales) }
        return base.createConfigurationContext(config)
    }

    /** 端末の好みの言語(アプリの設定を含まない)。 */
    private fun systemLocales(context: Context): List<Locale> {
        val list = if (usesLocaleManager) localeManager(context).systemLocales else systemLocaleList()
        return (0 until list.size()).map(list::get)
    }

    private fun systemLocaleList(): LocaleList = Resources.getSystem().configuration.locales

    private fun localeManager(context: Context): LocaleManager = context.getSystemService(LocaleManager::class.java)

    private fun prefs(context: Context) = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
}
