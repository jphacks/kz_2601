package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.i18n.Strings
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/** 送信待ちの今の様子(SessionPersistence から取る)。 */
data class SyncSnapshot(
    /** 送り先がある(サインインしている)か。無ければ端末に残すだけで送らない。 */
    val isLive: Boolean,
    val pending: Int,
    /** 最後に 1 件送れた時刻。まだ送れていなければ null。 */
    val lastSentAtMillis: Long?,
    /** 続けて失敗して止まっているか(圏外など)。 */
    val paused: Boolean,
)

/**
 * 設定の「同期」の行の中身(Android の `SyncStatusRow` と同じ形)。
 * 文言はキー(Strings)で持ち、[SyncStatus.texts] で文字にする。
 */
data class SyncStatusRow(
    val labelKey: String,
    /** 文言の %1$d(件数を出さない状態は null) */
    val count: Int?,
    val tone: Tone,
    /** 送れない理由の一言(送れる時・待っているだけの時は null) */
    val blockerKey: String?,
    /** 「今送る」を押せるか(送り先があって、待ちがある時だけ) */
    val canSyncNow: Boolean,
) {
    /** 丸の色: 最新 = 緑・待ち = 橙・送れない = 灰 */
    enum class Tone { SYNCED, PENDING, OFFLINE }
}

/** 画面に出す文字。 */
data class SyncStatusTexts(val status: String, val lastSent: String, val blocker: String?, val canSyncNow: Boolean)

object SyncStatus {
    /** 状態を行にする。送り先が無ければ「サインインしていない」、止まっていれば「圏外など」。 */
    fun of(s: SyncSnapshot): SyncStatusRow = when {
        !s.isLive -> SyncStatusRow("desktop_sync_not_signed_in", null, SyncStatusRow.Tone.OFFLINE, "desktop_sync_blocker_signed_out", false)
        s.pending <= 0 -> SyncStatusRow("desktop_sync_synced", null, SyncStatusRow.Tone.SYNCED, null, false)
        s.paused -> SyncStatusRow("desktop_sync_offline_pending", s.pending, SyncStatusRow.Tone.OFFLINE, "desktop_sync_blocker_offline", true)
        else -> SyncStatusRow("desktop_sync_pending", s.pending, SyncStatusRow.Tone.PENDING, null, true)
    }

    /** 最後に送れた時刻の言い方: 今日は「14:05」、別の日は「11/12 14:05」、まだなら「まだ送っていません」。 */
    fun lastSentText(
        lastSentAtMillis: Long?,
        nowMillis: Long = System.currentTimeMillis(),
        zone: ZoneId = ZoneId.systemDefault(),
        locale: Locale = Strings.locale,
        format: (String, Array<out Any?>) -> String = { k, a -> Strings.format(k, *a) },
    ): String {
        if (lastSentAtMillis == null) return format("desktop_sync_never", emptyArray<Any?>())
        val at = Instant.ofEpochMilli(lastSentAtMillis).atZone(zone)
        val today = LocalDate.ofInstant(Instant.ofEpochMilli(nowMillis), zone)
        val pattern = if (at.toLocalDate() == today) "HH:mm" else "M/d HH:mm"
        return format("desktop_sync_last_sent", arrayOf(DateTimeFormatter.ofPattern(pattern, locale).format(at)))
    }

    fun texts(
        s: SyncSnapshot,
        nowMillis: Long = System.currentTimeMillis(),
        zone: ZoneId = ZoneId.systemDefault(),
    ): SyncStatusTexts {
        val row = of(s)
        val status = if (row.count != null) Strings.format(row.labelKey, row.count) else Strings[row.labelKey]
        return SyncStatusTexts(
            status = status,
            lastSent = lastSentText(s.lastSentAtMillis, nowMillis, zone),
            blocker = row.blockerKey?.let { Strings[it] },
            canSyncNow = row.canSyncNow,
        )
    }
}
