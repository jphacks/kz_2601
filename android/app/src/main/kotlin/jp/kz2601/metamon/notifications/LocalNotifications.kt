package jp.kz2601.metamon.notifications

import android.Manifest
import android.app.AlarmManager
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.media.AudioAttributes
import android.media.RingtoneManager
import android.net.Uri
import android.os.Build
import android.text.format.DateFormat
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.notifications.AlarmPlanner
import jp.kz2601.metamon.services.notifications.ConditionNotifications
import jp.kz2601.metamon.services.notifications.NotificationPlanner
import jp.kz2601.metamon.services.notifications.NotificationRegistry
import jp.kz2601.metamon.services.notifications.PlannedNotification
import jp.kz2601.metamon.services.notifications.ScheduleDiff
import jp.kz2601.metamon.services.notifications.isConditional
import jp.kz2601.metamon.services.notifications.recordEntity
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.serializer
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.util.Locale

/**
 * 生成アプリのローカル通知(#886・iOS の UserNotificationScheduler)。
 * - アプリごとの予約を SharedPreferences に持ち、全アプリ分から近い順に [NotificationPlanner.LIMIT] 件を AlarmManager に渡す。
 * - AlarmManager は 1 回きりなので、鳴った時・再起動・時刻の変更・アプリの更新の後に [refresh] で次の回を予約し直す。
 * - 時刻は「正確でない」アラーム(`setWindow`・#354 §4)。アラーム(`alarm: true`)だけ、正確なアラームが許可されていれば正確に鳴らす。
 * - 場所の通知(near)は計画と登録までで、見張り(Geofencing)はまだ無い(子 Issue)。
 * 計画の中身は :services の純粋関数(JVM テスト)。ここは Android の部品に渡すだけ。
 */
class LocalNotifications private constructor(private val app: Context) {
    private val prefs = app.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    private val alarms = app.getSystemService(AlarmManager::class.java)
    private val zone: ZoneId get() = FieldCoding.zone

    /** `app.notifications[]` をそのアプリ分として置き換えて予約し直す。 */
    fun sync(appId: String, specs: List<Notification>, records: Map<String, List<Record>>) = synchronized(this) {
        save(registry().set(appId, NotificationPlanner.plan(appId, specs, records, Instant.now(), zone)))
        applyNow()
    }

    /** scheduleNotification(ボタン)。次に鳴る時刻、予約できなければ null。 */
    fun schedule(spec: Notification, appId: String): Instant? = synchronized(this) {
        val now = Instant.now()
        val p = NotificationPlanner.action(appId, spec, now, zone) ?: return null
        save(registry().upsertAction(appId, p))
        applyNow()
        p.nextFire(now, zone)
    }

    fun cancel(id: String, appId: String) = synchronized(this) {
        save(registry().remove(appId, id))
        applyNow()
    }

    /** 予約し直すだけ(鳴った後・再起動・時刻の変更)。 */
    fun refresh() = synchronized(this) { applyNow() }

    /**
     * DSL の読み込み・記録の変化の後。時刻の通知を予約し直し、条件の通知を評価する。
     * 読み込み前(記録の無い entity がある)は評価しない(空の記録で前回を覚えて誤って鳴らさない)。
     */
    fun onRecordsChanged(appId: String, dsl: AppDSL, records: Map<String, List<Record>>) {
        val specs = dsl.app.notifications
        if (specs.mapNotNull { it.recordEntity }.all { it in records }) sync(appId, specs, records)
        val conditional = specs.filter { it.isConditional }
        if (conditional.isEmpty() || dsl.entities.any { it.id !in records }) return
        val fire = synchronized(this) {
            val table = conditions()
            val (fire, next) = ConditionNotifications.transition(table[appId].orEmpty(), ConditionNotifications.evaluate(conditional, dsl, records))
            saveConditions(if (next.isEmpty()) table - appId else table + (appId to next))
            fire
        }
        for (spec in conditional.filter { it.id in fire }) {
            post(ConditionNotifications.identifier(appId, spec.id), spec.title, spec.body, alarm = false, channel = CHANNEL_CONDITIONS)
        }
    }

    fun removeApp(appId: String) = synchronized(this) {
        save(registry().removeApp(appId))
        saveConditions(conditions() - appId)
        applyNow()
    }

