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
import androidx.core.content.edit
import com.google.android.gms.location.Geofence
import com.google.android.gms.location.GeofencingRequest
import com.google.android.gms.location.LocationServices
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.notifications.AlarmPlanner
import jp.kz2601.metamon.services.notifications.ConditionNotifications
import jp.kz2601.metamon.services.notifications.GeofencePlanner
import jp.kz2601.metamon.services.notifications.NotificationPlanner
import jp.kz2601.metamon.services.notifications.NotificationRegistry
import jp.kz2601.metamon.services.notifications.PlannedNotification
import jp.kz2601.metamon.services.notifications.RegionOnceState
import jp.kz2601.metamon.services.notifications.ScheduleDiff
import jp.kz2601.metamon.services.notifications.isConditional
import jp.kz2601.metamon.services.notifications.recordEntity
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.SetSerializer
import kotlinx.serialization.builtins.serializer
import java.time.Instant
import java.time.ZoneId
import java.util.Locale

/**
 * 生成アプリのローカル通知(#886・iOS の UserNotificationScheduler)。
 * - アプリごとの予約を SharedPreferences に持ち、全アプリ分から近い順に [NotificationPlanner.LIMIT] 件を AlarmManager に渡す。
 * - AlarmManager は 1 回きりなので、鳴った時・再起動・時刻の変更・アプリの更新の後に [refresh] で次の回を予約し直す。
 * - 時刻は「正確でない」アラーム(`setWindow`・#354 §4)。アラーム(`alarm: true`)だけ、正確なアラームが許可されていれば正確に鳴らす。
 * - 場所の通知(near)は Geofencing で見張る([GeofencePlanner])。正確な位置と裏での位置が許可されている時だけ。
 *   着いたら [NotificationReceiver] が受けて出す。1 回きりは届いたら見張りから外す。再起動で消えるので [refresh] で置き直す。
 * 計画の中身は :services の純粋関数(JVM テスト)。ここは Android の部品に渡すだけ。
 */
class LocalNotifications private constructor(private val appContext: Context) {
    private val prefs = appContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    private val alarms = appContext.getSystemService(AlarmManager::class.java)
    private val geofencing by lazy { LocationServices.getGeofencingClient(appContext) }
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
     * 未読み込みの entity は呼ぶ側が端末内 DB で埋め、読めなかった entity は外して渡す(#1124・`NotificationRecords`)。
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
        saveRegionState(regionState().removeApp(appId))
        applyNow()
    }

    /**
     * 全アプリの予約・条件の前回値・出ている通知を消す(サインアウト・削除・別の人のサインイン・#841)。
     * iOS の `notifications.removeAll()` + `removeAllPending/DeliveredNotifications`。消さないと前の人の通知が鳴り、再起動でも予約し直される。
     */
    fun removeAll() = synchronized(this) {
        (scheduled() + registry().all.map { it.identifier }).forEach(::cancelAlarm)
        runCatching { geofencing.removeGeofences(geofenceIntent()) }
        // 次の人に残さないよう、すぐ書く
        prefs.edit(commit = true) {
            remove(KEY_REGISTRY).remove(KEY_SCHEDULED).remove(KEY_CONDITIONS).remove(KEY_REGIONS).remove(KEY_GEOFENCES)
        }
        runCatching { NotificationManagerCompat.from(appContext).cancelAll() }
        Unit
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
        val launch = appContext.packageManager.getLaunchIntentForPackage(appContext.packageName)
            ?.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
            ?.putExtra(EXTRA_IDENTIFIER, identifier)
            ?.apply { appId?.let { putExtra(EXTRA_APP_ID, it) } }
        val builder = NotificationCompat.Builder(appContext, channel)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(title.ifEmpty { null })
            .setContentText(body.ifEmpty { null })
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setAutoCancel(true)
            .setPriority(if (alarm) NotificationCompat.PRIORITY_MAX else NotificationCompat.PRIORITY_DEFAULT)
        launch?.let { builder.setContentIntent(PendingIntent.getActivity(appContext, id, it, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)) }
        if (alarm) {
            // 全画面の通知(USE_FULL_SCREEN_INTENT)は Play の制約で使わない。止めるまで音をくり返す通知にする
            builder.setCategory(NotificationCompat.CATEGORY_ALARM)
                .setContentTitle(AlarmPlanner.title(PlannedNotification(identifier, title, body, PlannedNotification.Trigger.At(0))))
                .addAction(0, appContext.getString(R.string.notification_alarm_stop), stopIntent(id))
                .setDeleteIntent(stopIntent(id))
        }
        val n = builder.build()
        if (alarm) n.flags = n.flags or android.app.Notification.FLAG_INSISTENT
        return try {
            NotificationManagerCompat.from(appContext).notify(id, n)
            true
        } catch (_: SecurityException) {
            false // canPost() の後に許可を外された
        } catch (_: RuntimeException) {
            false
        }
    }

