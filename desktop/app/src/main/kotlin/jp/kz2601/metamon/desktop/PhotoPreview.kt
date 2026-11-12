package jp.kz2601.metamon.desktop

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.isTraversalGroup
import androidx.compose.ui.semantics.paneTitle
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.desktop.io.FileKind
import jp.kz2601.metamon.desktop.io.ImageSource
import jp.kz2601.metamon.desktop.io.fileOf
import jp.kz2601.metamon.desktop.store.AppDataDir
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.form.FormDraft
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.awt.Desktop
import java.io.File
import java.net.URI
import java.nio.file.Files
import java.nio.file.attribute.PosixFilePermissions

// 記録の写真を大きく見る(Mac の Quick Look・#1850 に当たる)。クリック / Space で開き、Esc / Space で閉じ、← → で送る。

/** 見せる写真 1 枚。[src] は記録の値(data URL か手元の file URI)、[name] は項目名。 */
data class PreviewPhoto(val src: String, val name: String)

/** 見せる物を集める(Mac の `MacQuickLook.sources` と同じ規則)。 */
object PhotoPreviewSources {
    /** 開いている記録・フォームの写真。 */
    fun of(form: FormDraft?): List<PreviewPhoto> = form?.let { photos(it.values, it.entity.fields) }.orEmpty()

    /** 項目の並び(DSL の順)で写真の項目だけ。隠す項目(#262)・算出項目は出さない。 */
    fun photos(values: Map<String, JSONValue>, fields: List<Field>): List<PreviewPhoto> =
        fields.filter { it.type == FieldType.PHOTO && !it.hidden && !it.isComputed }.mapNotNull { f ->
            values[f.id]?.stringValue?.takeIf(::accepts)?.let { PreviewPhoto(it, f.name) }
        }

    /**
     * data URL の画像と、手元の画像ファイル(file URI・拡張子が画像)だけ。https は出さない(Mac と同じ)。
     * 読むのは一覧の写真と同じ [ImageSource](読める場所は広げない)。
     */
    fun accepts(src: String?): Boolean = when {
        src.isNullOrBlank() -> false
        src.startsWith("data:image/") -> src.contains(";base64,")
        src.startsWith("file:") -> localFile(src)?.let { FileKind.of(it.name) == FileKind.IMAGE } == true
        else -> false
    }

    /**
     * この端末のファイルを指す file URI だけファイルにする。ホスト付き(`file://host/...`)・`file:////host/...`・
     * `\\host\share` の形(UNC・ネットワーク上の場所)は null。
     */
    fun localFile(src: String): File? {
        if (!src.startsWith("file:")) return null
        val uri = runCatching { URI(src) }.getOrNull() ?: return null
        if (!uri.rawAuthority.isNullOrEmpty()) return null
        val path = uri.path ?: return null
        if (path.startsWith("//") || '\\' in path) return null
        val file = fileOf(src) ?: return null
        return file.takeUnless { it.path.startsWith("\\\\") || it.path.startsWith("//") }
    }
}

/** 押しっぱなしの自動の繰り返しを見分ける(Space で開いてすぐ閉じない)。同じ出来事を何度見ても同じ答え。 */
class KeyRepeatFilter {
    private val held = HashSet<Key>()
    private var last: Any? = null
    private var lastRepeat = false

    /** [event] は出来事そのもの(同じ出来事を外側と内側で 2 回見るため)。押し下げで、離す前の 2 回目以降なら true。 */
    fun isRepeat(event: Any, key: Key, down: Boolean): Boolean {
        if (event === last) return lastRepeat
        last = event
        lastRepeat = if (down) !held.add(key) else false.also { held.remove(key) }
        return lastRepeat
    }

    /** 押している物を忘れる(離した出来事を取りこぼしても、次の押し下げを 1 回目として扱う)。 */
    fun reset() {
        held.clear()
        last = null
        lastRepeat = false
    }
}

