package jp.kz2601.metamon.renderer.components.ai

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Length
import jp.kz2601.metamon.dsl.Task
import jp.kz2601.metamon.dsl.aiSaveTo
import jp.kz2601.metamon.dsl.ai.AiSaveTo
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.dsl.ai.AiTextShape
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.feed.LocalFeedHost
import jp.kz2601.metamon.renderer.components.feed.feeds
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/**
 * `aiButton`(#78・iOS の AiButtonLeaf)。要約・分類などは結果を output のドラフトに入れる(保存は利用者)。
 * `task: "generate"` はフォーム不要で、結果をボタンの下に出す(AiGenerateLeaf)。
 * 端末内 AI を優先し、使えない端末だけホストのオンライン版。どちらも無ければ無効にして理由を出す。
 */
@Composable
fun AiButtonLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val form = ctx.form
    when {
        c.task == Task.GENERATE -> AiGenerateLeaf(c, ctx, modifier)
        form != null -> AiTransformLeaf(c, ctx, form, modifier)
        else -> Text(
            stringResource(R.string.ai_needs_form),
            modifier.fillMaxWidth(),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** 部品の言語(ja | en)。描画中の言語設定に合わせる。 */
@Composable
private fun aiLocale(): String = if (LocalConfiguration.current.locales[0]?.language == "en") "en" else "ja"

@Composable
private fun AiTransformLeaf(c: Component, ctx: RenderContext, form: FormDraft, modifier: Modifier) {
    val scope = rememberCoroutineScope()
    val locale = aiLocale()
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<AiMessage?>(null) }
    var doneOnline by remember { mutableStateOf<Boolean?>(null) }
    val route = AiButtonRun.route(c, ctx.runtime)
    ShownMetric(c, ctx)
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        val hint = when (route) {
            AiRoute.OnDevice -> stringResource(R.string.ai_hint_on_device)
            AiRoute.Online -> stringResource(R.string.ai_hint_online)
            is AiRoute.Unavailable -> stringResource(AiCopy.reason(route.reason))
        }
        AiRunButton(
            label = c.label?.takeIf { it.isNotEmpty() } ?: stringResource(AiCopy.label(c.task)),
            busy = busy,
            enabled = route !is AiRoute.Unavailable,
            hint = hint,
            tag = "aiButton.${c.id}",
        ) {
            scope.launch {
                busy = true
                error = null
                doneOnline = null
                try {
                    doneOnline = AiButtonRun.transform(c, form, ctx.runtime, locale)
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    error = AiCopy.error(e)
                } finally {
                    busy = false
                }
            }
        }
        val done = doneOnline
        when {
            error != null -> Caption(error!!.text(), isError = true)
            route is AiRoute.Unavailable -> Caption(stringResource(R.string.ai_unavailable, stringResource(AiCopy.reason(route.reason))))
            done != null -> Caption(stringResource(if (done) R.string.ai_done_online else R.string.ai_done_on_device))
        }
    }
}

@Composable
private fun AiGenerateLeaf(c: Component, ctx: RenderContext, modifier: Modifier) {
    val scope = rememberCoroutineScope()
    val locale = aiLocale()
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<AiMessage?>(null) }
    var result by remember { mutableStateOf<AiTextResult?>(null) }
    var usedOnline by remember { mutableStateOf(false) }
    var runs by remember { mutableIntStateOf(0) }
    var outputs by remember { mutableStateOf<AiButtonRun.OutputsFill?>(null) } // outputs: 今回入れた / 入れなかった項目
    val aiValues = remember { mutableMapOf<String, JSONValue>() } // outputs: 前回 AI が入れた値(そのままなら次は入れ替えてよい)
    val route = AiButtonRun.route(c, ctx.runtime)
    val saveTo = remember(c.saveTo) { c.aiSaveTo }
    val feeds = ctx.runtime.feeds ?: LocalFeedHost.current
    val view = LocalView.current
    ShownMetric(c, ctx)
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        val label = c.label?.takeIf { it.isNotEmpty() }
            ?: stringResource(if (result == null) R.string.ai_generate_label else R.string.ai_generate_again)
        AiRunButton(
            label = label,
            busy = busy,
            enabled = route !is AiRoute.Unavailable,
            hint = (route as? AiRoute.Unavailable)?.let { stringResource(AiCopy.reason(it.reason)) },
            tag = "aiGenerate.${c.id}",
        ) {
            scope.launch {
                busy = true
                error = null
                try {
                    val out = AiButtonRun.generate(c, ctx.form, ctx.runtime, locale, route, ctx.screen, feeds)
                    usedOnline = route == AiRoute.Online
                    result = out
                    runs += 1
                    val form = ctx.form
                    outputs = form?.let { AiButtonRun.fillOutputs(c, out, it, aiValues) }
                    val names = outputs?.applied?.applied.orEmpty().map { it.fieldName }
                    if (names.isNotEmpty()) view.announceForAccessibility(view.context.getString(R.string.ai_outputs_announce, names.joinToString("、")))
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    error = AiCopy.error(e)
                } finally {
                    busy = false
                }
            }
        }
        val out = result
        if (out != null && out.shape == AiTextShape.LIST && saveTo != null) {
            // 新しい結果は「保存済み」を持ち越さない
            androidx.compose.runtime.key(runs) { AiSaveToList(saveTo, out.items.orEmpty(), ctx) }
        } else if (out != null) {
            AiTextResultView(out, long = c.length == Length.LONG)
        }
        when {
            error != null -> Caption(error!!.text(), isError = true)
            route is AiRoute.Unavailable -> Caption(stringResource(R.string.ai_unavailable, stringResource(AiCopy.reason(route.reason))))
            out != null && !outputs?.filled.isNullOrEmpty() -> OutputsCaption(outputs!!)
            out != null && c.output != null && ctx.form != null -> Caption(stringResource(R.string.ai_generate_filled))
            out != null && usedOnline -> Caption(stringResource(R.string.ai_generate_online))
        }
    }
}

