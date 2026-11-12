package jp.kz2601.metamon.desktop.io

import androidx.compose.foundation.Image
import androidx.compose.foundation.draganddrop.dragAndDropTarget
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.draganddrop.DragAndDropEvent
import androidx.compose.ui.draganddrop.DragAndDropTarget
import androidx.compose.ui.draganddrop.awtTransferable
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
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.photoPreviewClick
import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.logic.ImportPlan
import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.entityModels
import org.jetbrains.skia.Image as SkiaImage
import java.awt.FileDialog
import java.awt.Frame
import java.awt.Toolkit
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.StringSelection
import java.awt.datatransfer.Transferable
import java.awt.datatransfer.UnsupportedFlavorException
import java.awt.image.BufferedImage
import java.io.File
import javax.imageio.ImageIO

/** 取り込みの確認待ち(CSV・TSV)。 */
data class PendingImport(val entityId: String, val label: String, val plan: ImportPlan)

/** 部品のうち、このパッケージだけが描くもの(fileImport)。photoField・videoField は入力の部品(InputLeaves)で、中身は [MediaFieldLeaf]。 */
object IoLeaves {
    val types: Set<ComponentType> = setOf(ComponentType.FILE_IMPORT)
}

/**
 * ファイルの取り込み・書き出し・ドラッグ & ドロップ・クリップボード(#1753・#1764・#1766)。
 * モックの記録は [read] で読み [update] で差し替える(DesktopApp の store を持たない)。
 */
class DesktopIo(dsl: AppDSL, private val read: () -> MockStore, private val update: (MockStore) -> Unit) {
    val entities: List<Entity> = dsl.entityModels()
    var pending by mutableStateOf<PendingImport?>(null)
    var notice by mutableStateOf<String?>(null)

    fun entity(id: String?): Entity? = entities.firstOrNull { it.id == id }

    // --- 取り込み(#1753) ---

    fun pick(c: Component) {
        val file = openDialog(c.label ?: Strings["desktop_io_choose_file"]) ?: return
        importFile(file, c)
    }

    /** fileImport にファイルを 1 つ入れる。CSV は確認を出し、テキスト・PDF は本文を 1 件の記録にする。 */
    fun importFile(file: File, c: Component) {
        val entity = entity(c.entity) ?: return run { notice = Strings["desktop_io_no_import_target"] }
        val accept = c.accept.ifEmpty { listOf(Accept.CSV) }
        when (FileKind.of(file.name)) {
            FileKind.CSV -> if (Accept.CSV in accept) importCsvFile(file, entity, c, c.label ?: file.name) else notice = Strings["desktop_io_unsupported_file"]
            FileKind.TEXT -> if (Accept.TEXT in accept) {
                val bytes = ImageSource.readFile(file, RecordImport.MAX_TEXT_BYTES) ?: return run { notice = Strings["desktop_io_file_too_large"] }
                val text = RecordImport.plainText(bytes) ?: return run { notice = Strings["desktop_io_unreadable_encoding"] }
                addBody(text, file.name, entity, c)
            } else {
                notice = Strings["desktop_io_unsupported_file"]
            }
            FileKind.PDF -> if (Accept.PDF in accept) {
                val bytes = ImageSource.readFile(file, RecordImport.MAX_PDF_BYTES) ?: return run { notice = Strings["desktop_io_file_too_large"] }
                when (val r = PdfIo.text(bytes)) {
                    is PdfIo.TextResult.Text -> addBody(r.text, file.name, entity, c)
                    PdfIo.TextResult.Locked -> notice = Strings["desktop_io_pdf_locked"]
                    PdfIo.TextResult.NoText -> notice = Strings["desktop_io_pdf_no_text"]
                    PdfIo.TextResult.TooLarge -> notice = Strings["desktop_io_file_too_large"]
                    PdfIo.TextResult.Broken -> notice = Strings["desktop_io_pdf_broken"]
                }
            } else {
                notice = Strings["desktop_io_unsupported_file"]
            }
            else -> notice = Strings["desktop_io_unsupported_file"]
        }
    }

    private fun addBody(text: String, name: String, entity: Entity, c: Component) {
        val field = c.field ?: entity.storedFields.firstOrNull { it.type == jp.kz2601.metamon.dsl.records.FieldType.TEXT }?.id ?: return
        val body = RecordImport.bodyRecord(text, name, entity, field) ?: return run { notice = Strings["desktop_io_no_import_field"] }
        update(RecordIo.apply(read(), entity.id, listOf(body.data)))
        notice = Strings[if (body.truncated) "desktop_io_imported_truncated" else "desktop_io_imported"]
    }

