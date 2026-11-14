package jp.kz2601.metamon.notifications

import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.renderer.components.play.TimerRun
import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.serializer

/** 予約中の終了通知(再起動・時刻の変更で置き直す)。 */
data class TimerAlarm(val endsAtMillis: Long, val title: String, val body: String)

/**
 * timerDisplay の走行状態と終了通知の予約(#1128・iOS の `TimerRunStore`)。キーは `<appId>/<画面>/<部品 id>`。
 * プロセスが終わっても残り、次に開いた時に残り時間を出し直す。アプリの削除・サインアウトで消す。
 */
class TimerRunStore(private val store: KeyValueStore) {
    fun load(key: String): TimerRun? = synchronized(this) {
        runs()[key]?.takeIf { it.size == 2 }?.let { TimerRun(it[0].toInt(), it[1].takeIf { s -> s >= 0 }) }
    }

    /** 初期状態(0 秒・停止)は保存せず消す。 */
    fun save(key: String, run: TimerRun) = synchronized(this) {
        val all = runs()
        saveRuns(if (run == TimerRun()) all - key else all + (key to listOf(run.base.toLong(), run.startedAtMillis ?: -1L)))
    }

    fun alarms(): Map<String, TimerAlarm> = synchronized(this) {
        rawAlarms().mapNotNull { (k, v) -> v.getOrNull(0)?.toLongOrNull()?.let { k to TimerAlarm(it, v.getOrElse(1) { "" }, v.getOrElse(2) { "" }) } }.toMap()
    }

    /** null なら取り消す。 */
    fun setAlarm(key: String, alarm: TimerAlarm?) = synchronized(this) {
        val all = rawAlarms()
        saveAlarms(if (alarm == null) all - key else all + (key to listOf(alarm.endsAtMillis.toString(), alarm.title, alarm.body)))
    }

    /** アプリを消した時: そのアプリの状態と予約を消す。取り消すべき予約のキーを返す。 */
    fun clearApp(appId: String): Set<String> = synchronized(this) {
        val prefix = appPrefix(appId)
        saveRuns(runs().filterKeys { !it.startsWith(prefix) })
        val alarms = rawAlarms()
        val gone = alarms.keys.filter { it.startsWith(prefix) }.toSet()
        saveAlarms(alarms - gone)
        gone
    }

    /** サインアウト・別の人(LocalDataWipe)。取り消すべき予約のキーを返す。 */
    fun clearAll(): Set<String> = synchronized(this) {
        val gone = rawAlarms().keys
        store.remove(KEY_RUNS)
        store.remove(KEY_ALARMS)
        gone
    }

    private fun runs(): Map<String, List<Long>> = decode(KEY_RUNS, runsSerializer)

    private fun saveRuns(m: Map<String, List<Long>>) = encode(KEY_RUNS, runsSerializer, m)

    private fun rawAlarms(): Map<String, List<String>> = decode(KEY_ALARMS, alarmsSerializer)

    private fun saveAlarms(m: Map<String, List<String>>) = encode(KEY_ALARMS, alarmsSerializer, m)

    private fun <T> decode(key: String, s: kotlinx.serialization.KSerializer<Map<String, T>>): Map<String, T> =
        store.getString(key)?.let { runCatching { DslJson.decodeFromString(s, it) }.getOrNull() }.orEmpty()

    private fun <T> encode(key: String, s: kotlinx.serialization.KSerializer<Map<String, T>>, m: Map<String, T>) =
        if (m.isEmpty()) store.remove(key) else store.putString(key, DslJson.encodeToString(s, m))

    companion object {
        const val KEY_RUNS = "timer.runs.v1"
        const val KEY_ALARMS = "timer.alarms.v1"
        private val runsSerializer = MapSerializer(String.serializer(), ListSerializer(Long.serializer()))
        private val alarmsSerializer = MapSerializer(String.serializer(), ListSerializer(String.serializer()))

        fun appPrefix(appId: String) = "$appId/"
    }
}

/** 終了通知を AlarmManager にどう置くか(#1128)。 */
object TimerAlarmPolicy {
    enum class Mode { EXACT, INEXACT }

    /**
     * 正確なアラームが許可されていれば正確に。無ければ「正確でない」アラーム(`setWindow`・最大 10 分遅れる・#354 §4)。
     * Android 12 未満は許可が要らない(minSdk 31 なので実際は来ない)。
     */
    fun mode(sdkInt: Int, canScheduleExact: Boolean): Mode =
        if (sdkInt < 31 || canScheduleExact) Mode.EXACT else Mode.INEXACT

    /** 予約するか(まだ先の時だけ)。null なら取り消す。 */
    fun scheduleAt(endsAtMillis: Long?, nowMillis: Long): Long? = endsAtMillis?.takeIf { it > nowMillis }
}

/** 前面で画面に出ているタイマー(iOS の `TimerVisibility`)。見えているタイマーの終了はバナーを出さない。 */
object TimerVisibility {
    private val visible = mutableMapOf<String, Int>()

    fun set(key: String, shown: Boolean) {
        synchronized(visible) {
            val n = visible[key] ?: 0
            if (shown) visible[key] = n + 1 else if (n > 1) visible[key] = n - 1 else visible.remove(key)
        }
    }

    fun isShowing(key: String): Boolean = synchronized(visible) { key in visible }
}
