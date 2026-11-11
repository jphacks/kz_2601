package jp.kz2601.metamon.desktop.io

import androidx.compose.foundation.Image
import androidx.compose.foundation.draganddrop.dragAndDropSource
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
import androidx.compose.ui.draganddrop.DragAndDropTransferAction
import androidx.compose.ui.draganddrop.DragAndDropTransferData
import androidx.compose.ui.draganddrop.DragAndDropTransferable
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
import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionFormat
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.logic.ImportPlan
import jp.kz2601.metamon.dsl.logic.RecordExport
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
import java.time.LocalDateTime
import java.time.format.DateTimeFormatter
import javax.imageio.ImageIO

/** 取り込みの確認待ち(CSV・TSV)。 */
data class PendingImport(val entityId: String, val label: String, val plan: ImportPlan)

/** 部品のうち、このパッケージが描くもの(fileImport・photoField・videoField)。 */
object IoLeaves {
    val types: Set<ComponentType> = setOf(ComponentType.FILE_IMPORT, ComponentType.PHOTO_FIELD, ComponentType.VIDEO_FIELD)
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

    // --- 書き出し(#1753) ---

    fun export(action: Action, screenEntity: String?) {
        val entity = entity(action.entity ?: screenEntity) ?: return run { notice = "書き出す記録が見つかりません" }
        val format = action.format?.takeIf { it != ActionFormat.UNKNOWN } ?: return run { notice = "書き出せませんでした" }
        val store = read()
        val title = action.title?.trim()?.ifEmpty { null } ?: entity.name
        val date = LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm"))
        val count = store.list(entity.id).size
        val table = RecordIo.exportTable(title, entity, store, entities, action.fields, listOf("書き出し日 $date", "$count 件"))
        val (bytes, ext) = when (format) {
            ActionFormat.CSV -> RecordIo.csv(table) to "csv"
            else -> PdfIo.data(table) to "pdf"
        }
        val file = saveDialog(RecordExport.fileName(title, ext)) ?: return
        notice = runCatching { file.writeBytes(bytes) }.fold({ "書き出しました: ${file.name}" }, { "書き出せませんでした" })
    }

    // --- 取り込み(#1753) ---

    fun pick(c: Component) {
        val file = openDialog(c.label ?: "ファイルを選ぶ") ?: return
        importFile(file, c)
    }

    /** fileImport にファイルを 1 つ入れる。CSV は確認を出し、テキスト・PDF は本文を 1 件の記録にする。 */
    fun importFile(file: File, c: Component) {
        val entity = entity(c.entity) ?: return run { notice = "取り込み先のフォームがありません" }
        val accept = c.accept.ifEmpty { listOf(Accept.CSV) }
        when (FileKind.of(file.name)) {
            FileKind.CSV -> if (Accept.CSV in accept) importCsv(file.readBytes(), entity, c, c.label ?: file.name) else notice = "このファイルは取り込めません"
            FileKind.TEXT -> if (Accept.TEXT in accept) {
                if (file.length() > RecordImport.MAX_TEXT_BYTES) return run { notice = "ファイルが大きすぎます" }
                val text = RecordImport.plainText(file.readBytes()) ?: return run { notice = "文字コードが読めません" }
                addBody(text, file.name, entity, c)
            } else {
                notice = "このファイルは取り込めません"
            }
            FileKind.PDF -> if (Accept.PDF in accept) {
                when (val r = PdfIo.text(file.readBytes())) {
                    is PdfIo.TextResult.Text -> addBody(r.text, file.name, entity, c)
                    PdfIo.TextResult.Locked -> notice = "パスワードのかかった PDF は読めません"
                    PdfIo.TextResult.NoText -> notice = "文字の入っていない PDF(スキャン)はデスクトップ版では読めません"
                    PdfIo.TextResult.TooLarge -> notice = "ファイルが大きすぎます"
                    PdfIo.TextResult.Broken -> notice = "PDF が読めません"
                }
            } else {
                notice = "このファイルは取り込めません"
            }
            else -> notice = "このファイルは取り込めません"
        }
    }

    private fun addBody(text: String, name: String, entity: Entity, c: Component) {
        val field = c.field ?: entity.storedFields.firstOrNull { it.type == jp.kz2601.metamon.dsl.records.FieldType.TEXT }?.id ?: return
        val body = RecordImport.bodyRecord(text, name, entity, field) ?: return run { notice = "取り込み先の項目がありません" }
        update(RecordIo.apply(read(), entity.id, listOf(body.data)))
        notice = if (body.truncated) "取り込みました(長いので途中まで)" else "取り込みました"
    }

