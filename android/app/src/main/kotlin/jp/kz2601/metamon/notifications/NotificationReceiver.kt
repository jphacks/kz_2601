package jp.kz2601.metamon.notifications

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationManagerCompat

/**
 * 予約した時刻が来た(AlarmManager)・アラームを止めた・再起動 / 時刻の変更 / アプリの更新(OS)を受ける(#886)。
 * 鳴ったら出して、空いた枠と次の回を予約し直す。OS の知らせでは全件を置き直す(再起動で AlarmManager の予約は消える)。
 */
class NotificationReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val notifications = LocalNotifications.get(context)
        when (intent.action) {
            ACTION_FIRE -> {
                val id = intent.getStringExtra(LocalNotifications.EXTRA_IDENTIFIER) ?: return
                notifications.post(
                    identifier = id,
                    title = intent.getStringExtra(LocalNotifications.EXTRA_TITLE).orEmpty(),
                    body = intent.getStringExtra(LocalNotifications.EXTRA_BODY).orEmpty(),
                    alarm = intent.getBooleanExtra(LocalNotifications.EXTRA_ALARM, false),
                )
                notifications.refresh()
            }
            ACTION_STOP -> {
                val id = intent.getIntExtra(LocalNotifications.EXTRA_NOTIFICATION_ID, 0)
                NotificationManagerCompat.from(context).cancel(id)
            }
            Intent.ACTION_BOOT_COMPLETED,
            Intent.ACTION_TIME_CHANGED,
            Intent.ACTION_TIMEZONE_CHANGED,
            Intent.ACTION_MY_PACKAGE_REPLACED,
            ACTION_EXACT_ALARM_PERMISSION_CHANGED,
            -> notifications.refresh()
        }
    }

    companion object {
        const val ACTION_FIRE = "jp.kz2601.metamon.notifications.FIRE"
        const val ACTION_STOP = "jp.kz2601.metamon.notifications.STOP"
        private const val ACTION_EXACT_ALARM_PERMISSION_CHANGED = "android.app.action.SCHEDULE_EXACT_ALARM_PERMISSION_STATE_CHANGED"
    }
}
