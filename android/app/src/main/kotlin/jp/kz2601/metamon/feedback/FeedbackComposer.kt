package jp.kz2601.metamon.feedback

import android.content.Context
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
import androidx.compose.foundation.Image
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.AppContainer
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.feedback.FeedbackConsent
import jp.kz2601.metamon.services.feedback.FeedbackContext
import jp.kz2601.metamon.services.feedback.FeedbackDraft
import jp.kz2601.metamon.services.feedback.FeedbackReceipt
import jp.kz2601.metamon.services.feedback.FeedbackSender
import jp.kz2601.metamon.services.feedback.FeedbackSubmission
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * フィードバックの画面と、端末を振って開く仕組みをまとめて置く(iOS の `FeedbackShakeHost` + `FeedbackPresenter`)。
 * 設定のシートより後に置く(設定から開いた時に上に重ねる)。
 */
@Composable
fun FeedbackComposerHost(container: AppContainer) {
    val launcher = container.feedbackComposer
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    ShakeEffect {
        val activity = context.findActivity() ?: return@ShakeEffect
        scope.launch { launcher.capture(activity, container, withOverlays = true) }
    }
    val open by launcher.open.collectAsStateWithLifecycle()
    val sender = remember(container) { FeedbackSender(container.feedback) }
    open?.let { FeedbackComposerDialog(it, sender, onClose = launcher::close) }
}

/** 設定の入口(iOS の `feedbackSection`)。スクショはシートの下のアプリの画面を、開いた時点で撮る(送るかは本人が選ぶ)。 */
@Composable
fun FeedbackSettingsSection(container: AppContainer) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    TextButton(
        onClick = { context.findActivity()?.let { scope.launch { container.feedbackComposer.capture(it, container) } } },
        modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.feedback"),
    ) { Text(stringResource(R.string.feedback_composer_settings_entry)) }
    Text(
        stringResource(R.string.feedback_composer_settings_footer),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp),
    )
    HorizontalDivider(Modifier.padding(vertical = 8.dp))
}

/** 画面に出ている間(RESUMED)だけ加速度を見る。振ったら [onShake]。 */
@Composable
private fun ShakeEffect(onShake: () -> Unit) {
    val context = LocalContext.current
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val latest by rememberUpdatedState(onShake)
    DisposableEffect(lifecycle, context) {
        val sensors = context.getSystemService(Context.SENSOR_SERVICE) as? SensorManager
        val accel = sensors?.getDefaultSensor(Sensor.TYPE_ACCELEROMETER)
        val detector = ShakeDetector()
        val listener = object : SensorEventListener {
            override fun onSensorChanged(event: SensorEvent) {
                val v = event.values
                if (detector.onSample(v[0], v[1], v[2], event.timestamp / 1_000_000)) latest()
            }

            override fun onAccuracyChanged(sensor: Sensor?, accuracy: Int) = Unit
        }
        var registered = false
        fun register() {
            if (registered || sensors == null || accel == null) return
            registered = sensors.registerListener(listener, accel, SensorManager.SENSOR_DELAY_UI)
        }
        fun unregister() {
            if (!registered) return
            sensors?.unregisterListener(listener)
            registered = false
            detector.reset()
        }
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_RESUME -> register()
                Lifecycle.Event.ON_PAUSE -> unregister()
                else -> Unit
            }
        }
        lifecycle.addObserver(observer)
        if (lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)) register()
        onDispose {
            lifecycle.removeObserver(observer)
            unregister()
        }
    }
}

