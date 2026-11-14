package jp.kz2601.metamon.desktop.store

import java.io.File

/**
 * 端末の保存先(#1754)。
 * - Windows: `%APPDATA%\TailorIt`
 * - Linux: `$XDG_DATA_HOME/tailorit`(無ければ `~/.local/share/tailorit`。Flatpak では XDG_DATA_HOME がアプリ専用の場所を指す)
 * - macOS: `~/Library/Application Support/TailorIt`
 * `-Dtailorit.dataDir=<path>` で差し替える(テスト・複数の人を試す時)。
 */
object AppDataDir {
    const val OVERRIDE_PROPERTY = "tailorit.dataDir"

    fun resolve(
        osName: String = System.getProperty("os.name").orEmpty(),
        env: Map<String, String> = System.getenv(),
        home: String = System.getProperty("user.home").orEmpty(),
        override: String? = System.getProperty(OVERRIDE_PROPERTY),
    ): File {
        override?.takeIf { it.isNotBlank() }?.let { return File(it) }
        val os = osName.lowercase()
        return when {
            os.startsWith("windows") -> {
                val base = env["APPDATA"]?.takeIf { it.isNotBlank() } ?: File(home, "AppData/Roaming").path
                File(base, "TailorIt")
            }
            os.startsWith("mac") || os.startsWith("darwin") -> File(home, "Library/Application Support/TailorIt")
            else -> {
                // XDG は絶対パスの時だけ使う(仕様どおり、相対は無視する)
                val base = env["XDG_DATA_HOME"]?.takeIf { it.startsWith("/") } ?: File(home, ".local/share").path
                File(base, "tailorit")
            }
        }
    }
}
