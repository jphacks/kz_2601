package jp.kz2601.metamon.screenmap

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.hasTabs
import jp.kz2601.metamon.renderer.navigation.MapPoint
import jp.kz2601.metamon.renderer.navigation.Presentation
import jp.kz2601.metamon.renderer.navigation.ScreenGraph
import jp.kz2601.metamon.renderer.navigation.ScreenMapLayout

/**
 * 画面マップ(#897・iOS の `ScreenMapSheet`)。今のアプリの画面をカード、遷移を矢印で描く。
 * 段は [ScreenGraph](根 = タブを上段、BFS の深さで下へ)。push は実線、sheet / fullScreen は破線、到達できない画面は薄く最後の段に。
 * カードを選ぶと出入りの一覧と「この画面を開く」が出る。
 * @param onOpen 選んだ画面を開く(呼ぶ側がナビゲーションを変えてシートを閉じる)
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ScreenMapSheet(dsl: AppDSL, onDismiss: () -> Unit, onOpen: (String) -> Unit) {
    val graph = remember(dsl) { ScreenGraph(dsl) }
    var selected by rememberSaveable { mutableStateOf<String?>(null) }
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = Modifier.testTag("screenMap.sheet"),
    ) {
        Column(Modifier.padding(bottom = 16.dp)) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp), verticalAlignment = Alignment.Top) {
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.settings_screen_map), style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
                    Text(
                        stringResource(R.string.screen_map_subtitle, dsl.app.name),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.settings_close)) }
            }
            Legend(Modifier.padding(horizontal = 16.dp, vertical = 8.dp))
            BoxWithConstraints(Modifier.fillMaxWidth().weight(1f, fill = false).heightIn(min = 160.dp)) {
                MapCanvas(graph, hasTabs = dsl.app.hasTabs, selected = selected, availableWidth = maxWidth) {
                    selected = if (selected == it) null else it
                }
            }
            val node = selected?.let(graph::node)
            if (node != null) {
                Detail(node, graph, onOpen = { onOpen(node.id) })
            } else {
                Text(
                    stringResource(R.string.screen_map_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                )
            }
        }
    }
}

@Composable
private fun Legend(modifier: Modifier) {
    Row(modifier, horizontalArrangement = Arrangement.spacedBy(16.dp), verticalAlignment = Alignment.CenterVertically) {
        LegendItem(dashed = false, text = stringResource(R.string.screen_map_legend_push))
        LegendItem(dashed = true, text = stringResource(R.string.screen_map_legend_sheet))
    }
}

@Composable
private fun LegendItem(dashed: Boolean, text: String) {
    val color = MaterialTheme.colorScheme.onSurfaceVariant
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        Canvas(Modifier.size(22.dp, 10.dp)) {
            drawLine(
                color,
                Offset(0f, size.height / 2),
                Offset(size.width, size.height / 2),
                strokeWidth = 2.dp.toPx(),
                pathEffect = if (dashed) PathEffect.dashPathEffect(floatArrayOf(4.dp.toPx(), 3.dp.toPx())) else null,
            )
        }
        Text(text, style = MaterialTheme.typography.labelSmall, color = color)
    }
}

/** カードと矢印の図本体。並べ方と矢印の形は [ScreenMapLayout](iOS / Mac と同じ式)、矢印は Canvas でカードの後ろに描く。 */
@Composable
private fun MapCanvas(graph: ScreenGraph, hasTabs: Boolean, selected: String?, availableWidth: Dp, onSelect: (String) -> Unit) {
    val layout = remember(graph, availableWidth) { ScreenMapLayout(graph, availableWidth.value) }
    val accent = MaterialTheme.colorScheme.primary
    val muted = MaterialTheme.colorScheme.onSurfaceVariant
    val density = LocalDensity.current
    Box(Modifier.verticalScroll(rememberScrollState()).horizontalScroll(rememberScrollState())) {
        Box(Modifier.size(layout.contentWidth.dp, layout.contentHeight.dp)) {
            Canvas(Modifier.size(layout.contentWidth.dp, layout.contentHeight.dp)) {
                val px = density.density
                fun MapPoint.o() = Offset(x * px, y * px)
                for (arrow in layout.arrows(selected)) {
                    val color = (if (arrow.active) accent else muted).copy(alpha = if (arrow.faded) 0.25f else 0.9f)
                    val p = arrow.path
                    val line = Path().apply {
                        moveTo(p.start.x * px, p.start.y * px)
                        val c = p.control
                        if (c != null) quadraticTo(c.x * px, c.y * px, p.end.x * px, p.end.y * px) else lineTo(p.end.x * px, p.end.y * px)
                    }
                    val width = if (arrow.active && selected != null) 2.4f else 1.6f
                    drawPath(
                        line,
                        color,
                        style = Stroke(
                            width = width * px,
                            cap = StrokeCap.Round,
                            pathEffect = if (arrow.link.isModal) PathEffect.dashPathEffect(floatArrayOf(6 * px, 4 * px)) else null,
                        ),
                    )
                    val head = Path().apply {
                        val (a, b, c) = p.head.map { it.o() }
                        moveTo(a.x, a.y); lineTo(b.x, b.y); lineTo(c.x, c.y); close()
                    }
                    drawPath(head, color)
                }
            }
            if (layout.hasUnreachable) {
                Text(
                    stringResource(R.string.screen_map_unreachable_header),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.offset(layout.inset.dp, (layout.rowTop(graph.rows.size - 1) - layout.unreachableLabel).dp),
                )
            }
            val rootLabel = stringResource(if (hasTabs) R.string.screen_map_badge_tab else R.string.screen_map_badge_initial)
            for (node in graph.nodes) {
                val f = layout.frame(node)
                ScreenCard(
                    node,
                    rootLabel,
                    isSelected = selected == node.id,
                    modifier = Modifier.offset(f.x.dp, f.y.dp).size(f.width.dp, f.height.dp),
                ) { onSelect(node.id) }
            }
        }
    }
}

