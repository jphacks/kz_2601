package jp.kz2601.metamon.renderer.components.dashcam

import android.Manifest
import android.annotation.SuppressLint
import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
import android.os.BatteryManager
import android.os.Environment
import android.provider.MediaStore
import android.view.OrientationEventListener
import android.view.Surface
import androidx.camera.core.CameraSelector
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.lifecycle.awaitInstance
import androidx.camera.video.FallbackStrategy
import androidx.camera.video.FileOutputOptions
import androidx.camera.video.Quality
import androidx.camera.video.QualitySelector
import androidx.camera.video.Recorder
import androidx.camera.video.Recording
import androidx.camera.video.VideoCapture
import androidx.camera.video.VideoRecordEvent
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import androidx.lifecycle.LifecycleOwner
import jp.kz2601.metamon.renderer.components.sensors.MotionFusion
import jp.kz2601.metamon.renderer.components.sensors.Vector3
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

/** ドラレコの状態(iOS の `DashcamRecorder.Phase`)。 */
sealed interface DashcamPhase {
    data object Idle : DashcamPhase
    data object Preparing : DashcamPhase
    data object Ready : DashcamPhase
    data object Recording : DashcamPhase

    /** 背面カメラが無い */
    data object Unavailable : DashcamPhase

    /** カメラ(とマイク)の許可が無い */
    data object Denied : DashcamPhase

    /** 保存済みだけで容量の上限 / 端末の空きが少ない */
    data class Full(val deviceLow: Boolean) : DashcamPhase
}

/**
 * ドラレコの録画(iOS の `DashcamRecorder`)。部品が表示され、画面が前面にある間だけ動く。
 * 1 分ごとの区間で CameraX に録り、閉じるたびにループの古い区間を消す。衝撃(加速度)・「今の映像を保存」で前後を保存済みにする。
 * 裏に回ると(ON_STOP)区間を閉じて止め、戻ったら続ける(フォアグラウンドサービスは持たない)。
 */
