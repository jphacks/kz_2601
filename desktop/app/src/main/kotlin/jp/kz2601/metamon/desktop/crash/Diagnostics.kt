package jp.kz2601.metamon.desktop.crash

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.desktop.voice.VoiceFlag

/**
 * 問い合わせ用の「診断の情報」(#2200)。版・OS・Java・設定の要約だけを 1 つの文にする。
 * uid・トークン・記録・ファイルのパス(ホームのディレクトリ・保存先)は入れない。項目は名前で選んで足す(環境変数・システムプロパティを丸ごと出さない)。
 * 行の名前はサポートが読むので英語のまま(言語に合わせて訳さない)。
 */
data class DiagnosticsInfo(
    val appVersion: String,
    val appBuild: String,
    val osName: String,
    val osVersion: String,
    val arch: String,
    val javaVersion: String,
    val javaVendor: String,
    val language: String,
    /** 明暗。読めなければ null。 */
    val dark: Boolean?,
    val voiceEnabled: Boolean,
    /** 音声入力のライブラリ(whisper)が載るか。調べなかった・読めなければ null。 */
    val voiceLibrary: Boolean?,
    val traySupported: Boolean?,
    /** 接続先の種類(mock / stg)。URL・トークンは入れない。 */
    val api: String,
    /** 送り待ちの件数。分からなければ null。 */
    val pending: Int?,
)

object Diagnostics {
    /** 1 行の値に入れる前に改行を潰し、長さを止める(OS の版の文字に変な物が来ても 1 行のまま)。 */
    private fun clean(v: String): String = v.replace(Regex("[\\r\\n\\t]+"), " ").trim().take(MAX_VALUE).ifEmpty { "-" }

    private const val MAX_VALUE = 80

    private fun flag(v: Boolean?): String = when (v) {
        true -> "on"
        false -> "off"
        null -> "unknown"
    }

    fun format(i: DiagnosticsInfo): String = listOf(
        "app" to "${clean(i.appVersion)} (${clean(i.appBuild)})",
        "os" to "${clean(i.osName)} ${clean(i.osVersion)}",
        "arch" to clean(i.arch),
        "java" to "${clean(i.javaVersion)} (${clean(i.javaVendor)})",
        "language" to clean(i.language),
        "theme" to when (i.dark) {
            true -> "dark"
            false -> "light"
            null -> "unknown"
        },
        "voice" to "${flag(i.voiceEnabled)} (library: ${when (i.voiceLibrary) { true -> "yes"; false -> "no"; null -> "unknown" }})",
        "tray" to when (i.traySupported) { true -> "yes"; false -> "no"; null -> "unknown" },
        "api" to clean(i.api),
        "pending" to (i.pending?.toString() ?: "unknown"),
    ).joinToString("\n", prefix = "TailorIt Desktop diagnostics\n") { (k, v) -> "$k: $v" }

    /**
     * 今の端末から集める。[props] から読むのは os.name・os.version・os.arch・java.version・java.vendor だけ。
     * [env] からは接続先の種類だけ([DesktopApiConfig.Mode])を読む。
     * [voiceLibrary] は音声入力が on の時だけ呼ぶ(off なら native のライブラリを載せに行かない)。
     */
    fun collect(
        props: (String) -> String? = System::getProperty,
        env: Map<String, String> = System.getenv(),
        dark: Boolean?,
        traySupported: Boolean?,
        pending: Int?,
        voiceEnabled: Boolean = VoiceFlag.enabled(),
        voiceLibrary: () -> Boolean? = { null },
        language: String = Strings.lang.name.lowercase(),
    ): DiagnosticsInfo = DiagnosticsInfo(
        appVersion = DesktopApi.VERSION,
        appBuild = DesktopApi.BUILD,
        osName = props("os.name").orEmpty(),
        osVersion = props("os.version").orEmpty(),
        arch = props("os.arch").orEmpty(),
        javaVersion = props("java.version").orEmpty(),
        javaVendor = props("java.vendor").orEmpty(),
        language = language,
        dark = dark,
        voiceEnabled = voiceEnabled,
        voiceLibrary = if (voiceEnabled) runCatching { voiceLibrary() }.getOrNull() else null,
        traySupported = traySupported,
        api = DesktopApiConfig.fromEnvironment(env).mode.name.lowercase(),
        pending = pending,
    )
}
