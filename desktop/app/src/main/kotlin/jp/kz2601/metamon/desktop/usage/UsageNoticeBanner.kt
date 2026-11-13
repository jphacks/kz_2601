package jp.kz2601.metamon.desktop.usage

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings

/** 上限に近い時の 1 行の帯。落ち着いた見た目で、閉じられる。Pro の案内は出さない。 */
@Composable
fun UsageNoticeBanner(notice: UsageNotice, onDismiss: () -> Unit, modifier: Modifier = Modifier) {
    Surface(color = MaterialTheme.colorScheme.surfaceVariant, modifier = modifier.fillMaxWidth()) {
        Row(Modifier.padding(start = 12.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(
                UsageNoticeText.text(notice),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f).semantics { liveRegion = LiveRegionMode.Polite },
            )
            TextButton(onClick = onDismiss) { Text(Strings["desktop_close"]) }
        }
    }
}

/**
 * [next] が選んだお知らせを 1 回だけ読み、閉じるまで出す。[next] が null・何も無ければ何も描かない。
 * [key] が変わったら読み直す(別のアプリに替わった時)。
 */
@Composable
fun UsageNoticeSlot(key: Any?, next: (suspend () -> UsageNotice?)?, modifier: Modifier = Modifier) {
    var notice by remember(key) { mutableStateOf<UsageNotice?>(null) }
    val read by rememberUpdatedState(next)
    LaunchedEffect(key) { notice = read?.invoke() }
    notice?.let { UsageNoticeBanner(it, onDismiss = { notice = null }, modifier) }
}
