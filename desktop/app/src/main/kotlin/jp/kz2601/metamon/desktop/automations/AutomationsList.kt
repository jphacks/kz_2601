package jp.kz2601.metamon.desktop.automations

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.automations.AppAutomations
import jp.kz2601.metamon.services.automations.AutomationRule
import kotlinx.coroutines.launch

/**
 * 自動の動きの一覧と、ルールごとのオン / オフ(#3358・Android の `AutomationsSheet`)。
 * 作成者でない(403)時は一覧も切り替えも出さず一言だけ。切り替えに失敗したら [AutomationsModel] が戻し、ここで一言を読ませる。
 */
@Composable
fun AutomationsList(model: AutomationsModel, modifier: Modifier = Modifier) {
    val phase by model.phase.collectAsState()
    val pending by model.pending.collectAsState()
    val toggleError by model.toggleError.collectAsState()
    val scope = rememberCoroutineScope()
    LaunchedEffect(model) { model.load() }

    Column(modifier) {
        toggleError?.let { e ->
            Text(
                Strings[e.key],
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 6.dp).semantics { liveRegion = LiveRegionMode.Polite },
            )
        }
        when (val p = phase) {
            AutomationsModel.Phase.Loading -> Note(Strings["automations_loading"])
            AutomationsModel.Phase.NotCreator -> Note(Strings["automations_not_creator"])
            AutomationsModel.Phase.Failed -> {
                Note(Strings["automations_load_failed"])
                TextButton(onClick = { scope.launch { model.load() } }) { Text(Strings["automations_retry"]) }
            }
            is AutomationsModel.Phase.Loaded -> Loaded(p.automations, pending) { rule, on ->
                model.clearToggleError()
                scope.launch { model.setEnabled(on, rule.id) }
            }
        }
    }
}

@Composable
private fun Loaded(a: AppAutomations, pending: Set<String>, onToggle: (AutomationRule, Boolean) -> Unit) {
    a.stopped?.let { Note(Strings["automations_stopped"], error = true) }
    a.notifyPaused?.let { Note(Strings["automations_notify_paused"]) }
    if (a.rules.isEmpty()) Note(Strings["automations_empty"])
    a.rules.forEachIndexed { i, rule ->
        if (i > 0) HorizontalDivider(Modifier.padding(horizontal = 16.dp))
        RuleRow(rule, isPending = rule.id in pending, onToggle = { onToggle(rule, it) })
    }
    a.quota?.let { q ->
        HorizontalDivider(Modifier.padding(vertical = 8.dp))
        Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
            Text(Strings["automations_quota"], modifier = Modifier.weight(1f))
            Text("${q.used} / ${q.limit}")
        }
        Note(Strings["automations_quota_footer"])
    }
}

@Composable
private fun RuleRow(rule: AutomationRule, isPending: Boolean, onToggle: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(rule.label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        val enabled = rule.enabled
        if (enabled != null && AutomationsModel.canToggle(rule)) {
            val state = Strings[if (enabled) "automations_on" else "automations_off"]
            val label = Strings.format("automations_toggle_label", rule.label)
            Spacer(Modifier.width(12.dp))
            Text(state, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.width(8.dp))
            Switch(
                checked = enabled,
                onCheckedChange = onToggle,
                enabled = !isPending,
                modifier = Modifier.semantics {
                    contentDescription = label
                    stateDescription = state
                },
            )
        }
    }
}

@Composable
private fun Note(text: String, error: Boolean = false) {
    Text(
        text,
        style = MaterialTheme.typography.bodyMedium,
        color = if (error) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}
