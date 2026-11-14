package jp.kz2601.metamon.whisper

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okio.Buffer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.ByteArrayInputStream
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.security.MessageDigest

class WhisperModelManagerTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val body = ByteArray(300_000) { (it % 251).toByte() }
    private val spec = WhisperModelSpec("model.bin", "https://example.invalid/model.bin", sha256(body), body.size.toLong())
    private lateinit var dir: File
    private val plenty: (File) -> Long = { Long.MAX_VALUE }

    @Before
    fun setUp() {
        dir = File(tmp.root, "models")
    }

    private fun manager(spec: WhisperModelSpec = this.spec, free: (File) -> Long = plenty, source: ModelSource) =
        WhisperModelManager(dir, spec, source, free, Dispatchers.Unconfined)

    private fun serving(bytes: ByteArray = body, length: Long? = bytes.size.toLong()) =
        ModelSource { ModelStream(length, ByteArrayInputStream(bytes)) }

    private suspend fun failureOf(m: WhisperModelManager): ModelFailure = try {
        m.ensure()
        fail("失敗するはず")
        error("unreachable")
    } catch (e: ModelException) {
        e.failure
    }

    @Test
    fun downloadsVerifiesAndBecomesReady() = runTest {
        lateinit var m: WhisperModelManager
        val during = mutableListOf<ModelState>()
        m = manager(source = {
            ModelStream(body.size.toLong(), object : InputStream() {
                val inner = ByteArrayInputStream(body)
                override fun read(): Int = inner.read()
                override fun read(b: ByteArray, off: Int, len: Int): Int {
                    during += m.state.value
                    return inner.read(b, off, len)
                }
            })
        })
        assertEquals(ModelState.Absent, m.state.value)
        assertTrue(m.needsDownload)
        val file = m.ensure()
        assertEquals(File(dir, "model.bin"), file)
        assertTrue(body.contentEquals(file.readBytes()))
        assertEquals(ModelState.Ready(file), m.state.value)
        assertFalse(m.needsDownload)
        assertFalse("途中のファイルを残さない", File(dir, "model.bin.part").exists())
        assertTrue("落としている間は Downloading", during.isNotEmpty() && during.all { it is ModelState.Downloading })
        val fractions = during.map { (it as ModelState.Downloading).fraction ?: 0f }
        assertTrue("進み具合は増えるだけ", fractions.zipWithNext().all { (a, b) -> a <= b })
        assertTrue(fractions.last() > 0.5f)
    }

    @Test
    fun readyFileIsNotDownloadedAgain() = runTest {
        var opens = 0
        val m = manager { opens++; ModelStream(body.size.toLong(), ByteArrayInputStream(body)) }
        m.ensure()
        m.ensure()
        assertEquals(1, opens)
        // 次に起動した時も、そろったファイルがあれば落とさない
        val again = manager { error("落とさない") }
        assertEquals(ModelState.Ready(File(dir, "model.bin")), again.state.value)
    }

    @Test
    fun leftoverPartialIsDiscardedOnStart() {
        dir.mkdirs()
        File(dir, "model.bin.part").writeBytes(body)
        val m = manager(source = serving())
        assertEquals(ModelState.Absent, m.state.value)
        assertFalse(File(dir, "model.bin.part").exists())
    }

    @Test
    fun wrongChecksumIsRejectedAndNothingIsKept() = runTest {
        val tampered = body.copyOf().also { it[10] = (it[10] + 1).toByte() }
        val m = manager(source = serving(tampered))
        assertEquals(ModelFailure.Checksum, failureOf(m))
        assertEquals(ModelState.Failed(ModelFailure.Checksum), m.state.value)
        assertFalse(File(dir, "model.bin").exists())
        assertFalse(File(dir, "model.bin.part").exists())
    }

    @Test
    fun unexpectedContentLengthIsRejectedBeforeWriting() = runTest {
        val m = manager(source = serving(length = body.size + 1L))
        assertEquals(ModelFailure.Checksum, failureOf(m))
    }

    @Test
    fun tooLongBodyIsRejected() = runTest {
        val m = manager(source = serving(body + byteArrayOf(1), length = null))
        assertEquals(ModelFailure.Checksum, failureOf(m))
    }

    @Test
    fun shortBodyIsANetworkFailure() = runTest {
        val m = manager(source = serving(body.copyOf(1000), length = null))
        assertEquals(ModelFailure.Network, failureOf(m))
    }

    @Test
    fun networkFailureCanBeRetried() = runTest {
        var attempt = 0
        val m = manager {
            attempt++
            if (attempt == 1) throw IOException("offline")
            ModelStream(body.size.toLong(), ByteArrayInputStream(body))
        }
        assertEquals(ModelFailure.Network, failureOf(m))
        assertEquals(ModelState.Failed(ModelFailure.Network), m.state.value)
        m.ensure()
        assertTrue(m.state.value is ModelState.Ready)
    }

    @Test
    fun connectionDroppedMidwayIsANetworkFailure() = runTest {
        val m = manager {
            ModelStream(body.size.toLong(), object : InputStream() {
                var n = 0
                override fun read(): Int = if (n++ < 1000) 1 else throw IOException("reset")
                override fun read(b: ByteArray, off: Int, len: Int): Int {
                    if (n > 0) throw IOException("reset")
                    n++
                    return 100.also { b.fill(1, off, off + it) }
                }
            })
        }
        assertEquals(ModelFailure.Network, failureOf(m))
        assertFalse(File(dir, "model.bin.part").exists())
    }

    @Test
    fun notEnoughStorageStopsBeforeDownloading() = runTest {
        var opened = false
        val m = manager(source = { opened = true; error("落とさない") }, free = { 1_000L })
        val failure = failureOf(m)
        assertTrue(failure is ModelFailure.NotEnoughStorage)
        failure as ModelFailure.NotEnoughStorage
        assertEquals(body.size + WhisperModelManager.STORAGE_MARGIN_BYTES, failure.requiredBytes)
        assertEquals(1_000L, failure.availableBytes)
        assertFalse(opened)
    }

    @Test
    fun cancellingStopsAndReturnsToAbsent() = runTest {
        lateinit var job: Job
        val m = manager {
            ModelStream(body.size.toLong(), object : InputStream() {
                override fun read(): Int = 0
                override fun read(b: ByteArray, off: Int, len: Int): Int {
                    job.cancel()
                    return len
                }
            })
        }
        job = launch {
            try {
                m.ensure()
                fail("止まるはず")
            } catch (e: CancellationException) {
                throw e
            }
        }
        job.join()
        assertEquals(ModelState.Absent, m.state.value)
        assertFalse(File(dir, "model.bin.part").exists())
        assertFalse(File(dir, "model.bin").exists())
    }

    @Test
    fun concurrentCallersShareOneDownload() = runTest {
        var opens = 0
        val m = manager { opens++; ModelStream(body.size.toLong(), ByteArrayInputStream(body)) }
        val files = List(3) { async { m.ensure() } }.awaitAll()
        assertEquals(1, opens)
        assertEquals(1, files.toSet().size)
    }

    @Test
    fun deleteRemovesTheModel() = runTest {
        val m = manager(source = serving())
        val file = m.ensure()
        m.delete()
        assertFalse(file.exists())
        assertEquals(ModelState.Absent, m.state.value)
    }

    @Test
    fun okHttpSourceDownloadsAndReportsHttpErrors() = runTest {
        val server = MockWebServer()
        try {
            server.enqueue(MockResponse().setResponseCode(404))
            server.enqueue(MockResponse().setBody(Buffer().write(body)))
            server.start()
            val m = manager(spec.copy(url = server.url("/model.bin").toString()), source = OkHttpModelSource())
            assertEquals(ModelFailure.Network, failureOf(m))
            assertTrue(body.contentEquals(m.ensure().readBytes()))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun pinnedModelSpecIsAFullSha256() {
        val s = WhisperModels.DEFAULT
        assertTrue(s.sha256.matches(Regex("[0-9a-f]{64}")))
        assertTrue("取得先はコミットで固定する", Regex("/resolve/[0-9a-f]{40}/").containsMatchIn(s.url))
        assertTrue(s.url.endsWith(s.fileName))
        assertEquals(60, s.megabytes)
    }

    private fun sha256(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).toHex()
}
