package jp.kz2601.metamon.renderer.components.dashcam

import android.content.Intent
import android.net.Uri
import android.provider.Settings
import android.text.format.Formatter
import androidx.camera.view.PreviewView
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.media3.common.MediaItem
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView
import jp.kz2601.metamon.dsl.AutoStart
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.audio.rememberMicrophoneRequest
import jp.kz2601.metamon.renderer.components.media.LocalCameraPermission
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.io.File
import java.text.DateFormat
import java.util.Date

/** 映像の置き場を分けるアプリ名(DSLView が渡す。無ければ共通の置き場)。 */
val LocalDashcamAppName = compositionLocalOf { "" }

/**
 * `dashcam`(#872・iOS の `DashcamLeaf`): 背面カメラのプレビュー + 録画の開始/停止 + 保存した映像の一覧。
 * 1 分ごとの区間で録画し、直近 `keepMinutes` 分だけ残す(ループ)。衝撃・「今の映像を保存」で前後を保存済みにする。
 * `autoStart` の条件が続いたら自動で開始。録画中は画面を消さない。
 * iOS と同じく画面を出している間だけ録画する(フォアグラウンドサービスは使わない・その旨を常に表示する)。
 */
@Composable
fun DashcamLeaf(c: Component, @Suppress("UNUSED_PARAMETER") ctx: RenderContext, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val appName = LocalDashcamAppName.current
    val scope = rememberCoroutineScope()
    val config = remember(c) { DashcamConfig.of(c) }
    val recorder = remember(c.id, appName, config) {
        val app = context.applicationContext
        DashcamRecorder(app, config, DashcamStore(File(app.filesDir, DashcamStore.DIRECTORY), appName), scope)
    }
    val owner = LocalLifecycleOwner.current
    var bindRequested by remember(recorder) { mutableStateOf(false) }
    var showClips by remember { mutableStateOf(false) }

    // 権限: カメラ → (音を録るなら)マイク。どちらかを断られたら録らない(iOS と同じ)
    val askMicrophone = rememberMicrophoneRequest()
    val askCamera = LocalCameraPermission.current { granted ->
        when {
            !granted -> recorder.denied()
            config.audio -> askMicrophone { mic -> if (mic) bindRequested = true else recorder.denied() }
            else -> bindRequested = true
        }
    }
    LaunchedEffect(recorder) { if (recorder.appear()) askCamera() }
    LaunchedEffect(recorder, bindRequested) { if (bindRequested) recorder.bind(owner) }
    DisposableEffect(recorder, owner) {
        val observer = LifecycleEventObserver { _, e ->
            when (e) {
                Lifecycle.Event.ON_STOP -> recorder.enterBackground()
                Lifecycle.Event.ON_START -> recorder.enterForeground()
                else -> Unit
            }
        }
        owner.lifecycle.addObserver(observer)
        onDispose {
            owner.lifecycle.removeObserver(observer)
            recorder.disappear()
        }
    }

    // 録画中は画面を消さない
    val view = LocalView.current
    DisposableEffect(view, recorder.isRecording) {
        if (recorder.isRecording) view.keepScreenOn = true
        onDispose { view.keepScreenOn = false }
    }

    // 衝撃・保存: 振動 + TalkBack に読む
    val haptics = LocalHapticFeedback.current
    val announce = rememberAnnouncer()
    val impactText = recorder.lastImpact?.let { stringResource(R.string.renderer_dashcam_impact_saved, timeText(it)) }
    LaunchedEffect(recorder.impactCount) {
        if (recorder.impactCount == 0) return@LaunchedEffect
        haptics.performHapticFeedback(HapticFeedbackType.LongPress)
        impactText?.let(announce)
    }

    // グリッドのセル(高さが決まっている)ではプレビューが残りを埋める。高さが決まらない所では 16:9
    BoxWithConstraints(modifier.fillMaxWidth()) {
        val fills = constraints.hasBoundedHeight
        Column(
            Modifier.fillMaxWidth().then(if (fills) Modifier.fillMaxHeight() else Modifier).renderCard().semantics { testTag = "dashcam" },
            verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
        ) {
            Header(c.title ?: stringResource(R.string.renderer_dashcam_title), recorder)
            Preview(recorder, impactText, if (fills) Modifier.weight(1f) else Modifier.aspectRatio(16f / 9f))
            Controls(recorder) { showClips = true }
            Footnote(recorder, config)
        }
    }

    if (showClips) ClipsSheet(recorder) { showClips = false }
}

