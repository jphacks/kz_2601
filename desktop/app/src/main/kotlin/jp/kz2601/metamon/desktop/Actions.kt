package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import androidx.compose.runtime.snapshots.Snapshot
import jp.kz2601.metamon.desktop.io.PdfIo
import jp.kz2601.metamon.desktop.io.RecordIo
import jp.kz2601.metamon.desktop.io.Tsv
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionFormat
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.logic.BookInfo
import jp.kz2601.metamon.dsl.logic.BookLookupFields
import jp.kz2601.metamon.dsl.logic.BookLookupPlan
import jp.kz2601.metamon.dsl.logic.CalendarEventDraft
import jp.kz2601.metamon.dsl.logic.CalendarEventPlan
import jp.kz2601.metamon.dsl.logic.CalendarEventRequest
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.dsl.logic.RecordExport
import jp.kz2601.metamon.dsl.records.FieldCoding
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import java.awt.Desktop
import java.awt.GraphicsEnvironment
import java.awt.SystemTray
import java.awt.Toolkit
import java.awt.TrayIcon
import java.awt.datatransfer.StringSelection
import java.awt.image.BufferedImage
import java.io.File
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.util.Timer
import java.util.TimerTask
import kotlin.concurrent.thread

/**
 * OS の機能に頼むアクション(iOS の AppRuntime の各 host・Android の app/runtime)。デスクトップで出来ない物は理由を一言で返す。
 * - openUrl: 既定のブラウザ(許可した scheme だけ)
 * - addToCalendar: .ics を書き出して既定のカレンダーで開く
 * - lookupBook: 本の情報(モックでは見本の本・`TAILORIT_API_BASE` があれば `POST /v1/lookup/book`)
 * - scheduleNotification / cancelNotification: トレイの通知(起動中だけ・時刻は次の 1 回)。常駐トレイ(#1761)があればそのアイコンから
 * - export: CSV を書き出し、同じ中身をクリップボードにも入れる
 * - openVoice・startActivity・endActivity・follow・unfollow: 今は使えない旨
 */