/** 画面 1 枚分のカード。タブの根・フォームはバッジで区別、到達できない画面は薄く。 */
@Composable
private fun ScreenCard(node: ScreenGraph.Node, rootLabel: String, isSelected: Boolean, modifier: Modifier, onClick: () -> Unit) {
    val badge = when {
        node.isRoot -> rootLabel
        node.isForm -> stringResource(R.string.screen_map_badge_form)
        else -> stringResource(R.string.screen_map_badge_screen)
    }
    val hint = stringResource(R.string.screen_map_card_hint)
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(12.dp),
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        border = BorderStroke(
            if (isSelected) 2.5.dp else 1.dp,
            if (isSelected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outlineVariant,
        ),
        shadowElevation = if (isSelected) 6.dp else 1.dp,
        modifier = modifier
            .alpha(if (node.reachable) 1f else 0.5f)
            .semantics { onClick(label = hint) { onClick(); true } }
            .testTag("screenMap.node.${node.id}"),
    ) {
        Column(Modifier.padding(horizontal = 10.dp, vertical = 8.dp), verticalArrangement = Arrangement.Center) {
            Text(
                badge,
                style = MaterialTheme.typography.labelSmall,
                color = if (node.reachable) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
            )
            Text(node.title, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.SemiBold, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
    }
}

@Composable
private fun Detail(node: ScreenGraph.Node, graph: ScreenGraph, onOpen: () -> Unit) {
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp).testTag("screenMap.detail"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(node.title, style = MaterialTheme.typography.titleMedium)
                Text(node.id, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
                Spacer(Modifier.weight(1f))
                if (!node.reachable) {
                    Text(
                        stringResource(R.string.screen_map_unreachable),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
            LinkList(stringResource(R.string.screen_map_outgoing), graph.outgoing(node.id), graph, outgoing = true)
            LinkList(stringResource(R.string.screen_map_incoming), graph.incoming(node.id), graph, outgoing = false)
            Button(onClick = onOpen, modifier = Modifier.fillMaxWidth().testTag("screenMap.open")) {
                Text(stringResource(R.string.screen_map_open))
            }
        }
    }
}

@Composable
private fun LinkList(label: String, links: List<ScreenGraph.Link>, graph: ScreenGraph, outgoing: Boolean) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.outline)
        if (links.isEmpty()) {
            Text(stringResource(R.string.screen_map_none), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        val separator = stringResource(R.string.screen_map_separator)
        for (link in links) {
            val other = graph.node(if (outgoing) link.to else link.from)?.title.orEmpty()
            val how = link.edges.map { describe(it) }.joinToString(separator)
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(if (outgoing) "→" else "←", color = MaterialTheme.colorScheme.primary, fontWeight = FontWeight.Bold)
                Text(other, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold)
                Text(how, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2)
            }
        }
    }
}

/** 遷移 1 本を言葉にする(iOS の `ScreenMapCopy.describe`)。 */
@Composable
private fun describe(e: ScreenGraph.Edge): String {
    val how = when (val t = e.trigger) {
        is ScreenGraph.Trigger.Button -> stringResource(R.string.screen_map_trigger_button, t.label)
        is ScreenGraph.Trigger.Toolbar -> stringResource(R.string.screen_map_trigger_toolbar, t.label)
        ScreenGraph.Trigger.ItemTap -> stringResource(R.string.screen_map_trigger_item_tap)
        ScreenGraph.Trigger.DateTap -> stringResource(R.string.screen_map_trigger_date_tap)
        ScreenGraph.Trigger.AfterSave -> stringResource(R.string.screen_map_trigger_after_save)
        ScreenGraph.Trigger.Other -> stringResource(R.string.screen_map_trigger_other)
    }
    return when (e.presentation) {
        Presentation.PUSH -> how
        Presentation.SHEET -> how + stringResource(R.string.screen_map_suffix_sheet)
        Presentation.FULL_SCREEN -> how + stringResource(R.string.screen_map_suffix_full)
    }
}