/** outputs で入れた項目・入力済みで入れなかった項目・読めなかった項目(iOS の outputsCaption)。 */
@Composable
private fun OutputsCaption(fill: AiButtonRun.OutputsFill) {
    val ok = fill.applied.applied.size
    val kept = fill.applied.kept.map { it.fieldName }
    val missed = fill.missed.map { it.fieldName }
    Column(Modifier.testTag("aiGenerate.outputs"), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        if (ok > 0) Caption(pluralStringResource(R.plurals.ai_outputs_filled, ok, ok))
        if (kept.isNotEmpty()) Caption(stringResource(R.string.ai_outputs_kept, kept.joinToString("、")))
        if (missed.isNotEmpty()) Caption(stringResource(R.string.ai_outputs_missed, missed.joinToString("、")))
    }
}

/** generate の list を 1 行ずつ / 全部、entity に保存する(iOS の AiSaveToList)。押すまで何も保存しない。 */
@Composable
private fun AiSaveToList(saveTo: AiSaveTo, items: List<String>, ctx: RenderContext) {
    val scope = rememberCoroutineScope()
    var saved by remember { mutableStateOf(emptySet<Int>()) }
    var failed by remember { mutableStateOf(emptySet<Int>()) }
    var saving by remember { mutableStateOf(false) }
    var status by remember { mutableStateOf<AiButtonRun.SaveOutcome?>(null) }
    val canSave = ctx.runtime.entityModel(saveTo.entity) != null
    fun save(indices: List<Int>) {
        if (saving) return
        scope.launch {
            saving = true
            try {
                val o = AiButtonRun.save(saveTo, items, indices, ctx.runtime)
                saved = saved + o.saved
                failed = failed - o.saved + o.failed
                status = o
            } finally {
                saving = false
            }
        }
    }
    Column(verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        ResultBox {
            items.forEachIndexed { i, item ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("・", color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(
                        item,
                        Modifier.weight(1f),
                        color = if (i in saved) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
                    )
                    if (i in saved) {
                        Text(stringResource(R.string.ai_saved), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    } else {
                        TextButton(onClick = { save(listOf(i)) }, enabled = !saving && canSave, modifier = Modifier.testTag("aiGenerate.save.$i")) {
                            Text(stringResource(R.string.ai_save))
                        }
                        if (i in failed) {
                            val d = stringResource(R.string.ai_save_failed)
                            Text("!", color = MaterialTheme.colorScheme.error, modifier = Modifier.semantics { contentDescription = d })
                        }
                    }
                }
            }
        }
        val unsaved = items.indices.filter { it !in saved }
        if (unsaved.size > 1) {
            OutlinedButton(
                onClick = { save(unsaved) },
                enabled = !saving && canSave,
                modifier = Modifier.fillMaxWidth().testTag("aiGenerate.saveAll"),
                shape = RoundedCornerShape(RenderTheme.Radius.button),
            ) { Text(stringResource(R.string.ai_save_all)) }
        }
        status?.let { o ->
            val text = when {
                o.full -> stringResource(R.string.ai_save_full)
                o.notAdded == 0 -> pluralStringResource(R.plurals.ai_save_added, o.added, o.added)
                else -> pluralStringResource(R.plurals.ai_save_added_partial, o.added, o.added, o.notAdded)
            }
            Caption(text, isError = failed.isNotEmpty())
        }
    }
}

/** 結果の表示。line = 1 文 / list = 箇条書き / fields = 項目名と値。選んでコピーできる。 */
@Composable
private fun AiTextResultView(result: AiTextResult, long: Boolean) {
    ResultBox(Modifier.testTag(if (long) "aiGenerate.result.long" else "aiGenerate.result")) {
        when (result.shape) {
            AiTextShape.LINE -> Text(result.text, style = MaterialTheme.typography.bodyLarge)
            AiTextShape.LIST -> result.items.orEmpty().forEach { Text("・$it") }
            AiTextShape.FIELDS -> result.fields.orEmpty().forEach { f ->
                Column {
                    Text(f.name, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(f.value)
                }
            }
        }
    }
}

@Composable
private fun ResultBox(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    SelectionContainer(
        modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.5f), RoundedCornerShape(RenderTheme.Radius.control))
            .padding(RenderTheme.Spacing.md),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) { content() }
    }
}

@Composable
private fun AiRunButton(label: String, busy: Boolean, enabled: Boolean, hint: String?, tag: String, onClick: () -> Unit) {
    OutlinedButton(
        onClick = onClick,
        enabled = enabled && !busy,
        modifier = Modifier
            .fillMaxWidth()
            .testTag(tag)
            .semantics { if (hint != null) contentDescription = "$label. $hint" },
        shape = RoundedCornerShape(RenderTheme.Radius.button),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
            if (busy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
            Text(if (busy) stringResource(R.string.ai_thinking) else label)
        }
    }
}

@Composable
private fun Caption(text: String, isError: Boolean = false) {
    Text(
        text,
        style = MaterialTheme.typography.labelSmall,
        color = if (isError) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun AiMessage.text(): String = raw ?: if (arg != null) stringResource(res, arg) else stringResource(res)

/** 出した時の可用性を 1 回数える(#446)。 */
@Composable
private fun ShownMetric(c: Component, ctx: RenderContext) {
    LaunchedEffect(c.id) {
        val host = ctx.runtime.ai ?: return@LaunchedEffect
        host.record(AiButtonMetric.Shown(host.onDevice.availability().metric))
    }
}
