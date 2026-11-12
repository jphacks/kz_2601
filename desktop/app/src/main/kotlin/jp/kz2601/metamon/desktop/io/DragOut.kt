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
import java.io.InputStream
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

    /** 画像(PNG・JPEG など)→ PNG のファイル。読めない画像は null(引き出さない)。変換するので画面のスレッドでは呼ばない。 */
    fun image(bytes: ByteArray, name: String): ImageTransferable? = png(bytes)?.let { imageOfPng(it, name) }

    /** 読んだ時に変換しておいた PNG(LoadedImage.png)から。引き始めに重い処理をしない。 */
    fun imageOfPng(png: ByteArray, name: String): ImageTransferable =
        ImageTransferable(png) { temp(fileName(name, "image") + ".png", png) }

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

    /** ファイル名に使えない文字(Windows の \ / : * ? " < > | と制御文字)を _ に。予約された名前は頭に _。空なら [fallback]。 */
    fun fileName(title: String, fallback: String): String {
        val s = title.map { if (it in "\\/:*?\"<>|" || it.isISOControl()) '_' else it }.joinToString("").trim().trimEnd('.').take(80)
        val name = s.ifEmpty { fallback }
        return FileNames.guard(name)
    }

    /** 引き出したファイルを書く場所。 */
    fun tempRoot(): File = File(System.getProperty("java.io.tmpdir"), "tailorit-drag")

    /** [f] が引き出し用の一時ファイルか(終わる時に消えるので、落とし直された時は残る場所へ写す)。 */
    fun isTemp(f: File): Boolean {
        val root = runCatching { tempRoot().canonicalFile }.getOrNull() ?: return false
        var p: File? = runCatching { f.canonicalFile }.getOrNull()
        while (p != null) {
            if (p == root) return true
            p = p.parentFile
        }
        return false
    }

    private const val DAY_MS = 24L * 60 * 60 * 1000

    /** 起動時: 落ちた時などに残った [prefix]* のフォルダのうち [maxAgeMs](既定 1 日)より古いものを消す。消した数を返す。 */
    fun sweep(now: Long = System.currentTimeMillis(), maxAgeMs: Long = DAY_MS, root: File = tempRoot(), prefix: String = "drag-"): Int =
        root.listFiles { f -> f.isDirectory && f.name.startsWith(prefix) && now - f.lastModified() > maxAgeMs }
            .orEmpty()
            .count { it.deleteRecursively() }

    /** [root] の drag-* のフォルダを古さに関わらず全部消す(この端末のデータを消す時)。消した数を返す。 */
    fun clear(root: File = tempRoot()): Int =
        root.listFiles { f -> f.isDirectory && f.name.startsWith("drag-") }.orEmpty().count { it.deleteRecursively() }

    /**
     * 引き出すたびに別のフォルダ(名前はそのまま見せたいので、ファイル名ではなくフォルダで重ならないようにする)。
     * 写真のプレビューの「既定のアプリで開く」は [root] に自分の場所(PhotoOpen.tempRoot)を渡す。
     */
    internal fun temp(name: String, bytes: ByteArray, root: File = tempRoot()): File {
        root.mkdirs()
        val dir = Files.createTempDirectory(root.toPath(), "drag-").toFile().apply { deleteOnExit() }
        return File(dir, name).apply {
            writeBytes(bytes)
            deleteOnExit()
        }
    }
}

/** 画像の値(data URL・https・手元のファイル)のバイト列。 */
object ImageSource {
    /** 読む画像の大きさの上限(10 MB)。超えたら読まない。 */
    const val MAX_BYTES = 10 * 1024 * 1024
    const val CONNECT_TIMEOUT_MS = 10_000
    const val READ_TIMEOUT_MS = 15_000

