package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard

/**
 * `table`: 見出し行 + 行×列(filter / sort / limit は list と同じ経路)。iOS の `TableLeaf`。
 * `groupBy` + 集計列があれば 1 グループ = 1 行の集計表(#261・`TableGrouping`)。数の列は右寄せ。
 */
@Composable
fun TableLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val entity = data.entity(c.entity)
    val rows = TableCells.rows(c, data)
    val columns = c.columns
    Column(modifier.renderCard()) {
        Row(Modifier.fillMaxWidth().semantics { heading() }, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.row)) {
            columns.forEach { col ->
                Text(
                    TableCells.header(col, entity, data.labels),
                    Modifier.weight(1f),
                    style = RenderTheme.Fonts.tableHeader,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = if (TableCells.isNumericColumn(col, entity)) TextAlign.End else TextAlign.Start,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        HorizontalDivider(Modifier.padding(vertical = 6.dp), color = RenderTheme.colors.hairline)
        rows.forEach { row ->
            // 行は 1 要素で「見出し: 値」を続けて読む
            val spoken = columns.mapIndexed { i, col -> "${TableCells.header(col, entity, data.labels)}: ${row.texts.getOrElse(i) { "" }}" }.joinToString(", ")
            Row(
                Modifier.fillMaxWidth().padding(vertical = 5.dp).clearAndSetSemantics { contentDescription = spoken },
                horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.row),
            ) {
                columns.forEachIndexed { i, col ->
                    Text(
                        row.texts.getOrElse(i) { "" },
                        Modifier.weight(1f),
                        style = RenderTheme.Fonts.tableCell,
                        textAlign = if (TableCells.isNumericColumn(col, entity)) TextAlign.End else TextAlign.Start,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
        if (rows.isEmpty()) EmptyPlaceholder("tablecells", data.labels.empty)
    }
}

/** `timeline`: dateField の新しい順の縦の時系列(左に日付・レールとドット)。iOS の `TimelineLeaf`。 */
@Composable
fun TimelineLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val entity = data.entity(c.entity)
    val rows = TimelineRows.rows(c, data)
    if (rows.isEmpty()) {
        EmptyPlaceholder("clock", data.labels.empty, modifier)
        return
    }
    val colors = RenderTheme.colors
    Column(modifier.renderCard()) {
        rows.forEachIndexed { index, row ->
            val date = data.text(row, c.dateField, entity)
            val title = data.text(row, c.titleField, entity)
            val sub = c.subtitleField?.let { data.text(row, it, entity) }?.takeIf { it.isNotEmpty() }
            Row(
                Modifier.fillMaxWidth().height(IntrinsicSize.Min)
                    // 「10/24, 朝ラン, 5km」と 1 行で読む
                    .clearAndSetSemantics { contentDescription = listOfNotNull(date, title, sub).joinToString(", ") },
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Text(
                    date,
                    Modifier.width(84.dp),
                    style = MaterialTheme.typography.labelMedium.copy(fontFeatureSettings = "tnum"),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.End,
                )
                Column(Modifier.fillMaxHeight(), horizontalAlignment = Alignment.CenterHorizontally) {
                    Box(
                        Modifier.padding(top = 4.dp).size(10.dp)
                            .background(Brush.linearGradient(listOf(colors.brandFillStart, colors.brandFillEnd)), CircleShape),
                    )
                    if (index < rows.size - 1) Box(Modifier.width(2.dp).weight(1f).background(colors.hairline))
                }
                Column(Modifier.weight(1f).padding(bottom = 16.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(title, style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold))
                    sub?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                }
            }
        }
    }
}