class DesktopActionHost(
    private val books: BookLookupClient = BookLookupClient.fromEnv(),
    private val exportDir: File = defaultExportDir(),
    private val notifier: Notifier = Notifier.Tray,
) : ActionHost {
    override fun request(action: Action, ctx: ActionContext, session: DesktopSession): String? = when (action.type) {
        ActionType.OPEN_URL -> openUrl(action.url)
        ActionType.RESET_FORM -> null
        ActionType.ADD_TO_CALENDAR -> addToCalendar(action, ctx, session)
        ActionType.LOOKUP_BOOK -> lookupBook(action, ctx, session)
        ActionType.SCHEDULE_NOTIFICATION -> action.notification?.let { notifier.schedule(it) } ?: Strings["runtime_notification_failed"]
        ActionType.CANCEL_NOTIFICATION -> action.notificationId?.let { notifier.cancel(it) }
        ActionType.EXPORT -> export(action, ctx, session)
        ActionType.OPEN_VOICE -> UNAVAILABLE_VOICE
        ActionType.START_ACTIVITY, ActionType.END_ACTIVITY -> UNAVAILABLE_ACTIVITY
        ActionType.FOLLOW, ActionType.UNFOLLOW -> UNAVAILABLE_FOLLOW
        else -> Strings.format("runtime_unknown_action", action.rawType.ifEmpty { action.type.wire })
    }

    private fun openUrl(url: String?): String? {
        if (url == null || !ExternalUrlPolicy.isAllowed(url)) return Strings["runtime_open_url_rejected"]
        val ok = runCatching { Desktop.isDesktopSupported() && Desktop.getDesktop().isSupported(Desktop.Action.BROWSE) }.getOrDefault(false)
        if (!ok) return Strings.format("desktop_url_unsupported", url)
        return runCatching { Desktop.getDesktop().browse(URI(url)); null }.getOrElse { Strings["desktop_url_failed"] }
    }

    private fun addToCalendar(action: Action, ctx: ActionContext, session: DesktopSession): String? {
        val spec = action.calendarEvent ?: return Strings["host_calendar_no_record"]
        val form = ctx.form
        val entity = session.entityModel(action.entity ?: form?.entity?.id) ?: return Strings["host_calendar_no_record"]
        val data = ctx.recordId?.let { id -> session.records(entity.id).firstOrNull { it.id == id }?.data } ?: form?.payload
            ?: return Strings["host_calendar_no_record"]
        val request = CalendarEventRequest(spec.startField, spec.endField, spec.durationMinutes, spec.titleField, spec.locationField, spec.notesField, spec.alertMinutes)
        return when (val r = CalendarEventPlan.build(request, entity, data)) {
            is CalendarEventPlan.Result.NoStart -> Strings.format("host_calendar_no_start", r.fieldName)
            is CalendarEventPlan.Result.Success -> {
                val file = exportDir.resolve(RecordExport.fileName(r.draft.title, "ics"))
                runCatching {
                    exportDir.mkdirs()
                    file.writeText(Ics.event(r.draft, uid = "${entity.id}-${ctx.currentRecordId ?: "new"}@tailorit"))
                }.getOrElse { return Strings["desktop_calendar_write_failed"] }
                val opened = runCatching { Desktop.getDesktop().open(file); true }.getOrDefault(false)
                if (opened) Strings.format("desktop_calendar_opened", file.name) else Strings.format("desktop_calendar_written", file.absolutePath)
            }
        }
    }

    private fun lookupBook(action: Action, ctx: ActionContext, session: DesktopSession): String? {
        val spec = action.bookLookup ?: return Strings["host_book_unavailable"]
        val form = ctx.form ?: return Strings["host_book_form_only"]
        val raw = form.values[spec.isbnField]?.stringValue?.takeIf { it.isNotBlank() } ?: return Strings["host_book_isbn_empty"]
        val isbn = BookLookupPlan.normalizeISBN(raw) ?: return Strings["host_book_isbn_invalid"]
        val fields = BookLookupFields(spec.isbnField, spec.titleField, spec.authorsField, spec.publisherField, spec.publishedDateField)
        thread(isDaemon = true, name = "lookupBook") {
            val msg = try {
                val book = books.lookup(isbn)
                if (book == null) {
                    Strings["host_book_not_found"]
                } else {
                    val values = BookLookupPlan.values(fields, book, form.entity)
                    Snapshot.withMutableSnapshot { form.values.putAll(values) }
                    Strings.format("host_book_filled", book.title, book.source.name)
                }
            } catch (e: Exception) {
                Strings["host_book_unavailable"]
            }
            Snapshot.withMutableSnapshot { session.message = msg }
        }
        return Strings["desktop_book_searching"]
    }

    /**
     * 表は io/RecordIo(ref は名前・古い順)、PDF は io/PdfIo(#1856)。format が pdf なら PDF、他は CSV(iOS と同じ形)。
     * ファイルはダウンロードに書き、同じ表を表計算に貼れる TSV でクリップボードにも入れる(一覧の「表をコピー」と同じ形)。
     */
    private fun export(action: Action, ctx: ActionContext, session: DesktopSession): String? {
        val entity = session.entityModel(action.entity ?: ctx.form?.entity?.id) ?: return Strings["host_export_no_entity"]
        val count = session.store.list(entity.id).size
        if (count == 0) return Strings["host_export_empty"]
        val title = action.title?.trim()?.ifEmpty { null } ?: entity.name
        val date = java.time.LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm"))
        val table = RecordIo.exportTable(title, entity, session.store, session.models, action.fields, listOf(Strings.format("host_export_date", date), Strings.format("host_export_count", count)))
        val pdf = action.format == ActionFormat.PDF
        val file = exportDir.resolve(RecordExport.fileName(title, if (pdf) "pdf" else "csv"))
        runCatching {
            exportDir.mkdirs()
            file.writeBytes(if (pdf) PdfIo.data(table) else RecordIo.csv(table))
        }.getOrElse { return Strings["host_export_failed"] }
        val copied = !GraphicsEnvironment.isHeadless() &&
            runCatching { Toolkit.getDefaultToolkit().systemClipboard.setContents(StringSelection(Tsv.string(table)), null) }.isSuccess
        return Strings.format("desktop_export_done", count, file.absolutePath) + if (copied) Strings["desktop_export_clipboard"] else ""
    }

    companion object {
        val UNAVAILABLE_VOICE get() = Strings["desktop_unavailable_voice"]
        val UNAVAILABLE_ACTIVITY get() = Strings["desktop_unavailable_activity"]
        val UNAVAILABLE_FOLLOW get() = Strings["desktop_unavailable_follow"]

        fun defaultExportDir(): File {
            val home = File(System.getProperty("user.home"))
            return home.resolve("Downloads").takeIf { it.isDirectory } ?: home
        }
    }
}


