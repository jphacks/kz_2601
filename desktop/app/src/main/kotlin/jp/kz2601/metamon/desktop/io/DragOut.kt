package jp.kz2601.metamon.desktop.io

import androidx.compose.foundation.draganddrop.dragAndDropSource
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.draganddrop.DragAndDropTransferAction
import androidx.compose.ui.draganddrop.DragAndDropTransferData
import androidx.compose.ui.draganddrop.DragAndDropTransferable
import jp.kz2601.metamon.dsl.logic.ExportTable
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.Transferable
import java.awt.datatransfer.UnsupportedFlavorException
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.net.URI
import java.nio.file.Files
import java.util.Base64
import javax.imageio.ImageIO

/**
 * 外(エクスプローラー・表計算・他のアプリ)へ引き出す(#1764)。
 * 行はコピーと同じ TSV の文字 + CSV のファイル、画像は PNG のファイル。ファイルは落とした時に一時フォルダへ書く。
 */
object DragOut {
    private val PNG_MAGIC = byteArrayOf(0x89.toByte(), 'P'.code.toByte(), 'N'.code.toByte(), 'G'.code.toByte())

    /** 行(見出し付き)。文字は Ctrl+C と同じ TSV、ファイルは書き出しと同じ CSV(BOM 付き UTF-8)。 */
    fun rows(table: ExportTable): RowsTransferable =
        RowsTransferable(Tsv.string(table)) { temp(fileName(table.title, "rows") + ".csv", RecordIo.csv(table)) }

    /** 画像(PNG・JPEG など)→ PNG のファイル。読めない画像は null(引き出さない)。 */
    fun image(bytes: ByteArray, name: String): ImageTransferable? {
        val png = png(bytes) ?: return null
        return ImageTransferable(png) { temp(fileName(name, "image") + ".png", png) }
    }

    /** PNG にそろえる。PNG はそのまま、他は ImageIO(JPEG・GIF・BMP)、だめなら Skia(WebP など)。 */
    fun png(bytes: ByteArray): ByteArray? {
        if (bytes.size >= 4 && bytes.copyOfRange(0, 4).contentEquals(PNG_MAGIC)) return bytes
        runCatching { ImageIO.read(ByteArrayInputStream(bytes)) }.getOrNull()?.let { img ->
            val out = ByteArrayOutputStream()
            if (ImageIO.write(img, "png", out)) return out.toByteArray()
        }
        return runCatching {
            org.jetbrains.skia.Image.makeFromEncoded(bytes).encodeToData(org.jetbrains.skia.EncodedImageFormat.PNG)?.bytes
        }.getOrNull()
    }

    /** ファイル名に使えない文字(Windows の \ / : * ? " < > | と制御文字)を _ に。空なら [fallback]。 */
    fun fileName(title: String, fallback: String): String {
        val s = title.map { if (it in "\\/:*?\"<>|" || it.isISOControl()) '_' else it }.joinToString("").trim().trimEnd('.').take(80)
        return s.ifEmpty { fallback }
    }

    /** 引き出すたびに別のフォルダ(名前はそのまま見せたいので、ファイル名ではなくフォルダで重ならないようにする)。 */
    private fun temp(name: String, bytes: ByteArray): File {
        val root = File(System.getProperty("java.io.tmpdir"), "tailorit-drag").apply { mkdirs() }
        val dir = Files.createTempDirectory(root.toPath(), "drag-").toFile().apply { deleteOnExit() }
        return File(dir, name).apply {
            writeBytes(bytes)
            deleteOnExit()
        }
    }
}

/** 画像の値(data URL・https・手元のファイル)のバイト列。 */
object ImageSource {
    /** [allowFile] は記録・フォームの値の時だけ(photoField の値は file URI)。DSL に書かれた URL では手元のファイルを読まない。 */
    fun bytes(src: String?, allowFile: Boolean): ByteArray? {
        if (src.isNullOrBlank()) return null
        return runCatching {
            when {
                src.startsWith("data:") -> Base64.getDecoder().decode(src.substringAfter("base64,"))
                src.startsWith("https://") -> URI(src).toURL().openStream().use { it.readBytes() }
                allowFile && src.startsWith("file:") -> fileOf(src)?.takeIf { it.isFile && FileKind.of(it.name) == FileKind.IMAGE }?.readBytes()
                else -> null
            }
        }.getOrNull()
    }
}

/** 行: 文字(TSV)を先に、ファイル(CSV)を後に。ファイルを受ける OS・アプリはファイル、文字だけのアプリは TSV を取る。 */
class RowsTransferable(val tsv: String, csv: () -> File) : Transferable {
    private val csv by lazy(csv)

    override fun getTransferDataFlavors(): Array<DataFlavor> = arrayOf(DataFlavor.stringFlavor, DataFlavor.javaFileListFlavor)
    override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor in getTransferDataFlavors()
    override fun getTransferData(flavor: DataFlavor): Any = when (flavor) {
        DataFlavor.stringFlavor -> tsv
        DataFlavor.javaFileListFlavor -> listOf(csv)
        else -> throw UnsupportedFlavorException(flavor)
    }
}

/** 画像: PNG のファイル。画像を直接受けるアプリ向けに画像そのものも載せる。 */
class ImageTransferable(val png: ByteArray, file: () -> File) : Transferable {
    private val file by lazy(file)

    override fun getTransferDataFlavors(): Array<DataFlavor> = arrayOf(DataFlavor.javaFileListFlavor, DataFlavor.imageFlavor)
    override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor in getTransferDataFlavors()
    override fun getTransferData(flavor: DataFlavor): Any = when (flavor) {
        DataFlavor.javaFileListFlavor -> listOf(file)
        DataFlavor.imageFlavor -> ImageIO.read(ByteArrayInputStream(png)) ?: throw UnsupportedFlavorException(flavor)
        else -> throw UnsupportedFlavorException(flavor)
    }
}

/** ファイルそのもの(photoField・videoField の値)。 */
class FileTransferable(private val files: List<File>) : Transferable {
    override fun getTransferDataFlavors(): Array<DataFlavor> = arrayOf(DataFlavor.javaFileListFlavor)
    override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor == DataFlavor.javaFileListFlavor
    override fun getTransferData(flavor: DataFlavor): Any = if (isDataFlavorSupported(flavor)) files else throw UnsupportedFlavorException(flavor)
}

/** 押して引くと外へ。[transferable] が null なら引き出さない。 */
@OptIn(ExperimentalComposeUiApi::class)
fun Modifier.dragOut(transferable: () -> Transferable?): Modifier = dragAndDropSource { _ ->
    transferable()?.let {
        DragAndDropTransferData(
            transferable = DragAndDropTransferable(it),
            supportedActions = listOf(DragAndDropTransferAction.Copy),
        )
    }
}