    fun importCsv(bytes: ByteArray, entity: Entity, c: Component?, label: String) {
        val table = RecordIo.csvTable(bytes) ?: return run { notice = "文字コードが読めません" }
        offer(table, entity, c, label)
    }

    private fun offer(table: List<List<String>>, entity: Entity, c: Component?, label: String) {
        val plan = RecordIo.plan(table, entity, c?.map.orEmpty(), read(), entities)
        if (plan.matched.isEmpty()) return run { notice = "当てられる列がありません(1 行目の見出しを項目名にしてください)" }
        pending = PendingImport(entity.id, label, plan)
    }

    fun confirm() {
        val p = pending ?: return
        update(RecordIo.apply(read(), p.entityId, p.plan.rows))
        notice = "${p.plan.rows.size} 件を追加しました"
        pending = null
    }

    // --- クリップボード(#1766) ---

    /** 一覧の記録を表計算に貼れる形(TSV・見出し付き)でコピー。 */
    fun copyRows(entityId: String) {
        val entity = entity(entityId) ?: return
        val table = RecordIo.exportTable(entity.name, entity, read(), entities, null)
        Toolkit.getDefaultToolkit().systemClipboard.setContents(StringSelection(Tsv.string(table)), null)
        notice = "${table.rows.size} 件をコピーしました"
    }

    /** クリップボードの表(TSV / CSV)を一覧に取り込む確認。 */
    fun pasteRows(entityId: String) {
        val entity = entity(entityId) ?: return
        val text = clipboardText() ?: return run { notice = "クリップボードに表がありません" }
        offer(RecordIo.pastedTable(text), entity, null, "貼り付けた表")
    }

    /**
     * Ctrl+V(macOS は ⌘V)。クリップボードが画像・ファイルの時だけ受け取る(文字は入力欄の貼り付けに任せる)。
     * 画像は画面の最初の photoField、ファイルは [dropFiles] と同じ振り分け。
     */
    fun onKey(e: KeyEvent, screen: List<Component>, draft: MutableMap<String, String>): Boolean {
        if (e.type != KeyEventType.KeyDown || e.key != Key.V || !(e.isCtrlPressed || e.isMetaPressed)) return false
        val t = runCatching { Toolkit.getDefaultToolkit().systemClipboard.getContents(null) }.getOrNull() ?: return false
        return drop(t, screen, draft)
    }

    // --- ドラッグ & ドロップ(#1764) ---

    /** 落とした物(ファイル・画像)を画面の部品に振り分ける。受け取ったら true。 */
    fun drop(t: Transferable, screen: List<Component>, draft: MutableMap<String, String>): Boolean {
        files(t)?.let { return dropFiles(it, screen, draft) }
        image(t)?.let { img ->
            val field = screen.flatten().firstOrNull { it.type == ComponentType.PHOTO_FIELD }?.field ?: return false
            draft[field] = savePhoto(img).toURI().toString()
            notice = "写真を入れました"
            return true
        }
        return false
    }

