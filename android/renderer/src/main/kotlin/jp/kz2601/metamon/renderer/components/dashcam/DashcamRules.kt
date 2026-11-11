package jp.kz2601.metamon.renderer.components.dashcam

import jp.kz2601.metamon.dsl.AutoStart
import jp.kz2601.metamon.dsl.Component
import kotlin.math.abs
import kotlin.math.ceil
import kotlin.math.max
import kotlin.math.sqrt

/**
 * `dashcam`(#872)の純粋ロジック(iOS の MetamonDSL `Dashcam.swift` の写し)。カメラ・センサーに依存しない。
 * 時刻はすべて epoch ミリ秒。
 */
data class DashcamConfig(
    val keepMillis: Long,
    val maxBytes: Long,
    val impactG: Double,
    val autoStart: Set<AutoStart>,
    val audio: Boolean,
) {
    companion object {
        /** 1 区間の長さ。DSL では変えない */
        const val SEGMENT_MILLIS = 60_000L

        fun of(
            keepMinutes: Int? = null,
            maxStorageMB: Int? = null,
            impactG: Double? = null,
            autoStart: List<AutoStart> = emptyList(),
            audio: Boolean? = null,
        ) = DashcamConfig(
            keepMillis = (keepMinutes ?: 10).coerceIn(1, 60) * 60_000L,
            maxBytes = (maxStorageMB ?: 2000).coerceIn(200, 20_000) * 1_000_000L,
            impactG = (impactG ?: 2.5).coerceIn(1.2, 8.0),
            autoStart = autoStart.filter { it != AutoStart.UNKNOWN }.toSet(),
            audio = audio ?: false,
        )

        fun of(c: Component) = of(c.keepMinutes, c.maxStorageMB, c.impactG, c.autoStart, c.audio)
    }
}

/** 録画区間 1 つ。ファイル名 `seg-<開始 ms>-<長さ ms>.mp4` に開始時刻と長さを持たせる(別のメタデータを持たない)。 */
data class DashcamSegment(
    val start: Long,
    val duration: Long,
    val bytes: Long = 0,
    /** 保存済み(衝撃・手動で保存した区間。ループで消さない) */
    val saved: Boolean = false,
) {
    val end: Long get() = start + duration
    val fileName: String get() = "seg-$start-$duration.$EXTENSION"
    val id: String get() = fileName

    companion object {
        const val EXTENSION = "mp4"

        /** 録画中のファイル名(長さが決まる前)。 */
        fun recordingName(start: Long) = "rec-$start.$EXTENSION"

        /** ファイル名から読む。形が違えば null(区間として扱わない)。 */
        fun parse(fileName: String, bytes: Long = 0, saved: Boolean = false): DashcamSegment? {
            if (!fileName.startsWith("seg-") || !fileName.endsWith(".$EXTENSION")) return null
            val parts = fileName.removePrefix("seg-").removeSuffix(".$EXTENSION").split("-")
            if (parts.size != 2) return null
            val start = parts[0].toLongOrNull() ?: return null
            val duration = parts[1].toLongOrNull() ?: return null
            if (start < 0 || duration < 0) return null
            return DashcamSegment(start, duration, bytes, saved)
        }
    }
}

object DashcamLoop {
    /** 次の区間の見込みが分からない時(720p 1 分の目安) */
    const val DEFAULT_RESERVE = 60_000_000L

    /** 消すループ区間(古い順)。直近 [keepMillis] より前に終わった区間と、合計が [maxBytes] を超える分の古い区間。保存済みは消さない。 */
    fun expired(segments: List<DashcamSegment>, now: Long, keepMillis: Long, maxBytes: Long): List<DashcamSegment> {
        val loop = segments.filter { !it.saved }.sortedBy { it.start }
        val cutoff = now - keepMillis
        val drop = loop.filter { it.end < cutoff }.map { it.id }.toMutableSet()
        var total = segments.filter { it.id !in drop }.sumOf { it.bytes }
        for (s in loop) {
            if (total <= maxBytes) break
            if (s.id in drop) continue
            drop += s.id
            total -= s.bytes
        }
        return loop.filter { it.id in drop }
    }

    /** 保存済みだけで上限に届く(次の 1 区間 [reserve] を足すと超える)= これ以上録れない。 */
    fun isFull(segments: List<DashcamSegment>, maxBytes: Long, reserve: Long): Boolean =
        segments.filter { it.saved }.sumOf { it.bytes } + reserve > maxBytes

    /** 次の 1 区間の見込み容量: 直近 3 区間の最大(無ければ [DEFAULT_RESERVE])。 */
    fun reserve(segments: List<DashcamSegment>): Long =
        segments.sortedByDescending { it.start }.take(3).maxOfOrNull { it.bytes }?.takeIf { it > 0 } ?: DEFAULT_RESERVE
}

/** 衝撃の検知。重力を除いた加速度(G)の大きさが閾値以上なら衝撃。前後 [BEFORE] / [AFTER] にかかる区間を保存済みにする。 */
class DashcamImpact(val thresholdG: Double) {
    var lastImpact: Long? = null
        private set

