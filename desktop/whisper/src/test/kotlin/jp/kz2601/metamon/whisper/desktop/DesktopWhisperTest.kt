package jp.kz2601.metamon.whisper.desktop

import jp.kz2601.metamon.whisper.StreamingResampler
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files
import javax.sound.sampled.AudioFormat

/** ライブラリの探し方と、Java Sound の PCM → whisper の形。 */
class DesktopWhisperTest {
    @Test
    fun targetMatchesComposeResourceDirNames() {
        assertEquals("windows-x64", NativeLibrary.target("Windows 11", "amd64"))
        assertEquals("windows-arm64", NativeLibrary.target("Windows 11", "aarch64"))
        assertEquals("linux-x64", NativeLibrary.target("Linux", "amd64"))
        assertEquals("macos-arm64", NativeLibrary.target("Mac OS X", "aarch64"))
    }

    @Test
    fun candidatesKeepOrderAndSkipMissingFiles() {
        val dir = Files.createTempDirectory("whisper-lib").toFile()
        val explicit = File(dir, "explicit.so").apply { writeText("") }
        val resources = File(dir, "res").apply { mkdirs() }
        File(resources, "libmetamon_whisper.so").writeText("")
        val dev = File(dir, "out/linux-x64").apply { mkdirs() }
        File(dev, "libmetamon_whisper.so").writeText("")
        val props = mapOf(
            "compose.application.resources.dir" to resources.path,
            "tailorit.whisper.nativeDir" to File(dir, "out").path,
        )
        val env = mapOf("TAILORIT_WHISPER_LIB" to explicit.path)
        val all = NativeLibrary.candidates(props::get, env::get, "linux-x64", "libmetamon_whisper.so")
        assertEquals(listOf(explicit, File(resources, "libmetamon_whisper.so"), File(dev, "libmetamon_whisper.so")), all)
        // 指したファイルが無ければ飛ばす
        val none = NativeLibrary.candidates({ null }, mapOf("TAILORIT_WHISPER_LIB" to File(dir, "missing.so").path)::get, "linux-x64", "libmetamon_whisper.so")
        assertTrue(none.isEmpty())
    }

    @Test
    fun pcm16LittleAndBigEndian() {
        assertArrayEquals(shortArrayOf(1, -2), Pcm.shorts(byteArrayOf(1, 0, -2, -1)))
        assertArrayEquals(shortArrayOf(1, -2), Pcm.shorts(byteArrayOf(0, 1, -1, -2), bigEndian = true))
        // 半端なバイトは捨てる
        assertArrayEquals(shortArrayOf(1), Pcm.shorts(byteArrayOf(1, 0, 9), 3))
    }

    @Test
    fun stereo48kBecomesMono16k() {
        val format = AudioFormat(48_000f, 16, 2, true, false)
        val frames = 48_000
        val bytes = ByteArray(frames * 4)
        for (f in 0 until frames) {
            // 左 16384・右 0 → モノラルで 0.25
            bytes[f * 4] = 0
            bytes[f * 4 + 1] = 0x40
        }
        val out = Pcm.toWhisper(bytes, bytes.size, format, StreamingResampler(48_000))
        assertTrue("${out.size}", out.size in 15_990..16_000)
        assertEquals(0.25f, out[out.size / 2], 1e-4f)
    }
}