    /** CSV のファイル。上限までしか読まない(超えたら断る・全部をメモリに読まない)。 */
    fun importCsvFile(file: File, entity: Entity, c: Component?, label: String) {
        val bytes = ImageSource.readFile(file, RecordImport.MAX_TEXT_BYTES) ?: return run { notice = Strings["desktop_io_file_too_large"] }
        importCsv(bytes, entity, c, label)
    }

    fun importCsv(bytes: ByteArray, entity: Entity, c: Component?, label: String) {
        val table = RecordIo.csvTable(bytes) ?: return run { notice = Strings["desktop_io_unreadable_encoding"] }
        offer(table, entity, c, label)
    }

    private fun offer(table: List<List<String>>, entity: Entity, c: Component?, label: String) {
        val plan = RecordIo.plan(table, entity, c?.map.orEmpty(), read(), entities)
        if (plan.matched.isEmpty()) return run { notice = Strings["desktop_io_no_matching_columns"] }
        pending = PendingImport(entity.id, label, plan)
    }

    fun confirm() {
        val p = pending ?: return
        update(RecordIo.apply(read(), p.entityId, p.plan.rows))
        notice = Strings.format("desktop_io_added_rows", p.plan.rows.size)
        pending = null
    }

    // --- クリップボード(#1766) ---

    /** 一覧の記録を表計算に貼れる形(TSV・見出し付き)でコピー。[ids] があれば選んだ行だけ・その並び(画面の並び・#1766)。 */
    fun copyRows(entityId: String, ids: List<String>? = null) {
        val entity = entity(entityId) ?: return
        val table = RecordIo.exportTable(entity.name, entity, read(), entities, null, ids = ids?.takeIf { it.isNotEmpty() })
        copyText(Tsv.string(table), table.rows.size)
    }

    /** 一覧の選んだ行を外へ引き出す形(TSV の文字 + CSV のファイル・#1764)。中身は [copyRows] と同じ([ids] の並び)。 */
    fun rowsTransferable(entityId: String, ids: List<String>): Transferable? {
        val entity = entity(entityId) ?: return null
        if (ids.isEmpty()) return null
        return DragOut.rows(RecordIo.exportTable(entity.name, entity, read(), entities, null, ids = ids))
    }

    /** 作った TSV をクリップボードへ(table の選んだ行など)。 */
    fun copyText(tsv: String, rows: Int) {
        runCatching { Toolkit.getDefaultToolkit().systemClipboard.setContents(StringSelection(tsv), null) }
            .onFailure { return run { notice = Strings["desktop_io_copy_failed"] } }
        notice = Strings.format("desktop_io_copied_rows", rows)
    }

    /** クリップボードの表(TSV / CSV)を一覧に取り込む確認。 */
    fun pasteRows(entityId: String) {
        val entity = entity(entityId) ?: return
        val text = clipboardText() ?: return run { notice = Strings["desktop_io_no_table_on_clipboard"] }
        offer(RecordIo.pastedTable(text), entity, null, Strings["desktop_io_pasted_table"])
    }

    /**
     * Ctrl+V(macOS は ⌘V)。クリップボードが画像・ファイルの時だけ受け取る(文字は入力欄の貼り付けに任せる)。
     * 画像は画面の最初の photoField、ファイルは [dropFiles] と同じ振り分け。
     */
    fun onKey(e: KeyEvent, screen: List<Component>, draft: MutableMap<String, String>?): Boolean {
        if (e.type != KeyEventType.KeyDown || e.key != Key.V || !(e.isCtrlPressed || e.isMetaPressed)) return false
        val t = runCatching { Toolkit.getDefaultToolkit().systemClipboard.getContents(null) }.getOrNull() ?: return false
        return paste(t, screen, draft)
    }

    /** 貼り付け。ファイルは受け取る。文字もある時(表計算のセルは文字と画像の両方を載せる)は画像を取らず、入力欄に任せる。 */
    fun paste(t: Transferable, screen: List<Component>, draft: MutableMap<String, String>?): Boolean {
        files(t)?.let { return dropFiles(it, screen, draft) }
        if (t.isDataFlavorSupported(DataFlavor.stringFlavor)) return false
        return drop(t, screen, draft)
    }

    // --- ドラッグ & ドロップ(#1764) ---

    /**
     * 落とした物(ファイル・画像)を画面の部品に振り分ける。受け取ったら true。
     * [draft] は一番上の画面の書けるフォーム(無い・読み取りだけなら null。写真・動画は入れない)。
     */
    fun drop(t: Transferable, screen: List<Component>, draft: MutableMap<String, String>?): Boolean {
        // このアプリの行を落とし直しても取り込まない(#1764)
        if (InternalDrag.isInternal(t)) return false
        files(t)?.let { return dropFiles(it, screen, draft) }
        image(t)?.let { img ->
            val d = draft ?: return false
            val field = screen.flatten().firstOrNull { it.type == ComponentType.PHOTO_FIELD }?.field ?: return false
            d[field] = savePhoto(img).toURI().toString()
            notice = Strings["desktop_io_photo_added"]
            return true
        }
        return false
    }

