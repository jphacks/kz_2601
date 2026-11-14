package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import androidx.compose.foundation.Image
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toComposeImageBitmap
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isCtrlPressed
import androidx.compose.ui.input.key.isMetaPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.type
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.io.PdfIo
import jp.kz2601.metamon.desktop.io.RecordIo
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.logic.ExportTable
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.GridTableLayout
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.form.FormDraft
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.awt.Graphics
import java.awt.Graphics2D
import java.awt.RenderingHints
import java.awt.print.PageFormat
import java.awt.print.Pageable
import java.awt.print.Paper
import java.awt.print.Printable
import java.awt.print.PrinterException
import java.awt.print.PrinterJob
import java.time.LocalDateTime
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.concurrent.thread
import javax.print.attribute.HashPrintRequestAttributeSet
import javax.print.attribute.standard.JobName
import javax.print.attribute.standard.MediaSizeName
import javax.print.attribute.standard.OrientationRequested

/**
 * 印刷(#1767)。今の画面を「表」に直し、export の PDF と同じ頁(io/PdfIo.pages)を A4 でプリンターに送る。
 * 表(list・table・gridTable・chart)は表ごとに頁を改め、全行を複数頁に割る。stat などの値・フォームの入力は「項目 / 値」の表にする。
 */
class PrintDocument(val title: String, val sections: List<ExportTable>) {
    /** 頁(表ごとに PdfIo.pages)。[section] は [sections] の添字。 */
    class Page(val section: Int, val pdf: PdfIo.Page)

    val pages: List<Page> by lazy {
        sections.flatMapIndexed { i, t -> PdfIo.pages(t, Strings["desktop_print_empty"]).map { Page(i, it) } }
    }

    /** 印刷と同じ頁の PDF(export と同じ形)。 */
    fun pdf(): ByteArray = PdfIo.pdf(pages.map { it.pdf.image })
}

