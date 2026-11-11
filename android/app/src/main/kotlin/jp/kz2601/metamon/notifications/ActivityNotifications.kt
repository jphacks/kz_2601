package jp.kz2601.metamon.notifications

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import jp.kz2601.metamon.R

/**
 * 進行中の表示(#1030・iOS の Live Activity)を常駐の通知で出す。中身と出し分けは [ActivityNotificationPlanner]。
 * - 出ている間はスワイプで消えない(ongoing)。押すとそのアプリを開く。
 * - 時間は通知のクロノメーターが描くので、アプリが止まっていても進む。
 * - 通知の許可が無ければ出さない(許可は [NotificationsFirstAsk] が 1 回だけ聞く)。
 */
class ActivityNotifications private constructor(private val appContext: Context) {
    private val manager = appContext.getSystemService(NotificationManager::class.java)

    /** 出す / 差し替える。許可が無く出せなければ false。 */
    fun show(content: ActivityContent): Boolean = synchronized(this) {
        if (!LocalNotifications.get(appContext).canPost()) return false
        ensureChannel()
        val plan = ActivityNotificationPlanner.start(content, activeTags())
        return try {
            NotificationManagerCompat.from(appContext).notify(plan.tag, NOTIFICATION_ID, build(plan))
            Log.i(TAG, "activity ${if (plan.update) "updated" else "started"} tag=${plan.tag}")
            true
        } catch (_: SecurityException) {
            false // canPost() の後に許可を外された
        } catch (_: RuntimeException) {
            false
        }
    }

    fun end(appId: String) = synchronized(this) { cancel(ActivityNotificationPlanner.end(appId, activeTags())) }

    /** アプリを消した時。 */
    fun removeApp(appId: String) = end(appId)

    /** サインアウト・別の人(LocalDataWipe)。 */
    fun removeAll() = synchronized(this) { cancel(ActivityNotificationPlanner.wipe(activeTags())) }

    private fun cancel(tags: List<String>) {
        val nm = NotificationManagerCompat.from(appContext)
        tags.forEach { runCatching { nm.cancel(it, NOTIFICATION_ID) } }
    }

    private fun activeTags(): Set<String> = runCatching {
        manager.activeNotifications.filter { it.id == NOTIFICATION_ID }.mapNotNull { it.tag }.toSet()
    }.getOrDefault(emptySet())

    private fun build(plan: ActivityNotificationPlanner.Post): android.app.Notification {
        val c = plan.content
        val builder = NotificationCompat.Builder(appContext, CHANNEL)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(c.title)
            .setSubText(c.appName.ifEmpty { null })
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setCategory(NotificationCompat.CATEGORY_PROGRESS)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .setShowWhen(true)
            .setUsesChronometer(true)
            .setTimeoutAfter((c.timeoutAtMillis - System.currentTimeMillis()).coerceAtLeast(1L))
        when (val clock = c.clock) {
            is ActivityContent.Clock.Countdown -> builder.setWhen(clock.endsAtMillis).setChronometerCountDown(true)
            is ActivityContent.Clock.Elapsed -> builder.setWhen(clock.sinceMillis)
        }
        c.body?.let { builder.setContentText(it).setStyle(NotificationCompat.BigTextStyle().bigText(it)) }
        c.progressPercent?.let { builder.setProgress(100, it, false) }
        launchIntent(c.appId, plan.tag)?.let(builder::setContentIntent)
        return builder.build()
    }

    /** 押したらそのアプリを開く(LocalNotifications と同じ extra を MainActivity が読む)。 */
    private fun launchIntent(appId: String, tag: String): PendingIntent? {
        val launch = appContext.packageManager.getLaunchIntentForPackage(appContext.packageName) ?: return null
        launch.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
            .putExtra(LocalNotifications.EXTRA_IDENTIFIER, tag)
            .putExtra(LocalNotifications.EXTRA_APP_ID, appId)
        return PendingIntent.getActivity(appContext, tag.hashCode(), launch, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
    }

    private fun ensureChannel() {
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL, appContext.getString(R.string.notification_channel_activities), NotificationManager.IMPORTANCE_LOW)
                .apply {
                    description = appContext.getString(R.string.notification_channel_activities_description)
                    setShowBadge(false)
                },
        )
    }

    companion object {
        private const val TAG = "MetamonActivity"
        const val CHANNEL = "activities"
        /** tag(`activity:<appId>`)で見分けるので id は 1 つ */
        const val NOTIFICATION_ID = 1030

        @Volatile private var instance: ActivityNotifications? = null

        fun get(context: Context): ActivityNotifications =
            instance ?: synchronized(this) { instance ?: ActivityNotifications(context.applicationContext).also { instance = it } }
    }
}