    /** 鳴った通知を出す。許可が無ければ出さずに false。[appId] があれば押した時にそのアプリを開く(#887)。 */
    fun post(
        identifier: String,
        title: String,
        body: String,
        alarm: Boolean,
        channel: String = if (alarm) CHANNEL_ALARMS else CHANNEL_REMINDERS,
        appId: String? = null,
    ): Boolean {
        if (!canPost()) return false
        ensureChannels()
        val id = identifier.hashCode()
        val launch = app.packageManager.getLaunchIntentForPackage(app.packageName)
            ?.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
            ?.putExtra(EXTRA_IDENTIFIER, identifier)
            ?.apply { appId?.let { putExtra(EXTRA_APP_ID, it) } }
        val builder = NotificationCompat.Builder(app, channel)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(title.ifEmpty { null })
            .setContentText(body.ifEmpty { null })
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setAutoCancel(true)
            .setPriority(if (alarm) NotificationCompat.PRIORITY_MAX else NotificationCompat.PRIORITY_DEFAULT)
        launch?.let { builder.setContentIntent(PendingIntent.getActivity(app, id, it, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)) }
        if (alarm) {
            // 全画面の通知(USE_FULL_SCREEN_INTENT)は Play の制約で使わない。止めるまで音をくり返す通知にする
            builder.setCategory(NotificationCompat.CATEGORY_ALARM)
                .setContentTitle(AlarmPlanner.title(PlannedNotification(identifier, title, body, PlannedNotification.Trigger.At(0))))
                .addAction(0, app.getString(R.string.notification_alarm_stop), stopIntent(id))
                .setDeleteIntent(stopIntent(id))
        }
        val n = builder.build()
        if (alarm) n.flags = n.flags or android.app.Notification.FLAG_INSISTENT
        return runCatching { NotificationManagerCompat.from(app).notify(id, n) }.isSuccess
    }

    /** 「明日 9:00」の言い方(今日なら時刻だけ)。 */
    fun whenText(at: Instant, now: Instant = Instant.now()): String {
        val today = LocalDate.ofInstant(at, zone) == LocalDate.ofInstant(now, zone)
        val locale = app.resources.configuration.locales[0] ?: Locale.getDefault()
        val pattern = DateFormat.getBestDateTimePattern(locale, if (today) "Hm" else "MdEEEHm")
        return java.time.format.DateTimeFormatter.ofPattern(pattern, locale).withZone(zone).format(at)
    }

    // 予約し直し

    private fun applyNow() {
        val now = Instant.now()
        val registry = registry().pruned(now, zone).also(::save)
        val diff = ScheduleDiff.of(NotificationPlanner.select(registry.all, now, zone), scheduled())
        diff.cancel.forEach(::cancelAlarm)
        // 全件を置き直す(同じ PendingIntent は上書き。再起動・時刻の変更で消えた分もここで戻る)
        val placed = diff.schedule.filter { (p, at) -> setAlarm(p, at) }.map { it.first.identifier }.toSet()
        prefs.edit().putStringSet(KEY_SCHEDULED, placed).apply()
        Log.i(TAG, "notifications scheduled: total=${placed.size} cancelled=${diff.cancel.size} next=${diff.schedule.firstOrNull()?.second ?: "-"}")
    }

    private fun setAlarm(p: PlannedNotification, at: Instant): Boolean {
        val pi = PendingIntent.getBroadcast(app, 0, fireIntent(p), PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val ms = at.toEpochMilli()
        return runCatching {
            if (p.isAlarm && alarms.canScheduleExactAlarms()) {
                alarms.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, ms, pi)
            } else {
                alarms.setWindow(AlarmManager.RTC_WAKEUP, ms, WINDOW_MILLIS, pi)
            }
        }.onFailure { Log.w(TAG, "alarm failed id=${p.identifier}: $it") }.isSuccess
    }

    private fun cancelAlarm(identifier: String) {
        val intent = Intent(app, NotificationReceiver::class.java).setAction(NotificationReceiver.ACTION_FIRE).setData(dataUri(identifier))
        PendingIntent.getBroadcast(app, 0, intent, PendingIntent.FLAG_NO_CREATE or PendingIntent.FLAG_IMMUTABLE)?.let {
            alarms.cancel(it)
            it.cancel()
        }
    }

    private fun fireIntent(p: PlannedNotification) = Intent(app, NotificationReceiver::class.java)
        .setAction(NotificationReceiver.ACTION_FIRE)
        .setData(dataUri(p.identifier))
        .putExtra(EXTRA_IDENTIFIER, p.identifier)
        .putExtra(EXTRA_TITLE, p.title)
        .putExtra(EXTRA_BODY, p.body)
        .putExtra(EXTRA_ALARM, p.isAlarm)

