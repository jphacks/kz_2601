package jp.kz2601.metamon.renderer.components.audio

import android.content.Context
import android.content.pm.PackageManager
import android.media.MediaMetadataRetriever
import android.media.MediaPlayer
import android.media.MediaRecorder
import android.os.SystemClock
import android.util.Log
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableDoubleStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.io.File
import java.util.UUID

/**
 * `audioRecorder` の録音(iOS の `AudioRecorderController`)。MediaRecorder で AAC(m4a・モノラル 64kbps)を一時ファイルに録り、
 * 止めたら [LocalAudioStore] に取り込んで [onRecorded] で値を返す。上限は setMaxDuration が止める。メインスレッドで使う。
 */
class AudioRecorderController(
    private val context: Context,
    maxDuration: Int?,
    hasClip: Boolean,
    private val scope: CoroutineScope,
) {
    enum class Failure { DENIED, FAILED }

    val owner = "recorder-" + UUID.randomUUID()
    val maxDuration = AudioRecorderPolicy.maxDuration(maxDuration)
    private val store = LocalAudioStore.of(context)

    var machine by mutableStateOf(AudioRecorderMachine(hasClip))
        private set
    var phase by mutableStateOf(machine.phase)
        private set
    var elapsed by mutableDoubleStateOf(0.0)
        private set
    var level by mutableDoubleStateOf(0.0)
        private set
    var failure by mutableStateOf<Failure?>(null)
        private set

    var onRecorded: ((AudioValue) -> Unit)? = null

    private var recorder: MediaRecorder? = null
    private var file: File? = null
    private var startedAt = 0L
    private var ticker: Job? = null

    val isRecording: Boolean get() = phase == AudioRecorderMachine.Phase.RECORDING

    /** フォームの値が外から変わった(削除・別レコード)時に合わせる。録音中は触らない。 */
    fun sync(hasClip: Boolean) {
        if (isRecording) return
        machine = AudioRecorderMachine(hasClip)
        phase = machine.phase
    }

    fun denied() {
        failure = Failure.DENIED
    }

    /** 権限は呼ぶ側が確かめてから。 */
    fun start() {
        if (isRecording) return
        failure = null
        if (context.checkSelfPermission(android.Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            failure = Failure.DENIED
            return
        }
        if (!send(AudioRecorderMachine.Event.START)) return
        AudioArbiter.claim(context, owner, AudioClaims.Use.RECORDING) { stop() }
        val out = store.temporaryRecording(context.cacheDir)
        val r = MediaRecorder(context)
        try {
            r.setAudioSource(MediaRecorder.AudioSource.MIC)
            r.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
            r.setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
            r.setAudioSamplingRate(AudioRecorderPolicy.SAMPLE_RATE)
            r.setAudioChannels(AudioRecorderPolicy.CHANNELS)
            r.setAudioEncodingBitRate(AudioRecorderPolicy.BIT_RATE)
            r.setMaxDuration(maxDuration * 1000)
            r.setOutputFile(out)
            r.setOnInfoListener { _, what, _ ->
                if (what == MediaRecorder.MEDIA_RECORDER_INFO_MAX_DURATION_REACHED) scope.launch { finish(stopFirst = false) }
            }
            r.setOnErrorListener { _, _, _ -> scope.launch { fail() } }
            r.prepare()
            r.start()
        } catch (e: Exception) {
            Log.w(TAG, "audio record failed: ${e.javaClass.simpleName}")
            r.release()
            out.delete()
            AudioArbiter.release(owner)
            send(AudioRecorderMachine.Event.FAIL)
            failure = Failure.FAILED
            return
        }
        recorder = r
        file = out
        startedAt = SystemClock.elapsedRealtime()
        elapsed = 0.0
        level = 0.0
        ticker = scope.launch {
            while (isActive) {
                delay(100)
                val rec = recorder ?: break
                elapsed = (SystemClock.elapsedRealtime() - startedAt) / 1000.0
                level = AudioRecorderPolicy.meterLevel(AudioRecorderPolicy.amplitudeToPower(runCatching { rec.maxAmplitude }.getOrDefault(0)))
            }
        }
    }

    /** 止める(録れた分は残す)。 */
    fun stop() {
        if (isRecording) finish(stopFirst = true)
    }

    fun discard() {
        AudioPlayback.stop()
        send(AudioRecorderMachine.Event.DISCARD)
    }

    private fun finish(stopFirst: Boolean) {
        val r = recorder ?: return
        val out = file
        // 上限に着いた時は止まりかけなので、stop の失敗は見ない
        val stopped = runCatching { r.stop() }.isSuccess || !stopFirst
        teardown(r)
        val seconds = out?.let(::durationSeconds) ?: 0.0
        val duration = if (seconds > 0) seconds else elapsed
        val id = if (stopped && out != null && out.length() > 0 && duration > 0) store.importFile(out) else null
        if (id != null) {
            send(AudioRecorderMachine.Event.STOP)
            onRecorded?.invoke(AudioValue(id, AudioRecorderPolicy.storedSeconds(duration)))
        } else {
            out?.delete()
            send(AudioRecorderMachine.Event.FAIL)
            failure = Failure.FAILED
        }
    }

    private fun fail() {
        val r = recorder ?: return
        teardown(r)
        file?.delete()
        send(AudioRecorderMachine.Event.FAIL)
        failure = Failure.FAILED
    }

    private fun teardown(r: MediaRecorder) {
        ticker?.cancel()
        ticker = null
        recorder = null
        r.release()
        level = 0.0
        AudioArbiter.release(owner)
    }

    private fun send(event: AudioRecorderMachine.Event): Boolean {
        val ok = machine.send(event)
        phase = machine.phase
        return ok
    }

    private fun durationSeconds(f: File): Double = runCatching {
        val m = MediaMetadataRetriever()
        try {
            m.setDataSource(f.absolutePath)
            (m.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull() ?: 0L) / 1000.0
        } finally {
            m.release()
        }
    }.getOrDefault(0.0)

    private companion object {
        const val TAG = "Metamon"
    }
}

/** 録音の再生(アプリ全体で 1 本・iOS の `AudioPlayback`)。メインスレッドで使う。 */
object AudioPlayback {
    private const val OWNER = "audio-playback"
    private var player: MediaPlayer? = null

    var playingId by mutableStateOf<String?>(null)
        private set

    fun isPlaying(id: String?): Boolean = id != null && playingId == id

    fun play(context: Context, id: String) {
        stop()
        val f = LocalAudioStore.of(context).file(id)?.takeIf { it.isFile } ?: return
        val p = MediaPlayer()
        try {
            p.setDataSource(f.absolutePath)
            p.setOnCompletionListener { stop() }
            p.prepare()
        } catch (_: Exception) {
            p.release()
            return
        }
        AudioArbiter.claim(context, OWNER, AudioClaims.Use.PLAYBACK) { stop() }
        p.start()
        player = p
        playingId = id
    }

    /** 再生中の位置(秒)。 */
    fun position(): Double = runCatching { (player?.currentPosition ?: 0) / 1000.0 }.getOrDefault(0.0)

    fun stop() {
        player?.let { runCatching { it.stop() }; it.release() }
        player = null
        playingId = null
        AudioArbiter.release(OWNER)
    }
}

/**
 * マイクの権限を聞いてから [onResult]。:app の窓口([LocalMicrophoneAccess])があればそれで聞き、
 * 無ければ(プレビュー・テスト)OS の今の状態だけを見る。
 */
@Composable
fun rememberMicrophoneRequest(): ((Boolean) -> Unit) -> Unit {
    val access = LocalMicrophoneAccess.current
    val context = LocalContext.current
    val latest by rememberUpdatedState(access)
    return { onResult ->
        val a = latest
        if (a != null) {
            a.request(onResult)
        } else {
            onResult(context.checkSelfPermission(android.Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED)
        }
    }
}
