package jp.kz2601.metamon.desktop.io

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.setValue
import java.io.File
import java.net.URI
import java.util.Locale

/**
 * ファイルマネージャーの「別のアプリで開く」から渡されたファイル(#2994)。iOS の「共有で受け取る」に当たる。
 * 起動の引数(.desktop の `Exec=tailor-it %F`)に並ぶ。`file://` の URI でも、パスのままでも受ける。
 * 取り込みの規則・大きさの上限はドラッグ & ドロップと同じ(振り分けは [DesktopIo.dropFiles])。
 */
object OpenWithFiles {
    /** 受ける拡張子。xlsx は読み込みの実装ができるまで入れない(入れると「開く」に出て取り込めない)。 */
    val EXTENSIONS: List<String> = listOf("csv", "tsv")

    /** .desktop の `MimeType=`。 */
    val MIME_TYPES: List<String> = listOf("text/csv", "text/tab-separated-values")

    /** 1 度に受ける数(引き継ぎの引数の上限と揃える)。 */
    const val MAX_FILES = 8

    fun isSupported(name: String): Boolean = name.substringAfterLast('.', "").lowercase(Locale.ROOT) in EXTENSIONS

    /** `MimeType=text/csv;text/tab-separated-values;`(末尾の `;` を含む)。 */
    fun mimeTypeLine(): String = "MimeType=" + MIME_TYPES.joinToString("") { "$it;" }

    /** 引数 1 つをファイルに。URI でもパスでも。形が違う・無い・ディレクトリ・対応しない拡張子は null。 */
    fun fileOf(arg: String, exists: (File) -> Boolean = { it.isFile }): File? {
        val f = if (arg.startsWith("file:")) {
            runCatching { File(URI(arg)) }.getOrNull() ?: return null
        } else {
            File(arg)
        }
        return f.takeIf { isSupported(it.name) && exists(it) }
    }

    /** 引数のうちファイルとして受ける物(順番どおり・重複なし・[MAX_FILES] まで)。 */
    fun fromArgs(args: List<String>, exists: (File) -> Boolean = { it.isFile }): List<File> =
        args.mapNotNull { fileOf(it, exists) }.distinct().take(MAX_FILES)

    /** [fromArgs] で受ける物を除いた引数(アプリ名などの位置の引数として読む分)。 */
    fun withoutFiles(args: List<String>, exists: (File) -> Boolean = { it.isFile }): List<String> =
        args.filter { fileOf(it, exists) == null }
}

/**
 * 受けたファイルを画面へ渡す待ち行列。Main が [offer] し、前にあるウィンドウが [take] して dropFiles に流す。
 * [revision] が増えたら取りに行く(Compose の状態)。
 */
object OpenWithInbox {
    private val pending = mutableListOf<File>()

    var revision by mutableIntStateOf(0)
        private set

    @Synchronized
    fun offer(files: List<File>) {
        if (files.isEmpty()) return
        pending += files
        revision++
    }

    /** 待っている分を全部取る(無ければ空)。 */
    @Synchronized
    fun take(): List<File> = pending.toList().also { pending.clear() }
}
