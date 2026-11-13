package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.store.AppDataDir
import jp.kz2601.metamon.whisper.ModelSource
import jp.kz2601.metamon.whisper.WhisperModelManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.io.IOException

/** 文字起こしのモデルの置き場(VoiceModels の dir・override・#2801)。 */
class VoiceModelsTest {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun defaultDirIsModelsUnderAppData() {
        assertEquals(File(AppDataDir.resolve(), "models"), VoiceModels.dir())
    }

    @Test
    fun dirFollowsEachOsDataDir() {
        val win = AppDataDir.resolve("Windows 11", mapOf("APPDATA" to "C:/Users/u/AppData/Roaming"), "C:/Users/u", override = null)
        assertEquals(File("C:/Users/u/AppData/Roaming/TailorIt", "models"), VoiceModels.dir(win))
        val linux = AppDataDir.resolve("Linux", emptyMap(), "/home/u", override = null)
        assertEquals(File("/home/u/.local/share/tailorit", "models"), VoiceModels.dir(linux))
        val xdg = AppDataDir.resolve("Linux", mapOf("XDG_DATA_HOME" to "/data"), "/home/u", override = null)
        assertEquals(File("/data/tailorit", "models"), VoiceModels.dir(xdg))
        val mac = AppDataDir.resolve("Mac OS X", emptyMap(), "/Users/u", override = null)
        assertEquals(File("/Users/u/Library/Application Support/TailorIt", "models"), VoiceModels.dir(mac))
        // tailorit.dataDir で保存先を変えるとモデルも付いていく
        assertEquals(File("/x", "models"), VoiceModels.dir(AppDataDir.resolve("Linux", emptyMap(), "/home/u", override = "/x")))
    }

    @Test
    fun overrideFromEnvOrJvmProperty() {
        assertNull(VoiceModels.override({ null }, { null }))
        assertEquals(File("/env.bin"), VoiceModels.override({ null }, env("/env.bin")))
        assertEquals(File("/prop.bin"), VoiceModels.override(prop("/prop.bin"), { null }))
        // 両方あれば JVM の引数が勝つ
        assertEquals(File("/prop.bin"), VoiceModels.override(prop("/prop.bin"), env("/env.bin")))
    }

    @Test
    fun blankOverrideIsIgnored() {
        assertNull(VoiceModels.override(prop(""), env("  ")))
        // 空の -D は環境変数を隠さない
        assertEquals(File("/env.bin"), VoiceModels.override(prop(" "), env("/env.bin")))
    }

    @Test
    fun overrideToMissingFileIsKeptAndSkipsDownload() {
        val missing = File(tmp.root, "no-such-model.bin")
        // 置き場は確かめない(読み込みの時に WhisperUnavailableException で分かる)
        assertEquals(missing, VoiceModels.override(prop(missing.path), { null }))
        assertFalse(missing.exists())

        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val models = WhisperModelManager(tmp.newFolder("models"), source = ModelSource { throw IOException("no network in tests") })
            // 差し替えがあれば落とさない
            assertNull(WhisperCppTranscriber(models, modelOverride = missing, scope = scope).pendingDownloadMegabytes)
            // 無ければ、まだ落としていないモデルの大きさ
            assertEquals(models.spec.megabytes, WhisperCppTranscriber(models, modelOverride = null, scope = scope).pendingDownloadMegabytes)
        } finally {
            scope.cancel()
        }
    }

    private fun prop(v: String): (String) -> String? = mapOf("tailorit.whisper.model" to v)::get

    private fun env(v: String): (String) -> String? = mapOf("TAILORIT_WHISPER_MODEL" to v)::get
}
