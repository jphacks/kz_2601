package jp.kz2601.metamon.renderer.components.importing

import android.content.Context
import android.media.MediaMetadataRetriever
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.actions.ActionNotice
import jp.kz2601.metamon.renderer.form.FormChoices
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * `fileImport`(#865・iOS の FileImportLeaf): ボタンでファイルを選び(Storage Access Framework・権限の確認なし)、entity に記録を足す。
 * - csv: 1 行 = 1 件(列の当て方・型の変換は :dsl の RecordImport)。足す前に「n 件を追加・m 行は読めない」を確かめる。
 * - text / pdf: 本文を `field` に入れた記録を 1 件(PDF の画像だけのページは端末内の文字認識・[PdfTextReader])。
 * - audio: 録音を端末内で文字にして(whisper.cpp・[AudioFileTranscriptionHost]・30 分 / 200MB まで)、`field` に入れた記録を 1 件(#1180)。
 *   文字にしている間は進み具合と「キャンセル」を出し、アプリを離れたらやめる(前面にいる間だけ進める)。口が無い画面では選べる種類から外す。
 * ファイルは読むだけで、送らない・コピーしない・残さない。足すのは [DSLRuntime.addRecord](フォームの保存と同じく端末に入り同期)。
 */
@Composable
fun FileImportLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val scope = rememberCoroutineScope()
    val runtime = ctx.runtime
    val entity = c.entity?.let { runtime.entityModel(it) }
    var reading by remember { mutableStateOf(false) }
    var progress by remember { mutableStateOf<Pair<Int, Int>?>(null) }
    var pending by remember { mutableStateOf<ImportStep.Confirm?>(null) }
    var problem by remember { mutableStateOf<ImportProblem?>(null) }
    var status by remember { mutableStateOf<String?>(null) }
    var problemText by remember { mutableStateOf<String?>(null) }
    // 録音の文字起こし(#1180): 進み具合(0..1)・走っている仕事・アプリを離れたか
    var transcribing by remember { mutableStateOf<Double?>(null) }
    var transcribeJob by remember { mutableStateOf<Job?>(null) }
    var leftForeground by remember { mutableStateOf(false) }
    val audioHost = runtime.audioFileTranscription
    val wantsAudio = Accept.AUDIO in c.accept
    val audioReadiness = audioHost?.audioFileReadiness?.collectAsState()?.value
    val audioUsable = audioHost != null && audioReadiness != AudioModelReadiness.Unavailable

    // 前面にいる間だけ文字にする(裏に回ったらやめて、戻った時に一言)
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_STOP && transcribeJob?.isActive == true) {
                leftForeground = true
                transcribeJob?.cancel()
            }
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }

    if (entity == null || entity.scope == "public" || entity.single) {
        ActionNotice(stringResource(R.string.renderer_import_unusable), modifier)
        return
    }
    val mimes = FileImportFlow.mimeTypes(c.accept, audio = audioUsable)
    val usable = FileImportFlow.usable(c, entity, audio = audioUsable)

    /** 録音を端末内で文字にし、本文を `field` に入れた 1 件を確かめる(iOS の FileImportLeaf.transcribe)。 */
    fun startTranscription(a: Read.Audio) {
        val host = audioHost
        if (host == null) {
            problemText = resources.getString(R.string.renderer_import_audio_no_host)
            return
        }
        if (host.audioFileReadiness.value != AudioModelReadiness.Ready) {
            problemText = resources.getString(R.string.renderer_import_audio_not_ready)
            return
        }
        AudioFileImport.check(a.bytes, a.seconds)?.let {
            problemText = audioProblemText(resources, it)
            return
        }
        leftForeground = false
        transcribing = 0.0
        transcribeJob = scope.launch {
            try {
                val text = host.transcribeAudioFile(a.uri) { transcribing = it }
                if (text.isBlank()) {
                    problemText = resources.getString(R.string.renderer_import_audio_not_heard)
                    return@launch
                }
                val existing = FormChoices.records(runtime, entity.id).size
                val note = resources.getString(R.string.renderer_import_audio_length, durationText(resources, a.seconds ?: 0.0))
                when (val step = FileImportFlow.planText(text, a.fileName, c, entity, existing, note)) {
                    is ImportStep.Confirm -> pending = step
                    is ImportStep.Problem -> problem = step.problem
                }
            } catch (e: CancellationException) {
                status = resources.getString(if (leftForeground) R.string.renderer_import_audio_left_app else R.string.renderer_import_audio_stopped)
                throw e
            } catch (e: AudioFileTranscriptionException) {
                problemText = resources.getString(
                    when (e.kind) {
                        AudioFileTranscriptionException.Kind.NOT_READY -> R.string.renderer_import_audio_not_ready
                        AudioFileTranscriptionException.Kind.UNREADABLE -> R.string.renderer_import_audio_unreadable
                        AudioFileTranscriptionException.Kind.FAILED -> R.string.renderer_import_audio_failed
                    },
                )
            } catch (e: Exception) {
                problemText = resources.getString(R.string.renderer_import_audio_failed)
            } finally {
                transcribing = null
                transcribeJob = null
            }
        }
    }

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            reading = true
            try {
                val refs = entity.fields.mapNotNull { it.refEntity }.toSet()
                runtime.loadRecords(refs + entity.id)
                when (val step = read(context, uri, c, entity, ctx, audio = audioHost != null)) {
                    is Read.Done -> when (val s = step.step) {
                        is ImportStep.Confirm -> pending = s
                        is ImportStep.Problem -> problem = s.problem
                    }
                    is Read.Audio -> startTranscription(step)
                }
            } finally {
                reading = false
            }
        }
    }

    val p = progress
    val label = when {
        transcribing != null -> stringResource(R.string.renderer_import_audio_progress)
        p != null -> stringResource(R.string.renderer_import_adding, p.first, p.second)
        reading -> stringResource(R.string.renderer_import_reading)
        else -> c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.renderer_import_button)
    }
    Column(modifier, verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        ActionButton(
            label = label,
            kind = ActionButtonKind.SECONDARY,
            onClick = {
                status = null
                picker.launch(mimes.toTypedArray())
            },
            modifier = Modifier.testTag("fileImport.button"),
            icon = "doc",
            enabled = usable && !reading && p == null && transcribing == null,
            running = reading || p != null,
        )
        transcribing?.let { FileImportAudioProgress(it) { transcribeJob?.cancel() } }
        if (wantsAudio && !reading && transcribing == null) FileImportAudioModelRow(audioHost)
        status?.let {
            Text(it, Modifier.testTag("fileImport.status"), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }

    pending?.let { step ->
        val res = resources
        AlertDialog(
            onDismissRequest = { pending = null },
            title = { Text(stringResource(R.string.renderer_import_confirm_title)) },
            text = { Text(FileImportCopy.message(res, step.message)) },
            confirmButton = {
                TextButton(onClick = {
                    pending = null
                    scope.launch {
                        val total = step.rows.size
                        progress = 0 to total
                        val added = FileImportFlow.add(runtime,entity.id, step.rows) { progress = it to total }
                        progress = null
                        status = FileImportCopy.result(res, added, total)
                    }
                }) { Text(stringResource(R.string.renderer_import_confirm_add)) }
            },
            dismissButton = { TextButton(onClick = { pending = null }) { Text(stringResource(R.string.renderer_import_cancel)) } },
        )
    }
    problemText?.let { text ->
        AlertDialog(
            onDismissRequest = { problemText = null },
            title = { Text(stringResource(R.string.renderer_import_problem_title)) },
            text = { Text(text) },
            confirmButton = { TextButton(onClick = { problemText = null }) { Text(stringResource(R.string.renderer_import_ok)) } },
        )
    }
    problem?.let { pr ->
        AlertDialog(
            onDismissRequest = { problem = null },
            title = { Text(stringResource(R.string.renderer_import_problem_title)) },
            text = { Text(FileImportCopy.problem(resources, pr)) },
            confirmButton = { TextButton(onClick = { problem = null }) { Text(stringResource(R.string.renderer_import_ok)) } },
        )
    }
}

/** 選んだファイルを読んだ結果: 確かめる中身(または理由)か、文字にする録音。 */
private sealed interface Read {
    data class Done(val step: ImportStep) : Read

    data class Audio(val uri: Uri, val fileName: String, val bytes: Long?, val seconds: Double?) : Read
}

/** 選んだファイルを読み、確認する中身にする。上限 + 1 バイトまでしか読まない。録音は大きさと長さだけ下見する。 */
private suspend fun read(context: Context, uri: Uri, c: Component, entity: Entity, ctx: RenderContext, audio: Boolean): Read =
    withContext(Dispatchers.IO) {
        val resolver = context.contentResolver
        val name = runCatching {
            resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cur ->
                if (cur.moveToFirst()) cur.getString(0) else null
            }
        }.getOrNull() ?: uri.lastPathSegment.orEmpty()
        val kind = FileImportFlow.kind(name, resolver.getType(uri), c.accept)
            ?: return@withContext Read.Done(ImportStep.Problem(ImportProblem.Unsupported))
        if (kind == ImportKind.AUDIO) {
            if (!audio) return@withContext Read.Done(ImportStep.Problem(ImportProblem.KindUnavailable))
            return@withContext Read.Audio(uri, name, fileSize(context, uri), durationSeconds(context, uri))
        }
        val limit = FileImportFlow.byteLimit(kind)
        val bytes = runCatching {
            resolver.openInputStream(uri)?.use { input -> FileImportFlow.readUpTo(input, limit + 1) }
        }.getOrNull() ?: return@withContext Read.Done(ImportStep.Problem(ImportProblem.CannotOpen))
        val existing = FormChoices.records(ctx.runtime, entity.id).size
        Read.Done(
            FileImportFlow.prepare(bytes, kind, name, c, entity, existing, pdfText = { PdfTextReader.text(context, it) }) { field, ref ->
                FileImportFlow.refId(ctx.runtime, field, ref)
            },
        )
    }

/** 大きさ(バイト)。分からなければ null。 */
private fun fileSize(context: Context, uri: Uri): Long? = runCatching {
    context.contentResolver.query(uri, arrayOf(OpenableColumns.SIZE), null, null, null)?.use { cur ->
        if (cur.moveToFirst() && !cur.isNull(0)) cur.getLong(0) else null
    }
}.getOrNull()

/** 長さ(秒)。読めなければ null。 */
private fun durationSeconds(context: Context, uri: Uri): Double? {
    val retriever = MediaMetadataRetriever()
    return try {
        retriever.setDataSource(context, uri)
        retriever.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull()?.let { it / 1000.0 }
    } catch (_: RuntimeException) {
        null
    } finally {
        runCatching { retriever.release() }
    }
}
