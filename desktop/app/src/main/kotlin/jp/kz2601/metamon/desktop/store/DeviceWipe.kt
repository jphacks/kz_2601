package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.ImageLoader
import jp.kz2601.metamon.desktop.Notifier
import jp.kz2601.metamon.desktop.auth.TokenStore
import jp.kz2601.metamon.desktop.io.ExportClipboard
import jp.kz2601.metamon.desktop.io.UserDirs
import java.awt.datatransfer.Clipboard
import java.io.File

/**
 * 「この端末のデータを消す」の後片付け(DesktopStore.wipe の後)。
 * - 保存先に残った `*.corrupt`(読めずに退けた物)・`*.tmp`(書きかけ)
 * - 貼った写真・引き出す一時ファイル・カレンダーに渡したファイルのフォルダの中身(フォルダは本人だけの 700 のまま残す)
 * - 予約した通知・覚えた画像
 * - クリップボードが書き出した表のままなら空に
 */
object DeviceWipe {
    class Result(val strayFiles: Int, val tempFiles: Int, val timers: Int, val clipboardCleared: Boolean)

    fun run(
        dataRoot: File?,
        tempDirs: List<File> = listOf(UserDirs.photos, UserDirs.drag, UserDirs.calendar),
        clipboard: Clipboard? = ExportClipboard.system(),
    ): Result {
        val stray = dataRoot?.let { runCatching { sweepStray(it) }.getOrDefault(0) } ?: 0
        val temp = tempDirs.sumOf { runCatching { emptyDir(it) }.getOrDefault(0) }
        val timers = runCatching { Notifier.Tray.cancelAll() }.getOrDefault(0)
        runCatching { ImageLoader.clear() }
        val cleared = runCatching { ExportClipboard.clearIfOurs(clipboard) }.getOrDefault(false)
        return Result(stray, temp, timers, cleared)
    }

    /** [root] の下の `*.corrupt`・`*.tmp` を消す(リンクはたどらない)。消した数。 */
    fun sweepStray(root: File): Int {
        if (!root.isDirectory) return 0
        return root.walkTopDown()
            .onEnter { !java.nio.file.Files.isSymbolicLink(it.toPath()) }
            .filter { it.isFile && (it.name.endsWith(".corrupt") || it.name.endsWith(".tmp")) }
            .toList()
            .count { it.delete() }
    }

    /** [dir] の中身を消す(フォルダそのものは残す)。消した数。 */
    fun emptyDir(dir: File): Int =
        dir.listFiles().orEmpty().count { if (it.isDirectory && !java.nio.file.Files.isSymbolicLink(it.toPath())) it.deleteRecursively() else it.delete() }
}

/** サインアウトの確認(#2375)で選んだ物。 */
enum class SignOutChoice { WIPE, KEEP, CANCEL }

/**
 * サインアウトの片付け(#2375)。
 * - [SignOutChoice.WIPE]: この端末のデータ(`AppHub.wipeLocalData`)を消してから、保存したトークンを消す
 * - [SignOutChoice.KEEP]: トークンだけ消す(記録・送信待ち・設定は残す)
 * - [SignOutChoice.CANCEL]: 何もしない
 * 戻り値はサインアウトしたか。データを消せなかった時はトークンも残す(サインインのまま・例外は呼んだ側へ)。
 */
object SignOutCleanup {
    suspend fun run(choice: SignOutChoice, wipeLocalData: suspend () -> Unit, tokens: TokenStore?): Boolean {
        when (choice) {
            SignOutChoice.CANCEL -> return false
            SignOutChoice.WIPE -> wipeLocalData()
            SignOutChoice.KEEP -> Unit
        }
        tokens?.clear()
        return true
    }
}
