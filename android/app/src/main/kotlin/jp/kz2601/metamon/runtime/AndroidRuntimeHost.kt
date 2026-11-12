package jp.kz2601.metamon.runtime

import android.content.ActivityNotFoundException
import android.content.ClipData
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.CalendarContract
import androidx.core.content.FileProvider
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.Activity
import jp.kz2601.metamon.dsl.ActionFormat
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.logic.RecordExport
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.notifications.ActivityNotificationPlanner
import jp.kz2601.metamon.notifications.ActivityNotifications
import jp.kz2601.metamon.notifications.ActivityStatReading
import jp.kz2601.metamon.notifications.LocalNotifications
import jp.kz2601.metamon.notifications.TimerActivityUpdate
import jp.kz2601.metamon.notifications.TimerAlarms
import jp.kz2601.metamon.notifications.TimerVisibility
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.components.lists.ValueLabels
import jp.kz2601.metamon.renderer.components.play.TimerRun
import jp.kz2601.metamon.services.books.BookLookupClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.time.LocalDateTime
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Android の端末機能。URL を開く・ローカル通知(#886)・書き出し・カレンダー・本の情報(#911)。
 * [books] はサーバーにつながる構成だけ(モックは null =「手で入力してください」・iOS と同じ)。
 */
class AndroidRuntimeHost(
    private val context: Context,
    private val books: BookLookupClient? = null,
) : RuntimeHost {
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

    private val timers by lazy { TimerAlarms.get(context) }

    private val activities by lazy { ActivityNotifications.get(context) }

    override suspend fun startActivity(spec: Activity, appId: String, appName: String, stat: ActivityStatReading?): Boolean =
        withContext(Dispatchers.IO) {
            activities.show(ActivityNotificationPlanner.content(spec, appId, appName, stat, System.currentTimeMillis()))
        }

    override suspend fun endActivity(appId: String) = withContext(Dispatchers.IO) { activities.end(appId) }

    override suspend fun timerChanged(key: String, appName: String, timerName: String, endsAtEpochMillis: Long?) =
        withContext(Dispatchers.IO) { timers.changed(key, appName, timerName, endsAtEpochMillis) }

    override fun loadTimer(key: String): TimerRun? = timers.load(key)

    override fun saveTimer(key: String, run: TimerRun) = timers.save(key, run)

    override fun timerVisible(key: String, visible: Boolean) = TimerVisibility.set(key, visible)

    override suspend fun timerActivity(update: TimerActivityUpdate): Boolean =
        withContext(Dispatchers.IO) { activities.timer(update) }

    override suspend fun notificationsChanged(appId: String, dsl: AppDSL, records: Map<String, List<Record>>) = withContext(Dispatchers.IO) {
        notifications.onRecordsChanged(appId, dsl, records)
    }

    /** CSV / PDF を cache に書き、共有シート(ファイル・メール・Drive など)を開く。 */
    override suspend fun export(action: Action, context: ActionContext, runtime: AppRuntime) {
        val res = this.context.resources
        val dateText = LocalDateTime.now().format(
            DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT).withLocale(res.configuration.locales[0]),
        )
        val prepared = ExportSteps.prepare(action, runtime, ValueLabels.from(res)) { count ->
            listOf(res.getString(R.string.host_export_date, dateText), res.getString(R.string.host_export_count, count))
        }
        val ready = when (prepared) {
            is ExportSteps.Prepared.Invalid -> return runtime.showToast(prepared.message)
            is ExportSteps.Prepared.Ready -> prepared
        }
        val file = try {
            withContext(Dispatchers.IO) {
                val (bytes, ext) = ExportSteps.encode(ready.table, ready.format) { ExportPdf.data(it, res.getString(R.string.host_export_empty)) }
                val name = RecordExport.fileName(ready.table.title, ext, res.getString(R.string.host_export_file_fallback))
                ExportSteps.writeFile(this@AndroidRuntimeHost.context.cacheDir, name, bytes)
            }
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            return runtime.showToast(ExportSteps.FAILED)
        }
        val uri = FileProvider.getUriForFile(this.context, "${this.context.packageName}.metamon.media", file)
        val send = Intent(Intent.ACTION_SEND)
            .setType(if (ready.format == ActionFormat.PDF) "application/pdf" else "text/csv")
            .putExtra(Intent.EXTRA_STREAM, uri)
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        // chooser 越しでも読めるよう ClipData にも載せる
        send.clipData = ClipData.newRawUri(file.name, uri)
        try {
            this.context.startActivity(Intent.createChooser(send, null).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION))
        } catch (_: ActivityNotFoundException) {
            runtime.showToast(ExportSteps.FAILED)
        }
    }

    /** OS の「新規イベント」画面を中身を埋めて開く(ユーザーが保存する・カレンダーの権限は要らない)。 */
    override suspend fun addToCalendar(action: Action, context: ActionContext, runtime: AppRuntime) {
        val draft = when (val d = CalendarSteps.draft(action, context, runtime)) {
            is CalendarSteps.Draft.Invalid -> return runtime.showToast(d.message)
            is CalendarSteps.Draft.Ready -> d.draft
        }
        val x = CalendarSteps.insertExtras(draft)
        val intent = Intent(Intent.ACTION_INSERT)
            .setData(CalendarContract.Events.CONTENT_URI)
            .putExtra(CalendarContract.Events.TITLE, x.title)
            .putExtra(CalendarContract.EXTRA_EVENT_BEGIN_TIME, x.beginMillis)
            .putExtra(CalendarContract.EXTRA_EVENT_END_TIME, x.endMillis)
            .putExtra(CalendarContract.EXTRA_EVENT_ALL_DAY, x.allDay)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        x.location?.let { intent.putExtra(CalendarContract.Events.EVENT_LOCATION, it) }
        x.description?.let { intent.putExtra(CalendarContract.Events.DESCRIPTION, it) }
        try {
            this.context.startActivity(intent)
        } catch (_: ActivityNotFoundException) {
            runtime.showToast(UiText.Res(R.string.host_calendar_no_app))
        }
    }

    override suspend fun lookupBook(action: Action, context: ActionContext, runtime: AppRuntime) {
        BookLookupSteps.run(action, context.form, books)?.let(runtime::showToast)
    }
}
