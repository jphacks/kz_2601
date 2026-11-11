package jp.kz2601.metamon.feedback

import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.sync.HotReloadStatus

/** ホットリロードのつながり方の文言(#1108)。購読していない時は出さない。 */
object HotReloadStatusCopy {
    data class Copy(@StringRes val label: Int, @StringRes val description: Int, val live: Boolean)

    fun of(status: HotReloadStatus): Copy? = when (status) {
        HotReloadStatus.OFF -> null
        HotReloadStatus.REALTIME -> Copy(R.string.hot_reload_realtime, R.string.hot_reload_realtime_a11y, live = true)
        HotReloadStatus.POLLING -> Copy(R.string.hot_reload_polling, R.string.hot_reload_polling_a11y, live = false)
    }
}

/** 「リアルタイム / ポーリング中」の小さな表示(下部の開発用の行)。TalkBack は説明つきの 1 項目として読む。 */
@Composable
fun HotReloadStatusBadge(status: HotReloadStatus, modifier: Modifier = Modifier) {
    val copy = HotReloadStatusCopy.of(status) ?: return
    val description = stringResource(copy.description)
    val dot = if (copy.live) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline
    Row(
        modifier.padding(horizontal = 8.dp).clearAndSetSemantics { contentDescription = description },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Box(Modifier.size(6.dp).background(dot, CircleShape))
        Text(stringResource(copy.label), style = MaterialTheme.typography.labelSmall)
    }
}
