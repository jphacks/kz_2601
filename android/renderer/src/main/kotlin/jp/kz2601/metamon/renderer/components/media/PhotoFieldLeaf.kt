package jp.kz2601.metamon.renderer.components.media

import android.content.Context
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.components.PhotoView
import jp.kz2601.metamon.renderer.components.inputs.FormInput
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

/**
 * `photoField`(iOS の `PhotoFieldLeaf`): source に応じてカメラ / 写真選択(Photo Picker・権限なし)/ 二択。
 * 撮った・選んだ写真は [PhotoCodec] で 800px 以下・JPEG 品質 50 の data URL にして field に入れる。
 * カメラの権限は使う直前に [LocalCameraPermission](:app の窓口)で聞く。カメラが無い端末では写真選択に落ちる。
 * `analyze` は端末内の読み取り([PhotoReader])がある時だけ動く。`illustrate` は Android に代わりが無いので出さない。
 */
@Composable
fun PhotoFieldLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    PhotoFieldPanel(c, ctx, form, field)
}

private sealed interface ReadStatus {
    data class Done(val missed: List<String>) : ReadStatus
    data object Unreadable : ReadStatus
}

@Composable
private fun PhotoFieldPanel(c: Component, ctx: RenderContext, form: FormDraft, field: String) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val value = (form.values[field] as? JSONValue.Str)?.value
    val hasPhoto = PhotoValue.hasPhoto(value)
    val title = c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.media_photo)
    val reader = ctx.runtime.photoReader?.takeIf { c.analyze.isNotEmpty() }
    val describePrefix = stringResource(R.string.media_photo_describe_prefix)

    var showChooser by remember { mutableStateOf(false) }
    var showDenied by remember { mutableStateOf(false) }
    var reading by remember { mutableStateOf(false) }
    var readStatus by remember { mutableStateOf<ReadStatus?>(null) }
    var pendingCapture by rememberSaveable { mutableStateOf<String?>(null) }

    fun read() {
        if (reader == null || reading) return
        val bytes = PhotoValue.dataBytes((form.values[field] as? JSONValue.Str)?.value)
        if (bytes == null) {
            readStatus = ReadStatus.Unreadable
            return
        }
        reading = true
        readStatus = null
        scope.launch {
            val out = runCatching { PhotoReading.apply(reader.clues(bytes, c.analyze), c.analyze, form.entity, describePrefix = describePrefix) }
                .getOrElse { PhotoReadOutcome(missed = c.analyze.map { it.into }) }
            out.values.forEach { (id, v) -> form.values[id] = v }
            readStatus = ReadStatus.Done(out.missed.map { form.entity.field(it)?.name ?: it })
            reading = false
        }
    }

    fun setPhoto(encoded: String?) {
        if (encoded == null) return
        form.values[field] = JSONValue.Str(encoded)
        readStatus = null
        read()
    }

    fun encode(uri: Uri, cleanup: File? = null) {
        scope.launch {
            val encoded = withContext(Dispatchers.IO) {
                PhotoCodec.dataUrl(context, uri).also { cleanup?.delete() }
            }
            setPhoto(encoded)
        }
    }

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri -> uri?.let { encode(it) } }
    val camera = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { ok ->
        val path = pendingCapture ?: return@rememberLauncherForActivityResult
        pendingCapture = null
        val file = File(path)
        if (ok) encode(Uri.fromFile(file), cleanup = file) else file.delete()
    }
    val openLibrary = { picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) }
    val launchCamera = {
        val file = captureFile(context)
        pendingCapture = file.absolutePath
        camera.launch(FileProvider.getUriForFile(context, "${context.packageName}.metamon.media", file))
    }
    val askCamera = LocalCameraPermission.current { granted -> if (granted) launchCamera() else showDenied = true }

    val plan = PhotoSourcePolicy.plan(c.source, context.hasCamera())
    val start = {
        when (plan) {
            PhotoCapturePlan.CAMERA -> askCamera()
            PhotoCapturePlan.LIBRARY -> openLibrary()
            PhotoCapturePlan.CHOOSE -> showChooser = true
        }
    }
    val hint = stringResource(
        when (plan) {
            PhotoCapturePlan.CAMERA -> R.string.media_photo_hint_camera
            PhotoCapturePlan.LIBRARY -> R.string.media_photo_hint_library
            PhotoCapturePlan.CHOOSE -> R.string.media_photo_hint_choose
        },
    )

    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        if (hasPhoto) {
            val attached = stringResource(R.string.media_photo_attached, title)
            PhotoView(value, 160.dp, Modifier.semantics { contentDescription = attached })
            Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                MediaButton(stringResource(R.string.media_photo_retake), "camera", Modifier.weight(1f), hint, onClick = start)
                val deleteLabel = stringResource(R.string.media_delete_named, title)
                MediaButton(stringResource(R.string.media_delete), "trash", Modifier.weight(1f), a11yLabel = deleteLabel, destructive = true) {
                    form.values[field] = JSONValue.Null
                    readStatus = null
                }
            }
            if (reader != null) ReadControls(reading, readStatus, canRead = PhotoValue.dataBytes(value) != null, onRead = ::read)
        } else {
            MediaButton(
                c.label ?: stringResource(R.string.media_photo_take_or_pick),
                if (plan == PhotoCapturePlan.LIBRARY) "photo" else "camera",
                Modifier.fillMaxWidth(),
                hint,
                onClick = start,
            )
        }
    }

    if (showChooser) {
        AlertDialog(
            onDismissRequest = { showChooser = false },
            confirmButton = {},
            dismissButton = { TextButton(onClick = { showChooser = false }) { Text(stringResource(R.string.media_cancel)) } },
            title = { Text(title) },
            text = {
                Column {
                    TextButton(onClick = { showChooser = false; askCamera() }) { Text(stringResource(R.string.media_photo_take)) }
                    TextButton(onClick = { showChooser = false; openLibrary() }) { Text(stringResource(R.string.media_photo_pick)) }
                }
            },
        )
    }
    if (showDenied) {
        AlertDialog(
            onDismissRequest = { showDenied = false },
            title = { Text(stringResource(R.string.media_camera_denied_title)) },
            text = { Text(stringResource(R.string.media_camera_denied_body)) },
            confirmButton = { TextButton(onClick = { showDenied = false; openLibrary() }) { Text(stringResource(R.string.media_photo_pick)) } },
            dismissButton = { TextButton(onClick = { showDenied = false }) { Text(stringResource(R.string.media_cancel)) } },
        )
    }
}

