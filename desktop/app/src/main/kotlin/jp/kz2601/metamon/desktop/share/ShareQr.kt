package jp.kz2601.metamon.desktop.share

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.DesktopActionHost
import jp.kz2601.metamon.desktop.QrMatrix
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.desktop.io.ExportClipboard
import jp.kz2601.metamon.desktop.io.FileNames
import jp.kz2601.metamon.desktop.store.writeAtomically
import java.awt.datatransfer.Clipboard
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.Transferable
import java.awt.datatransfer.UnsupportedFlavorException
import java.awt.image.BufferedImage
import java.io.ByteArrayOutputStream
import java.io.File
import javax.imageio.ImageIO

/**
 * 共有のリンクの QR(#3205)。iPhone のカメラで読んで参加する。QR は zxing(QR の部品と同じ [QrMatrix])。
 * 白地に黒・周りに 4 マスの余白(ダークモードでも読み取りやすさ優先)。
 * 保存は書き出しと同じ名前の決まり([FileNames]: 既存は上書きしない・Windows の予約名は頭に `_`)。
 */
object ShareQr {
    /** 1 マスの画素。保存・コピーの画像の大きさ(37 マス + 余白 8 で 360px ほど) */
    const val MODULE_PX = 8

    /** 保存の名前が空の時 */
    const val FALLBACK_NAME = "share-qr"

    /** リンクの QR の画像。空・長すぎる時は null。 */
    fun image(url: String, modulePx: Int = MODULE_PX): BufferedImage? {
        val m = QrMatrix.encode(url.trim()) ?: return null
        val px = m.size * modulePx
        val img = BufferedImage(px, px, BufferedImage.TYPE_INT_RGB)
        for (y in 0 until px) for (x in 0 until px) img.setRGB(x, y, if (m[x / modulePx, y / modulePx]) BLACK else WHITE)
        return img
    }

    fun png(url: String, modulePx: Int = MODULE_PX): ByteArray? {
        val img = image(url, modulePx) ?: return null
        val out = ByteArrayOutputStream()
        return if (ImageIO.write(img, "png", out)) out.toByteArray() else null
    }

    /** 保存の名前(`<label>.png`)。使えない字は `_`・予約名は頭に `_`(DragOut・書き出しと同じ)。 */
    fun fileName(label: String): String = DragOut.fileName(label, FALLBACK_NAME) + ".png"

    /** [dir] に空いている名前で PNG を書く(既存は上書きしない)。書いたファイル。QR にできない時は null。 */
    fun save(url: String, dir: File, label: String): File? {
        val bytes = png(url) ?: return null
        val file = FileNames.createFree(dir, fileName(label))
        try {
            writeAtomically(file, bytes)
        } catch (e: Exception) {
            file.delete()
            throw e
        }
        return file
    }

    /** クリップボードへ画像で入れる。入れられたか。 */
    fun copy(url: String, clipboard: Clipboard? = ExportClipboard.system()): Boolean {
        val cb = clipboard ?: return false
        val img = image(url) ?: return false
        return runCatching { cb.setContents(ImageTransfer(img), null) }.isSuccess
    }

    private class ImageTransfer(private val image: BufferedImage) : Transferable {
        override fun getTransferDataFlavors(): Array<DataFlavor> = arrayOf(DataFlavor.imageFlavor)

        override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor == DataFlavor.imageFlavor

        override fun getTransferData(flavor: DataFlavor): Any =
            if (flavor == DataFlavor.imageFlavor) image else throw UnsupportedFlavorException(flavor)
    }

    private const val BLACK = 0x000000
    private const val WHITE = 0xFFFFFF
}

/**
 * 共有のリンクのダイアログに入れる QR の欄(#3205)。絵・案内・「画像を保存」「画像をコピー」。
 * 結果の一言は読み上げで知らせる。リンクが QR にできない(空・長すぎる)時は何も出さない。
 */
@Composable
fun ShareQrPanel(
    url: String,
    appName: String,
    modifier: Modifier = Modifier,
    exportDir: File = DesktopActionHost.defaultExportDir(),
) {
    val matrix = remember(url) { QrMatrix.encode(url.trim()) } ?: return
    var message by remember(url) { mutableStateOf<String?>(null) }
    val description = Strings["desktop_share_qr_description"]
    Column(modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Canvas(
            Modifier.widthIn(max = 240.dp).fillMaxWidth().aspectRatio(1f).background(Color.White)
                .semantics { contentDescription = description; role = Role.Image },
        ) {
            val cell = size.width / matrix.size
            for (y in 0 until matrix.size) for (x in 0 until matrix.size) {
                if (matrix[x, y]) drawRect(Color.Black, Offset(x * cell, y * cell), Size(cell + 0.5f, cell + 0.5f))
            }
        }
        Text(Strings["desktop_share_qr_hint"], style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            TextButton(onClick = {
                message = runCatching { ShareQr.save(url, exportDir, Strings.format("desktop_share_qr_file", appName)) }.getOrNull()
                    ?.let { Strings.format("desktop_share_qr_saved", it.absolutePath) } ?: Strings["desktop_share_qr_failed"]
            }) { Text(Strings["desktop_share_qr_save"]) }
            TextButton(onClick = {
                if (ShareQr.copy(url)) message = Strings["desktop_share_qr_copied"]
            }) { Text(Strings["desktop_qr_copy_image"]) }
        }
        message?.let { Text(it, style = MaterialTheme.typography.bodySmall, modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite }) }
    }
}
