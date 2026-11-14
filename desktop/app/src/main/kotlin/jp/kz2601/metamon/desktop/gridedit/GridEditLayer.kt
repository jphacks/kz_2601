package jp.kz2601.metamon.desktop.gridedit

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.paneTitle
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.DesktopGridMetrics
import jp.kz2601.metamon.desktop.GridArea
import jp.kz2601.metamon.desktop.i18n.Strings

private val GAP = 8.dp
private val INSET = 16.dp

/** 読み上げの名前(見出しが無ければ「部品」)と今の位置。 */
internal fun GridEditDraft.Item.spoken(): String {
    val (row, col) = positionLabel()
    return Strings.format("desktop_grid_edit_position", title ?: Strings["desktop_grid_edit_untitled"], row, col.toString())
}

/**
 * 並びを直す編集(#3207)。画面の上に重ね、部品を枠で出す。ドラッグ・矢印のキーで 1 マスずつ・Tab で次の部品。
 * Enter で [onSave]、Esc で [onCancel]。重なる所には置かない(iOS の GridEditLayer と同じ)。
 */
@Composable
fun GridEditLayer(initial: GridEditDraft, title: String, onSave: (GridEditDraft) -> Unit, onCancel: () -> Unit) {
    var session by remember(initial) { mutableStateOf(GridEditSession(initial)) }
    // 読み上げに渡す一言(選んだ部品と位置・動かせない理由)
    var spoken by remember { mutableStateOf(session.selectedItem?.spoken().orEmpty()) }
    val focus = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
    val announce = { s: GridEditSession -> spoken = s.selectedItem?.spoken().orEmpty() }
    val save = { if (session.draft.hasChanges) onSave(session.draft) else onCancel() }

    Surface(
        Modifier.fillMaxSize()
            .semantics { paneTitle = title }
            .onPreviewKeyEvent { e ->
                if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                val step = when (e.key) {
                    Key.DirectionLeft -> -1 to 0
                    Key.DirectionRight -> 1 to 0
                    Key.DirectionUp -> 0 to -1
                    Key.DirectionDown -> 0 to 1
                    else -> null
                }
                when {
                    step != null -> true.also {
                        val (next, result) = session.nudge(step.first, step.second)
                        session = next
                        spoken = if (result == GridEditSession.Nudge.BLOCKED) Strings["desktop_grid_edit_blocked"] else next.selectedItem?.spoken().orEmpty()
                    }
                    e.key == Key.Tab -> true.also { session = session.cycle(!e.isShiftPressed).also(announce) }
                    e.key == Key.Enter || e.key == Key.NumPadEnter -> true.also { save() }
                    e.key == Key.Escape -> true.also { onCancel() }
                    else -> false
                }
            }
            .focusRequester(focus)
            .focusable(),
    ) {
        Column(Modifier.fillMaxSize()) {
            Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(title, style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f).semantics { heading() })
                TextButton(onClick = onCancel) { Text(Strings["desktop_grid_edit_cancel"]) }
                TextButton(onClick = save) { Text(Strings["desktop_grid_edit_save"]) }
            }
            Text(
                Strings["desktop_grid_edit_hint"],
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 12.dp),
            )
            Text(
                spoken,
                style = MaterialTheme.typography.labelMedium,
                modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp).semantics { liveRegion = LiveRegionMode.Polite },
            )
            HorizontalDivider()
            EditGrid(session, onSelect = { id -> session = session.copy(selected = id).also(announce) }) { id, to ->
                val (next, ok) = session.drop(id, to)
                session = next
                spoken = if (ok) next.selectedItem?.spoken().orEmpty() else Strings["desktop_grid_edit_blocked"]
            }
        }
    }
}

@Composable
private fun EditGrid(session: GridEditSession, onSelect: (String) -> Unit, onDrop: (String, GridArea) -> Unit) {
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val m = DesktopGridMetrics.resolve(maxWidth.value, maxHeight.value, INSET.value)
        val cellW = m.cellW.dp
        val cellH = m.cellH.dp
        val density = LocalDensity.current
        val cellWpx = with(density) { cellW.toPx() }.toDouble()
        val cellHpx = with(density) { cellH.toPx() }.toDouble()
        val placed = session.draft.placed
        // 2 行ぶん余白を出して下へも動かせるように
        val rows = minOf(GridArea.MAX_ROWS, maxOf(GridArea.VISIBLE_ROWS, (placed.maxOfOrNull { it.area.row2 } ?: 0) + 2))
        var dragging by remember { mutableStateOf<Pair<String, Offset>?>(null) }
        Box(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), contentAlignment = Alignment.TopCenter) {
            Box(Modifier.width(m.frameWidth.dp).padding(INSET).height(cellH * rows)) {
                // マス目
                for (r in 0 until rows) for (c in 0 until GridArea.COLUMNS) {
                    Box(
                        Modifier.offset(x = cellW * c, y = cellH * r).size(cellW, cellH)
                            .border(0.5.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f)),
                    )
                }
                placed.forEach { item ->
                    val drag = dragging?.takeIf { it.first == item.componentId }?.second
                    val shown = drag?.let { GridEditMath.moved(item.area, it.x.toDouble(), it.y.toDouble(), cellWpx, cellHpx) } ?: item.area
                    val blocked = drag != null && session.draft.conflicts(item.componentId, shown).isNotEmpty()
                    val selected = session.selected == item.componentId
                    val label = item.spoken()
                    val color = when {
                        blocked -> MaterialTheme.colorScheme.error
                        selected -> MaterialTheme.colorScheme.primary
                        else -> MaterialTheme.colorScheme.outline
                    }
                    Box(
                        Modifier
                            .offset(x = cellW * shown.col1 + GAP / 2, y = cellH * (shown.row1 - 1) + GAP / 2)
                            .size(width = cellW * shown.colSpan - GAP, height = cellH * shown.rowSpan - GAP)
                            .background(MaterialTheme.colorScheme.secondaryContainer.copy(alpha = if (selected) 0.9f else 0.5f), MaterialTheme.shapes.small)
                            .border(if (selected) 2.dp else 1.dp, color, MaterialTheme.shapes.small)
                            .clickable { onSelect(item.componentId) }
                            .pointerInput(item.componentId, item.area, cellWpx, cellHpx) {
                                detectDragGestures(
                                    onDragStart = { onSelect(item.componentId); dragging = item.componentId to Offset.Zero },
                                    onDragEnd = {
                                        val d = dragging?.second ?: Offset.Zero
                                        dragging = null
                                        onDrop(item.componentId, GridEditMath.moved(item.area, d.x.toDouble(), d.y.toDouble(), cellWpx, cellHpx))
                                    },
                                    onDragCancel = { dragging = null },
                                ) { change, amount ->
                                    change.consume()
                                    dragging = dragging?.let { it.first to it.second + amount }
                                }
                            }
                            .semantics {
                                contentDescription = label
                                role = Role.Button
                                this.selected = selected
                            }
                            .padding(6.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        Text(item.title ?: item.type.wire.ifEmpty { Strings["desktop_grid_edit_untitled"] }, maxLines = 2, overflow = TextOverflow.Ellipsis, color = Color.Unspecified)
                    }
                }
            }
        }
    }
}
