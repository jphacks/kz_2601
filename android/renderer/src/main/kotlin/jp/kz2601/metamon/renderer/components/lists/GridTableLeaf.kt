package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.GridTableLayout
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlin.math.floor
import kotlin.math.max

private val ROW_HEADER_W = 30.dp
private val MIN_COLUMN_W = 48.dp
private val CELL_H = 58.dp
private val GAP = 4.dp

/**
 * `gridTable`(#234): 時間割のマス目。横 = columnField(曜日など)、縦 = rowField(時限など)。iOS の `GridTableLeaf`。
 * 配置は :dsl の [GridTableLayout]。マスのタップ = `onItemTap`: 記録 1 件 → 編集、空 → 列・行の値を `FormPrefill` に積んで新規、
 * 複数 → メニューで選ぶ。列が画面幅に収まらない時だけ横スクロール。
 */
@Composable
fun GridTableLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val entity = data.entity(c.entity)
    val layout = GridTableLayout(c, entity, data.filtered(c))
    val today = GridTableLayout.todayColumn(layout.columns.map { it.label }, DSLClock.now())
    Column(modifier.renderCard(padding = RenderTheme.Spacing.row), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        c.title?.takeIf { it.isNotEmpty() }?.let {
            Text(it, Modifier.semantics { heading() }, style = RenderTheme.Fonts.sectionTitle, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        BoxWithConstraints {
            val count = layout.columns.size
            val free = maxWidth - ROW_HEADER_W - GAP * (count + 1) - 8.dp
            val columnW: Dp = if (count == 0) MIN_COLUMN_W else max(MIN_COLUMN_W.value, floor(free.value / count)).dp
            Column(Modifier.horizontalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(GAP)) {
                Row(horizontalArrangement = Arrangement.spacedBy(GAP)) {
                    Box(Modifier.width(ROW_HEADER_W))
                    layout.columns.forEachIndexed { i, col -> ColumnHeader(col.label, i == today, Modifier.width(columnW)) }
                }
                layout.rows.forEachIndexed { r, row ->
                    Row(horizontalArrangement = Arrangement.spacedBy(GAP), verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            row.label,
                            Modifier.width(ROW_HEADER_W),
                            style = RenderTheme.Fonts.tableHeader.copy(fontFeatureSettings = "tnum"),
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            textAlign = TextAlign.Center,
                            maxLines = 2,
                        )
                        layout.columns.forEachIndexed { ci, col ->
                            GridCell(c, ctx, layout, entity, data, GridTableLayout.Cell(ci, r), "${col.label} ${row.label}", ci == today, Modifier.size(columnW, CELL_H))
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ColumnHeader(label: String, isToday: Boolean, modifier: Modifier) {
    val today = stringResource(R.string.lists_today, label)
    val colors = RenderTheme.colors
    Text(
        label,
        modifier
            .background(if (isToday) Brush.linearGradient(listOf(colors.brandFillStart, colors.brandFillEnd)) else Brush.linearGradient(listOf(Color.Transparent, Color.Transparent)), CircleShape)
            .padding(vertical = 4.dp)
            .clearAndSetSemantics { contentDescription = if (isToday) today else label },
        style = RenderTheme.Fonts.tableHeader.copy(fontWeight = if (isToday) FontWeight.Bold else FontWeight.SemiBold),
        color = if (isToday) Color.White else MaterialTheme.colorScheme.onSurfaceVariant,
        textAlign = TextAlign.Center,
        maxLines = 1,
    )
}

@Composable
private fun GridCell(
    c: Component,
    ctx: RenderContext,
    layout: GridTableLayout,
    entity: Entity?,
    data: LeafData,
    cell: GridTableLayout.Cell,
    place: String,
    isToday: Boolean,
    modifier: Modifier,
) {
    val rows = layout.records(cell)
    val tap = c.onItemTap
    var menu by remember { mutableStateOf(false) }
    fun edit(r: Record) = tap?.let { ctx.dispatch(it, ActionContext(ctx.route, ctx.form, recordId = r.id)) }
    fun add() {
        val action = tap ?: return
        FormPrefill.set(layout.prefill(cell))
        ctx.dispatch(action, ActionContext(ctx.route, ctx.form))
    }
    val titles = rows.map { gridTitle(c, it, entity, data) }
    val emptyLabel = stringResource(R.string.lists_cell_empty)
    val addLabel = stringResource(R.string.lists_add)
    val editLabel = stringResource(R.string.lists_edit)
    val shape = RoundedCornerShape(RenderTheme.Radius.inner)
    Box(
        modifier
            .clickable(enabled = tap != null) {
                when {
                    rows.size > 1 -> menu = true
                    rows.size == 1 -> edit(rows[0])
                    else -> add()
                }
            }
            .clearAndSetSemantics {
                contentDescription = place
                stateDescription = if (rows.isEmpty()) emptyLabel else titles.joinToString("、")
                testTag = "gridTable.cell.${cell.column}.${cell.row}"
                if (tap != null) onClick(if (rows.isEmpty()) addLabel else editLabel) { if (rows.size > 1) menu = true else if (rows.size == 1) edit(rows[0]) else add(); true }
            },
    ) {
        val colors = RenderTheme.colors
        if (rows.isEmpty()) {
            Box(
                Modifier.fillMaxSize()
                    .background((if (isToday) colors.brandSoft else colors.canvas).copy(alpha = 0.6f), shape)
                    .border(1.dp, colors.hairline, shape),
            )
        } else {
            val tint = colors.category(titles[0])
            Column(
                Modifier.fillMaxSize()
                    .background(tint.copy(alpha = 0.18f), shape)
                    .border(if (isToday) 1.5.dp else 1.dp, if (isToday) colors.brandA else tint.copy(alpha = 0.45f), shape)
                    .padding(horizontal = 5.dp, vertical = 4.dp),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                Text(titles[0], style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold), maxLines = 2, overflow = TextOverflow.Ellipsis)
                gridSubtitle(c, rows[0], entity, data)?.let {
                    Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                }
            }
            if (rows.size > 1) {
                Text(
                    "+${rows.size - 1}",
                    Modifier.align(Alignment.BottomEnd).padding(3.dp).background(colors.brandA, CircleShape).padding(horizontal = 4.dp, vertical = 1.dp),
                    style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
                    color = Color.White,
                )
            }
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            rows.forEachIndexed { i, r ->
                DropdownMenuItem(text = { Text(titles[i]) }, onClick = { menu = false; edit(r) })
            }
            DropdownMenuItem(text = { Text(addLabel) }, onClick = { menu = false; add() })
        }
    }
}

private fun gridTitle(c: Component, r: Record, entity: Entity?, data: LeafData): String {
    val f = c.titleField ?: entity?.displayTitleField
    return data.text(r, f, entity).ifEmpty { "(無題)" }
}

private fun gridSubtitle(c: Component, r: Record, entity: Entity?, data: LeafData): String? {
    val f = c.subtitleField?.takeIf { it.isNotEmpty() } ?: return null
    val s = data.text(r, f, entity)
    return if (s.isEmpty() || s == FieldCoding.UNAVAILABLE) null else s
}