    private fun stopIntent(notificationId: Int): PendingIntent = PendingIntent.getBroadcast(
        app,
        notificationId,
        Intent(app, NotificationReceiver::class.java).setAction(NotificationReceiver.ACTION_STOP).putExtra(EXTRA_NOTIFICATION_ID, notificationId),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )

    fun canPost(): Boolean {
        if (!NotificationManagerCompat.from(app).areNotificationsEnabled()) return false
        return Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            app.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
    }

    /** チャンネルを作る(何度呼んでも良い)。FCM が自分で出す通知もこのチャンネルに入るので起動時にも呼ぶ。 */
    fun ensureChannels() {
        val manager = app.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannels(
            listOf(
                NotificationChannel(CHANNEL_REMINDERS, app.getString(R.string.notification_channel_reminders), NotificationManager.IMPORTANCE_DEFAULT)
                    .apply { description = app.getString(R.string.notification_channel_reminders_description) },
                NotificationChannel(CHANNEL_CONDITIONS, app.getString(R.string.notification_channel_conditions), NotificationManager.IMPORTANCE_DEFAULT)
                    .apply { description = app.getString(R.string.notification_channel_conditions_description) },
                NotificationChannel(CHANNEL_OTHERS, app.getString(R.string.push_channel_others), NotificationManager.IMPORTANCE_DEFAULT)
                    .apply { description = app.getString(R.string.push_channel_others_description) },
                NotificationChannel(CHANNEL_ALARMS, app.getString(R.string.notification_channel_alarms), NotificationManager.IMPORTANCE_HIGH).apply {
                    description = app.getString(R.string.notification_channel_alarms_description)
                    setSound(
                        RingtoneManager.getDefaultUri(RingtoneManager.TYPE_ALARM),
                        AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_ALARM).setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION).build(),
                    )
                },
            ),
        )
    }

    // 保存

    private fun registry(): NotificationRegistry = prefs.getString(KEY_REGISTRY, null)
        ?.let { runCatching { DslJson.decodeFromString(NotificationRegistry.serializer(), it) }.getOrNull() } ?: NotificationRegistry()

    private fun save(r: NotificationRegistry) = prefs.edit().putString(KEY_REGISTRY, DslJson.encodeToString(NotificationRegistry.serializer(), r)).apply()

    private fun scheduled(): Set<String> = prefs.getStringSet(KEY_SCHEDULED, null).orEmpty()

    private val conditionSerializer = MapSerializer(String.serializer(), MapSerializer(String.serializer(), Boolean.serializer()))

    private fun conditions(): Map<String, Map<String, Boolean>> = prefs.getString(KEY_CONDITIONS, null)
        ?.let { runCatching { DslJson.decodeFromString(conditionSerializer, it) }.getOrNull() }.orEmpty()

    private fun saveConditions(t: Map<String, Map<String, Boolean>>) =
        prefs.edit().putString(KEY_CONDITIONS, DslJson.encodeToString(conditionSerializer, t)).apply()

    companion object {
        private const val TAG = "MetamonNotif"
        private const val PREFS = "notifications"
        private const val KEY_REGISTRY = "planned.v1"
        private const val KEY_SCHEDULED = "scheduled.v1"
        private const val KEY_CONDITIONS = "when.v1"

        /** 正確でないアラームの幅。Android 14 以降は 10 分より短くできない(#351 の 3: 数分のずれは許す)。 */
        private const val WINDOW_MILLIS = 10 * 60 * 1000L

        const val CHANNEL_REMINDERS = "reminders"
        const val CHANNEL_CONDITIONS = "conditions"
        const val CHANNEL_ALARMS = "alarms"
        /** 他の人の行動で届く通知(FCM・#887)。AndroidManifest の既定のチャンネルと同じ値 */
        const val CHANNEL_OTHERS = "others"

        const val EXTRA_IDENTIFIER = "metamon.notification.identifier"
        const val EXTRA_TITLE = "metamon.notification.title"
        const val EXTRA_BODY = "metamon.notification.body"
        const val EXTRA_ALARM = "metamon.notification.alarm"
        const val EXTRA_NOTIFICATION_ID = "metamon.notification.id"
        /** 押した時に開くアプリ。FCM が自分で出した通知は data のキー(`appId`)がそのまま extra に入る */
        const val EXTRA_APP_ID = "appId"

        private fun dataUri(identifier: String): Uri = Uri.Builder().scheme("metamon-notification").opaquePart(identifier).build()

        @Volatile private var instance: LocalNotifications? = null

        fun get(context: Context): LocalNotifications =
            instance ?: synchronized(this) { instance ?: LocalNotifications(context.applicationContext).also { instance = it } }
    }
}
