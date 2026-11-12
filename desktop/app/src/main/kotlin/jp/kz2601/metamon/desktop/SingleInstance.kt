package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.AppDataDir
import jp.kz2601.metamon.desktop.store.writeAtomically
import java.awt.GraphicsEnvironment
import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.File
import java.io.IOException
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket
import java.nio.channels.FileChannel
import java.nio.channels.FileLock
import java.nio.channels.OverlappingFileLockException
import java.nio.file.StandardOpenOption
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.HexFormat
import javax.swing.JOptionPane
import kotlin.concurrent.thread

/**
 * 同じ保存先(AppDataDir)で 2 つ目を起動しない(同じ JSON を 2 つのプロセスが書き合わない)。
 * `<保存先>/.lock` を開いている間ロックする。ロックはプロセスが終われば OS が外す(落ちても残らない)。
 * `-Dtailorit.dataDir` で保存先を分ければ同時に起動できる。
 *
 * 2 つ目の起動は起動中の方へ引き継いで終わる(トレイに隠れていても前に出る)。
 * 起動中の方は 127.0.0.1 の空いた口で待ち、口の番号と合言葉を `<保存先>/.instance` に書く。
 * 2 つ目はそれを読んで合言葉と引数を送る。合言葉が合った時だけ受け取る(同じ人の他のプロセス以外からは開けない)。
 */
internal object SingleInstance {
    const val LOCK_FILE = ".lock"
    const val INSTANCE_FILE = ".instance"

    private const val CONNECT_TIMEOUT_MS = 1_000
    private const val READ_TIMEOUT_MS = 2_000
    private const val MAX_ARGS = 8
    private const val MAX_ARG_CHARS = 1_024
    private const val REPLY_OK = "ok"

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

    @Volatile
    private var server: Server? = null

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

    /** 起動時に 1 度。取れたら終わるまで持ち、引き継ぎを待ち始める。false なら他で起動中。 */
    fun claim(dir: File = AppDataDir.resolve()): Boolean {
        val held = acquire(dir) ?: return false
        current = held
        server = runCatching { Server.start(dir) }
            .onFailure { System.err.println("[TailorIt] 2 つ目の起動を受ける口を開けませんでした: $it") }
            .getOrNull()
        return true
    }

    /** 2 つ目の起動から届いた引数を受ける(画面のスレッドに移すのは受け手)。設定より前に届いた分は設定した時に渡す。 */
    fun onHandoff(handler: (List<String>) -> Unit) {
        server?.setHandler(handler)
    }

    /** 起動中の方へ引数を渡す。渡せたら true(起動中の方が合言葉を確かめて受け取った)。 */
    fun handOff(dir: File = AppDataDir.resolve(), args: List<String>): Boolean = runCatching {
        val info = File(dir, INSTANCE_FILE).readText().lines()
        val port = info.getOrNull(0)?.trim()?.toIntOrNull() ?: return false
        val token = info.getOrNull(1)?.trim()?.takeIf { it.isNotEmpty() } ?: return false
        Socket().use { s ->
            s.connect(InetSocketAddress(InetAddress.getLoopbackAddress(), port), CONNECT_TIMEOUT_MS)
            s.soTimeout = READ_TIMEOUT_MS
            val out = DataOutputStream(s.getOutputStream())
            out.writeUTF(token)
            val sent = args.take(MAX_ARGS).map { it.take(MAX_ARG_CHARS) }
            out.writeInt(sent.size)
            sent.forEach(out::writeUTF)
            out.flush()
            DataInputStream(s.getInputStream()).readUTF() == REPLY_OK
        }
    }.getOrElse {
        System.err.println("[TailorIt] 起動中の方へ引き継げませんでした: $it")
        false
    }

    /** 他で起動中の一言(端末と、画面があればダイアログ)。引き継げなかった時だけ。 */
    fun reportRunning() {
        val msg = Strings["desktop_already_running"]
        System.err.println("[TailorIt] $msg")
        if (!GraphicsEnvironment.isHeadless()) {
            runCatching { JOptionPane.showMessageDialog(null, msg, "Tailor It", JOptionPane.INFORMATION_MESSAGE) }
        }
    }

    /** 127.0.0.1 の空いた口で 2 つ目の起動を待つ。 */
    internal class Server private constructor(private val socket: ServerSocket, private val token: String, val file: File) : AutoCloseable {
        private val pending = mutableListOf<List<String>>()
        private var handler: ((List<String>) -> Unit)? = null

        val port: Int get() = socket.localPort

        fun setHandler(h: (List<String>) -> Unit) {
            val queued = synchronized(this) {
                handler = h
                pending.toList().also { pending.clear() }
            }
            queued.forEach(h)
        }

        private fun deliver(args: List<String>) {
            val h = synchronized(this) { handler ?: run { pending += args; null } } ?: return
            h(args)
        }

        private fun loop() {
            while (!socket.isClosed) {
                val client = try {
                    socket.accept()
                } catch (_: IOException) {
                    return
                }
                runCatching { client.use(::serve) }
            }
        }

        private fun serve(client: Socket) {
            client.soTimeout = READ_TIMEOUT_MS
            val input = DataInputStream(client.getInputStream())
            val got = input.readUTF()
            if (!MessageDigest.isEqual(got.toByteArray(), token.toByteArray())) return
            val n = input.readInt()
            if (n !in 0..MAX_ARGS) return
            val args = List(n) { input.readUTF().take(MAX_ARG_CHARS) }
            deliver(args)
            DataOutputStream(client.getOutputStream()).apply {
                writeUTF(REPLY_OK)
                flush()
            }
        }

        override fun close() {
            runCatching { socket.close() }
        }

        companion object {
            fun start(dir: File): Server {
                val socket = ServerSocket(0, 8, InetAddress.getLoopbackAddress())
                val token = HexFormat.of().formatHex(ByteArray(32).also(SecureRandom()::nextBytes))
                val file = File(dir, INSTANCE_FILE)
                try {
                    writeAtomically(file, "${socket.localPort}\n$token\n")
                } catch (e: Exception) {
                    socket.close()
                    throw e
                }
                return Server(socket, token, file).also { s ->
                    thread(isDaemon = true, name = "single-instance") { s.loop() }
                }
            }
        }
    }
}
