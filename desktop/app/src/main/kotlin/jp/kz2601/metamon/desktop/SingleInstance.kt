package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.AppDataDir
import java.awt.GraphicsEnvironment
import java.io.File
import java.io.IOException
import java.nio.channels.FileChannel
import java.nio.channels.FileLock
import java.nio.channels.OverlappingFileLockException
import java.nio.file.StandardOpenOption
import javax.swing.JOptionPane

/**
 * 同じ保存先(AppDataDir)で 2 つ目を起動しない(同じ JSON を 2 つのプロセスが書き合わない)。
 * `<保存先>/.lock` を開いている間ロックする。ロックはプロセスが終われば OS が外す(落ちても残らない)。
 * `-Dtailorit.dataDir` で保存先を分ければ同時に起動できる。
 */
internal object SingleInstance {
    const val LOCK_FILE = ".lock"

    /** 持っている間はロックしたまま(終わるまで参照を持つ)。 */
    class Held internal constructor(private val channel: FileChannel?, private val lock: FileLock?) : AutoCloseable {
        override fun close() {
            runCatching { lock?.release() }
            runCatching { channel?.close() }
        }
    }

    /** 終わるまで持つ(GC で閉じない)。 */
    @Volatile
    private var current: Held? = null

    /**
     * ロックを取る。他のプロセス(または同じプロセスの別の取得)が持っていれば null。
     * 保存先に書けないなどロックそのものが使えない時は起動を止めない(空の [Held] を返す)。
     */
    fun acquire(dir: File = AppDataDir.resolve()): Held? {
        val channel = try {
            dir.mkdirs()
            FileChannel.open(File(dir, LOCK_FILE).toPath(), StandardOpenOption.CREATE, StandardOpenOption.WRITE)
        } catch (e: IOException) {
            System.err.println("[TailorIt] 起動のロックを作れませんでした: $e")
            return Held(null, null)
        }
        val lock = try {
            channel.tryLock()
        } catch (_: OverlappingFileLockException) {
            null
        } catch (e: IOException) {
            System.err.println("[TailorIt] 起動のロックを取れませんでした: $e")
            channel.close()
            return Held(null, null)
        }
        if (lock == null) {
            channel.close()
            return null
        }
        return Held(channel, lock)
    }

    /** 起動時に 1 度。取れたら終わるまで持つ。false なら他で起動中。 */
    fun claim(dir: File = AppDataDir.resolve()): Boolean {
        val held = acquire(dir) ?: return false
        current = held
        return true
    }

    /** 他で起動中の一言(端末と、画面があればダイアログ)。今は起動中の方を前に出す道が無いので知らせて終わる。 */
    fun reportRunning() {
        val msg = Strings["desktop_already_running"]
        System.err.println("[TailorIt] $msg")
        if (!GraphicsEnvironment.isHeadless()) {
            runCatching { JOptionPane.showMessageDialog(null, msg, "Tailor It", JOptionPane.INFORMATION_MESSAGE) }
        }
    }
}