    fun dropFiles(files: List<File>, screen: List<Component>, draft: MutableMap<String, String>?): Boolean {
        val all = screen.flatten()
        var used = false
        for (f in files) {
            val kind = FileKind.of(f.name)
            val media = when (kind) {
                FileKind.IMAGE -> ComponentType.PHOTO_FIELD
                FileKind.VIDEO -> ComponentType.VIDEO_FIELD
                else -> null
            }
            if (media != null && draft != null) {
                val field = all.firstOrNull { it.type == media && it.field != null && draft[it.field!!].isNullOrEmpty() }?.field
                    ?: all.firstOrNull { it.type == media }?.field
                if (field != null) {
                    // 引き出した一時ファイル(終わる時に消える)は残る場所へ写してから
                    draft[field] = (if (DragOut.isTemp(f)) keep(f) else f).toURI().toString()
                    used = true
                    continue
                }
            }
            val accept = when (kind) {
                FileKind.CSV -> Accept.CSV
                FileKind.TEXT -> Accept.TEXT
                FileKind.PDF -> Accept.PDF
                else -> null
            }
            val importer = all.firstOrNull { it.type == ComponentType.FILE_IMPORT && accept != null && accept in it.accept.ifEmpty { listOf(Accept.CSV) } }
            if (importer != null) {
                importFile(f, importer)
                used = true
                continue
            }
            // fileImport が無い画面でも、一覧に CSV を落とせば同じ取り込みの確認
            val list = all.firstOrNull { it.type == ComponentType.LIST && it.entity != null }
            val entity = entity(list?.entity)
            if (kind == FileKind.CSV && entity != null) {
                importCsvFile(f, entity, null, f.name)
                used = true
            }
        }
        if (!used) notice = Strings["desktop_io_drop_not_here"]
        return used
    }

    companion object {
        fun files(t: Transferable): List<File>? =
            if (t.isDataFlavorSupported(DataFlavor.javaFileListFlavor)) {
                runCatching { (t.getTransferData(DataFlavor.javaFileListFlavor) as List<*>).filterIsInstance<File>() }.getOrNull()?.takeIf { it.isNotEmpty() }
            } else {
                null
            }

        fun image(t: Transferable): BufferedImage? =
            if (t.isDataFlavorSupported(DataFlavor.imageFlavor)) {
                runCatching { toBuffered(t.getTransferData(DataFlavor.imageFlavor) as java.awt.Image) }.getOrNull()
            } else {
                null
            }

        fun clipboardText(): String? = try {
            Toolkit.getDefaultToolkit().systemClipboard.getData(DataFlavor.stringFlavor) as? String
        } catch (_: UnsupportedFlavorException) {
            null
        } catch (_: Exception) {
            null
        }

        private fun toBuffered(img: java.awt.Image): BufferedImage {
            if (img is BufferedImage) return img
            val b = BufferedImage(img.getWidth(null).coerceAtLeast(1), img.getHeight(null).coerceAtLeast(1), BufferedImage.TYPE_INT_ARGB)
            b.createGraphics().apply { drawImage(img, 0, 0, null) }.dispose()
            return b
        }

        /** 貼った画像の置き場(一時フォルダ・モックの写真は端末に残すだけ)。 */
        fun savePhoto(img: BufferedImage): File =
            File.createTempFile("paste-", ".png", photoDir()).also { ImageIO.write(img, "png", it) }

        /** 落としたファイルを [savePhoto] と同じ置き場へ写す(拡張子は残す)。 */
        fun keep(f: File): File {
            val ext = f.name.substringAfterLast('.', "").takeIf { it.isNotEmpty() }?.let { ".$it" } ?: ""
            return File.createTempFile("drop-", ext, photoDir()).also { f.copyTo(it, overwrite = true) }
        }

        private fun photoDir(): File = File(System.getProperty("java.io.tmpdir"), "tailorit-photos").apply { mkdirs() }

        fun openDialog(title: String): File? {
            val d = FileDialog(null as Frame?, title, FileDialog.LOAD)
            d.isVisible = true
            val f = d.file ?: return null
            return File(d.directory, f)
        }
    }
}

private fun List<Component>.flatten(): List<Component> = flatMap { listOf(it) + it.children.flatten() }

@Composable
fun rememberDesktopIo(dsl: AppDSL, read: () -> MockStore, update: (MockStore) -> Unit): DesktopIo = remember(dsl) { DesktopIo(dsl, read, update) }