/** Space の押しっぱなしを見る所は 1 つ(キーボードは 1 つ・出来事は画面のスレッドでだけ来る)。部品・一番外・ダイアログが同じ物を見る。 */
private val spaceRepeats = KeyRepeatFilter()

/** ウィンドウがフォーカスを失った時に呼ぶ(外で離した Space の出来事は届かない)。 */
fun resetSpaceRepeats() = spaceRepeats.reset()

/** Space の自動の繰り返し(押しっぱなし)か。プレビューを開く・閉じるのは押し下げた 1 回目だけ。 */
fun KeyEvent.isAutoRepeatSpace(): Boolean =
    key == Key.Spacebar && type != KeyEventType.Unknown && spaceRepeats.isRepeat(nativeKeyEvent, key, type == KeyEventType.KeyDown)

/** プレビューの中のキー。 */
enum class PreviewKey { CLOSE, PREVIOUS, NEXT }

/** 開いている写真の並びと今の 1 枚(ウィンドウごと・DesktopKeyboard が持つ)。 */
class PhotoPreview {
    var photos by mutableStateOf<List<PreviewPhoto>>(emptyList())
        private set
    var index by mutableIntStateOf(0)
        private set

    val isOpen: Boolean get() = photos.isNotEmpty()
    val current: PreviewPhoto? get() = photos.getOrNull(index)

    /** [list] を開き、[start] の写真から見せる(無ければ最初)。見せる物が無ければ開かない。 */
    fun open(list: List<PreviewPhoto>, start: String? = null): Boolean {
        if (list.isEmpty()) return false
        photos = list
        index = start?.let { s -> list.indexOfFirst { it.src == s } }?.takeIf { it >= 0 } ?: 0
        return true
    }

    /** 押した写真から開く。並びに無い写真(隠す項目に結んだ部品など)はその 1 枚だけ。見せられない値なら開かない。 */
    fun openAt(list: List<PreviewPhoto>, src: String, name: String): Boolean = when {
        list.any { it.src == src } -> open(list, src)
        PhotoPreviewSources.accepts(src) -> open(listOf(PreviewPhoto(src, name)))
        else -> false
    }

    fun close() {
        photos = emptyList()
        index = 0
    }

    fun next(): Boolean = (index < photos.lastIndex).also { if (it) index++ }

    fun previous(): Boolean = (index > 0).also { if (it) index-- }

    /** 読み上げ・見出しの名前(複数の時は「何枚目 / 全部」も)。 */
    fun title(): String {
        val name = current?.name.orEmpty()
        return if (photos.size > 1) Strings.format("desktop_preview_count", name, index + 1, photos.size) else name
    }

    companion object {
        /** Esc・Space で閉じる、← → で送る。修飾キー付きは取らない。 */
        fun keyOf(p: KeyPress): PreviewKey? {
            if (p.ctrl || p.meta || p.alt || p.shift) return null
            return when (p.key) {
                Key.Escape, Key.Spacebar -> PreviewKey.CLOSE
                Key.DirectionLeft -> PreviewKey.PREVIOUS
                Key.DirectionRight -> PreviewKey.NEXT
                else -> null
            }
        }
    }
}

/**
 * 「既定のアプリで開く」: 写真をこの人専用の一時フォルダ([tempRoot])へ PNG で書き、OS のビューアに渡す。元のファイルの場所は渡さない。
 * 書いた物は終わる時と「この端末のデータを消す」で消す。
 */
