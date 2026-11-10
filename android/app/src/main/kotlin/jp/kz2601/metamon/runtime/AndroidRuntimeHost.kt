package jp.kz2601.metamon.runtime

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.notifications.LocalNotifications
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Android の端末機能。URL を開く・ローカル通知(#886)。書き出し・カレンダーなどは機能ごとの Issue で足す。 */
class AndroidRuntimeHost(private val context: Context) : RuntimeHost {
    private val notifications by lazy { LocalNotifications.get(context) }

    override suspend fun openUrl(url: String): Boolean = try {
        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        true
    } catch (_: ActivityNotFoundException) {
        false
    }

    override suspend fun scheduleNotification(spec: Notification, appId: String): String? = withContext(Dispatchers.IO) {
        notifications.schedule(spec, appId)?.let(notifications::whenText)
    }

    override suspend fun cancelNotification(id: String, appId: String) = withContext(Dispatchers.IO) {
        notifications.cancel(id, appId)
    }

    override suspend fun notificationsChanged(appId: String, dsl: AppDSL, records: Map<String, List<Record>>) = withContext(Dispatchers.IO) {
        notifications.onRecordsChanged(appId, dsl, records)
    }
}