/** 画面全体でファイル・画像を受ける。このアプリの中から引いた行は受けない(#1764)。 */
@OptIn(ExperimentalComposeUiApi::class)
fun Modifier.ioDropTarget(io: DesktopIo, screen: () -> List<Component>, draft: () -> MutableMap<String, String>?): Modifier =
    dragAndDropTarget(
        shouldStartDragAndDrop = { e -> !InternalDrag.isInternal(runCatching { e.awtTransferable }.getOrNull()) },
        target = object : DragAndDropTarget {
            override fun onDrop(event: DragAndDropEvent): Boolean = io.drop(event.awtTransferable, screen(), draft())
        },
    )

/** Ctrl+V の画像・ファイルの貼り付け(onPreviewKeyEvent に渡す)。 */
fun DesktopIo.previewKey(screen: () -> List<Component>, draft: () -> MutableMap<String, String>?): (KeyEvent) -> Boolean = { onKey(it, screen(), draft()) }

/** `fileImport`: ボタンでファイルを選ぶ(落としても入る)。 */
@Composable
fun FileImportLeaf(c: Component, io: DesktopIo) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        OutlinedButton(onClick = { io.pick(c) }) { Text(c.label ?: Strings["desktop_io_import_file"]) }
        val kinds = c.accept.ifEmpty { listOf(Accept.CSV) }.joinToString(Strings["desktop_io_kind_separator"]) { it.wire.uppercase() }
        Text(Strings.format("desktop_io_import_drop_hint", kinds), style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

/** `photoField` / `videoField`: ファイルを選ぶ・落とす・Ctrl+V。写真は縮小して見せ、押すと大きく見る・外へ引き出せる。[readOnly] の時は見せるだけ。 */
@Composable
fun MediaFieldLeaf(c: Component, label: String, draft: MutableMap<String, String>, readOnly: Boolean = false) {
    val field = c.field ?: return
    val uri = draft[field]
    val file = uri?.let(::fileOf)
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(label, style = MaterialTheme.typography.labelLarge)
        if (file != null && c.type == ComponentType.PHOTO_FIELD) {
            val bitmap = remember(uri) { runCatching { ImageSource.readFile(file)?.let { SkiaImage.makeFromEncoded(it).toComposeImageBitmap() } }.getOrNull() }
            bitmap?.let { Image(it, contentDescription = label, modifier = Modifier.heightIn(max = 160.dp).widthIn(max = 240.dp).photoPreviewClick(uri, label).fileDragSource(file)) }
        } else if (file != null) {
            Text(file.name, modifier = Modifier.fileDragSource(file))
        }
        if (readOnly) return@Column
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { DesktopIo.openDialog(label)?.let { draft[field] = it.toURI().toString() } }) { Text(Strings["desktop_io_choose_file"]) }
            if (file != null) TextButton(onClick = { draft.remove(field) }) { Text(Strings["desktop_io_remove"]) }
        }
        Text(Strings[if (c.type == ComponentType.PHOTO_FIELD) "desktop_io_photo_hint" else "desktop_io_video_hint"], style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

/** file URI(file:/C:/a%20b.png など)→ File。読めない値は null(描く途中で投げない)。 */
fun fileOf(uri: String): File? = runCatching { File(java.net.URI(uri)) }.getOrNull()

/** 一覧の上のコピー・貼り付け(表計算と行き来する)。 */
@Composable
fun ListIoBar(entityId: String, io: DesktopIo, selected: List<String> = emptyList()) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        // 行を選んでいればその行だけ、無ければ全部(今までどおり)
        TextButton(onClick = { io.copyRows(entityId, selected) }) {
            Text(if (selected.isEmpty()) Strings["desktop_io_copy_table"] else Strings.format("desktop_io_copy_selected", selected.size))
        }
        TextButton(onClick = { io.pasteRows(entityId) }) { Text(Strings["desktop_io_paste_table"]) }
    }
}

/** 取り込みの確認と結果の一言。 */
@Composable
fun IoOverlay(io: DesktopIo) {
    io.pending?.let { p ->
        AlertDialog(
            onDismissRequest = { io.pending = null },
            title = { Text(Strings.format("desktop_io_import_confirm", p.label)) },
            text = { Column(verticalArrangement = Arrangement.spacedBy(4.dp)) { RecordIo.summary(p.plan).forEach { Text(it) } } },
            confirmButton = { TextButton(onClick = io::confirm, enabled = p.plan.rows.isNotEmpty()) { Text(Strings["desktop_io_import"]) } },
            dismissButton = { TextButton(onClick = { io.pending = null }) { Text(Strings["desktop_io_cancel"]) } },
        )
    }
}

/** 写真・ファイルを外(エクスプローラー・他のアプリ)へ引き出す。 */
private fun Modifier.fileDragSource(file: File): Modifier = dragOut { FileTransferable(listOf(file)) }
