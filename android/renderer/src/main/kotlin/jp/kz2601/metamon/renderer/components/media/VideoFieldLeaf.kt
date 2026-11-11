package jp.kz2601.metamon.renderer.components.media

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.MediaStore
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContract
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.FileProvider
import androidx.media3.common.MediaItem
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Height
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.LocalVideoStore
import jp.kz2601.metamon.dsl.records.VideoValue
import jp.kz2601.metamon.renderer.R
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
 * `videoField`(iOS の `VideoFieldLeaf`): source に応じて動画の撮影 / 選択(Photo Picker・権限なし)/ 二択。
 * 撮影は端末のカメラアプリ(上限 maxDuration 秒)。取り込んだ動画は端末内に置き([VideoCodec])、
 * field には `"video:<id>|<サムネイル>"` を入れる。カメラの権限は [LocalCameraPermission] で聞く
 * (録音はカメラアプリが自分の権限で行うので、こちらはマイクを聞かない)。カメラが無い端末は選択に落ちる。
 */
@Composable
fun VideoFieldLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    VideoFieldPanel(c, form, field)
}

/** `videoPlayer`(iOS の `VideoPlayerLeaf`): video field を再生する。height は small 140 / large 320 / それ以外 220。 */
@Composable
fun VideoPlayerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    VideoPreview(c.field?.let { ctx.form?.values?.get(it) }, c.videoHeight, modifier)
}

val Component.videoHeight: Dp
    get() = when (height) {
        Height.SMALL -> 140.dp
        Height.LARGE -> 320.dp
        else -> 220.dp
    }

/** カメラアプリで動画を撮る(CaptureVideo に長さの上限を足したもの)。 */
private class CaptureVideoWithLimit : ActivityResultContract<Pair<Uri, Int>, Boolean>() {
    override fun createIntent(context: Context, input: Pair<Uri, Int>): Intent =
        Intent(MediaStore.ACTION_VIDEO_CAPTURE)
            .putExtra(MediaStore.EXTRA_OUTPUT, input.first)
            .putExtra(MediaStore.EXTRA_DURATION_LIMIT, input.second)

    override fun parseResult(resultCode: Int, intent: Intent?): Boolean = resultCode == android.app.Activity.RESULT_OK
}

@Composable
private fun VideoFieldPanel(c: Component, form: FormDraft, field: String) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val value = form.values[field]
    val video = VideoValue.of(value)
    val title = c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.media_video)
    val maxDuration = VideoCodec.limitSeconds(c.maxDuration)

    var showChooser by remember { mutableStateOf(false) }
    var showDenied by remember { mutableStateOf(false) }
    var saving by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    var pendingCapture by rememberSaveable { mutableStateOf<String?>(null) }

    fun ingest(uri: Uri, cleanup: File? = null) {
        saving = true
        failed = false
        scope.launch {
            val result = withContext(Dispatchers.IO) {
                runCatching { VideoCodec.ingest(context, uri, maxDuration) }.also { cleanup?.delete() }
            }
            result.onSuccess { form.values[field] = JSONValue.Str(it.encoded) }.onFailure { failed = true }
            saving = false
        }
    }

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri -> uri?.let { ingest(it) } }
    val camera = rememberLauncherForActivityResult(CaptureVideoWithLimit()) { ok ->
        val path = pendingCapture ?: return@rememberLauncherForActivityResult
        pendingCapture = null
        val file = File(path)
        if (ok && file.length() > 0) ingest(Uri.fromFile(file), cleanup = file) else file.delete()
    }
    val openLibrary = { picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.VideoOnly)) }
    val launchCamera = {
        val file = VideoCodec.tempFile(context, "capture")
        pendingCapture = file.absolutePath
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.metamon.media", file)
        runCatching { camera.launch(uri to maxDuration) }.onFailure {
            // 撮影アプリが無い端末
            pendingCapture = null
            file.delete()
            openLibrary()
        }
        Unit
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
            PhotoCapturePlan.CAMERA -> R.string.media_video_hint_camera
            PhotoCapturePlan.LIBRARY -> R.string.media_video_hint_library
            PhotoCapturePlan.CHOOSE -> R.string.media_video_hint_choose
        },
    )

    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        when {
            saving -> Row(
                Modifier.fillMaxWidth().heightIn(min = 44.dp),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                Spacer(Modifier.width(RenderTheme.Spacing.sm))
                Text(stringResource(R.string.media_video_saving), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            video != null -> {
                VideoPreview(value, 200.dp)
                Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                    MediaButton(stringResource(R.string.media_photo_retake), "video", Modifier.weight(1f), hint, onClick = start)
                    val deleteLabel = stringResource(R.string.media_delete_named, title)
                    MediaButton(stringResource(R.string.media_delete), "trash", Modifier.weight(1f), a11yLabel = deleteLabel, destructive = true) {
                        form.values[field] = JSONValue.Null
                    }
                }
            }
            else -> MediaButton(
                c.label ?: stringResource(R.string.media_video_take_or_pick),
                "video",
                Modifier.fillMaxWidth(),
                hint,
                onClick = start,
            )
        }
        if (failed) {
            Text(stringResource(R.string.media_video_failed), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.error)
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
                    TextButton(onClick = { showChooser = false; openLibrary() }) { Text(stringResource(R.string.media_video_pick)) }
                }
            },
        )
    }
    if (showDenied) {
        AlertDialog(
            onDismissRequest = { showDenied = false },
            title = { Text(stringResource(R.string.media_camera_denied_title)) },
            text = { Text(stringResource(R.string.media_video_camera_denied_body)) },
            confirmButton = { TextButton(onClick = { showDenied = false; openLibrary() }) { Text(stringResource(R.string.media_video_pick)) } },
            dismissButton = { TextButton(onClick = { showDenied = false }) { Text(stringResource(R.string.media_cancel)) } },
        )
    }
}

