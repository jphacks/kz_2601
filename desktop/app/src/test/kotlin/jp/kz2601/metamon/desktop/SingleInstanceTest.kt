package jp.kz2601.metamon.desktop

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.File
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit

/** 2 つ目の起動から引数を受け渡す(SingleInstance の deliver・serve)。 */
class SingleInstanceTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun instanceLines(dir: File): List<String> = File(dir, SingleInstance.INSTANCE_FILE).readLines().map { it.trim() }

    /** 合言葉は合っているが中身が壊れた便り。受け取ったら答え(ok)を返す。 */
    private fun sendRaw(dir: File, count: Int, args: List<String> = emptyList()): String? {
        val (port, token) = instanceLines(dir)
        return Socket().use { s ->
            s.connect(InetSocketAddress(InetAddress.getLoopbackAddress(), port.toInt()), 1_000)
            s.soTimeout = 2_000
            val out = DataOutputStream(s.getOutputStream())
            out.writeUTF(token)
            out.writeInt(count)
            args.forEach(out::writeUTF)
            out.flush()
            runCatching { DataInputStream(s.getInputStream()).readUTF() }.getOrNull()
        }
    }

    @Test
    fun claimMakesTheFirstStartTheOwnerAndTakesArgsFromTheSecond() {
        val dir = tmp.newFolder("claim")
        assertTrue("1 つ目は起動できる", SingleInstance.claim(dir))
        assertTrue(File(dir, SingleInstance.LOCK_FILE).isFile)
        val (port, token) = instanceLines(dir)
        assertTrue("待っている口の番号を書く", port.toInt() > 0)
        assertEquals("合言葉は 32 バイトの 16 進", 64, token.length)

        val got = LinkedBlockingQueue<List<String>>()
        SingleInstance.onHandoff { got += it }
        assertTrue(SingleInstance.handOff(dir, listOf("kakeibo", "home")))
        assertEquals(listOf("kakeibo", "home"), got.poll(5, TimeUnit.SECONDS))

        assertFalse("同じ保存先の 2 つ目は起動しない", SingleInstance.claim(dir))
    }

    @Test
    fun argsThatArriveBeforeTheHandlerAreKeptInOrder() {
        val dir = tmp.newFolder("pending")
        SingleInstance.Server.start(dir).use { server ->
            assertEquals(instanceLines(dir)[0].toInt(), server.port)
            assertTrue(SingleInstance.handOff(dir, listOf("kakeibo")))
            assertTrue(SingleInstance.handOff(dir, listOf("shopping")))
            // handOff が true の時点で 2 件とも届いている。受け手を決めた時に届いた順で渡る
            val got = LinkedBlockingQueue<List<String>>()
            server.setHandler { got += it }
            assertEquals(listOf("kakeibo"), got.poll(5, TimeUnit.SECONDS))
            assertEquals(listOf("shopping"), got.poll(5, TimeUnit.SECONDS))
        }
    }

    @Test
    fun atMostEightArgsGoThroughAndLongOnesAreCut() {
        val dir = tmp.newFolder("limits")
        SingleInstance.Server.start(dir).use { server ->
            val got = LinkedBlockingQueue<List<String>>()
            server.setHandler { got += it }
            val long = "a".repeat(2_000)
            assertTrue(SingleInstance.handOff(dir, listOf(long) + (1..9).map { "app$it" }))
            val args = got.poll(5, TimeUnit.SECONDS)!!
            assertEquals(8, args.size)
            assertEquals(1_024, args.first().length)
            assertEquals(listOf("app1", "app2", "app3", "app4", "app5", "app6", "app7"), args.drop(1))
        }
    }

    @Test
    fun brokenArgCountsAreDroppedAndTheNextHandOffStillArrives() {
        val dir = tmp.newFolder("broken")
        SingleInstance.Server.start(dir).use { server ->
            val got = LinkedBlockingQueue<List<String>>()
            server.setHandler { got += it }
            assertNull("多すぎる数は受け取らない", sendRaw(dir, 9))
            assertNull("負の数も受け取らない", sendRaw(dir, -1))
            assertNull(got.poll(300, TimeUnit.MILLISECONDS))
            // 引数が無い便りは受け取る(トレイから前に出す依頼)
            assertEquals("ok", sendRaw(dir, 0))
            assertEquals(emptyList<String>(), got.poll(5, TimeUnit.SECONDS))
        }
    }

    @Test
    fun handOffWithoutAListenerIsAlreadyRunning() {
        val dir = tmp.newFolder("gone")
        val stale = SingleInstance.Server.start(dir).use { instanceLines(dir) }
        // 口が閉じた後も .instance は残る。引き継げないので「もう起動しています」に戻る
        assertEquals(stale, instanceLines(dir))
        assertFalse(SingleInstance.handOff(dir, listOf("kakeibo")))
        assertFalse(".instance が無い時も落ちない", SingleInstance.handOff(tmp.newFolder("empty"), emptyList()))
    }
}
