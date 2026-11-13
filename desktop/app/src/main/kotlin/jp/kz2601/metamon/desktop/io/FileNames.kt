package jp.kz2601.metamon.desktop.io

import java.io.File
import java.io.IOException

/** 書き出すファイルの名前(Windows の予約名・同じ名前の既存ファイル)。 */
object FileNames {
    /** Windows で使えない名前(最初の `.` の前)。CON・PRN・AUX・NUL・CONIN$・CONOUT$・COM/LPT + 0〜9・¹²³。 */
    private val RESERVED = Regex("(?i)(CON|PRN|AUX|NUL|CONIN\\$|CONOUT\\$|COM[0-9¹²³]|LPT[0-9¹²³])")

    fun isReserved(name: String): Boolean = RESERVED.matches(name.substringBefore('.').trimEnd())

    /** 予約名なら頭に `_`。 */
    fun guard(name: String): String = if (isReserved(name)) "_$name" else name

    /**
     * [dir] の中で空いている名前のファイルを作って返す(既存は上書きしない)。`a.csv` → `a (2).csv` → `a (3).csv`…
     * 作るのは [File.createNewFile] で 1 度に(同時に書き出しても同じ名前を取り合わない)。[dir] は作る。
     */
    fun createFree(dir: File, name: String): File {
        dir.mkdirs()
        val safe = guard(name)
        val dot = safe.lastIndexOf('.').takeIf { it > 0 } ?: safe.length
        val stem = safe.substring(0, dot)
        val ext = safe.substring(dot)
        for (n in 1..MAX_TRIES) {
            val f = File(dir, if (n == 1) safe else "$stem ($n)$ext")
            if (f.createNewFile()) return f
        }
        throw IOException("空いている名前がありません: $safe")
    }

    private const val MAX_TRIES = 10_000
}
