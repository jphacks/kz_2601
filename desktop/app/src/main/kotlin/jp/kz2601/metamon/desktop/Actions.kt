package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import androidx.compose.runtime.snapshots.Snapshot
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.desktop.io.FileNames
import jp.kz2601.metamon.desktop.io.ExportClipboard
import jp.kz2601.metamon.desktop.io.PdfIo
import jp.kz2601.metamon.desktop.io.RecordIo
import jp.kz2601.metamon.desktop.io.Tsv
import jp.kz2601.metamon.desktop.io.UserDirs
import jp.kz2601.metamon.desktop.notify.DesktopNotice
import jp.kz2601.metamon.desktop.notify.TrayNotifier
import jp.kz2601.metamon.desktop.notify.Notifier as DisplayNotifier
import jp.kz2601.metamon.desktop.store.writeAtomically
import jp.kz2601.metamon.desktop.voice.VoiceInput
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
import java.awt.EventQueue
import java.io.File
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.file.Files
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

/** OS に URL・メーラーを渡す口(java.awt.Desktop)。テストでは偽物に替える。 */
interface SystemOpener {
    fun supports(action: Desktop.Action): Boolean

    fun browse(uri: URI)

    fun mail()

    object Awt : SystemOpener {
        override fun supports(action: Desktop.Action): Boolean =
            runCatching { Desktop.isDesktopSupported() && Desktop.getDesktop().isSupported(action) }.getOrDefault(false)

        override fun browse(uri: URI) = Desktop.getDesktop().browse(uri)

        override fun mail() = Desktop.getDesktop().mail()
    }
}

/**
 * OS の機能に頼むアクション(iOS の AppRuntime の各 host・Android の app/runtime)。デスクトップで出来ない物は理由を一言で返す。
 * - openUrl: 既定のブラウザ(許可した scheme だけ)
 * - addToCalendar: .ics を一時フォルダ(人ごと)に書いて既定のカレンダーで開く(ダウンロードには残さない)
 * - lookupBook: 本の情報(モックでは見本の本・`TAILORIT_API_BASE` があれば `POST /v1/lookup/book`)
 * - scheduleNotification / cancelNotification: トレイの通知(起動中だけ・時刻は次の 1 回)。常駐トレイ(#1761)があればそのアイコンから
 * - export: CSV を書き出し、同じ中身をクリップボードにも入れる
 * - openVoice: `TAILORIT_VOICE=1` の時だけマイクで録って端末の中で文字にする(voice/VoiceInput・#1751)。既定は使えない旨
 * - startActivity・endActivity・follow・unfollow: 今は使えない旨
 */