    fun dropFiles(files: List<File>, screen: List<Component>, draft: MutableMap<String, String>): Boolean {
        val all = screen.flatten()
        var used = false
        for (f in files) {
            val kind = FileKind.of(f.name)
            val media = when (kind) {
                FileKind.IMAGE -> ComponentType.PHOTO_FIELD
                FileKind.VIDEO -> ComponentType.VIDEO_FIELD
                else -> null
            }
            if (media != null) {
                val field = all.firstOrNull { it.type == media && it.field != null && draft[it.field!!].isNullOrEmpty() }?.field
                    ?: all.firstOrNull { it.type == media }?.field
                if (field != null) {
                    draft[field] = f.toURI().toString()
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
                importCsv(f.readBytes(), entity, null, f.name)
                used = true
            }
        }
        if (!used) notice = "この画面には落とせません"
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
        fun savePhoto(img: BufferedImage): File {
            val dir = File(System.getProperty("java.io.tmpdir"), "tailorit-photos").apply { mkdirs() }
            return File.createTempFile("paste-", ".png", dir).also { ImageIO.write(img, "png", it) }
        }

        fun saveDialog(name: String): File? {
            val d = FileDialog(null as Frame?, "書き出す場所", FileDialog.SAVE)
            d.file = name
            d.isVisible = true
            val f = d.file ?: return null
            return File(d.directory, f)
        }

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

/** 画面全体でファイル・画像を受ける。 */
@OptIn(ExperimentalComposeUiApi::class)
fun Modifier.ioDropTarget(io: DesktopIo, screen: () -> List<Component>, draft: () -> MutableMap<String, String>): Modifier =
    dragAndDropTarget(
        shouldStartDragAndDrop = { true },
        target = object : DragAndDropTarget {
            override fun onDrop(event: DragAndDropEvent): Boolean = io.drop(event.awtTransferable, screen(), draft())
        },
    )

/** Ctrl+V の画像・ファイルの貼り付け(onPreviewKeyEvent に渡す)。 */
fun DesktopIo.previewKey(screen: () -> List<Component>, draft: () -> MutableMap<String, String>): (KeyEvent) -> Boolean = { onKey(it, screen(), draft()) }

/** `fileImport`: ボタンでファイルを選ぶ(落としても入る)。 */
@Composable
fun FileImportLeaf(c: Component, io: DesktopIo) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        OutlinedButton(onClick = { io.pick(c) }) { Text(c.label ?: "ファイルを取り込む") }
        val kinds = c.accept.ifEmpty { listOf(Accept.CSV) }.joinToString("・") { it.wire.uppercase() }
        Text("$kinds をここへ落としても取り込めます", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

/** `photoField` / `videoField`: ファイルを選ぶ・落とす・Ctrl+V。写真は縮小して見せ、外へ引き出せる。 */
@Composable
fun MediaFieldLeaf(c: Component, label: String, draft: MutableMap<String, String>) {
    val field = c.field ?: return
    val uri = draft[field]
    val file = uri?.let { runCatching { File(java.net.URI(it)) }.getOrNull() }
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(label, style = MaterialTheme.typography.labelLarge)
        if (file != null && c.type == ComponentType.PHOTO_FIELD) {
            val bitmap = remember(uri) { runCatching { SkiaImage.makeFromEncoded(file.readBytes()).toComposeImageBitmap() }.getOrNull() }
            bitmap?.let { Image(it, contentDescription = label, modifier = Modifier.heightIn(max = 160.dp).widthIn(max = 240.dp).fileDragSource(file)) }
        } else if (file != null) {
            Text(file.name, modifier = Modifier.fileDragSource(file))
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { DesktopIo.openDialog(label)?.let { draft[field] = it.toURI().toString() } }) { Text("ファイルを選ぶ") }
            if (file != null) TextButton(onClick = { draft.remove(field) }) { Text("外す") }
        }
        Text(if (c.type == ComponentType.PHOTO_FIELD) "画像を落とすか Ctrl+V で貼れます" else "動画を落とせます", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

/** 一覧の上のコピー・貼り付け(表計算と行き来する)。 */
@Composable
fun ListIoBar(entityId: String, io: DesktopIo) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        TextButton(onClick = { io.copyRows(entityId) }) { Text("表をコピー") }
        TextButton(onClick = { io.pasteRows(entityId) }) { Text("表を貼り付け") }
    }
}

/** 取り込みの確認と結果の一言。 */
@Composable
fun IoOverlay(io: DesktopIo) {
    io.pending?.let { p ->
        AlertDialog(
            onDismissRequest = { io.pending = null },
            title = { Text("${p.label} を取り込みますか？") },
            text = { Column(verticalArrangement = Arrangement.spacedBy(4.dp)) { RecordIo.summary(p.plan).forEach { Text(it) } } },
            confirmButton = { TextButton(onClick = io::confirm, enabled = p.plan.rows.isNotEmpty()) { Text("取り込む") } },
            dismissButton = { TextButton(onClick = { io.pending = null }) { Text("やめる") } },
        )
    }
}

/** 写真・ファイルを外(エクスプローラー・他のアプリ)へ引き出す。 */
@OptIn(ExperimentalComposeUiApi::class)
private fun Modifier.fileDragSource(file: File): Modifier = dragAndDropSource { _ ->
    DragAndDropTransferData(
        transferable = DragAndDropTransferable(FileTransferable(listOf(file))),
        supportedActions = listOf(DragAndDropTransferAction.Copy),
    )
}

private class FileTransferable(private val files: List<File>) : Transferable {
    override fun getTransferDataFlavors(): Array<DataFlavor> = arrayOf(DataFlavor.javaFileListFlavor)
    override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor == DataFlavor.javaFileListFlavor
    override fun getTransferData(flavor: DataFlavor): Any = if (isDataFlavorSupported(flavor)) files else throw UnsupportedFlavorException(flavor)
}