object PhotoOpen {
    /**
     * 書く場所(この人だけの・消してよい場所)。皆で使う java.io.tmpdir には置かない。
     * - Windows: `%LOCALAPPDATA%\TailorIt\tmp\open`(ローミングする %APPDATA% には置かない)
     * - macOS: `~/Library/Caches/TailorIt/tmp/open`
     * - Linux: `$XDG_RUNTIME_DIR/tailorit/open`(絶対パスの時だけ)、無ければ端末の保存先の下
     */
    fun tempRoot(
        osName: String = System.getProperty("os.name").orEmpty(),
        env: Map<String, String> = System.getenv(),
        home: String = System.getProperty("user.home").orEmpty(),
        dataDir: () -> File = { AppDataDir.resolve() },
    ): File {
        val os = osName.lowercase()
        return when {
            os.startsWith("windows") -> {
                val base = env["LOCALAPPDATA"]?.takeIf { it.isNotBlank() } ?: File(home, "AppData/Local").path
                File(File(File(base, "TailorIt"), "tmp"), "open")
            }
            os.startsWith("mac") || os.startsWith("darwin") -> File(File(File(home, "Library/Caches/TailorIt"), "tmp"), "open")
            else -> env["XDG_RUNTIME_DIR"]?.takeIf { it.startsWith("/") }?.let { File(File(it, "tailorit"), "open") }
                ?: File(File(dataDir(), "tmp"), "open")
        }
    }

    /** 書き出したファイル。読めない・画像でない・手元でない時は null。裏のスレッドで呼ぶ。 */
    fun export(photo: PreviewPhoto, root: File = tempRoot()): File? {
        if (!PhotoPreviewSources.accepts(photo.src)) return null
        val bytes = ImageSource.bytes(photo.src, allowFile = true) ?: return null
        val png = DragOut.png(bytes)?.takeIf { decodes(it) } ?: return null
        return runCatching {
            ownerOnly(root)
            DragOut.temp(DragOut.fileName(photo.name, Strings["desktop_drag_image_name"]) + ".png", png, root)
        }.getOrNull()
    }

    /** 書いた写真([root])と、引き出しの一時ファイル([dragRoot])を消す(この端末のデータを消す時)。 */
    fun clearTemp(root: File = tempRoot(), dragRoot: File = DragOut.tempRoot()) {
        runCatching { DragOut.clear(root) }
        runCatching { DragOut.clear(dragRoot) }
    }

    /** POSIX ならこの人だけが読める形で作る(Windows は保存先がもとから人ごと)。 */
    private fun ownerOnly(root: File) {
        root.mkdirs()
        runCatching { Files.setPosixFilePermissions(root.toPath(), PosixFilePermissions.fromString("rwx------")) }
    }

    private fun decodes(png: ByteArray): Boolean = runCatching { org.jetbrains.skia.Image.makeFromEncoded(png).close() }.isSuccess

    /** 開けなければ知らせる一言。開けたら null。 */
    fun open(photo: PreviewPhoto): String? {
        val file = export(photo) ?: return Strings["desktop_preview_failed"]
        val ok = runCatching {
            Desktop.isDesktopSupported() && Desktop.getDesktop().isSupported(Desktop.Action.OPEN) && run { Desktop.getDesktop().open(file); true }
        }.getOrDefault(false)
        return if (ok) null else Strings["desktop_preview_open_failed"]
    }
}

/** 写真の部品から開く口(DesktopApp が渡す)。(値, 項目名) → 開けたら true。 */
val LocalPhotoPreview = staticCompositionLocalOf<((String, String) -> Boolean)?> { null }

/** 写真を押す / Enter・Space で大きく見る。見せられない値(https・空)なら何もしない。 */
fun Modifier.photoPreviewClick(src: String?, name: String): Modifier = composed {
    val open = LocalPhotoPreview.current
    if (open == null || src == null || !PhotoPreviewSources.accepts(src)) return@composed Modifier
    val label = Strings["desktop_preview_open"]
    Modifier
        .onKeyEvent { e -> KeyPress.of(e)?.takeIf { it.key == Key.Spacebar && !it.ctrl && !it.meta && !it.alt && !it.shift && !e.isAutoRepeatSpace() }?.let { open(src, name) } ?: false }
        .clickable(role = Role.Button, onClickLabel = label) { open(src, name) }
}