    /**
     * 場所に着いた(Geofencing の ENTER)。着いた場所の通知を出し、1 回きりは見張りから外して予約し直す。
     * [requestIds] は OS の requestId([GeofencePlanner.requestId])。
     */
    fun onArrived(requestIds: List<String>) = synchronized(this) {
        val hits = GeofencePlanner.arrived(requestIds, registry().all)
        for (p in hits) post(p.identifier, p.title, p.body, alarm = false)
        val once = hits.filterNot(GeofencePlanner::keepsWatching).map { it.identifier }
        if (once.isNotEmpty()) {
            saveGeofences(geofences() - once.toSet())
            runCatching { geofencing.removeGeofences(once.map(GeofencePlanner::requestId)) }
        }
        applyNow()
    }

    /** このアプリに場所の通知があるのに、位置が許可されていないので届かない(iOS の placesLocationOff)。 */
    fun placesLocationOff(appId: String): Boolean = synchronized(this) {
        GeofencePlanner.plan(registry().all, regionState(), geofences(), canWatchPlaces(), appId).locationOff
    }

    /** このアプリの次の通知と見張っている場所の数(設定の「次の通知」・#1124・iOS の `NotificationSyncResult`)。 */
    fun upcoming(appId: String): UpcomingNotificationList = synchronized(this) {
        val registry = registry()
        val armed = geofences()
        UpcomingNotificationList.of(
            registry.upcoming(appId, Instant.now(), zone).map { (p, at) -> UpcomingNotification(p.title, p.body, at) },
            places = registry.planned(appId).count { it.isRegion && it.identifier in armed },
            locationOff = placesLocationOff(appId),
        )
    }

    /** Geofencing に要る許可: 正確な位置と裏での位置(10+)。おおよそだけでは見張れない。 */
    fun canWatchPlaces(): Boolean =
        isGranted(Manifest.permission.ACCESS_FINE_LOCATION) && isGranted(Manifest.permission.ACCESS_BACKGROUND_LOCATION)

    private fun isGranted(name: String) = appContext.checkSelfPermission(name) == PackageManager.PERMISSION_GRANTED

    /** 「明日 9:00」の言い方(今日なら時刻だけ)。 */
    fun whenText(at: Instant, now: Instant = Instant.now()): String {
        val today = at.atZone(zone).toLocalDate() == now.atZone(zone).toLocalDate()
        val locale = appContext.resources.configuration.locales[0] ?: Locale.getDefault()
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
        val regions = applyPlaces(registry.all)
        Log.i(TAG, "notifications scheduled: total=${placed.size} cancelled=${diff.cancel.size} regions=$regions next=${diff.schedule.firstOrNull()?.second ?: "-"}")
    }

    /**
     * 場所の通知を Geofencing に置き直す(同じ requestId は上書き・再起動で消えた分も戻る)。渡した数を返す。
     * 置けなかった(位置がオフ・許可を外された)時は、届いた扱いにしないよう記録を空にする。
     */
    private fun applyPlaces(all: List<PlannedNotification>): Int {
        val plan = GeofencePlanner.plan(all, regionState(), geofences(), canWatchPlaces())
        saveRegionState(plan.state)
        saveGeofences(plan.arm.map { it.identifier }.toSet())
        if (plan.remove.isNotEmpty()) runCatching { geofencing.removeGeofences(plan.remove.map(GeofencePlanner::requestId)) }
        if (plan.arm.isEmpty()) return 0
        val request = GeofencingRequest.Builder()
            // 置き直すたびに「もう中にいる」で鳴らさない(着いた時だけ)
            .setInitialTrigger(0)
            .addGeofences(plan.arm.map(::geofence))
            .build()
        val failed = { e: Exception ->
            Log.w(TAG, "geofences failed: $e")
            synchronized(this) {
                saveGeofences(emptySet())
                saveRegionState(regionState().armed(emptyList()))
            }
        }
        try {
            geofencing.addGeofences(request, geofenceIntent()).addOnFailureListener { failed(it) }
        } catch (e: SecurityException) {
            failed(e) // canWatchPlaces() の後に許可を外された
            return 0
        }
        return plan.arm.size
    }

