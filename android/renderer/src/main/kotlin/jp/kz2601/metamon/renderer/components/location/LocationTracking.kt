package jp.kz2601.metamon.renderer.components.location

import android.content.Context
import android.content.Intent
import android.location.Location
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Track
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.time.Instant

/**
 * 位置の記録(`locationTracker`・#869)のセッション。アプリ全体で 1 つ(iOS の `LocationTracking.shared`)。
 *
 * - 使う人が開始を押した間だけ動く。最長 [Track.autoStopSeconds] で自動停止。
 * - 位置は [LocationTrackingService](Foreground Service・type location・停止ボタンつきの通知)が受ける。
 *   前面で開始するので、使用中の位置の許可だけで裏でも続く(ACCESS_BACKGROUND_LOCATION は要らない)。
 * - 1 点 = 1 レコードで、届いた順に保存する(停止しても点は残る)。
 */
object LocationTracking {
    private val _state = MutableStateFlow(TrackingState())
    val state: StateFlow<TrackingState> = _state.asStateFlow()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val saving = Mutex()
    private var appContext: Context? = null
    private var entity: (() -> Entity?)? = null
    private var save: (suspend (Map<String, JSONValue>) -> Boolean)? = null

    /** ホストの「記録中」バーと停止(iOS の `LocationTrackingControl`)。 */
    val status: TrackingStatus get() = _state.value.status

    /**
     * 記録を始める。位置の許可は呼ぶ前に聞いておく。[entity] は記録先の今の定義(アプリが変わったら null / 別物)、
     * [save] はレコードを 1 件足す。
     */
    fun start(
        context: Context,
        target: TrackingTarget,
        autoStopSeconds: Int,
        entity: () -> Entity?,
        save: suspend (Map<String, JSONValue>) -> Boolean,
    ) {
        if (_state.value.isActive) return
        val app = context.applicationContext
        appContext = app
        this.entity = entity
        this.save = save
        _state.value = _state.value.start(target, autoStopSeconds)
        try {
            app.startForegroundService(Intent(app, LocationTrackingService::class.java).setAction(LocationTrackingService.ACTION_START))
        } catch (_: RuntimeException) {
            // 前面でない・OS が許さない(ForegroundServiceStartNotAllowedException 等)
            finish(TrackingStopReason.UNAVAILABLE)
        }
    }

    /** 止める(手で・通知の停止・自動停止)。 */
    fun stop(reason: TrackingStopReason? = null) {
        if (!_state.value.isActive) return
        finish(reason)
    }

    /** 位置の許可が無かった(サービスは起こさない)。部品が「設定を開く」を出す。 */
    fun refused(target: TrackingTarget, autoStopSeconds: Int) {
        if (_state.value.isActive) return
        _state.value = _state.value.start(target, autoStopSeconds).denied()
    }

    // サービスから

    internal fun began(nowMillis: Long = System.currentTimeMillis()) = _state.update { it.began(nowMillis) }

    internal fun denied() {
        if (!_state.value.isActive) return
        _state.update { it.denied() }
        release()
    }

    /** 1 秒ごと(サービスのタイマー)。自動停止の時刻を過ぎたら止める。 */
    internal fun tick(nowMillis: Long = System.currentTimeMillis()) {
        if (_state.value.isExpired(nowMillis)) stop(TrackingStopReason.AUTO_STOP)
    }

    internal fun onLocation(loc: Location) {
        val accuracy = if (loc.hasAccuracy()) loc.accuracy.toDouble() else -1.0
        val p = Track.Point(loc.latitude, loc.longitude, Instant.ofEpochMilli(loc.time))
        scope.launch {
            saving.withLock {
                val s = _state.value
                val target = s.target ?: return@withLock
                if (!s.shouldKeep(p, accuracy)) return@withLock
                val data = TrackingRules.pointData(target, entity?.invoke(), p)
                val ok = data != null && save?.invoke(data) == true
                if (!_state.value.isRecording(target)) return@withLock
                if (ok) _state.update { it.kept(p) } else stop(TrackingStopReason.SAVE_FAILED)
            }
        }
    }

    private fun finish(reason: TrackingStopReason?) {
        _state.update { it.stop(reason) }
        release()
    }

    private fun release() {
        save = null
        entity = null
        appContext?.let { it.stopService(Intent(it, LocationTrackingService::class.java)) }
    }
}
