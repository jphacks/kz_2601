package jp.kz2601.metamon.desktop.usage

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.background
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings

/** 設定の「使っている量」(#2814・Android の `UsageSection`・`RemainingTodaySection`)。読むだけ・Pro の案内は出さない。 */
@Composable
fun UsageDialog(loader: UsageLoader, onDismiss: () -> Unit) {
    LaunchedEffect(loader) { loader.refresh() }
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.widthIn(max = 480.dp),
        title = { Text(Strings["settings_usage_header"], modifier = Modifier.semantics { heading() }) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                when (val s = loader.state) {
                    UsageLoader.State.Loading -> UsageSkeleton()
                    is UsageLoader.State.Loaded -> s.summary?.let { UsageRows(it) }
                    is UsageLoader.State.Failed -> {
                        s.last?.let { UsageRows(it) }
                        Text(
                            Strings["desktop_usage_failed"],
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                        )
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(Strings["desktop_close"]) } },
    )
}

@Composable
private fun UsageRows(s: UsageText.Summary) {
    s.remaining?.let { text ->
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(Strings["voice_remaining_settings_title"], modifier = Modifier.weight(1f))
            if (s.pro) Text("Pro", color = MaterialTheme.colorScheme.primary)
        }
        Text(text, color = if (s.exhausted) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
    }
    s.apps?.let { m ->
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(m.title, modifier = Modifier.weight(1f))
            Text(m.amount, color = if (m.near) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
    s.footer?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
}

/** 読み込み中(`LoadingSkeleton` と同じ見た目の小さい版)。読み上げは「読み込み中」。 */
@Composable
private fun UsageSkeleton() {
    val label = Strings["desktop_loading"]
    Column(
        Modifier.fillMaxWidth().clearAndSetSemantics { contentDescription = label; liveRegion = LiveRegionMode.Polite },
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        repeat(2) { Box(Modifier.fillMaxWidth().height(24.dp).background(MaterialTheme.colorScheme.surfaceVariant, MaterialTheme.shapes.small)) }
    }
}