/** iCalendar(RFC 5545)の 1 件。終日は DATE、時刻ありは UTC。 */
object Ics {
    private val stamp = DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss'Z'").withZone(ZoneOffset.UTC)
    private val day = DateTimeFormatter.ofPattern("yyyyMMdd")

    fun event(d: CalendarEventDraft, uid: String, now: Instant = Instant.now(), zone: ZoneId = FieldCoding.zone): String {
        val lines = mutableListOf("BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Tailor It//Desktop//JA", "BEGIN:VEVENT", "UID:${escape(uid)}", "DTSTAMP:${stamp.format(now)}")
        if (d.isAllDay) {
            val start = LocalDate.ofInstant(d.start, zone)
            val end = LocalDate.ofInstant(d.end, zone).let { if (it.isAfter(start)) it else start.plusDays(1) }
            lines += "DTSTART;VALUE=DATE:${day.format(start)}"
            lines += "DTEND;VALUE=DATE:${day.format(end)}"
        } else {
            lines += "DTSTART:${stamp.format(d.start)}"
            lines += "DTEND:${stamp.format(d.end)}"
        }
        lines += "SUMMARY:${escape(d.title)}"
        d.location?.let { lines += "LOCATION:${escape(it)}" }
        d.notes?.let { lines += "DESCRIPTION:${escape(it)}" }
        d.alertOffsetSeconds?.let {
            lines += listOf("BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:${escape(d.title)}", "TRIGGER:-PT${-it / 60}M", "END:VALARM")
        }
        lines += listOf("END:VEVENT", "END:VCALENDAR")
        return lines.joinToString("\r\n", postfix = "\r\n")
    }

    fun escape(s: String): String = s.replace("\\", "\\\\").replace(";", "\\;").replace(",", "\\,").replace("\r\n", "\\n").replace("\n", "\\n")
}

/** 本の情報の取得(iOS の `BookLookupClient`)。見つからなければ null。 */
fun interface BookLookupClient {
    fun lookup(isbn: String): BookInfo?

    companion object {
        /** モック: どの ISBN でも見本の本を返す(サインインも通信もしない)。 */
        val mock = BookLookupClient { isbn ->
            BookInfo(isbn, "見本の本", authors = "見本 太郎", publisher = "見本書房", publishedDate = "2024-04", source = BookInfo.Source("モック", "https://example.com"))
        }

        /** `TAILORIT_API_BASE` があればその appconfig に聞く。無ければモック。 */
        fun fromEnv(): BookLookupClient = System.getenv("TAILORIT_API_BASE")?.takeIf { it.isNotBlank() }?.let(::HttpBookLookupClient) ?: mock
    }
}