@Composable
private fun Header(title: String, recorder: DashcamRecorder) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            title,
            Modifier.weight(1f).semantics { heading() },
            style = RenderTheme.Fonts.sectionTitle,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        val since = recorder.recordingSince
        val left = recorder.autoStartIn
        if (recorder.isRecording && since != null) {
            var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
            LaunchedEffect(since) {
                while (true) {
                    now = System.currentTimeMillis()
                    delay(1_000)
                }
            }
            val label = stringResource(R.string.renderer_dashcam_recording)
            val clock = DashcamText.clock(now - since)
            Row(
                Modifier.clearAndSetSemantics { contentDescription = "$label $clock"; testTag = "dashcam.recording" },
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Box(Modifier.size(8.dp).background(Color.Red, CircleShape))
                Text(label, style = MaterialTheme.typography.labelLarge.copy(fontWeight = FontWeight.SemiBold))
                Text(clock, style = MaterialTheme.typography.labelLarge.copy(fontFeatureSettings = "tnum"))
            }
        } else if (left != null) {
            Text(
                stringResource(R.string.renderer_dashcam_auto_start_in, left),
                Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold),
                color = RenderTheme.colors.brandText,
            )
        }
    }
}

@Composable
private fun Preview(recorder: DashcamRecorder, impactText: String?, modifier: Modifier = Modifier) {
    val description = stringResource(R.string.renderer_dashcam_preview)
    Box(
        modifier
            .fillMaxWidth()
            .heightIn(min = 120.dp)
            .clip(RoundedCornerShape(RenderTheme.Radius.inner))
            .background(Color.Black)
            .semantics { testTag = "dashcam.preview" },
        contentAlignment = Alignment.Center,
    ) {
        when (recorder.phase) {
            DashcamPhase.Ready, DashcamPhase.Recording -> AndroidView(
                factory = {
                    PreviewView(it).apply {
                        // Compose の中で重ねても崩れない TextureView 側
                        implementationMode = PreviewView.ImplementationMode.COMPATIBLE
                        scaleType = PreviewView.ScaleType.FILL_CENTER
                        recorder.preview.setSurfaceProvider(surfaceProvider)
                    }
                },
                modifier = Modifier.fillMaxSize().semantics { contentDescription = description },
            )
            DashcamPhase.Unavailable -> Placeholder(stringResource(R.string.renderer_dashcam_unavailable))
            DashcamPhase.Denied -> Placeholder(stringResource(R.string.renderer_dashcam_denied))
            else -> CircularProgressIndicator(color = Color.White)
        }
        if (impactText != null && recorder.isRecording) {
            Text(
                impactText,
                Modifier
                    .align(Alignment.BottomCenter)
                    .padding(8.dp)
                    .background(Color.Black.copy(alpha = 0.6f), RoundedCornerShape(50))
                    .padding(horizontal = 10.dp, vertical = 6.dp),
                style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold),
                color = Color.White,
            )
        }
    }
}

@Composable
private fun Placeholder(text: String) {
    Column(
        Modifier.padding(RenderTheme.Spacing.md),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        SymbolIcon("video", 28.dp, Color.White.copy(alpha = 0.85f))
        Text(text, style = MaterialTheme.typography.bodySmall, color = Color.White.copy(alpha = 0.85f), textAlign = TextAlign.Center)
    }
}

