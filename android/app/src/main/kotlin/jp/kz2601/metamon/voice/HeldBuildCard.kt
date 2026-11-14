package jp.kz2601.metamon.voice

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R

/**
 * 作らずに止めた新規作成のカード(#1261・iOS の `HeldBuildCard` / `PaidFeatureCard`)。会話の下に出す。
 * - 無料枠切れ: 「続きから作る」
 * - 402: 「Pro にする」/「この機能なしで作る」
 */
@Composable
internal fun HeldBuildCard(held: HeldBuild, busy: Boolean, vm: VoiceViewModel) {
    OutlinedCard(Modifier.fillMaxWidth().padding(vertical = 4.dp).testTag("held.card")) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            val headline = when (held) {
                is HeldBuild.LimitReached -> stringResource(R.string.held_headline, held.dailyFreeLimit)
                is HeldBuild.ProRequired -> stringResource(
                    R.string.held_paid_headline,
                    held.features.map { it.title }.filter { it.isNotEmpty() }.joinToString("・")
                        .ifEmpty { stringResource(R.string.paywall_shared_data) },
                )
            }
            Row(verticalAlignment = Alignment.Top) {
                Text(
                    headline,
                    style = MaterialTheme.typography.titleSmall,
                    fontWeight = FontWeight.Bold,
                    modifier = Modifier.weight(1f).testTag("held.headline"),
                )
                val close = stringResource(if (held is HeldBuild.ProRequired) R.string.held_paid_cancel else R.string.held_close)
                IconButton(onClick = vm::dismissHeldBuild, modifier = Modifier.testTag("held.close")) {
                    Icon(Icons.Filled.Close, contentDescription = close)
                }
            }
            when (held) {
                is HeldBuild.LimitReached -> {
                    Text(
                        stringResource(R.string.held_body),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Button(onClick = vm::continueHeldBuild, enabled = !busy, modifier = Modifier.fillMaxWidth().testTag("held.continue")) {
                        Text(stringResource(R.string.held_continue))
                    }
                }
                is HeldBuild.ProRequired -> {
                    held.features.map { it.reason }.filter { it.isNotEmpty() }.forEach {
                        Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Button(onClick = vm::purchaseAndBuild, enabled = !busy, modifier = Modifier.fillMaxWidth().testTag("paid.purchase")) {
                        Text(stringResource(R.string.paywall_purchase))
                    }
                    OutlinedButton(
                        onClick = vm::buildWithoutPaidFeatures,
                        enabled = !busy,
                        modifier = Modifier.fillMaxWidth().testTag("paid.withoutFeature"),
                    ) { Text(stringResource(R.string.held_paid_without)) }
                }
            }
        }
    }
}
