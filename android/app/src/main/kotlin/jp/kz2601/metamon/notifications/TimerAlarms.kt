package jp.kz2601.metamon.notifications

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.util.Log
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.components.play.TimerRun
import jp.kz2601.metamon.services.consent.SharedPreferencesKeyValueStore

/**
 * timerDisplay(countdown)の終了通知と走行状態(#1128・iOS の `TimerNotifier` + `TimerRunStore`)。
 * - 開始で終了時刻に AlarmManager を置き、一時停止・リセットで取り消す。
 * - 正確なアラームが許可されていれば正確に、無ければ最大 10 分遅れる([TimerAlarmPolicy])。案内は [ExactAlarmFirstAsk]。
 * - 予約は [TimerRunStore] に残し、再起動・時刻の変更で [refresh] が置き直す。
 */
class TimerAlarms private constructor(private val appContext: Context) {
    val store = TimerRunStore(SharedPreferencesKeyValueStore.of(appContext, PREFS))
    private val alarms = appContext.getSystemService(AlarmManager::class.java)

    fun changed(key: String, appName: String, timerName: String, endsAtMillis: Long?) = synchronized(this) {
        val at = TimerAlarmPolicy.scheduleAt(endsAtMillis, System.currentTimeMillis())
        if (at == null) {
            store.setAlarm(key, null)
            cancel(key)
            return@synchronized
        }
        val alarm = TimerAlarm(at, appName, appContext.getString(R.string.timer_finished_body, timerName))
        store.setAlarm(key, alarm)
        place(key, alarm)
    }

    fun load(key: String): TimerRun? = store.load(key)

    fun save(key: String, run: TimerRun) = store.save(key, run)

    /** 鳴った: 予約を消し、そのタイマーが前面で見えていなければ通知を出す。 */
    fun fired(key: String) {
        val alarm = synchronized(this) { store.alarms()[key].also { store.setAlarm(key, null) } } ?: return
        if (TimerVisibility.isShowing(key)) return
        // 進行中の表示(liveActivity)は 0 を過ぎたら消す(#913)。終わったことはこの後の通知が伝える
        ActivityNotifications.get(appContext).timerEnded(key)
        LocalNotifications.get(appContext).post(
            identifier = IDENTIFIER_PREFIX + key,
            title = alarm.title,
            body = alarm.body,
            alarm = false,
            appId = key.substringBefore('/'),
        )
    }

    /** 再起動・時刻の変更・正確なアラームの許可が変わった時に置き直す(過ぎた分はすぐ鳴る)。 */
    fun refresh() = synchronized(this) { store.alarms().forEach { (k, a) -> place(k, a) } }

    fun removeApp(appId: String) = synchronized(this) { store.clearApp(appId).forEach(::cancel) }

    fun removeAll() = synchronized(this) { store.clearAll().forEach(::cancel) }

    private fun place(key: String, alarm: TimerAlarm) {
        val pi = PendingIntent.getBroadcast(appContext, 0, intent(key), PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val mode = TimerAlarmPolicy.mode(Build.VERSION.SDK_INT, alarms.canScheduleExactAlarms())
        runCatching {
            when (mode) {
                TimerAlarmPolicy.Mode.EXACT -> alarms.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, alarm.endsAtMillis, pi)
                // 正確なアラームが許可されていない: 最大 10 分遅れる(Android 14 以降は窓を 10 分より短くできない)
                TimerAlarmPolicy.Mode.INEXACT -> alarms.setWindow(AlarmManager.RTC_WAKEUP, alarm.endsAtMillis, WINDOW_MILLIS, pi)
            }
        }.onSuccess { Log.i(TAG, "timer alarm key=$key mode=$mode at=${alarm.endsAtMillis}") }
            .onFailure { Log.w(TAG, "timer alarm failed key=$key: $it") }
    }

    private fun cancel(key: String) {
        PendingIntent.getBroadcast(appContext, 0, intent(key), PendingIntent.FLAG_NO_CREATE or PendingIntent.FLAG_IMMUTABLE)?.let {
            alarms.cancel(it)
            it.cancel()
        }
    }

    private fun intent(key: String) = Intent(appContext, NotificationReceiver::class.java)
        .setAction(NotificationReceiver.ACTION_TIMER)
        .setData(Uri.Builder().scheme("metamon-timer").opaquePart(key).build())
        .putExtra(EXTRA_KEY, key)

    companion object {
        private const val TAG = "MetamonTimer"
        const val PREFS = "timers"
        const val EXTRA_KEY = "metamon.timer.key"
        /** 通知の識別子(DSL の通知と分ける。iOS の `timer.`) */
        const val IDENTIFIER_PREFIX = "timer."
        private const val WINDOW_MILLIS = 10 * 60 * 1000L

        @Volatile private var instance: TimerAlarms? = null

        fun get(context: Context): TimerAlarms =
            instance ?: synchronized(this) { instance ?: TimerAlarms(context.applicationContext).also { instance = it } }
    }
}