    /**
     * [allowFile] は記録・フォームの値の時だけ(photoField の値は file URI)。DSL に書かれた URL では手元のファイルを読まない。
     * どの読み方も [max] バイトまで(手元のファイルも読みながら数える・途中で大きくなる物も止まる)。
     */
    fun bytes(src: String?, allowFile: Boolean, max: Int = MAX_BYTES): ByteArray? {
        if (src.isNullOrBlank()) return null
        return runCatching {
            when {
                src.startsWith("data:") -> dataUrl(src, max)
                src.startsWith("https://") -> URI(src).toURL().openConnection().run {
                    connectTimeout = CONNECT_TIMEOUT_MS
                    readTimeout = READ_TIMEOUT_MS
                    if (contentLengthLong > max) null else getInputStream().use { capped(it, max) }
                }
                allowFile && src.startsWith("file:") -> fileOf(src)?.takeIf { it.isFile && FileKind.of(it.name) == FileKind.IMAGE }?.let { readFile(it, max) }
                else -> null
            }
        }.getOrNull()
    }

    /** data URL の base64。解く前に長さで上限を確かめる(大きな文字列を解いてから捨てない)。 */
    internal fun dataUrl(src: String, max: Int = MAX_BYTES): ByteArray? {
        val b64 = src.substringAfter("base64,")
        if (b64.length.toLong() > base64Length(max)) return null
        return Base64.getDecoder().decode(b64).takeIf { it.size <= max }
    }

    /** [bytes] バイトを base64 にした長さ(= の詰め物を含む)。 */
    internal fun base64Length(bytes: Int): Long = (bytes.toLong() + 2) / 3 * 4

    /** ファイルを [max] バイトまで読む。超えたら null(`length()` は当てにせず、読んだ量で数える)。 */
    fun readFile(file: File, max: Int = MAX_BYTES): ByteArray? = file.inputStream().use { capped(it, max) }

    /** [max] バイトまで読む。超えたら null。 */
    fun capped(input: InputStream, max: Int = MAX_BYTES): ByteArray? {
        val out = ByteArrayOutputStream()
        val buf = ByteArray(16 * 1024)
        while (true) {
            val n = input.read(buf)
            if (n < 0) return out.toByteArray()
            if (out.size() + n > max) return null
            out.write(buf, 0, n)
        }
    }
}

/**
 * このアプリの中から引いた物の印(#1764)。同じアプリの中に落とした時は受け取らない(行を落とし直して取り込みが始まらないように)。
 * 同じ JVM の中のドラッグでは AWT が元の Transferable の種類をそのまま見せるので、種類で見分けられる。
 */
object InternalDrag {
    val FLAVOR: DataFlavor = DataFlavor(DataFlavor.javaJVMLocalObjectMimeType + ";class=java.lang.String", "TailorIt internal drag")

    fun isInternal(t: Transferable?): Boolean = t != null && runCatching { t.isDataFlavorSupported(FLAVOR) }.getOrDefault(false)
}

/** 行: 文字(TSV)を先に、ファイル(CSV)を後に。ファイルを受ける OS・アプリはファイル、文字だけのアプリは TSV を取る。中の印も載せる。 */
class RowsTransferable(val tsv: String, csv: () -> File) : Transferable {
    private val csv by lazy(csv)

    override fun getTransferDataFlavors(): Array<DataFlavor> = arrayOf(DataFlavor.stringFlavor, DataFlavor.javaFileListFlavor, InternalDrag.FLAVOR)
    override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor in getTransferDataFlavors()
    override fun getTransferData(flavor: DataFlavor): Any = when (flavor) {
        DataFlavor.stringFlavor -> tsv
        DataFlavor.javaFileListFlavor -> listOf(csv)
        InternalDrag.FLAVOR -> "rows"
        else -> throw UnsupportedFlavorException(flavor)
    }
}

/** 画像: PNG のファイル。画像を直接受けるアプリ向けに画像そのものも載せる(受け手が求めた時に、変換済みの PNG から 1 度だけ作る)。 */
class ImageTransferable(val png: ByteArray, file: () -> File) : Transferable {
    private val file by lazy(file)
    private val image by lazy { runCatching { ImageIO.read(ByteArrayInputStream(png)) }.getOrNull() }

    override fun getTransferDataFlavors(): Array<DataFlavor> = arrayOf(DataFlavor.javaFileListFlavor, DataFlavor.imageFlavor)
    override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor in getTransferDataFlavors()
    override fun getTransferData(flavor: DataFlavor): Any = when (flavor) {
        DataFlavor.javaFileListFlavor -> listOf(file)
        DataFlavor.imageFlavor -> image ?: throw UnsupportedFlavorException(flavor)
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
