package jp.kz2601.metamon.whisper.desktop

import java.io.File

/**
 * whisper.cpp の JNI のライブラリ(metamon_whisper)を探して読む。上から順に、有る物を 1 つ。
 * 1. `-Dtailorit.whisper.lib=<ファイル>` か `TAILORIT_WHISPER_LIB`(作ったライブラリを直接指す)
 * 2. 配った形の中(Compose が渡す `compose.application.resources.dir`。native/out/<os>-<arch>/ がそこに入る)
 * 3. `-Dtailorit.whisper.nativeDir=<native/out>` の <os>-<arch>/(手元の :desktop:run・:whisper:transcribeWav)
 * 4. java.library.path
 */
object NativeLibrary {
    const val NAME = "metamon_whisper"

    /** 試すファイル(有る物だけ・上の順)。 */
    fun candidates(
        props: (String) -> String? = System::getProperty,
        env: (String) -> String? = System::getenv,
        target: String = target(),
        fileName: String = System.mapLibraryName(NAME),
    ): List<File> = buildList {
        (props("tailorit.whisper.lib") ?: env("TAILORIT_WHISPER_LIB"))?.takeIf { it.isNotBlank() }?.let { add(File(it)) }
        props("compose.application.resources.dir")?.takeIf { it.isNotBlank() }?.let { add(File(it, fileName)) }
        props("tailorit.whisper.nativeDir")?.takeIf { it.isNotBlank() }?.let { add(File(File(it, target), fileName)) }
    }.filter { it.isFile }

    /** Compose の appResourcesRootDir と同じ名前(windows-x64・linux-x64・macos-arm64 など)。 */
    fun target(osName: String = System.getProperty("os.name").orEmpty(), arch: String = System.getProperty("os.arch").orEmpty()): String {
        val os = osName.lowercase().let {
            when {
                it.startsWith("windows") -> "windows"
                it.startsWith("mac") || it.startsWith("darwin") -> "macos"
                else -> "linux"
            }
        }
        val cpu = if (arch.lowercase() in setOf("aarch64", "arm64")) "arm64" else "x64"
        return "$os-$cpu"
    }

    /** 読めたら true。読めなければ false(理由は [lastError])。 */
    fun load(): Boolean {
        for (f in candidates()) {
            try {
                System.load(f.absolutePath)
                return true
            } catch (e: UnsatisfiedLinkError) {
                lastError = "${f.absolutePath}: ${e.message}"
            }
        }
        return try {
            System.loadLibrary(NAME)
            true
        } catch (e: UnsatisfiedLinkError) {
            lastError = lastError ?: e.message
            false
        } catch (e: SecurityException) {
            lastError = e.message
            false
        }
    }

    @Volatile
    var lastError: String? = null
        private set
}
