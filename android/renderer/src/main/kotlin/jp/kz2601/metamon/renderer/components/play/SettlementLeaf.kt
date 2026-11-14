package jp.kz2601.metamon.renderer.components.play

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.Settlement
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ValueLabels
import jp.kz2601.metamon.renderer.components.lists.rememberValueLabels
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard

/**
 * `settlement`(#101): balanceField(正 = 受け取る / 負 = 払う。多くは computed)から「A → B ¥1,200」を貪欲法で並べる。
 * 全員 0 なら「精算は不要です」。iOS の SettlementLeaf。
 */
@Composable
fun SettlementLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val all by rememberRecords(ctx.runtime)
    val entityId = c.entity.orEmpty()
    val hasMembers = LeafData(ctx.runtime, all).visibleRecords(entityId).isNotEmpty()
    val transfers = settlementTransfers(c, ctx.runtime, all, rememberValueLabels())
    val colors = RenderTheme.colors
    Column(modifier.renderCard(padding = 0.dp)) {
        Row(
            Modifier
                .padding(horizontal = RenderTheme.Spacing.md, vertical = 12.dp)
                .semantics { heading() },
            horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            SymbolIcon("arrow.left.arrow.right", 20.dp, colors.brandA)
            Text(c.title ?: stringResource(R.string.play_settlement_title), style = MaterialTheme.typography.titleMedium)
        }
        HorizontalDivider(color = colors.hairline)
        when {
            !hasMembers -> PlayNote("person.3", stringResource(R.string.play_settlement_empty))
            transfers.isEmpty() -> PlayNote("checkmark.circle", stringResource(R.string.play_settlement_none))
            else -> transfers.forEachIndexed { i, t ->
                if (i > 0) HorizontalDivider(Modifier.padding(start = RenderTheme.Spacing.md), color = colors.hairline)
                TransferRow(t)
            }
        }
    }
}

@Composable
private fun TransferRow(t: Settlement.Transfer) {
    val amount = FieldCoding.currency(t.amount)
    val spoken = stringResource(R.string.play_settlement_transfer, t.from, t.to, amount)
    Row(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = RenderTheme.Spacing.md, vertical = 12.dp)
            .clearAndSetSemantics { contentDescription = spoken },
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(t.from, maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodyLarge)
        SymbolIcon("arrow.right", 14.dp, MaterialTheme.colorScheme.onSurfaceVariant)
        Text(t.to, Modifier.weight(1f, fill = false), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodyLarge)
        Spacer(Modifier.weight(1f))
        Text(amount, maxLines = 1, style = MaterialTheme.typography.bodyLarge.copy(fontWeight = FontWeight.SemiBold))
    }
}

/** 空・完了の一言(アイコン + 小さめの文字)。 */
@Composable
internal fun PlayNote(icon: String, text: String, modifier: Modifier = Modifier) {
    Row(
        modifier.padding(RenderTheme.Spacing.md),
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SymbolIcon(icon, 18.dp, MaterialTheme.colorScheme.onSurfaceVariant)
        Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/** 精算の送金(誰が誰にいくら)。名前は nameField(無ければ entity の見出し・"name")、残高は balanceField(既定 "balance")。 */
fun settlementTransfers(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>, labels: ValueLabels = ValueLabels.JAPANESE): List<Settlement.Transfer> {
    val entityId = c.entity.orEmpty()
    val entity = runtime.entityModel(entityId)
    val nameField = c.nameField ?: entity?.displayTitleField ?: "name"
    val balanceField = c.balanceField ?: "balance"
    val balances = LeafData(runtime, all).visibleRecords(entityId).mapNotNull { r ->
        val b = r[balanceField].doubleValue ?: return@mapNotNull null
        LeafData(runtime, all, labels = labels).display(r[nameField], entity?.field(nameField)) to b
    }
    return Settlement.transfers(balances)
}