/** 今の画面 → 印刷する表。画面に触らない(JVM で確かめる)。画面が 1 つも無ければ null。 */
object ScreenPrint {
    fun document(
        dsl: AppDSL,
        screenId: String,
        store: MockStore,
        form: FormDraft? = null,
        printedAt: LocalDateTime = LocalDateTime.now(),
        locale: Locale = Locale.JAPAN,
    ): PrintDocument? {
        val screen = dsl.screen(screenId) ?: dsl.screens.firstOrNull() ?: return null
        val title = screen.title.ifEmpty { dsl.app.name }
        val entities = dsl.entityModels()
        val data = LeafData.of(dsl, store)
        val summary = mutableListOf<List<String>>()
        val tables = mutableListOf<ExportTable>()

        fun walk(c: Component, section: String?) {
            when (c.type) {
                ComponentType.LIST, ComponentType.TABLE -> data.entity(c.entity)?.let { e ->
                    val t = RecordIo.exportTable(c.title ?: section ?: e.name, e, store, entities, null, locale = locale)
                    tables += t.copy(notes = listOf(Strings.format("host_export_count", t.rows.size)))
                }
                ComponentType.GRID_TABLE -> tables += gridTable(c, section, data)
                ComponentType.CHART -> tables += chart(c, section, data, locale)
                ComponentType.STAT -> summary += listOf(c.label ?: c.title.orEmpty(), StatValue.resolve(c, data).first)
                ComponentType.PROGRESS_BAR -> summary += listOf(c.label ?: c.title.orEmpty(), ProgressValue.percent(c, data))
                ComponentType.GAUGE -> GaugeValue.value(c, data).let { (v, unit) ->
                    summary += listOf(c.label ?: c.title.orEmpty(), Aggregator.format(v, c.format?.wire, unit, locale))
                }
                in InputLeaves.types -> if (form != null && c.field != null) {
                    val f = form.entity.field(c.field!!)
                    summary += listOf(InputLeaves.caption(c, form), FieldCoding.display(form.values[c.field!!] ?: JSONValue.Null, f, locale))
                }
                else -> Unit
            }
            c.children.forEach { walk(it, if (c.type == ComponentType.SECTION || c.type == ComponentType.CARD) c.title ?: section else section) }
        }
        screen.components.forEach { walk(it, null) }

        val sections = buildList {
            if (summary.isNotEmpty() || tables.isEmpty()) {
                add(ExportTable(title, listOf(Strings["desktop_print_item"], Strings["desktop_print_value"]), summary, numericColumns = listOf(false, true)))
            }
            addAll(tables)
        }
        val stamp = Strings.format("desktop_print_date", printedAt.format(DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm")))
        return PrintDocument(title, sections.mapIndexed { i, t -> if (i == 0) t.copy(notes = listOf(stamp) + t.notes) else t })
    }

    /** gridTable は画面と同じマス目(行見出し + 列)。マスは「見出し(副題)」、複数は「、」でつなぐ。 */
    internal fun gridTable(c: Component, section: String?, data: LeafData): ExportTable {
        val entity = data.entity(c.entity)
        val layout = GridTableLayout(c, entity, data.filtered(c))
        val titleField = c.titleField ?: entity?.displayTitleField
        fun text(field: String?, v: JSONValue): String = field?.let { FieldCoding.display(v, entity?.field(it), data.locale) }.orEmpty()
        val rows = layout.rows.mapIndexed { ri, row ->
            listOf(row.label) + layout.columns.indices.map { ci ->
                layout.records(GridTableLayout.Cell(ci, ri)).joinToString(Strings["desktop_list_separator"]) { r ->
                    val head = text(titleField, titleField?.let { r[it] } ?: JSONValue.Null).ifEmpty { r.id }
                    val sub = text(c.subtitleField, c.subtitleField?.let { r[it] } ?: JSONValue.Null)
                    if (sub.isEmpty()) head else "$head($sub)"
                }
            }
        }
        return ExportTable(c.title ?: section ?: entity?.name.orEmpty(), listOf("") + layout.columns.map { it.label }, rows)
    }

    /** chart は「まとまり / 値」の表(画面の棒・扇と同じ集計)。 */
    internal fun chart(c: Component, section: String?, data: LeafData, locale: Locale): ExportTable {
        val entity = data.entity(c.entity)
        val group = c.groupBy?.let { entity?.field(it)?.name } ?: Strings["desktop_print_item"]
        val fieldDef = c.field?.let { entity?.field(it) }
        val count = c.aggregate?.wire == "count"
        val format = c.format?.wire ?: if (count) "plain" else if (fieldDef?.type == FieldType.CURRENCY) "currency" else null
        val unit = if (count) StatValue.COUNT_UNIT else fieldDef?.unit
        val rows = ChartBuckets.buckets(c, data).map { listOf(it.label, Aggregator.format(it.value, format, unit, locale)) }
        return ExportTable(c.title ?: c.label ?: section ?: entity?.name.orEmpty(), listOf(group, if (count) Strings["desktop_print_count"] else fieldDef?.name ?: Strings["desktop_print_value"]), rows, numericColumns = listOf(false, true))
    }
}

/** 頁をプリンターへ(java.awt.print)。用紙は A4 縦、頁の画像を用紙いっぱいに描く(PDF と同じ位置・余白)。 */
class PrintPages(private val doc: PrintDocument) : Pageable, Printable {
    override fun getNumberOfPages(): Int = doc.pages.size

    override fun getPageFormat(pageIndex: Int): PageFormat = a4()

    override fun getPrintable(pageIndex: Int): Printable = this

    override fun print(graphics: Graphics, pageFormat: PageFormat, pageIndex: Int): Int {
        val page = doc.pages.getOrNull(pageIndex) ?: return Printable.NO_SUCH_PAGE
        val g = graphics as Graphics2D
        g.setRenderingHint(RenderingHints.KEY_INTERPOLATION, RenderingHints.VALUE_INTERPOLATION_BILINEAR)
        g.drawImage(page.pdf.image, 0, 0, pageFormat.width.toInt(), pageFormat.height.toInt(), null)
        return Printable.PAGE_EXISTS
    }

    companion object {
        /** A4(595 × 842 pt)・縦・余白なし(余白は頁の画像に入っている)。 */
        fun a4(): PageFormat = PageFormat().apply {
            orientation = PageFormat.PORTRAIT
            paper = Paper().apply {
                setSize(A4_W, A4_H)
                setImageableArea(0.0, 0.0, A4_W, A4_H)
            }
        }

        const val A4_W = 595.27563
        const val A4_H = 841.8898
    }
}

object DesktopPrint {
    /** Ctrl+P(macOS は ⌘P)。 */
    fun isPrintKey(e: KeyEvent): Boolean = e.type == KeyEventType.KeyDown && e.key == Key.P && (e.isCtrlPressed || e.isMetaPressed)

    /** 印刷できる画面があるか(画面の無い DSL では Ctrl+P を受けない)。 */
    fun canPrint(dsl: AppDSL): Boolean = dsl.screens.isNotEmpty()

    /** 印刷ダイアログで選んだ印刷。[job] が null ならやめた・送れない(理由は [message])。 */
    class Prepared(val job: PrinterJob?, val attrs: HashPrintRequestAttributeSet, val message: String?)

    /** OS の印刷ダイアログ(プリンター・部数・範囲)を出す。ダイアログなので画面のスレッド(EDT)で呼ぶ。 */
    fun prepare(doc: PrintDocument): Prepared {
        val attrs = HashPrintRequestAttributeSet().apply {
            add(MediaSizeName.ISO_A4)
            add(OrientationRequested.PORTRAIT)
            add(JobName(doc.title, null))
        }
        if (PrinterJob.lookupPrintServices().isEmpty()) return Prepared(null, attrs, Strings["desktop_print_no_printer"])
        val job = PrinterJob.getPrinterJob()
        job.jobName = doc.title
        job.setPageable(PrintPages(doc))
        if (!job.printDialog(attrs)) return Prepared(null, attrs, Strings["desktop_print_cancelled"])
        return Prepared(job, attrs, null)
    }

    /** プリンターへ送る(頁を描くので時間がかかる)。画面のスレッドの外で呼ぶ。結果の一言を返す。 */
    fun send(p: Prepared, pages: Int, print: (PrinterJob, HashPrintRequestAttributeSet) -> Unit = { j, a -> j.print(a) }): String {
        val job = p.job ?: return p.message ?: Strings["desktop_print_failed"]
        return try {
            print(job, p.attrs)
            Strings.format("desktop_print_sent", pages)
        } catch (_: PrinterException) {
            Strings["desktop_print_failed"]
        } catch (_: RuntimeException) {
            Strings["desktop_print_failed"]
        }
    }
}

/** 印刷の操作(押した時に 1 つだけ動かす)。2 回押しても 2 つの印刷を始めない。 */
class PrintRunner(private val background: (() -> Unit) -> Unit = { thread(isDaemon = true, name = "print") { it() } }) {
    var busy by mutableStateOf(false)
        private set

    /** [dialog] を今のスレッド(EDT)で出し、送るのは [background] で。終わったら [onDone] を画面のスレッドで呼ぶ。 */
    fun start(dialog: () -> DesktopPrint.Prepared, send: (DesktopPrint.Prepared) -> String, onDone: (String) -> Unit, onMain: (() -> Unit) -> Unit = ::onEdt): Boolean {
        if (busy) return false
        busy = true
        val prepared = runCatching(dialog).getOrNull()
        if (prepared?.job == null) {
            busy = false
            onDone(prepared?.message ?: Strings["desktop_print_failed"])
            return true
        }
        background {
            val msg = runCatching { send(prepared) }.getOrElse { Strings["desktop_print_failed"] }
            onMain {
                busy = false
                onDone(msg)
            }
        }
        return true
    }

    private companion object {
        fun onEdt(block: () -> Unit) = if (java.awt.EventQueue.isDispatchThread()) block() else java.awt.EventQueue.invokeLater(block)
    }
}

/** 印刷のプレビュー。頁を送って見て、「印刷…」で OS の印刷ダイアログへ。頁は画面のスレッドの外で描き、描き終えるまで回る印を出す。 */
@Composable
fun PrintPreview(doc: PrintDocument, onDismiss: () -> Unit, onResult: (String) -> Unit) {
    var index by remember(doc) { mutableIntStateOf(0) }
    val runner = remember(doc) { PrintRunner() }
    var built by remember(doc) { mutableStateOf<List<PrintDocument.Page>?>(null) }
    LaunchedEffect(doc) { built = withContext(Dispatchers.Default) { doc.pages } }
    val pages = built
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(Strings.format("desktop_print_title", doc.title)) },
        text = {
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (pages == null) {
                    CircularProgressIndicator(Modifier.padding(32.dp))
                } else {
                    val bitmap = remember(pages, index) { pages[index].pdf.image.toComposeImageBitmap() }
                    Image(bitmap, Strings.format("desktop_print_page", index + 1), Modifier.heightIn(max = 520.dp).border(1.dp, Color.LightGray))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        TextButton(onClick = { index-- }, enabled = index > 0) { Text("‹") }
                        Text(Strings.format("desktop_print_page_of", index + 1, pages.size), style = MaterialTheme.typography.bodySmall)
                        TextButton(onClick = { index++ }, enabled = index < pages.lastIndex) { Text("›") }
                    }
                }
            }
        },
        confirmButton = {
            // ダイアログは画面のスレッド、プリンターへ送るのは別のスレッド。頁を描いている間・送っている間は押せない
            TextButton(enabled = pages != null && !runner.busy, onClick = {
                runner.start(
                    dialog = { DesktopPrint.prepare(doc) },
                    send = { DesktopPrint.send(it, doc.pages.size) },
                    onDone = { msg ->
                        onResult(msg)
                        onDismiss()
                    },
                )
            }) { Text(Strings["desktop_print_dialog"]) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(Strings["desktop_close"]) } },
    )
}