    private fun geofence(p: PlannedNotification): Geofence {
        val t = p.trigger as PlannedNotification.Trigger.Region
        return Geofence.Builder()
            .setRequestId(GeofencePlanner.requestId(p.identifier))
            .setCircularRegion(t.latitude, t.longitude, t.radius.toFloat())
            .setExpirationDuration(Geofence.NEVER_EXPIRE)
            .setTransitionTypes(Geofence.GEOFENCE_TRANSITION_ENTER)
            .build()
    }

    /** Geofencing が着いた場所を詰めて返すので、書き換えられる PendingIntent にする(宛先は自分の receiver だけ)。 */
    private fun geofenceIntent(): PendingIntent = PendingIntent.getBroadcast(
        appContext,
        REQUEST_GEOFENCE,
        Intent(appContext, NotificationReceiver::class.java).setAction(NotificationReceiver.ACTION_GEOFENCE),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
    )

    private fun setAlarm(p: PlannedNotification, at: Instant): Boolean {
        val pi = PendingIntent.getBroadcast(appContext, 0, fireIntent(p), PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
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
        val intent = Intent(appContext, NotificationReceiver::class.java).setAction(NotificationReceiver.ACTION_FIRE).setData(dataUri(identifier))
        PendingIntent.getBroadcast(appContext, 0, intent, PendingIntent.FLAG_NO_CREATE or PendingIntent.FLAG_IMMUTABLE)?.let {
            alarms.cancel(it)
            it.cancel()
        }
    }

    private fun fireIntent(p: PlannedNotification) = Intent(appContext, NotificationReceiver::class.java)
        .setAction(NotificationReceiver.ACTION_FIRE)
        .setData(dataUri(p.identifier))
        .putExtra(EXTRA_IDENTIFIER, p.identifier)
        .putExtra(EXTRA_TITLE, p.title)
        .putExtra(EXTRA_BODY, p.body)
        .putExtra(EXTRA_ALARM, p.isAlarm)

    private fun stopIntent(notificationId: Int): PendingIntent = PendingIntent.getBroadcast(
        appContext,
        notificationId,
        Intent(appContext, NotificationReceiver::class.java).setAction(NotificationReceiver.ACTION_STOP).putExtra(EXTRA_NOTIFICATION_ID, notificationId),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )

    fun canPost(): Boolean {
        if (!NotificationManagerCompat.from(appContext).areNotificationsEnabled()) return false
        return Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            appContext.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
    }

    /** チャンネルを作る(何度呼んでも良い)。FCM が自分で出す通知もこのチャンネルに入るので起動時にも呼ぶ。 */
    fun ensureChannels() {
        val manager = appContext.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannels(
            listOf(
                NotificationChannel(CHANNEL_REMINDERS, appContext.getString(R.string.notification_channel_reminders), NotificationManager.IMPORTANCE_DEFAULT)
                    .apply { description = appContext.getString(R.string.notification_channel_reminders_description) },
                NotificationChannel(CHANNEL_CONDITIONS, appContext.getString(R.string.notification_channel_conditions), NotificationManager.IMPORTANCE_DEFAULT)
                    .apply { description = appContext.getString(R.string.notification_channel_conditions_description) },
                NotificationChannel(CHANNEL_OTHERS, appContext.getString(R.string.push_channel_others), NotificationManager.IMPORTANCE_DEFAULT)
                    .apply { description = appContext.getString(R.string.push_channel_others_description) },
                NotificationChannel(CHANNEL_ALARMS, appContext.getString(R.string.notification_channel_alarms), NotificationManager.IMPORTANCE_HIGH).apply {
                    description = appContext.getString(R.string.notification_channel_alarms_description)
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

    private fun regionState(): RegionOnceState = prefs.getString(KEY_REGIONS, null)
        ?.let { runCatching { DslJson.decodeFromString(RegionOnceState.serializer(), it) }.getOrNull() } ?: RegionOnceState()

    private fun saveRegionState(s: RegionOnceState) = prefs.edit().putString(KEY_REGIONS, DslJson.encodeToString(RegionOnceState.serializer(), s)).apply()

    /** 前回 Geofencing に渡した場所の通知の識別子(iOS の pending)。 */
    private fun geofences(): Set<String> = prefs.getString(KEY_GEOFENCES, null)
        ?.let { runCatching { DslJson.decodeFromString(SetSerializer(String.serializer()), it) }.getOrNull() }.orEmpty()

    private fun saveGeofences(ids: Set<String>) =
        prefs.edit().putString(KEY_GEOFENCES, DslJson.encodeToString(SetSerializer(String.serializer()), ids)).apply()

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
        private const val KEY_REGIONS = "regions.v1"
        private const val KEY_GEOFENCES = "geofences.v1"
        private const val REQUEST_GEOFENCE = 1

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
