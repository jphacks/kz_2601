package jp.kz2601.metamon.renderer.components.social

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.records.InterestState
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors

/** `list.item.interest`(iOS の InterestButton・#507): プロフィールの行の右端。送った後は「送りました」、つながったら「つながりました」。 */
@Composable
fun InterestButton(state: InterestState, busy: Boolean, onClick: () -> Unit) {
    if (state.tappable) {
        Button(
            onClick = onClick,
            enabled = !busy,
            contentPadding = PaddingValues(horizontal = 14.dp, vertical = 0.dp),
            modifier = Modifier.heightIn(min = 32.dp).testTag("interest.send"),
        ) { Text(interestTitle(state), style = MaterialTheme.typography.labelLarge.copy(fontWeight = FontWeight.SemiBold), maxLines = 1) }
    } else {
        val matched = state == InterestState.MATCHED
        val color = if (matched) RenderTheme.colors.brandA else MaterialTheme.colorScheme.onSurfaceVariant
        Row(
            Modifier.testTag("interest.state"),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            SymbolIcon(if (matched) "checkmark.circle" else "paperplane", 16.dp, color)
            Text(interestTitle(state), style = MaterialTheme.typography.labelLarge, color = color, maxLines = 1)
        }
    }
}

@Composable
fun interestTitle(state: InterestState): String = stringResource(
    when (state) {
        InterestState.NONE -> R.string.social_interest_send
        InterestState.INCOMING -> R.string.social_interest_accept
        InterestState.SENT -> R.string.social_interest_sent
        InterestState.MATCHED -> R.string.social_interest_matched
    },
)