@Stable
class DashcamRecorder(
    private val context: Context,
    val config: DashcamConfig,
    val store: DashcamStore,
    private val scope: CoroutineScope,
) {
    var phase by mutableStateOf<DashcamPhase>(DashcamPhase.Idle)
        private set
    var recordingSince by mutableStateOf<Long?>(null)
        private set

    /** 保存済みの区間(新しい順) */
    var clips by mutableStateOf<List<DashcamSegment>>(emptyList())
        private set
    var lastImpact by mutableStateOf<Long?>(null)
        private set
    var autoStartIn by mutableStateOf<Int?>(null)
        private set

    /** 裏に回って録画を止めた(戻ったら続ける) */
    var stoppedInBackground by mutableStateOf(false)
        private set

    /** 衝撃・手動の保存の回数(振動・読み上げのきっかけ) */
    var impactCount by mutableIntStateOf(0)
        private set

    val isRecording: Boolean get() = phase == DashcamPhase.Recording

    val preview: Preview = Preview.Builder().build()
    private var provider: ProcessCameraProvider? = null
    private var videoCapture: VideoCapture<Recorder>? = null
    private val impact = DashcamImpact(config.impactG)
    private val autoStart = DashcamAutoStart(config.autoStart)

    /** 録画中・閉じている途中の区間のファイルと開始時刻 */
    private val starts = mutableMapOf<File, Long>()
    private var active: File? = null
    private var recording: Recording? = null
    private var rotateJob: Job? = null
    private var wantsRecording = false
    private var resumeAfterBackground = false
    private var motion: Motion? = null

    // MARK: - lifecycle(部品の表示・裏/表)

    /** 置き場を整え、カメラがあるかを見る。続けてよければ true(権限を聞く)。 */
    fun appear(): Boolean {
        store.prepare()
        refresh()
        if (phase != DashcamPhase.Idle) return false
        if (!context.packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)) {
            phase = DashcamPhase.Unavailable
            return false
        }
        phase = DashcamPhase.Preparing
        return true
    }

    fun denied() {
        phase = DashcamPhase.Denied
    }

    /** 権限を得た後: 背面カメラのプレビューと録画を [owner] の寿命に結ぶ。 */
    suspend fun bind(owner: LifecycleOwner) {
        if (phase != DashcamPhase.Preparing) return
        val p = runCatching { ProcessCameraProvider.awaitInstance(context) }.getOrNull()
        if (p == null || !runCatching { p.hasCamera(CameraSelector.DEFAULT_BACK_CAMERA) }.getOrDefault(false)) {
            phase = DashcamPhase.Unavailable
            return
        }
        // 720p: 1 分 ≈ 40〜60MB(容量の上限・ループの見込みの前提)
        val recorder = Recorder.Builder()
            .setQualitySelector(QualitySelector.from(Quality.HD, FallbackStrategy.lowerQualityOrHigherThan(Quality.HD)))
            .build()
        val vc = VideoCapture.withOutput(recorder)
        val bound = runCatching { p.bindToLifecycle(owner, CameraSelector.DEFAULT_BACK_CAMERA, preview, vc) }.isSuccess
        if (!bound) {
            phase = DashcamPhase.Unavailable
            return
        }
        provider = p
        videoCapture = vc
        phase = DashcamPhase.Ready
        startMotion()
    }

    fun disappear() {
        stop(manual = false)
        motion?.stop()
        motion = null
        videoCapture?.let { vc -> runCatching { provider?.unbind(preview, vc) } }
        videoCapture = null
        if (phase != DashcamPhase.Unavailable && phase != DashcamPhase.Denied) phase = DashcamPhase.Idle
    }

    /** 裏に回った: カメラは止まるので、区間を閉じて止める。戻ったら続ける。 */
    fun enterBackground() {
        if (phase != DashcamPhase.Recording && phase != DashcamPhase.Ready) return
        resumeAfterBackground = phase == DashcamPhase.Recording
        if (resumeAfterBackground) stoppedInBackground = true
        stop(manual = false)
        motion?.stop()
    }

    fun enterForeground() {
        if (phase != DashcamPhase.Ready) return
        motion?.start()
        autoStart.rearm()
        if (resumeAfterBackground) {
            resumeAfterBackground = false
            start()
        }
    }

    // MARK: - recording

    fun start() {
        if (phase != DashcamPhase.Ready) return
        storageProblem()?.let {
            phase = DashcamPhase.Full(it)
            return
        }
        stoppedInBackground = false
        wantsRecording = true
        autoStart.disarm()
        phase = DashcamPhase.Recording
        recordingSince = System.currentTimeMillis()
        if (active == null) beginSegment() // 前の区間を閉じている途中なら、閉じた後に始まる
    }

    /** [manual] = 利用者が止めた(自動開始は条件が一度外れるまで待つ)。 */
    fun stop(manual: Boolean = true) {
        if (manual) autoStart.disarm()
        if (!wantsRecording) return
        wantsRecording = false
        rotateJob?.cancel()
        recording?.stop()
        recording = null
        if (phase == DashcamPhase.Recording) phase = DashcamPhase.Ready
        recordingSince = null
    }

    /** 「今の映像を保存」: 衝撃が無くても前後を保存済みにする。 */
    fun saveNow() = mark(System.currentTimeMillis())

    fun delete(clip: DashcamSegment) {
        store.delete(clip)
        refresh()
        if (phase is DashcamPhase.Full && storageProblem() == null) phase = DashcamPhase.Ready
    }

    fun file(clip: DashcamSegment): File = store.file(clip)

    /** 端末の動画(Movies)に追加する。Android 10 以降は自分で足すだけなら権限が要らない。 */
    suspend fun saveToGallery(clip: DashcamSegment): Boolean = withContext(Dispatchers.IO) {
        val resolver = context.contentResolver
        val values = ContentValues().apply {
            put(MediaStore.Video.Media.DISPLAY_NAME, "dashcam-${clip.start}.${DashcamSegment.EXTENSION}")
            put(MediaStore.Video.Media.MIME_TYPE, "video/mp4")
            put(MediaStore.Video.Media.RELATIVE_PATH, Environment.DIRECTORY_MOVIES)
            put(MediaStore.Video.Media.IS_PENDING, 1)
        }
        val collection = MediaStore.Video.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
        val uri = runCatching { resolver.insert(collection, values) }.getOrNull() ?: return@withContext false
        val ok = runCatching {
            resolver.openOutputStream(uri)?.use { out -> store.file(clip).inputStream().use { it.copyTo(out) } } != null
        }.getOrDefault(false)
        if (ok) {
            resolver.update(uri, ContentValues().apply { put(MediaStore.Video.Media.IS_PENDING, 0) }, null, null)
        } else {
            runCatching { resolver.delete(uri, null, null) }
        }
        ok
    }

    /** OS の共有シート(FileProvider で読ませる)。 */
    fun shareIntent(clip: DashcamSegment): Intent {
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.metamon.media", store.file(clip))
        val send = Intent(Intent.ACTION_SEND).setType("video/mp4").putExtra(Intent.EXTRA_STREAM, uri)
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        return Intent.createChooser(send, null).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }

    @SuppressLint("MissingPermission") // 音は RECORD_AUDIO を確かめてから
    private fun beginSegment() {
        if (active != null) return
        val vc = videoCapture ?: return
        val start = System.currentTimeMillis()
        val file = store.recordingFile(start)
        starts[file] = start
        active = file
        val pending = vc.output.prepareRecording(context, FileOutputOptions.Builder(file).build())
            .let { if (config.audio && hasMicrophonePermission()) it.withAudioEnabled() else it }
        recording = runCatching {
            pending.start(ContextCompat.getMainExecutor(context)) { e -> if (e is VideoRecordEvent.Finalize) segmentFinished(file) }
        }.getOrNull()
        rotateJob?.cancel()
        if (recording == null) {
            scope.launch { segmentFinished(file) }
            return
        }
        rotateJob = scope.launch {
            delay(DashcamConfig.SEGMENT_MILLIS)
            recording?.stop()
            recording = null
        }
    }

    private fun segmentFinished(file: File) {
        val start = starts.remove(file) ?: System.currentTimeMillis()
        if (active == file) active = null
        val end = System.currentTimeMillis()
        val saved = impact.protects(start, end)
        val kept = store.finalize(file, start, end - start, saved)
        impact.forget(end - DashcamImpact.BEFORE - DashcamConfig.SEGMENT_MILLIS)
        prune()
        if (!wantsRecording) return
        storageProblem()?.let {
            stop(manual = false)
            phase = DashcamPhase.Full(it)
            return
        }
        if (kept != null) return beginSegment()
        // 録れなかった(カメラが他に取られた等): 少し待ってやり直す
        rotateJob?.cancel()
        rotateJob = scope.launch {
            delay(1_000)
            if (wantsRecording) beginSegment()
        }
    }

    /** ループの古い区間を消して一覧を読み直す。 */
    private fun prune() {
        DashcamLoop.expired(store.segments(), System.currentTimeMillis(), config.keepMillis, config.maxBytes).forEach(store::delete)
        refresh()
    }

    private fun refresh() {
        clips = store.segments().filter { it.saved }.sortedByDescending { it.start }
    }

    private fun storageProblem(): Boolean? = DashcamStore.storageProblem(store.segments(), store.deviceFreeBytes(), config.maxBytes)

    private fun mark(at: Long) {
        impact.mark(at)
        protectFinished()
        lastImpact = at
        impactCount += 1
    }

    /** 既に閉じたループ区間で衝撃の前後にかかるものを保存済みに移す(録画中の区間は閉じた時に判定)。 */
    private fun protectFinished() {
        store.segments().filter { !it.saved && impact.protects(it.start, it.end) }.forEach { store.save(it) }
        refresh()
    }

    private fun hasMicrophonePermission() =
        context.checkSelfPermission(Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED

    // MARK: - motion(衝撃・自動開始・録画の向き)

    private fun startMotion() {
        if (motion == null) motion = Motion()
        motion?.start()
    }

    private fun handle(sample: Motion.Sample) {
        val now = System.currentTimeMillis()
        val a = sample.userAcceleration
        if (phase == DashcamPhase.Recording && impact.feed(a.x, a.y, a.z, now)) {
            protectFinished()
            lastImpact = now
            impactCount += 1
        }
        if (config.autoStart.isEmpty()) return
        val g = sample.gravity
        val go = autoStart.feed(
            DashcamAutoStart.Sample(g.x, g.y, g.z, a.magnitude, sample.rotationRate.magnitude, sample.charging, now),
        )
        val left = if (phase == DashcamPhase.Ready) autoStart.remainingSeconds(now) else null
        if (left != autoStartIn) autoStartIn = left
        if (go && phase == DashcamPhase.Ready) start()
    }

    /** 加速度計・ジャイロ(20 Hz)と画面の向き。前面にいる間だけ登録する。 */
    private inner class Motion {
        inner class Sample(val gravity: Vector3, val userAcceleration: Vector3, val rotationRate: Vector3, val charging: Boolean)

        private val manager = context.getSystemService(Context.SENSOR_SERVICE) as? SensorManager
        private val battery = context.getSystemService(Context.BATTERY_SERVICE) as? BatteryManager
        private val fusion = MotionFusion()
        private var charging = false
        private var chargingAt = 0L
        private var running = false

        private val listener = object : SensorEventListener {
            override fun onSensorChanged(e: SensorEvent) {
                if (e.values.size < 3) return
                val v = Vector3(e.values[0].toDouble(), e.values[1].toDouble(), e.values[2].toDouble())
                if (e.sensor.type == Sensor.TYPE_GYROSCOPE) {
                    fusion.rotation(v)
                    return
                }
                val s = fusion.accelerometer(v, e.timestamp / 1_000_000)
                val now = System.currentTimeMillis()
                if (now - chargingAt >= 1_000) { // 充電の状態は 1 秒に 1 回だけ読む
                    charging = battery?.isCharging == true
                    chargingAt = now
                }
                handle(Sample(s.gravity, s.userAcceleration, s.rotationRate, charging))
            }

            override fun onAccuracyChanged(sensor: Sensor?, accuracy: Int) = Unit
        }

        /** 端末の向き → 録画の向き(画面の向きを固定していても水平に録る) */
        private val orientation = object : OrientationEventListener(context) {
            override fun onOrientationChanged(degrees: Int) {
                if (degrees == ORIENTATION_UNKNOWN) return
                videoCapture?.targetRotation = when (degrees) {
                    in 45 until 135 -> Surface.ROTATION_270
                    in 135 until 225 -> Surface.ROTATION_180
                    in 225 until 315 -> Surface.ROTATION_90
                    else -> Surface.ROTATION_0
                }
            }
        }

        fun start() {
            if (running) return
            running = true
            manager?.getDefaultSensor(Sensor.TYPE_ACCELEROMETER)?.let { manager.registerListener(listener, it, PERIOD_US) }
            manager?.getDefaultSensor(Sensor.TYPE_GYROSCOPE)?.let { manager.registerListener(listener, it, PERIOD_US) }
            if (orientation.canDetectOrientation()) orientation.enable()
        }

        fun stop() {
            if (!running) return
            running = false
            manager?.unregisterListener(listener)
            orientation.disable()
            autoStartIn = null
        }
    }

    private companion object {
        /** 20 Hz(200 Hz 未満なので HIGH_SAMPLING_RATE_SENSORS は要らない) */
        const val PERIOD_US = 50_000
    }
}