/**
 * フィードバックを書いて送る画面(iOS の `FeedbackComposerView`)。
 * - 添付はどれも既定でオフ。オンにした添付は、送る中身をその場で見せる
 * - 送る前に同意文を見せ、同意のトグルを入れるまで送れない([FeedbackConsent.VERSION] を一緒に送る)
 * - 記録(入力したデータ)の中身は送らない
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun FeedbackComposerDialog(open: FeedbackOpen, sender: FeedbackSender, onClose: () -> Unit) {
    var draft by remember(open) { mutableStateOf(FeedbackDraft()) }
    var sending by remember(open) { mutableStateOf(false) }
    var error by remember(open) { mutableStateOf<Int?>(null) }
    var receipt by remember(open) { mutableStateOf<FeedbackReceipt?>(null) }
    val scope = rememberCoroutineScope()
    val ctx = open.context
    // 書きかけ・送信中は外側のタップ・戻るで閉じない(iOS の interactiveDismissDisabled)
    val dismissible = !sending && draft.trimmed.isEmpty()

    fun send() {
        sending = true
        scope.launch {
            try {
                // 縮めるのは同意があってスクショをオンにした時だけ(重いのでメインスレッドの外で)
                val shot = open.screenshot
                val jpeg = if (draft.canSend && draft.attachScreenshot && shot != null) {
                    withContext(Dispatchers.Default) { FeedbackScreenshot.jpeg(shot) }
                } else {
                    null
                }
                val result = sender.send(draft, ctx) { jpeg }
                when (result) {
                    is FeedbackSender.Result.Sent -> receipt = result.receipt
                    FeedbackSender.Result.ScreenshotTooLarge -> error = R.string.feedback_composer_error_screenshot
                    FeedbackSender.Result.NotReady -> Unit
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                error = errorMessage(e)
            } finally {
                sending = false
            }
        }
    }

    Dialog(
        onDismissRequest = { if (dismissible) onClose() },
        properties = DialogProperties(usePlatformDefaultWidth = false, dismissOnBackPress = dismissible, dismissOnClickOutside = false),
    ) {
        Scaffold(
            modifier = Modifier.fillMaxSize().testTag("feedback.composer"),
            topBar = {
                TopAppBar(
                    title = { Text(stringResource(R.string.feedback_composer_title)) },
                    navigationIcon = {
                        TextButton(onClick = onClose, enabled = !sending) { Text(stringResource(R.string.feedback_composer_cancel)) }
                    },
                    actions = {
                        if (sending) {
                            CircularProgressIndicator(Modifier.padding(horizontal = 16.dp).heightIn(max = 24.dp))
                        } else {
                            TextButton(
                                onClick = ::send,
                                enabled = draft.canSend && receipt == null,
                                modifier = Modifier.testTag("feedback.send"),
                            ) { Text(stringResource(R.string.feedback_composer_send), fontWeight = FontWeight.SemiBold) }
                        }
                    },
                )
            },
        ) { padding ->
            Column(
                Modifier.padding(padding).fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                MessageSection(draft) { draft = it }
                HorizontalDivider()
                AttachmentSection(draft, open) { draft = it }
                HorizontalDivider()
                ConsentSection(draft) { draft = it }
            }
        }
    }

    error?.let { res ->
        AlertDialog(
            onDismissRequest = { error = null },
            title = { Text(stringResource(R.string.feedback_composer_error_title)) },
            text = { Text(stringResource(res)) },
            confirmButton = { TextButton(onClick = { error = null }) { Text(stringResource(R.string.feedback_composer_ok)) } },
        )
    }
    if (receipt != null) {
        AlertDialog(
            onDismissRequest = onClose,
            title = { Text(stringResource(R.string.feedback_composer_sent_title)) },
            text = { Text(stringResource(R.string.feedback_composer_sent_body)) },
            confirmButton = { TextButton(onClick = onClose) { Text(stringResource(R.string.feedback_composer_close)) } },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun MessageSection(draft: FeedbackDraft, onChange: (FeedbackDraft) -> Unit) {
    val kinds = listOf(
        FeedbackSubmission.Kind.BUG to R.string.feedback_composer_kind_bug,
        FeedbackSubmission.Kind.IDEA to R.string.feedback_composer_kind_idea,
        FeedbackSubmission.Kind.OTHER to R.string.feedback_composer_kind_other,
    )
    SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
        kinds.forEachIndexed { i, (kind, label) ->
            SegmentedButton(
                selected = draft.kind == kind,
                onClick = { onChange(draft.copy(kind = kind)) },
                shape = SegmentedButtonDefaults.itemShape(i, kinds.size),
            ) { Text(stringResource(label)) }
        }
    }
    OutlinedTextField(
        value = draft.message,
        onValueChange = { onChange(draft.copy(message = it)) },
        placeholder = { Text(stringResource(R.string.feedback_composer_message_placeholder)) },
        isError = draft.isTooLong,
        supportingText = if (draft.isTooLong) {
            { Text(stringResource(R.string.feedback_composer_max_length, FeedbackConsent.MAX_MESSAGE_LENGTH)) }
        } else {
            null
        },
        minLines = 5,
        modifier = Modifier.fillMaxWidth().testTag("feedback.message"),
    )
}

@Composable
private fun AttachmentSection(draft: FeedbackDraft, open: FeedbackOpen, onChange: (FeedbackDraft) -> Unit) {
    val ctx = open.context
    SectionHeader(stringResource(R.string.feedback_composer_attach_header))
    open.screenshot?.let { shot ->
        ToggleRow(stringResource(R.string.feedback_composer_attach_screenshot), draft.attachScreenshot, "feedback.attachScreenshot") {
            onChange(draft.copy(attachScreenshot = it))
        }
        if (draft.attachScreenshot) {
            val image = remember(shot) { shot.asImageBitmap() }
            Image(
                image,
                contentDescription = stringResource(R.string.feedback_composer_screenshot_description),
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxWidth().heightIn(max = 280.dp).clip(RoundedCornerShape(8.dp)),
            )
            Note(stringResource(R.string.feedback_composer_screenshot_note))
        }
    }
    ctx.app?.let { app ->
        ToggleRow(stringResource(R.string.feedback_composer_attach_app), draft.attachApp, "feedback.attachApp") {
            onChange(draft.copy(attachApp = it))
        }
        if (draft.attachApp) AppPreview(app)
    }
    ToggleRow(stringResource(R.string.feedback_composer_attach_device), draft.attachDevice, "feedback.attachDevice") {
        onChange(draft.copy(attachDevice = it))
    }
    if (draft.attachDevice) {
        val d = ctx.device
        LabeledValue(stringResource(R.string.feedback_composer_device_app_version), "${d.appVersion} (${d.build})")
        LabeledValue(stringResource(R.string.feedback_composer_device_os), d.os)
        LabeledValue(stringResource(R.string.feedback_composer_device_model), d.model)
        LabeledValue(stringResource(R.string.feedback_composer_device_language), d.locale)
    }
    Note(stringResource(R.string.feedback_composer_attach_footer))
}

@Composable
private fun AppPreview(app: FeedbackContext.AppSummary) {
    val separator = stringResource(R.string.feedback_composer_list_separator)
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(stringResource(R.string.feedback_composer_app_title, app.name), style = MaterialTheme.typography.titleSmall)
        Text(stringResource(R.string.feedback_composer_app_screens, app.screens.joinToString(separator)), style = MaterialTheme.typography.bodySmall)
        if (app.entities.isNotEmpty()) {
            Text(stringResource(R.string.feedback_composer_app_fields, app.entities.joinToString(separator)), style = MaterialTheme.typography.bodySmall)
        }
        Note(stringResource(R.string.feedback_composer_app_note))
    }
    app.json?.let { json ->
        var expanded by remember { mutableStateOf(false) }
        Text(
            stringResource(R.string.feedback_composer_show_dsl),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.clickable { expanded = !expanded }.padding(vertical = 4.dp),
        )
        if (expanded) {
            Text(
                json,
                style = MaterialTheme.typography.labelSmall,
                fontFamily = FontFamily.Monospace,
                softWrap = false,
                modifier = Modifier.heightIn(max = 240.dp).verticalScroll(rememberScrollState()).horizontalScroll(rememberScrollState()),
            )
        }
    }
}

@Composable
private fun ConsentSection(draft: FeedbackDraft, onChange: (FeedbackDraft) -> Unit) {
    SectionHeader(stringResource(R.string.feedback_composer_consent_header))
    Text(stringResource(R.string.feedback_composer_consent, FeedbackConsent.RETENTION_DAYS), style = MaterialTheme.typography.bodySmall)
    ToggleRow(stringResource(R.string.feedback_composer_agree), draft.agreed, "feedback.agree") { onChange(draft.copy(agreed = it)) }
}

@Composable
private fun SectionHeader(text: String) {
    Text(text, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

@Composable
private fun Note(text: String) {
    Text(text, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

@Composable
private fun LabeledValue(label: String, value: String) {
    Row(Modifier.fillMaxWidth()) {
        Text(label, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
        Text(value, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun ToggleRow(label: String, checked: Boolean, tag: String, onChange: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = onChange, modifier = Modifier.testTag(tag))
    }
}

/** 失敗の文言(iOS の `FeedbackComposerView.message(for:)`)。 */
internal fun errorMessage(error: Throwable): Int = when (FailureKind.classify(error)) {
    is FailureKind.RateLimited -> R.string.feedback_composer_error_rate_limited
    FailureKind.Offline -> R.string.feedback_composer_error_offline
    FailureKind.Timeout, is FailureKind.Busy -> R.string.feedback_composer_error_server_busy
    FailureKind.SessionExpired -> R.string.feedback_composer_error_session
    else -> R.string.feedback_composer_error_default
}
