package jp.kz2601.metamon.notifications

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import jp.kz2601.metamon.R

/**
 * 進行中の表示(#1030・#913・iOS の Live Activity)を常駐の通知で出す。中身と出し分けは [ActivityNotificationPlanner]。
 * - 出ている間はスワイプで消えない(ongoing)。押すとそのアプリを開く。
 * - 時間は通知のクロノメーターが描くので、アプリが止まっていても進む。
 * - Android 16(API 36)以降は Live Updates(ProgressStyle + promoted ongoing)で画面の上に出す。それ未満は ongoing の通知のまま。
 * - 通知の許可が無ければ出さない(許可は [NotificationsFirstAsk] が 1 回だけ聞く)。
 */
class ActivityNotifications private constructor(private val appContext: Context) {
    private val manager = appContext.getSystemService(NotificationManager::class.java)

    /** 出す / 差し替える。許可が無く出せなければ false。 */
    fun show(content: ActivityContent): Boolean = synchronized(this) { post(content) }

    /**
     * `timerDisplay.liveActivity`: 開始・一時停止・リセット・終了を 1 つの通知に映す(iOS の `timerActivity`)。
     * 許可が無く出せなければ false。
     */
    fun timer(update: TimerActivityUpdate): Boolean = synchronized(this) {
        val labels = TimerActivityLabels(
            paused = appContext.getString(R.string.activity_timer_paused),
            finished = appContext.getString(R.string.activity_timer_finished),
        )
        when (val plan = ActivityNotificationPlanner.timerPlan(update, labels, System.currentTimeMillis())) {
            is ActivityNotificationPlanner.TimerPlan.Show -> post(plan.content)
            is ActivityNotificationPlanner.TimerPlan.Finish -> post(plan.content)
            ActivityNotificationPlanner.TimerPlan.End -> {
                cancel(listOf(ActivityNotificationPlanner.timerTag(update.key)))
                true
            }
        }
    }

    /** そのタイマーの表示を消す(終了のアラームが鳴った時・#913)。 */
    fun timerEnded(key: String) = synchronized(this) { cancel(listOf(ActivityNotificationPlanner.timerTag(key))) }

    fun end(appId: String) = synchronized(this) { cancel(ActivityNotificationPlanner.end(appId, activeTags())) }

    /** アプリを消した時: startActivity の表示とそのアプリのタイマーの表示を全部。 */
    fun removeApp(appId: String) = synchronized(this) { cancel(ActivityNotificationPlanner.forApp(appId, activeTags())) }

    /** サインアウト・別の人(LocalDataWipe)。 */
    fun removeAll() = synchronized(this) { cancel(ActivityNotificationPlanner.wipe(activeTags())) }

    private fun post(content: ActivityContent): Boolean {
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
            .setOngoing(!c.finished)
            .setAutoCancel(c.finished)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setCategory(NotificationCompat.CATEGORY_PROGRESS)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .setTimeoutAfter((c.timeoutAtMillis - System.currentTimeMillis()).coerceAtLeast(1L))
        when (val clock = c.clock) {
            // 残り / 経過は OS の時計が描く(アプリが止まっていても進む)
            is ActivityContent.Clock.Countdown ->
                builder.setShowWhen(true).setWhen(clock.endsAtMillis).setUsesChronometer(true).setChronometerCountDown(true)
            is ActivityContent.Clock.Elapsed -> builder.setShowWhen(true).setWhen(clock.sinceMillis).setUsesChronometer(true)
            // 止まっている間は時計を動かさない(秒は本文に出す)
            is ActivityContent.Clock.Paused -> builder.setShowWhen(false).setUsesChronometer(false)
        }
        c.body?.let(builder::setContentText)
        if (liveUpdates) progressStyle(builder, c) else legacyProgress(builder, c)
        launchIntent(c.appId, plan.tag)?.let(builder::setContentIntent)
        return builder.build()
    }

    /** Android 16 の Live Updates: ProgressStyle と promoted ongoing で画面の上(ステータスバー)に出す。 */
    private fun progressStyle(builder: NotificationCompat.Builder, c: ActivityContent) {
        val percent = c.progressPercent
        builder.setStyle(
            NotificationCompat.ProgressStyle()
                .setProgress(percent ?: 0)
                .setProgressIndeterminate(percent == null),
        )
        builder.setRequestPromotedOngoing(!c.finished)
        c.value?.let(builder::setShortCriticalText)
    }

    private fun legacyProgress(builder: NotificationCompat.Builder, c: ActivityContent) {
        c.body?.let { builder.setStyle(NotificationCompat.BigTextStyle().bigText(it)) }
        c.progressPercent?.let { builder.setProgress(100, it, false) }
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
        /** tag(`activity:<appId>` / `activity:timer:<key>`)で見分けるので id は 1 つ */
        const val NOTIFICATION_ID = 1030

        /** Android 16(API 36)以降は Live Updates が使える */
        private val liveUpdates: Boolean get() = Build.VERSION.SDK_INT >= 36

        @Volatile private var instance: ActivityNotifications? = null

        fun get(context: Context): ActivityNotifications =
            instance ?: synchronized(this) { instance ?: ActivityNotifications(context.applicationContext).also { instance = it } }
    }
}