    /** 保存する窓(開始, 終了) */
    val windows: List<LongRange> get() = _windows
    private val _windows = mutableListOf<LongRange>()

    /** 加速度 1 サンプル(G)。衝撃として数えたら true。 */
    fun feed(x: Double, y: Double, z: Double, at: Long): Boolean {
        if (sqrt(x * x + y * y + z * z) < thresholdG) return false
        lastImpact?.let { if (at - it < COOLDOWN) return false }
        mark(at)
        return true
    }

    /** 衝撃が無くても前後を保存する(「今の映像を保存」)。 */
    fun mark(at: Long) {
        lastImpact = at
        _windows += (at - BEFORE)..(at + AFTER)
    }

    /** この区間を保存済みにするか(どれかの窓にかかる)。 */
    fun protects(start: Long, end: Long): Boolean = _windows.any { it.first < end && start < it.last }

    /** [at] より前に終わった窓を捨てる(録画が長く続いても伸ばさない)。 */
    fun forget(before: Long) {
        _windows.removeAll { it.last < before }
    }

    companion object {
        const val BEFORE = 60_000L
        const val AFTER = 30_000L

        /** 同じ衝撃の揺れ戻しを 2 回と数えない */
        const val COOLDOWN = 5_000L
    }
}

/**
 * 自動開始。条件が全部 [HOLD] 続いたら 1 回だけ開始を返す。開始・手での停止の後は、条件が一度外れるまで再び開始しない。
 */
class DashcamAutoStart(val conditions: Set<AutoStart>) {
    data class Sample(
        /** 重力の向き(G・端末座標)。横向きの判定に使う */
        val gx: Double,
        val gy: Double,
        val gz: Double,
        /** 重力を除いた加速度の大きさ(G) */
        val acceleration: Double,
        /** 回転の速さ(rad/s) */
        val rotation: Double,
        val charging: Boolean,
        val at: Long,
    )

    private var since: Long? = null
    private var armed = true

    fun met(s: Sample): Boolean = conditions.all {
        when (it) {
            AutoStart.LANDSCAPE -> isLandscape(s.gx, s.gy)
            AutoStart.STILL -> s.acceleration < STILL_ACCELERATION && s.rotation < STILL_ROTATION
            AutoStart.CHARGING -> s.charging
            AutoStart.UNKNOWN -> false
        }
    }

    /** 1 サンプル。今始めるなら true。 */
    fun feed(s: Sample): Boolean {
        if (conditions.isEmpty()) return false
        if (!met(s)) {
            since = null
            armed = true
            return false
        }
        if (!armed) return false
        val first = since ?: s.at
        since = first
        if (s.at - first < HOLD) return false
        disarm()
        return true
    }

    /** 開始まであと何秒か(切り上げ・条件を満たしていない・待っていない時は null)。 */
    fun remainingSeconds(at: Long): Int? {
        val first = since ?: return null
        if (!armed) return null
        return ceil(max(0L, HOLD - (at - first)) / 1000.0).toInt()
    }

    /** 録画を始めた・手で止めた時。条件が一度外れるまで自動で始めない。 */
    fun disarm() {
        armed = false
        since = null
    }

    /** 裏から戻った時など、条件が続いていればまた始めてよい状態に戻す。 */
    fun rearm() {
        armed = true
        since = null
    }

    companion object {
        const val HOLD = 5_000L
        const val STILL_ACCELERATION = 0.06
        const val STILL_ROTATION = 0.3

        /** 横向き: 重力が端末の横(x)方向に寄っている(画面の向きのロックに関係なく、置いた姿勢で判定する)。 */
        fun isLandscape(gx: Double, gy: Double): Boolean = abs(gx) >= 0.5 && abs(gx) > abs(gy)
    }
}

/** 端末内の置き場のキー(アプリ名 → 英数字・FNV-1a 64bit)。iOS の `DashcamStoreKey` と同じ値。 */
object DashcamStoreKey {
    fun make(appName: String): String {
        var h = 0xcbf29ce484222325uL
        for (b in appName.toByteArray(Charsets.UTF_8)) {
            h = h xor b.toUByte().toULong()
            h *= 0x100000001b3uL
        }
        return h.toString(16)
    }
}

/** 表示の小物。 */
object DashcamText {
    /** 経過・長さ(1 時間未満は mm:ss)。 */
    fun clock(millis: Long): String {
        val s = max(0L, millis) / 1000
        return if (s >= 3600) "%d:%02d:%02d".format(s / 3600, s / 60 % 60, s % 60) else "%02d:%02d".format(s / 60, s % 60)
    }

    /** 残す長さ(分)。 */
    fun keepMinutes(config: DashcamConfig): Int = (config.keepMillis / 60_000L).toInt()

    /** 自動開始の条件(並びは landscape・still・charging)。 */
    fun orderedConditions(conditions: Set<AutoStart>): List<AutoStart> =
        listOf(AutoStart.LANDSCAPE, AutoStart.STILL, AutoStart.CHARGING).filter { it in conditions }
}
