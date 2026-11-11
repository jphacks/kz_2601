package jp.kz2601.metamon.renderer.components.location

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Track
import java.time.Instant
import java.util.Locale

/** 記録先(iOS の `LocationTracking.Target`)。同じアプリの同じ entity + location field の部品なら「この部品の記録」。 */
data class TrackingTarget(val appName: String, val entity: String, val locationField: String, val dateField: String?) {
    companion object {
        /** entity と locationField が無い部品は記録できない(代わりの枠)。 */
        fun of(c: Component, appName: String): TrackingTarget? {
            val entity = c.entity ?: return null
            val field = c.locationField ?: return null
            return TrackingTarget(appName, entity, field, c.dateField)
        }
    }
}

enum class TrackingPhase { IDLE, REQUESTING, RECORDING, DENIED, FAILED }

/** 止まった理由(次の開始で消える)。文言は部品が出す。 */
enum class TrackingStopReason { AUTO_STOP, SAVE_FAILED, UNAVAILABLE }

/**
 * 位置の記録(`locationTracker`・#869)の状態。iOS の `LocationTracking` の値の部分を、遷移を純粋な関数にしたもの。
 * 時刻はミリ秒(System.currentTimeMillis)で受け取る(テストで固定できる)。
 */
data class TrackingState(
    val phase: TrackingPhase = TrackingPhase.IDLE,
    val target: TrackingTarget? = null,
    val startedAtMillis: Long? = null,
    val endsAtMillis: Long? = null,
    val autoStopSeconds: Int = Track.autoStopSeconds(null),
    val savedCount: Int = 0,
    val distance: Double = 0.0,
    val lastKept: Track.Point? = null,
    val stopReason: TrackingStopReason? = null,
) {
    /** 記録中(許可を待っている間も含む)。同時に 2 本は取らない。 */
    val isActive: Boolean get() = phase == TrackingPhase.RECORDING || phase == TrackingPhase.REQUESTING

    fun isRecording(t: TrackingTarget): Boolean = isActive && target == t

    /** 開始を押した。記録中なら何もしない。 */
    fun start(t: TrackingTarget, autoStopSeconds: Int): TrackingState =
        if (isActive) this else TrackingState(phase = TrackingPhase.REQUESTING, target = t, autoStopSeconds = autoStopSeconds)

    /** 位置の許可が取れた(サービスが前面で動き出した)。 */
    fun began(nowMillis: Long): TrackingState =
        if (phase != TrackingPhase.REQUESTING) this
        else copy(phase = TrackingPhase.RECORDING, startedAtMillis = nowMillis, endsAtMillis = nowMillis + autoStopSeconds * 1000L)

    fun denied(): TrackingState = if (isActive) copy(phase = TrackingPhase.DENIED, endsAtMillis = null) else this

    /** 止める。[reason] は自動停止など(手で止めた時は null)。 */
    fun stop(reason: TrackingStopReason? = null): TrackingState =
        copy(
            phase = if (reason == TrackingStopReason.SAVE_FAILED || reason == TrackingStopReason.UNAVAILABLE) TrackingPhase.FAILED else TrackingPhase.IDLE,
            endsAtMillis = null,
            stopReason = reason,
        )

    /** 自動停止の時刻を過ぎたか。 */
    fun isExpired(nowMillis: Long): Boolean = phase == TrackingPhase.RECORDING && endsAtMillis != null && nowMillis >= endsAtMillis

    /** 届いた位置を保存するか([Track.shouldKeep])。 */
    fun shouldKeep(p: Track.Point, accuracy: Double, minDistance: Double = 25.0): Boolean =
        phase == TrackingPhase.RECORDING && target != null && Track.shouldKeep(p, accuracy, lastKept, minDistance)

    /** 1 点を保存できた。 */
    fun kept(p: Track.Point): TrackingState =
        copy(savedCount = savedCount + 1, distance = distance + (lastKept?.let { Track.distance(it, p) } ?: 0.0), lastKept = p)

    /** 記録中バーが読む形。 */
    val status: TrackingStatus
        get() = when (phase) {
            TrackingPhase.RECORDING -> TrackingStatus(isTracking = true, startedAtMillis = startedAtMillis, appName = target?.appName, entity = target?.entity)
            TrackingPhase.REQUESTING -> TrackingStatus(isTracking = true, isRequesting = true, appName = target?.appName, entity = target?.entity)
            else -> TrackingStatus.IDLE
        }
}

/** ホストの「記録中」バーが読む状態(iOS の `LocationTrackingStatus`)。 */
data class TrackingStatus(
    val isTracking: Boolean,
    val isRequesting: Boolean = false,
    val startedAtMillis: Long? = null,
    val appName: String? = null,
    val entity: String? = null,
) {
    fun elapsedSeconds(nowMillis: Long): Long = startedAtMillis?.let { maxOf(0L, (nowMillis - it) / 1000) } ?: 0L

    companion object {
        val IDLE = TrackingStatus(isTracking = false)
    }
}

/** 記録の値の組み立て(iOS の LocationTrackerLeaf / LocationTracking の static な部分)。 */
object TrackingRules {
    /** 点のレコードに入れる他の field の既定値(defaultValue を持つ保存 field だけ)。 */
    fun baseData(entity: Entity?, skipping: Collection<String>, at: Instant): Map<String, JSONValue> =
        entity?.fields.orEmpty()
            .filter { !it.isComputed && it.defaultValue != null && it.id !in skipping }
            .associateTo(LinkedHashMap()) { it.id to FieldCoding.defaultValue(it, at) }

    /** 1 点 = 1 レコードの data(location field = "lat,lng"、dateField = 時刻、他は既定値)。記録先が無くなっていれば null。 */
    fun pointData(target: TrackingTarget, entity: Entity?, p: Track.Point): Map<String, JSONValue>? {
        if (entity?.field(target.locationField)?.type != FieldType.LOCATION) return null
        val data = LinkedHashMap(baseData(entity, listOfNotNull(target.locationField, target.dateField), p.date))
        data[target.locationField] = JSONValue.Str(Track.encode(p.lat, p.lng))
        target.dateField?.let { data[it] = JSONValue.Str(FieldCoding.formatDatetime(p.date)) }
        return data
    }

    /** 経過時間 `12:34` / `1:02:03`(iOS の clock)。 */
    fun clock(seconds: Long): String {
        val s = maxOf(0L, seconds)
        return if (s >= 3600) String.format(Locale.ROOT, "%d:%02d:%02d", s / 3600, s / 60 % 60, s % 60)
        else String.format(Locale.ROOT, "%02d:%02d", s / 60, s % 60)
    }

    /** あと何分で自動停止か(切り上げ・iOS と同じく +1)。 */
    fun remainingMinutes(state: TrackingState, nowMillis: Long): Long =
        maxOf(0L, ((state.endsAtMillis ?: nowMillis) - nowMillis) / 1000) / 60 + 1
}