@Composable
private fun ReadControls(reading: Boolean, status: ReadStatus?, canRead: Boolean, onRead: () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        OutlinedButton(onClick = onRead, enabled = !reading && canRead, modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(RenderTheme.Radius.button)) {
            if (reading) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp) else SymbolIcon("sparkles", 18.dp, MaterialTheme.colorScheme.primary)
            Spacer(Modifier.width(RenderTheme.Spacing.sm))
            Text(stringResource(if (reading) R.string.media_photo_reading else R.string.media_photo_read_again))
        }
        val text = when (status) {
            is ReadStatus.Done -> if (status.missed.isEmpty()) stringResource(R.string.media_photo_read_done)
            else stringResource(R.string.media_photo_read_missed, status.missed.joinToString("、"))
            ReadStatus.Unreadable -> stringResource(R.string.media_photo_unreadable)
            null -> null
        }
        text?.let { Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
    }
}

/** 写真・手書きの操作ボタン(iOS の RenderButtonStyle.secondary / destructive)。 */
@Composable
internal fun MediaButton(
    label: String,
    icon: String?,
    modifier: Modifier = Modifier,
    hint: String? = null,
    a11yLabel: String? = null,
    destructive: Boolean = false,
    onClick: () -> Unit,
) {
    val tint = if (destructive) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary
    OutlinedButton(
        onClick = onClick,
        modifier = modifier.semantics {
            a11yLabel?.let { contentDescription = it }
            hint?.let { onClick(label = it) { onClick(); true } }
        },
        shape = RoundedCornerShape(RenderTheme.Radius.button),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            icon?.let {
                SymbolIcon(it, 18.dp, tint)
                Spacer(Modifier.width(RenderTheme.Spacing.xs))
            }
            Text(label, color = tint)
        }
    }
}

/** カメラの保存先(cache/metamon_photos・値にしたら消す)。 */
private fun captureFile(context: Context): File =
    File(context.cacheDir, "metamon_photos").apply { mkdirs() }.let { File(it, "capture-${System.currentTimeMillis()}.jpg") }