/** appconfig の `POST /v1/lookup/book`(Android の HttpBookLookupClient と同じ形)。POST なので ISBN を URL に載せない。 */
class HttpBookLookupClient(private val base: String, private val client: HttpClient = HttpClient.newHttpClient()) : BookLookupClient {
    override fun lookup(isbn: String): BookInfo? {
        val body = JsonObject(mapOf("isbn" to JsonPrimitive(isbn))).toString()
        val req = HttpRequest.newBuilder(URI(base.trimEnd('/') + "/v1/lookup/book"))
            .timeout(Duration.ofSeconds(20))
            .header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(body))
            .build()
        val res = client.send(req, HttpResponse.BodyHandlers.ofString())
        if (res.statusCode() == 404) return null
        check(res.statusCode() in 200..299) { "HTTP ${res.statusCode()}" }
        return parse(res.body())
    }

    companion object {
        fun parse(text: String): BookInfo {
            val o = Json.parseToJsonElement(text) as JsonObject
            fun JsonObject.s(k: String) = (this[k] as? JsonPrimitive)?.contentOrNull
            val src = o["source"] as? JsonObject ?: JsonObject(emptyMap())
            return BookInfo(
                isbn = o.s("isbn").orEmpty(),
                title = o.s("title").orEmpty(),
                authors = o.s("authors"),
                publisher = o.s("publisher"),
                publishedDate = o.s("publishedDate"),
                coverUrl = o.s("coverUrl"),
                source = BookInfo.Source(src.s("name").orEmpty(), src.s("url").orEmpty(), src.s("license")),
            )
        }
    }
}

/** 通知。デスクトップはトレイの吹き出しで出す(アプリが起動している間だけ・OS の予約は無い)。 */
interface Notifier {
    fun schedule(n: Notification): String
    fun cancel(id: String): String?

    /** 次に鳴らす時刻(`time` の HH:mm。過ぎていれば明日)。時刻が無ければ null(すぐ出す)。 */
    fun nextFire(n: Notification, now: ZonedDateTime = ZonedDateTime.now(FieldCoding.zone)): ZonedDateTime? {
        val t = n.time?.let { runCatching { LocalTime.parse(it) }.getOrNull() } ?: return null
        val today = now.with(t).withSecond(0).withNano(0)
        return if (today.isAfter(now)) today else today.plusDays(1)
    }

    object Tray : Notifier {
        private val timers = HashMap<String, Timer>()
        private var icon: TrayIcon? = null

        /** 常駐トレイ(Tray.kt の DesktopTray)が出ている間はそのアイコンから出す。無ければ通知用のアイコンを足す。 */
        @Volatile
        var sink: ((title: String, body: String) -> Unit)? = null

        /** すぐ出す(予約しない)。トレイが無ければ何もしない。 */
        fun notify(title: String, body: String) {
            if (GraphicsEnvironment.isHeadless()) return
            show(title, body)
        }

        override fun schedule(n: Notification): String {
            if (GraphicsEnvironment.isHeadless() || !SystemTray.isSupported()) return Strings["desktop_tray_unavailable"]
            cancel(n.id)
            val at = nextFire(n)
            val timer = Timer("notify-${n.id}", true)
            val task = object : TimerTask() {
                override fun run() = show(n.title, n.body)
            }
            if (at == null) timer.schedule(task, 0) else timer.schedule(task, Duration.between(ZonedDateTime.now(at.zone), at).toMillis().coerceAtLeast(0))
            timers[n.id] = timer
            val whenText = at?.let { DateTimeFormatter.ofPattern("M/d HH:mm").format(it) } ?: Strings["desktop_notification_now"]
            return Strings.format("desktop_notification_scheduled", whenText)
        }

        override fun cancel(id: String): String? {
            timers.remove(id)?.cancel()
            return null
        }

        @Synchronized
        private fun show(title: String, body: String) {
            sink?.let { return it(title, body) }
            runCatching {
                val tray = SystemTray.getSystemTray()
                val i = icon ?: TrayIcon(BufferedImage(16, 16, BufferedImage.TYPE_INT_ARGB), "Tailor It").also {
                    it.isImageAutoSize = true
                    tray.add(it)
                    icon = it
                }
                i.displayMessage(title, body, TrayIcon.MessageType.INFO)
            }
        }
    }
}