private const val PREVIEW_PX = 2048

private sealed interface Loaded {
    data object Loading : Loaded
    data object Failed : Loaded
    class Done(val image: LoadedImage) : Loaded
}

/** ウィンドウの中のダイアログ。下の画面は DesktopApp が inert にする。 */
@Composable
fun PhotoPreviewDialog(preview: PhotoPreview, onMessage: (String) -> Unit, onDismiss: () -> Unit) {
    val photo = preview.current ?: return
    val focus = remember { FocusRequester() }
    val scope = rememberCoroutineScope()
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
    var loaded by remember(photo.src) { mutableStateOf<Loaded>(Loaded.Loading) }
    LaunchedEffect(photo.src) {
        loaded = withContext(Dispatchers.IO) { ImageLoader.load(photo.src, allowFile = true, maxPx = PREVIEW_PX, withPng = false, useCache = false) }
            ?.let { Loaded.Done(it) } ?: Loaded.Failed
    }
    val title = preview.title()
    val pane = A11y.previewTitle(title)

    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        // 暗幕は中身と兄弟にする(sheet と同じ・中身が 1 つにまとまらない)
        val closeLabel = Strings["desktop_close"]
        Box(
            Modifier.matchParentSize().background(Color(0xCC000000))
                .clickable(role = Role.Button) { onDismiss() }
                .semantics { contentDescription = closeLabel },
        )
        Surface(
            Modifier.widthIn(max = 1200.dp).fillMaxWidth(0.94f).fillMaxHeight(0.92f)
                .semantics { paneTitle = pane; isTraversalGroup = true }
                .onKeyEvent { e ->
                    val repeat = e.isAutoRepeatSpace()
                    when (KeyPress.of(e)?.let(PhotoPreview::keyOf)) {
                        // 開いた時の Space を押しっぱなしでもすぐ閉じない
                        PreviewKey.CLOSE -> true.also { if (!repeat) onDismiss() }
                        PreviewKey.PREVIOUS -> true.also { preview.previous() }
                        PreviewKey.NEXT -> true.also { preview.next() }
                        null -> false
                    }
                },
            shape = MaterialTheme.shapes.large,
            tonalElevation = 6.dp,
        ) {
            Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(title, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f).semantics { heading() })
                    TextButton(onClick = onDismiss) { Text(closeLabel) }
                }
                Box(
                    Modifier.weight(1f).fillMaxWidth().focusRequester(focus).focusable()
                        .semantics { contentDescription = photo.name.ifEmpty { Strings["desktop_a11y_image"] } },
                    contentAlignment = Alignment.Center,
                ) {
                    when (val l = loaded) {
                        is Loaded.Done -> Image(l.image.bitmap, null, Modifier.fillMaxSize(), contentScale = ContentScale.Fit)
                        Loaded.Loading -> Text(Strings["desktop_preview_loading"], color = Color.Gray)
                        Loaded.Failed -> Text(Strings["desktop_preview_failed"], color = Color.Gray)
                    }
                }
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (preview.photos.size > 1) {
                        TextButton(onClick = { preview.previous() }, enabled = preview.index > 0) { Text("‹ " + Strings["desktop_preview_previous"]) }
                        TextButton(onClick = { preview.next() }, enabled = preview.index < preview.photos.lastIndex) { Text(Strings["desktop_preview_next"] + " ›") }
                    }
                    Text(Strings["desktop_preview_hint"], style = MaterialTheme.typography.bodySmall, color = Color.Gray, modifier = Modifier.weight(1f))
                    Spacer(Modifier)
                    OutlinedButton(
                        onClick = { scope.launch { withContext(Dispatchers.IO) { PhotoOpen.open(photo) }?.let(onMessage) } },
                        enabled = loaded is Loaded.Done,
                    ) { Text(Strings["desktop_preview_open_default"]) }
                }
            }
        }
    }
}
