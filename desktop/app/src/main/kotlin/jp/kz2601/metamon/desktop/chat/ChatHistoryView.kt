package jp.kz2601.metamon.desktop.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings

/** 履歴の文(Android の VoiceScreen と同じ)。 */
object ChatHistoryText {
    /** 「ここで反映しました(v3)・要約」 */
    fun applied(entry: ChatHistoryEntry): String {
        val head = entry.version?.let { Strings.format("voice_history_applied", it) } ?: Strings["voice_history_applied_plain"]
        val text = entry.text.trim()
        return if (text.isEmpty()) head else "$head・$text"
    }
}

/** 閉じた会話の薄さ(今の会話と見分ける)。 */
private const val CLOSED_ALPHA = 0.55f

/**
 * 会話の画面の吹き出しの列: 閉じた会話と反映の区切りを薄く出し、今の会話をその後ろに続ける。
 * 会話の画面(#2162)ができたら、その列の中に置く。
 */
@Composable
fun ChatHistoryBubbles(state: ChatHistoryState, modifier: Modifier = Modifier) {
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        state.history.forEach { ChatEntryRow(it, Modifier.alpha(CLOSED_ALPHA)) }
        state.live.forEach { ChatEntryRow(it) }
    }
}

@Composable
private fun ChatEntryRow(entry: ChatHistoryEntry, modifier: Modifier = Modifier) {
    when (entry.kind) {
        ChatHistoryEntry.Kind.APPLIED -> Row(modifier.fillMaxWidth().padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            HorizontalDivider(Modifier.weight(1f))
            Text(
                ChatHistoryText.applied(entry),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 8.dp),
            )
            HorizontalDivider(Modifier.weight(1f))
        }
        else -> {
            val mine = entry.kind == ChatHistoryEntry.Kind.USER
            Box(modifier.fillMaxWidth(), contentAlignment = if (mine) Alignment.CenterEnd else Alignment.CenterStart) {
                Surface(
                    color = if (mine) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceVariant,
                    shape = MaterialTheme.shapes.medium,
                    modifier = Modifier.widthIn(max = 480.dp),
                ) {
                    Text(entry.text, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
                }
            }
        }
    }
}

/** 「最初から」のボタン。押すと [ChatHistoryState.requestRestart]。 */
@Composable
fun RestartChatButton(state: ChatHistoryState, enabled: Boolean = true) {
    TextButton(onClick = state::requestRestart, enabled = enabled) { Text(Strings["voice_restart"]) }
}

/** これまでの会話がある時の「最初から」の確認(Android の VoiceScreen と同じ 3 択)。 */
@Composable
fun RestartChatDialog(state: ChatHistoryState) {
    if (!state.restartConfirmOpen) return
    AlertDialog(
        onDismissRequest = state::dismissRestart,
        title = { Text(Strings["voice_restart_confirm_title"]) },
        text = { Text(Strings["voice_restart_confirm_body"]) },
        confirmButton = {
            TextButton(onClick = state::restartAll) {
                Text(Strings["voice_restart_clear_all"], color = MaterialTheme.colorScheme.error)
            }
        },
        dismissButton = {
            Column(horizontalAlignment = Alignment.End) {
                TextButton(onClick = state::restartLive) { Text(Strings["voice_restart_live_only"]) }
                TextButton(onClick = state::dismissRestart) { Text(Strings["inputs_cancel"]) }
            }
        },
    )
}