/** 動画の枠で何を出すか(iOS の VideoPreview の分岐)。 */
sealed interface VideoDisplay {
    /** 再生できる(端末内のファイルか https) */
    data class Playable(val uri: String) : VideoDisplay

    /** 値が無い */
    data object Empty : VideoDisplay

    /** 他の端末で撮った動画(サムネイルと「この端末にはありません」) */
    data class Elsewhere(val thumbnail: String?) : VideoDisplay

    companion object {
        fun of(value: JSONValue?, store: LocalVideoStore): VideoDisplay {
            val video = VideoValue.of(value) ?: return Empty
            video.remoteUrl?.let { return Playable(it) }
            val id = video.id
            if (id != null && store.exists(id)) return Playable("file://" + store.file(id)!!.absolutePath)
            return Elsewhere(video.thumbnail)
        }
    }
}

/** 動画の枠。この端末にファイルがあれば ExoPlayer で再生、無ければサムネイル +「この端末にはありません」。 */
@Composable
fun VideoPreview(value: JSONValue?, height: Dp, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val store = remember { VideoCodec.store(context) }
    val display = remember(value) { VideoDisplay.of(value, store) }
    val shape = RoundedCornerShape(RenderTheme.Radius.inner)
    Box(modifier.fillMaxWidth().height(height).clip(shape), contentAlignment = Alignment.Center) {
        when (display) {
            is VideoDisplay.Playable -> ExoVideo(display.uri)
            VideoDisplay.Empty -> {
                val label = stringResource(R.string.media_video_none)
                Box(
                    Modifier.fillMaxSize().background(MaterialTheme.colorScheme.surfaceVariant).clearAndSetSemantics { contentDescription = label },
                    contentAlignment = Alignment.Center,
                ) { SymbolIcon("video", 28.dp, MaterialTheme.colorScheme.onSurfaceVariant) }
            }
            is VideoDisplay.Elsewhere -> {
                val label = stringResource(R.string.media_video_not_on_device)
                PhotoView(display.thumbnail, height, Modifier.semantics { contentDescription = label })
                Row(
                    Modifier
                        .align(Alignment.BottomCenter)
                        .padding(8.dp)
                        .clip(RoundedCornerShape(50))
                        .background(MaterialTheme.colorScheme.surface.copy(alpha = 0.85f))
                        .padding(horizontal = 10.dp, vertical = 6.dp)
                        .clearAndSetSemantics {},
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    SymbolIcon("iphone", 14.dp, MaterialTheme.colorScheme.onSurface)
                    Spacer(Modifier.width(4.dp))
                    Text(label, style = MaterialTheme.typography.labelSmall, fontWeight = FontWeight.SemiBold)
                }
            }
        }
    }
}

/** ExoPlayer の再生枠(操作ボタンは PlayerView の標準)。勝手に再生は始めない。 */
@Composable
private fun ExoVideo(uri: String) {
    val context = LocalContext.current
    val label = stringResource(R.string.media_video)
    val player = remember(uri) {
        ExoPlayer.Builder(context).build().apply {
            setMediaItem(MediaItem.fromUri(uri))
            prepare()
        }
    }
    DisposableEffect(player) { onDispose { player.release() } }
    AndroidView(
        factory = { PlayerView(it).apply { this.player = player } },
        update = { it.player = player },
        modifier = Modifier.fillMaxSize().semantics { contentDescription = label },
    )
}
