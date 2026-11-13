package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.services.consent.KeyValueStore
import java.util.Locale

/** 表示言語の 3 択(Android の `AppLanguage` と同じ)。[SYSTEM] は OS に従う。 */
enum class LanguageChoice(val tag: String?) {
    SYSTEM(null),
    JA("ja"),
    EN("en"),
    ;

    companion object {
        /** 保存した値から戻す。読めない・対応外は [SYSTEM]。 */
        fun fromTag(tag: String?): LanguageChoice = entries.firstOrNull { it.tag != null && it.tag == tag?.trim()?.lowercase() } ?: SYSTEM
    }
}

/**
 * 表示言語の設定(#3016)。端末に保存し、効くのは次の起動から。
 * 決める順は 環境変数(`-Dtailorit.lang`・`TAILORIT_LANG`) > 設定 > OS。
 */
class LanguageSetting(private val store: KeyValueStore) {
    var choice: LanguageChoice
        get() = LanguageChoice.fromTag(store.getString(KEY))
        set(value) = if (value == LanguageChoice.SYSTEM) store.remove(KEY) else store.putString(KEY, value.tag!!)

    fun resolve(env: String?, system: Locale = Locale.getDefault(Locale.Category.DISPLAY)): Lang = resolve(env, choice, system)

    companion object {
        const val KEY = "tailorit.language"

        fun resolve(env: String?, choice: LanguageChoice, system: Locale = Locale.getDefault(Locale.Category.DISPLAY)): Lang =
            Lang.resolve(env?.trim()?.takeIf { it.isNotEmpty() } ?: choice.tag, system)
    }
}
