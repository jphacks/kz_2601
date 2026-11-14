package jp.kz2601.metamon.language

import jp.kz2601.metamon.services.http.ClientInfo
import java.util.Locale

/**
 * アプリ内の UI の言語の設定(iOS の `AppLanguage`・#843)。[SYSTEM] は端末の言語(ja / en のうち近い方)に従う。
 * 日本語が既定のリソース(values)、英語は values-en。
 */
enum class AppLanguage(
    /** BCP 47 のタグ。[SYSTEM] は null */
    val tag: String?,
    /** 選ぶ画面に出す名前。訳さない(その言語の読み手が見つけられるように・iOS と同じ) */
    val nativeName: String?,
) {
    SYSTEM(null, null),
    JA("ja", "日本語"),
    EN("en", "English"),
    ;

    /** 実際に使う言語コード("ja" | "en")。[system] は端末の好みの言語(上から順)。 */
    fun resolve(system: List<Locale>): String = tag ?: ClientInfo.languageCode(system)

    companion object {
        /** 保存した値・LocaleManager の言語から戻す。対応外(ja / en 以外)は [SYSTEM]。 */
        fun fromTag(tag: String?): AppLanguage {
            val language = tag?.let { Locale.forLanguageTag(it).language }
            return entries.firstOrNull { it.tag != null && it.tag == language } ?: SYSTEM
        }
    }
}