class DesktopActionHost(
    private val books: BookLookupClient = BookLookupClient.fromEnv(),
    private val exportDir: File = defaultExportDir(),
    private val notifier: Notifier = Notifier.Tray,
    /** null なら openVoice は使えない旨(既定はフラグが立っている時だけ作る) */
    private val voice: VoiceInput? = VoiceInput.shared,
    /** 重い書き出し(PDF の頁を描く)を画面のスレッドの外で。 */
    private val background: (() -> Unit) -> Unit = { thread(isDaemon = true, name = "export") { it() } },
    /** カレンダーに渡す .ics を置く所(一時フォルダ・渡すたびに別のフォルダ)。 */
    private val calendarDir: File = calendarTempRoot(),
    /** 既定のアプリで開く。開けたら true。 */
    private val openFile: (File) -> Boolean = { f -> runCatching { Desktop.getDesktop().open(f); true }.getOrDefault(false) },
    /** URL を渡す先(既定は java.awt.Desktop)。テストで替える。 */
    private val system: SystemOpener = SystemOpener.Awt,
) : ActionHost {
    override fun request(action: Action, ctx: ActionContext, session: DesktopSession): String? = when (action.type) {
        ActionType.OPEN_URL -> openUrl(action.url)
        ActionType.RESET_FORM -> null
        ActionType.ADD_TO_CALENDAR -> addToCalendar(action, ctx, session)
        ActionType.LOOKUP_BOOK -> lookupBook(action, ctx, session)
        ActionType.SCHEDULE_NOTIFICATION -> action.notification?.let { notifier.schedule(it) } ?: Strings["runtime_notification_failed"]
        ActionType.CANCEL_NOTIFICATION -> action.notificationId?.let { notifier.cancel(it) }
        ActionType.EXPORT -> export(action, ctx, session)
        // 結果は画面のスレッドで出す。dispatch が toggle の返す一言を入れた後になるので、速く終わっても上書きされない
        ActionType.OPEN_VOICE -> voice?.toggle { msg -> EventQueue.invokeLater { Snapshot.withMutableSnapshot { session.message = msg } } } ?: UNAVAILABLE_VOICE
        ActionType.START_ACTIVITY, ActionType.END_ACTIVITY -> UNAVAILABLE_ACTIVITY
        ActionType.FOLLOW, ActionType.UNFOLLOW -> UNAVAILABLE_FOLLOW
        else -> Strings.format("runtime_unknown_action", action.rawType.ifEmpty { action.type.wire })
    }

    private fun openUrl(url: String?): String? {
        if (url == null || !ExternalUrlPolicy.isAllowed(url)) return Strings["runtime_open_url_rejected"]
        // 宛先の無い mailto:(openUrlButton の mail で to が空・不正)は URI にできないので、宛先なしでメーラーを開く
        val bareMail = isBareMailto(url)
        val action = if (bareMail) Desktop.Action.MAIL else Desktop.Action.BROWSE
        if (!system.supports(action)) return Strings.format("desktop_url_unsupported", url)
        return runCatching {
            if (bareMail) system.mail() else system.browse(URI(url))
            null
        }.getOrElse { Strings["desktop_url_failed"] }
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
                val text = Ics.event(r.draft, uid = "${entity.id}-${ctx.currentRecordId ?: "new"}@tailorit")
                val file = runCatching { writeHandOff(RecordExport.fileName(r.draft.title, "ics"), text.toByteArray(Charsets.UTF_8)) }
                    .getOrElse { return Strings["desktop_calendar_write_failed"] }
                val opened = openFile(file)
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
     * 同じ名前のファイルがあれば上書きせず `名前 (2).csv` に。PDF は頁を描くので [background] で書き、終わったら一言を出し直す。
     */
    private fun export(action: Action, ctx: ActionContext, session: DesktopSession): String? {
        val entity = session.entityModel(action.entity ?: ctx.form?.entity?.id) ?: return Strings["host_export_no_entity"]
        val count = session.store.list(entity.id).size
        if (count == 0) return Strings["host_export_empty"]
        val title = action.title?.trim()?.ifEmpty { null } ?: entity.name
        val date = java.time.LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm"))
        val table = RecordIo.exportTable(title, entity, session.store, session.models, action.fields, listOf(Strings.format("host_export_date", date), Strings.format("host_export_count", count)))
        val pdf = action.format == ActionFormat.PDF
        val name = RecordExport.fileName(title, if (pdf) "pdf" else "csv")
        val copied = ExportClipboard.put(Tsv.string(table))
        fun done(file: File) = Strings.format("desktop_export_done", count, file.absolutePath) + if (copied) Strings["desktop_export_clipboard"] else ""
        if (!pdf) {
            val file = runCatching { writeNew(name, RecordIo.csv(table)) }.getOrElse { return Strings["host_export_failed"] }
            return done(file)
        }
        val writing = Strings["desktop_export_writing"]
        background {
            val msg = runCatching { done(writeNew(name, PdfIo.data(table))) }.getOrElse { Strings["host_export_failed"] }
            // 書いている間に別の一言(他の操作の結果)が出ていれば上書きしない
            Snapshot.withMutableSnapshot { if (session.message == writing) session.message = msg }
        }
        return writing
    }

    /**
     * [exportDir] に空いている名前で書く(既存のファイルは上書きしない・Windows の予約名は頭に `_`)。
     * 名前を空のファイルで取ってから、同じフォルダの一時ファイルに書き切って置き換える(書きかけのファイルを見せない)。書けなければ取った名前を消す。
     */
    private fun writeNew(name: String, bytes: ByteArray): File {
        val file = FileNames.createFree(exportDir, name)
        try {
            writeAtomically(file, bytes)
        } catch (e: Exception) {
            file.delete()
            throw e
        }
        return file
    }

    /** 他のアプリに渡すだけのファイル。渡すたびに別のフォルダ(名前はそのまま・`(2)` を付けない)。古い物は起動時に消す。 */
    private fun writeHandOff(name: String, bytes: ByteArray): File {
        calendarDir.mkdirs()
        val dir = Files.createTempDirectory(calendarDir.toPath(), CALENDAR_PREFIX).toFile()
        return File(dir, FileNames.guard(name)).apply { writeBytes(bytes) }
    }

    companion object {
        const val CALENDAR_PREFIX = "ics-"

        /** カレンダーに渡す .ics の置き場(利用者ごとのフォルダ・[UserDirs.calendar])。 */
        fun calendarTempRoot(): File = UserDirs.calendar

        /** 起動時: 前に渡した .ics のうち 1 日より古い物を消す。 */
        fun sweepCalendar(now: Long = System.currentTimeMillis(), root: File = calendarTempRoot()): Int =
            DragOut.sweep(now = now, root = root, prefix = CALENDAR_PREFIX)

        /** scheme が mailto で、その後ろが空(`mailto:`)。 */
        fun isBareMailto(url: String): Boolean =
            ExternalUrlPolicy.scheme(url) == "mailto" && url.substringAfter(':').isBlank()

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
        return lines.joinToString("\r\n", postfix = "\r\n") { fold(it) }
    }

    /** TEXT の値(RFC 5545 §3.3.11)。改行(CRLF・CR・LF・U+2028/2029)は `\n`、他の制御文字は落とす。 */
    fun escape(s: String): String {
        val sb = StringBuilder(s.length)
        var i = 0
        while (i < s.length) {
            val c = s[i]
            when {
                c == '\r' -> {
                    sb.append("\\n")
                    if (i + 1 < s.length && s[i + 1] == '\n') i++
                }
                c == '\n' || c == '\u2028' || c == '\u2029' -> sb.append("\\n")
                c == '\\' -> sb.append("\\\\")
                c == ';' -> sb.append("\\;")
                c == ',' -> sb.append("\\,")
                c.code < 0x20 && c != '\t' -> {}
                c.code == 0x7F -> {}
                else -> sb.append(c)
            }
            i++
        }
        return sb.toString()
    }

    /** 1 行を 75 オクテット(UTF-8)ごとに折る(RFC 5545 §3.1)。続きの行は空白 1 つで始める。文字の途中では切らない。 */
    fun fold(line: String, max: Int = 75): String {
        if (line.toByteArray(Charsets.UTF_8).size <= max) return line
        val out = StringBuilder()
        var used = 0
        var limit = max
        var i = 0
        while (i < line.length) {
            val cp = line.codePointAt(i)
            val len = Character.charCount(cp)
            val bytes = String(Character.toChars(cp)).toByteArray(Charsets.UTF_8).size
            if (used + bytes > limit) {
                out.append("\r\n ")
                used = 0
                limit = max - 1
            }
            out.appendCodePoint(cp)
            used += bytes
            i += len
        }
        return out.toString()
    }
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

/** 通知の予約。鳴らす時は [Tray.display](既定はトレイの吹き出し・[jp.kz2601.metamon.desktop.notify.TrayNotifier])で出す(アプリが起動している間だけ・OS の予約は無い)。 */
interface Notifier {
    fun schedule(n: Notification): String
    fun cancel(id: String): String?

    /** 次に鳴らす時刻(`time` の HH:mm。過ぎていれば明日)。時刻が無ければ null(すぐ出す)。 */
    fun nextFire(n: Notification, now: ZonedDateTime = ZonedDateTime.now(FieldCoding.zone)): ZonedDateTime? {
        val t = n.time?.let { runCatching { LocalTime.parse(it) }.getOrNull() } ?: return null
        val today = now.with(t).withSecond(0).withNano(0)
        return if (today.isAfter(now)) today else today.plusDays(1)
    }

    /** 予約・取り消しは画面のスレッドからもタイマーのスレッドからも来るので、[timers] はこの object で同期する。 */
    object Tray : Notifier {
        private val timers = HashMap<String, Timer>()

        /** 通知を出す口。テストで作り物に替える。 */
        @Volatile
        var display: DisplayNotifier = TrayNotifier

        /** すぐ出す(予約しない)。出せなければ何もしない。 */
        fun notify(title: String, body: String) {
            display.show(DesktopNotice(title, body))
        }

        override fun schedule(n: Notification): String {
            if (!display.available) return Strings["desktop_tray_unavailable"]
            val at = nextFire(n)
            arm(n.id, at?.let { Duration.between(ZonedDateTime.now(it.zone), it).toMillis().coerceAtLeast(0) } ?: 0) { display.show(DesktopNotice(n.title, n.body)) }
            val whenText = at?.let { DateTimeFormatter.ofPattern("M/d HH:mm").format(it) } ?: Strings["desktop_notification_now"]
            return Strings.format("desktop_notification_scheduled", whenText)
        }

        override fun cancel(id: String): String? {
            synchronized(this) { timers.remove(id) }?.cancel()
            return null
        }

        /** 予約した通知を全部取り消す(この端末のデータを消す時)。取り消した数。 */
        fun cancelAll(): Int {
            val all = synchronized(this) { timers.values.toList().also { timers.clear() } }
            all.forEach { it.cancel() }
            return all.size
        }

        /** 予約している数(テスト用)。 */
        internal fun pendingCount(): Int = synchronized(this) { timers.size }

        /** [id] の前の予約を消して [delayMillis] 後に [fire] を 1 回。鳴ったら表から外す。 */
        internal fun arm(id: String, delayMillis: Long, fire: () -> Unit) {
            val timer = Timer("notify-$id", true)
            val previous = synchronized(this) { timers.put(id, timer) }
            previous?.cancel()
            timer.schedule(
                object : TimerTask() {
                    override fun run() {
                        try {
                            fire()
                        } finally {
                            synchronized(this@Tray) { if (timers[id] === timer) timers.remove(id) }
                            timer.cancel()
                        }
                    }
                },
                delayMillis,
            )
        }

        /** 予約中の id(テスト用)。 */
        internal fun pending(): Set<String> = synchronized(this) { timers.keys.toSet() }
    }
}
