package jp.kz2601.metamon.settings

import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.automations.AppAutomations
import jp.kz2601.metamon.services.automations.AutomationRule
import jp.kz2601.metamon.services.automations.AutomationsClient
import kotlinx.coroutines.launch
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * 自動の動きの一覧(iOS の `AutomationsView`・#905): 説明・最後に動いた時刻・オン / オフ(楽観的・失敗で戻してトースト)。
 * @param onFix 続けて失敗しているルールを直す依頼を入力欄へ(送らない・設定を閉じるのは呼び出し側)
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AutomationsSheet(appId: String, client: AutomationsClient, onDismiss: () -> Unit, onFix: (String) -> Unit) {
    val model = remember(appId, client) { AutomationsModel(appId, client) }
    val phase by model.phase.collectAsStateWithLifecycle()
    val pending by model.pending.collectAsStateWithLifecycle()
    val toggleError by model.toggleError.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    LaunchedEffect(model) { model.load() }
    val conflictText = stringResource(R.string.automations_conflict)
    val failedText = stringResource(R.string.automations_toggle_failed)
    LaunchedEffect(toggleError) {
        val e = toggleError ?: return@LaunchedEffect
        val text = if (e == AutomationsModel.ToggleError.CONFLICT) conflictText else failedText
        Toast.makeText(context, text, Toast.LENGTH_LONG).show()
        model.clearToggleError()
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = Modifier.testTag("automations.sheet"),
    ) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(bottom = 24.dp)) {
            Text(
                stringResource(R.string.settings_automations),
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            )
            when (val p = phase) {
                AutomationsModel.Phase.Loading -> Skeleton()
                AutomationsModel.Phase.NotCreator -> Note(stringResource(R.string.automations_not_creator))
                AutomationsModel.Phase.Failed -> {
                    Note(stringResource(R.string.automations_load_failed))
                    TextButton(
                        onClick = { scope.launch { model.load() } },
                        modifier = Modifier.padding(horizontal = 4.dp).testTag("automations.retry"),
                    ) { Text(stringResource(R.string.automations_retry)) }
                }
                is AutomationsModel.Phase.Loaded -> Loaded(
                    p.automations,
                    pending = pending,
                    onToggle = { rule, on -> scope.launch { model.setEnabled(on, rule.id) } },
                    onFix = onFix,
                )
            }
        }
    }
}

@Composable
private fun Loaded(a: AppAutomations, pending: Set<String>, onToggle: (AutomationRule, Boolean) -> Unit, onFix: (String) -> Unit) {
    a.stopped?.let { StopNotice(stringResource(R.string.automations_stopped), it.until) }
    a.notifyPaused?.let { StopNotice(stringResource(R.string.automations_notify_paused), it.until) }
    if (a.rules.isEmpty()) {
        Note(stringResource(R.string.automations_empty), Modifier.testTag("automations.empty"))
    }
    a.rules.forEachIndexed { i, rule ->
        if (i > 0) HorizontalDivider(Modifier.padding(horizontal = 16.dp))
        RuleRow(rule, isPending = rule.id in pending, onToggle = { onToggle(rule, it) }, onFix = onFix)
    }
    a.quota?.let { q ->
        HorizontalDivider(Modifier.padding(vertical = 8.dp))
        Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
            Text(stringResource(R.string.automations_quota), modifier = Modifier.weight(1f))
            Text("${q.used} / ${q.limit}")
        }
        Note(stringResource(R.string.automations_quota_footer), small = true)
    }
}

@Composable
private fun RuleRow(rule: AutomationRule, isPending: Boolean, onToggle: (Boolean) -> Unit, onFix: (String) -> Unit) {
    val toggleLabel = stringResource(R.string.automations_toggle_label, rule.label)
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp).testTag("automations.rule.${rule.id}")) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(rule.label, style = MaterialTheme.typography.bodyLarge)
                formatTime(rule.lastRunAt)?.let {
                    Text(
                        stringResource(R.string.automations_last_run, it),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            val enabled = rule.enabled
            if (enabled != null) {
                Spacer(Modifier.width(12.dp))
                Text(
                    stringResource(if (enabled) R.string.automations_on else R.string.automations_off),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.width(8.dp))
                Switch(
                    checked = enabled,
                    onCheckedChange = onToggle,
                    enabled = !isPending,
                    modifier = Modifier.semantics { contentDescription = toggleLabel }.testTag("automations.toggle.${rule.id}"),
                )
            }
        }
        if (rule.suggestsFix) {
            val request = stringResource(R.string.automations_fix_request, rule.label)
            TextButton(onClick = { onFix(request) }, modifier = Modifier.testTag("automations.fix.${rule.id}")) {
                Text(stringResource(R.string.automations_fix))
            }
        }
    }
}

@Composable
private fun StopNotice(text: String, until: String?) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp)) {
        Text(text, color = MaterialTheme.colorScheme.error)
        formatTime(until)?.let {
            Text(stringResource(R.string.automations_until, it), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** 読み込み中の形だけの行(データが無いのと区別する) */
@Composable
private fun Skeleton() {
    val loading = stringResource(R.string.automations_loading)
    val shade = MaterialTheme.colorScheme.surfaceVariant
    Column(Modifier.semantics(mergeDescendants = true) { contentDescription = loading }.testTag("automations.skeleton")) {
        repeat(2) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Box(Modifier.fillMaxWidth(0.8f).height(16.dp).background(shade, RoundedCornerShape(4.dp)))
                    Spacer(Modifier.height(6.dp))
                    Box(Modifier.fillMaxWidth(0.4f).height(12.dp).background(shade, RoundedCornerShape(4.dp)))
                }
                Spacer(Modifier.width(12.dp))
                Box(Modifier.width(52.dp).height(32.dp).background(shade, RoundedCornerShape(16.dp)))
            }
        }
    }
}

@Composable
private fun Note(text: String, modifier: Modifier = Modifier, small: Boolean = false) {
    Text(
        text,
        style = if (small) MaterialTheme.typography.bodySmall else MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

private val timeFormat: DateTimeFormatter = DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)

/** サーバーの ISO 8601 を端末の時刻で。読めなければ出さない */
private fun formatTime(iso: String?): String? = iso?.let {
    runCatching { timeFormat.withZone(ZoneId.systemDefault()).format(OffsetDateTime.parse(it)) }.getOrNull()
}