@Composable
private fun Controls(recorder: DashcamRecorder, onClips: () -> Unit) {
    Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        if (recorder.isRecording) {
            ActionButton(
                stringResource(R.string.renderer_dashcam_stop),
                ActionButtonKind.DESTRUCTIVE,
                onClick = { recorder.stop() },
                modifier = Modifier.weight(1f).semantics { testTag = "dashcam.stop" },
            )
            ActionButton(
                stringResource(R.string.renderer_dashcam_save_now),
                ActionButtonKind.SECONDARY,
                onClick = { recorder.saveNow() },
                modifier = Modifier.weight(1f).semantics { testTag = "dashcam.saveNow" },
                icon = "exclamationmark.triangle",
            )
        } else {
            ActionButton(
                stringResource(R.string.renderer_dashcam_start),
                ActionButtonKind.PRIMARY,
                onClick = { recorder.start() },
                modifier = Modifier.weight(1f).semantics { testTag = "dashcam.start" },
                icon = "video",
                enabled = recorder.phase == DashcamPhase.Ready,
            )
        }
        ActionButton(
            stringResource(R.string.renderer_dashcam_clips, recorder.clips.size),
            ActionButtonKind.SECONDARY,
            onClick = onClips,
            modifier = Modifier.weight(1f).semantics { testTag = "dashcam.clips" },
            icon = "play",
        )
    }
}

@Composable
private fun Footnote(recorder: DashcamRecorder, config: DashcamConfig) {
    val context = LocalContext.current
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        when (val p = recorder.phase) {
            DashcamPhase.Denied -> TextButton(onClick = {
                context.startActivity(
                    Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", context.packageName, null))
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                )
            }) { Text(stringResource(R.string.renderer_dashcam_open_settings)) }
            is DashcamPhase.Full -> Text(
                stringResource(if (p.deviceLow) R.string.renderer_dashcam_full_device else R.string.renderer_dashcam_full_saved),
                Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                style = MaterialTheme.typography.bodySmall,
                color = RenderTheme.colors.negative,
            )
            else -> Unit
        }
        if (recorder.stoppedInBackground) {
            Text(
                stringResource(R.string.renderer_dashcam_stopped_in_background),
                style = MaterialTheme.typography.bodySmall,
                color = RenderTheme.colors.warning,
            )
        }
        Text(
            footnote(config),
            Modifier.semantics { testTag = "dashcam.notice" },
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** 常に出す説明: 画面を出している間だけ録画すること・残す長さ・自動開始の条件。 */
@Composable
private fun footnote(config: DashcamConfig): String {
    val base = stringResource(R.string.renderer_dashcam_footnote, DashcamText.keepMinutes(config))
    val conditions = DashcamText.orderedConditions(config.autoStart)
    if (conditions.isEmpty()) return base
    val words = conditions.map {
        stringResource(
            when (it) {
                AutoStart.LANDSCAPE -> R.string.renderer_dashcam_condition_landscape
                AutoStart.STILL -> R.string.renderer_dashcam_condition_still
                else -> R.string.renderer_dashcam_condition_charging
            },
        )
    }
    val joined = words.joinToString(stringResource(R.string.renderer_dashcam_condition_separator))
    return base + stringResource(R.string.renderer_dashcam_footnote_auto, joined)
}

private fun timeText(millis: Long): String = DateFormat.getTimeInstance(DateFormat.MEDIUM).format(Date(millis))

private fun dateTimeText(millis: Long): String =
    DateFormat.getDateTimeInstance(DateFormat.MEDIUM, DateFormat.MEDIUM).format(Date(millis))

/** 保存した映像の一覧: 再生・ギャラリーに保存・共有・削除。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ClipsSheet(recorder: DashcamRecorder, onDismiss: () -> Unit) {
    val context = LocalContext.current
    var playing by remember { mutableStateOf<DashcamSegment?>(null) }
    ModalBottomSheet(onDismissRequest = onDismiss, containerColor = RenderTheme.colors.canvas) {
        Column(
            Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = RenderTheme.Spacing.md, vertical = RenderTheme.Spacing.sm),
            verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
        ) {
            Text(
                stringResource(R.string.renderer_dashcam_clips_title),
                Modifier.semantics { heading() },
                style = RenderTheme.Fonts.cardTitle,
            )
            if (recorder.clips.isEmpty()) {
                Text(stringResource(R.string.renderer_dashcam_clips_empty), style = MaterialTheme.typography.bodyLarge)
                Text(
                    stringResource(R.string.renderer_dashcam_clips_empty_detail),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            recorder.clips.forEach { clip ->
                val title = dateTimeText(clip.start)
                Row(
                    Modifier.fillMaxWidth().renderCard(padding = RenderTheme.Spacing.sm),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(
                        Modifier.weight(1f).clickable { playing = clip }.padding(RenderTheme.Spacing.xs),
                        verticalArrangement = Arrangement.spacedBy(2.dp),
                    ) {
                        Text(title, style = MaterialTheme.typography.bodyLarge)
                        Text(
                            "${DashcamText.clock(clip.duration)} ・ ${Formatter.formatShortFileSize(context, clip.bytes)}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    val deleteLabel = stringResource(R.string.renderer_dashcam_delete_clip, title)
                    IconButton(onClick = { recorder.delete(clip) }, Modifier.semantics { contentDescription = deleteLabel }) {
                        SymbolIcon("trash", 20.dp, RenderTheme.colors.negative)
                    }
                }
            }
        }
    }
    playing?.let { clip -> ClipDialog(recorder, clip) { playing = null } }
}

@Composable
private fun ClipDialog(recorder: DashcamRecorder, clip: DashcamSegment, onDismiss: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var savedToGallery by remember(clip) { mutableStateOf<Boolean?>(null) }
    var saving by remember(clip) { mutableStateOf(false) }
    val player = remember(clip) {
        ExoPlayer.Builder(context).build().apply {
            setMediaItem(MediaItem.fromUri(Uri.fromFile(recorder.file(clip))))
            prepare()
        }
    }
    DisposableEffect(player) { onDispose { player.release() } }
    val videoLabel = stringResource(R.string.renderer_dashcam_preview)
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().background(RenderTheme.colors.surface, RoundedCornerShape(RenderTheme.Radius.sheet)).padding(RenderTheme.Spacing.md),
            verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
        ) {
            Text(dateTimeText(clip.start), Modifier.semantics { heading() }, style = RenderTheme.Fonts.cardTitle)
            AndroidView(
                factory = { PlayerView(it).apply { this.player = player } },
                update = { it.player = player },
                modifier = Modifier.fillMaxWidth().aspectRatio(16f / 9f).clip(RoundedCornerShape(RenderTheme.Radius.inner))
                    .semantics { contentDescription = videoLabel },
            )
            ActionButton(
                stringResource(R.string.renderer_dashcam_save_to_gallery),
                ActionButtonKind.PRIMARY,
                onClick = {
                    saving = true
                    scope.launch {
                        savedToGallery = recorder.saveToGallery(clip)
                        saving = false
                    }
                },
                running = saving,
            )
            ActionButton(
                stringResource(R.string.renderer_dashcam_share),
                ActionButtonKind.SECONDARY,
                onClick = { runCatching { context.startActivity(recorder.shareIntent(clip)) } },
                icon = "square.and.arrow.up",
            )
            savedToGallery?.let { ok ->
                Text(
                    stringResource(if (ok) R.string.renderer_dashcam_gallery_saved else R.string.renderer_dashcam_gallery_failed),
                    Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    style = MaterialTheme.typography.bodySmall,
                    color = if (ok) RenderTheme.colors.positive else RenderTheme.colors.negative,
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                ActionButton(
                    stringResource(R.string.renderer_dashcam_delete),
                    ActionButtonKind.DESTRUCTIVE,
                    onClick = {
                        recorder.delete(clip)
                        onDismiss()
                    },
                    modifier = Modifier.weight(1f),
                    icon = "trash",
                )
                ActionButton(
                    stringResource(R.string.renderer_dashcam_close),
                    ActionButtonKind.PLAIN,
                    onClick = onDismiss,
                    modifier = Modifier.weight(1f),
                )
            }
        }
    }
}
