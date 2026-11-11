package jp.kz2601.metamon.desktop

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshots.SnapshotStateMap
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Entity
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.desktop.io.DesktopIo
import jp.kz2601.metamon.desktop.io.FileImportLeaf
import jp.kz2601.metamon.desktop.io.IoOverlay
import jp.kz2601.metamon.desktop.io.ListIoBar
import jp.kz2601.metamon.desktop.io.MediaFieldLeaf
import jp.kz2601.metamon.desktop.io.ioDropTarget
import jp.kz2601.metamon.desktop.io.previewKey
import jp.kz2601.metamon.desktop.io.rememberDesktopIo
import androidx.compose.ui.input.key.onPreviewKeyEvent
import java.awt.Desktop
import java.net.URI

private data class NavEntry(val screenId: String, val recordId: String?, val key: Int)

/** 画面 1 枚の描画に要るもの。 */
private class ScreenScope(
    val entity: Entity?,
    val draft: SnapshotStateMap<String, String>,
    val store: MockStore,
    val perform: (Action, String?) -> Unit,
    val entityOf: (String?) -> Entity?,
    val io: DesktopIo,
    val data: LeafData,
    val dataEnv: DataEnv,
)

/** DSL を 1 本読み、スタックで画面を積んで描く。画面は 6 列のグリッド(フォームは段)に並べる。 */
@Composable
fun DesktopApp(dsl: AppDSL, initialStore: MockStore = MockStore(), startScreen: String? = null) {
    var store by remember { mutableStateOf(initialStore) }
    var stack by remember { mutableStateOf(listOf(NavEntry(startScreen ?: dsl.initialScreen, null, 0))) }
    var nextKey by remember { mutableStateOf(1) }
    var message by remember { mutableStateOf<String?>(null) }
    val io = rememberDesktopIo(dsl, { store }, { store = it })

    val top = stack.last()
    val screen = dsl.screen(top.screenId) ?: dsl.screens.firstOrNull() ?: return
    val entityOf = { id: String? -> dsl.entities.firstOrNull { it.id == id } }
    val entity = entityOf(screen.entity)
    val draft = remember(top.key) {
        mutableStateMapOf<String, String>().apply {
            val saved = if (entity != null && top.recordId != null) store.find(entity.id, top.recordId) else null
            putAll(saved?.data ?: entity?.defaultDraft().orEmpty())
        }
    }

    fun pop() {
        if (stack.size > 1) stack = stack.dropLast(1)
    }

    fun perform(action: Action, recordId: String?) {
        message = null
        when (action.type) {
            ActionType.NAVIGATE -> action.screen?.let {
                stack = stack + NavEntry(it, recordId, nextKey)
                nextKey++
            }
            ActionType.BACK -> pop()
            ActionType.SAVE_RECORD -> {
                val target = entityOf(action.entity ?: screen.entity)
                if (target == null) {
                    message = "保存先のフォームがありません"
                    return
                }
                val missing = target.missingRequired(draft)
                if (missing.isNotEmpty()) {
                    message = "入力してください: ${missing.joinToString("、")}"
                    return
                }
                store = store.save(target.id, top.recordId, draft.toMap())
                pop()
            }
            ActionType.DELETE_RECORD -> {
                val target = entityOf(action.entity ?: screen.entity)
                if (target != null && top.recordId != null) store = store.delete(target.id, top.recordId)
                pop()
            }
            ActionType.OPEN_URL -> action.url?.let(::openBrowser)
            ActionType.EXPORT -> io.export(action, screen.entity)
            else -> message = "デスクトップ版ではまだ使えない操作: ${action.rawType.ifEmpty { action.type.name }}"
        }
    }

    val leafData = remember(store) { LeafData.of(dsl, store) }
    val search = remember(top.key) { mutableStateMapOf<String, String>() }
    val dataEnv = DataEnv(leafData, search, ::perform, { e, id, patch -> store.find(e, id)?.let { store = store.save(e, id, it.data + patch) } }, { e, id -> store = store.delete(e, id) }, { ListIoBar(it, io) })
    val scope = ScreenScope(entity, draft, store, ::perform, entityOf, io, leafData, dataEnv)
    Surface(Modifier.fillMaxSize().ioDropTarget(io, { screen.components }, { draft }).onPreviewKeyEvent(io.previewKey({ screen.components }, { draft }))) {
        IoOverlay(io)
        Column(Modifier.fillMaxSize()) {
            Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically) {
                if (stack.size > 1) TextButton(onClick = ::pop) { Text("戻る") }
                Text(screen.title.ifEmpty { dsl.app.name }, style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
                screen.toolbar.forEach { b -> TextButton(onClick = { perform(b.action, top.recordId) }) { Text(b.label) } }
            }
            HorizontalDivider()
            message?.let { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(12.dp)) }
            io.notice?.let { Text(it, modifier = Modifier.padding(12.dp)) }
            GridScreen(screen, scope)
        }
    }
}

private val GAP = 8.dp
private val INSET = 16.dp

/**
 * 画面グリッド(iOS の GridScreenLayout・Android の screen/GridScreenLayout.kt)。
 * area のある部品は 6 列 × 行の矩形に絶対配置、area の無い部品はその下に全幅で積む。フォーム画面は段ごとに中身の高さで積む。
 * ウィンドウが広い時は幅を止めて中央に寄せ、低い時は画面ごと縦にスクロールする。
 */
@Composable
private fun GridScreen(screen: Screen, s: ScreenScope) {
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val m = DesktopGridMetrics.resolve(maxWidth.value, maxHeight.value, INSET.value)
        val cellW = m.cellW.dp
        val cellH = m.cellH.dp
        val placement = screen.gridPlacement
        Box(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), contentAlignment = Alignment.TopCenter) {
            Column(Modifier.width(m.frameWidth.dp).padding(INSET), verticalArrangement = Arrangement.spacedBy(GAP)) {
                if (screen.isFormLayout) {
                    FormBands(placement.placed, cellW, cellH, s)
                } else if (placement.placed.isNotEmpty()) {
                    PlacedGrid(placement.placed, cellW, cellH, placement.lastRow, s)
                }
                placement.stacked.forEach { Node(it, s) }
            }
        }
    }
}

/** 矩形に絶対配置。中身がはみ出す部品はセルの内側でスクロールする。 */
@Composable
private fun PlacedGrid(placed: List<Placed>, cellW: Dp, cellH: Dp, rows: Int, s: ScreenScope) {
    Box(Modifier.fillMaxWidth().height(cellH * rows)) {
        placed.forEach { (c, a) ->
            Box(
                Modifier
                    .offset(x = cellW * a.col1 + GAP / 2, y = cellH * (a.row1 - 1) + GAP / 2)
                    .size(width = cellW * a.colSpan - GAP, height = cellH * a.rowSpan - GAP)
                    .verticalScroll(rememberScrollState()),
            ) { Node(c, s) }
        }
    }
}

/** フォーム画面: 行が重なる部品を 1 段にまとめ、段ごとに上から積む。横位置・幅は area のまま。 */
@Composable
private fun FormBands(placed: List<Placed>, cellW: Dp, cellH: Dp, s: ScreenScope) {
    val bands = formBands(placed)
    bands.forEachIndexed { i, band ->
        val detached = band.isDetached(bands.getOrNull(i - 1))
        Box(Modifier.fillMaxWidth().padding(top = if (detached) 16.dp else 0.dp)) {
            band.items.forEach { (c, a) ->
                val m = Modifier.offset(x = cellW * a.col1 + GAP / 2).width(cellW * a.colSpan - GAP)
                Box(if (c.type in DISPLAY_LEAVES) m.heightIn(min = cellH * 2) else m) { Node(c, s) }
            }
        }
    }
}

/** Container の子。子が全員 area を持つ card / section / vstack はローカルグリッド(段で流す)。 */
@Composable
private fun LocalGridOrChildren(c: Component, s: ScreenScope, modifier: Modifier = Modifier) {
    val local = c.localGridPlacement
    if (local == null) {
        Children(c, s, modifier)
        return
    }
    BoxWithConstraints(modifier.fillMaxWidth()) {
        val cellW = maxWidth / GridArea.COLUMNS
        Column(verticalArrangement = Arrangement.spacedBy(GAP)) {
            c.title?.let { Text(it, style = MaterialTheme.typography.titleMedium) }
            FormBands(local, cellW, 40.dp, s)
        }
    }
}

@Composable
private fun Node(c: Component, s: ScreenScope) {
    when (c.type) {
        ComponentType.HSTACK -> Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            c.children.forEach { Box1(Modifier.weight(1f)) { Node(it, s) } }
        }
        ComponentType.CARD -> Card(Modifier.fillMaxWidth()) { LocalGridOrChildren(c, s, Modifier.padding(12.dp)) }
        in DesktopLeaves.containers -> LocalGridOrChildren(c, s)
        ComponentType.TEXT -> Text(c.text.orEmpty())
        ComponentType.MARKDOWN -> MarkdownLeaf(c.text.orEmpty())
        ComponentType.LABEL -> Text(c.label ?: c.text.orEmpty(), style = MaterialTheme.typography.labelLarge)
        ComponentType.BADGE -> BadgeLeaf(c)
        ComponentType.IMAGE -> ImageLeaf(c, c.field?.let { s.draft[it] })
        ComponentType.AVATAR -> AvatarLeaf(c, c.field?.let { s.draft[it] })
        ComponentType.STAT -> StatLeaf(c, s.data)
        ComponentType.CHART -> ChartLeaf(c, s.data)
        ComponentType.PROGRESS_BAR -> ProgressLeaf(c, s.data)
        ComponentType.GAUGE -> GaugeLeaf(c, s.data)
        ComponentType.CALENDAR -> CalendarLeaf(c, s.data, c.onDateTap?.let { a -> { _ -> s.perform(a, null) } })
        ComponentType.RATING -> {
            val field = c.field
            val value = field?.let { s.draft[it]?.toDoubleOrNull()?.toInt() } ?: (c.value as? jp.kz2601.metamon.dsl.ExprNumber)?.value?.toInt() ?: 0
            RatingLeaf(c, value, if (field != null) fieldLabel(c, s) else c.label, field?.let { f -> { n: Int -> s.draft[f] = n.toString() } })
        }
        ComponentType.EMPTY_STATE -> Text(c.title ?: c.message ?: c.text.orEmpty(), color = Color.Gray)
        ComponentType.SPACER -> Spacer(Modifier.height(8.dp))
        ComponentType.DIVIDER -> HorizontalDivider()
        ComponentType.BUTTON -> Button(onClick = { c.action?.let { s.perform(it, null) } }) { Text(c.label ?: c.title ?: c.text.orEmpty()) }
        ComponentType.LINK -> TextButton(onClick = { c.url?.let(::openBrowser) }) { Text(c.text ?: c.label ?: c.url.orEmpty()) }
        in DATA_LEAVES -> DataLeaf(c, s.dataEnv)
        ComponentType.FILE_IMPORT -> FileImportLeaf(c, s.io)
        ComponentType.PHOTO_FIELD, ComponentType.VIDEO_FIELD -> MediaFieldLeaf(c, fieldLabel(c, s), s.draft)
        ComponentType.TEXT_FIELD, ComponentType.NUMBER_FIELD, ComponentType.TEXT_EDITOR, ComponentType.SECURE_FIELD -> {
            val field = c.field ?: return
            OutlinedTextField(
                value = s.draft[field].orEmpty(),
                onValueChange = { s.draft[field] = it },
                label = { Text(fieldLabel(c, s)) },
                placeholder = c.placeholder?.let { p -> { Text(p) } },
                singleLine = c.type != ComponentType.TEXT_EDITOR,
                visualTransformation = if (c.type == ComponentType.SECURE_FIELD) PasswordVisualTransformation() else VisualTransformation.None,
                modifier = Modifier.fillMaxWidth(),
            )
        }
        ComponentType.TOGGLE -> {
            val field = c.field ?: return
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(fieldLabel(c, s), Modifier.weight(1f))
                Switch(checked = s.draft[field] == "true", onCheckedChange = { s.draft[field] = it.toString() })
            }
        }
        ComponentType.PICKER -> {
            val field = c.field ?: return
            val options = s.entity?.fields?.firstOrNull { it.id == field }?.options.orEmpty()
            Text(fieldLabel(c, s), style = MaterialTheme.typography.labelLarge)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                options.forEach { o -> FilterChip(selected = s.draft[field] == o, onClick = { s.draft[field] = o }, label = { Text(o) }) }
            }
        }
        ComponentType.STEPPER -> {
            val field = c.field ?: return
            val n = s.draft[field]?.toDoubleOrNull()?.toInt() ?: 0
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(fieldLabel(c, s), Modifier.weight(1f))
                TextButton(onClick = { s.draft[field] = (n - 1).toString() }) { Text("−") }
                Text(n.toString())
                TextButton(onClick = { s.draft[field] = (n + 1).toString() }) { Text("+") }
            }
        }
        else -> Text(
            if (c.type == ComponentType.UNKNOWN) "知らない部品: ${DesktopLeaves.typeName(c)}" else "デスクトップ版ではまだ使えない部品: ${DesktopLeaves.typeName(c)}",
            color = Color.Gray,
            style = MaterialTheme.typography.bodySmall,
        )
    }
}

@Composable
private fun Children(c: Component, s: ScreenScope, modifier: Modifier = Modifier) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        c.title?.let { Text(it, style = MaterialTheme.typography.titleMedium) }
        c.children.forEach { Node(it, s) }
    }
}

@Composable
private fun Box1(modifier: Modifier, content: @Composable () -> Unit) {
    Column(modifier) { content() }
}

private fun fieldLabel(c: Component, s: ScreenScope): String =
    c.label ?: s.entity?.fields?.firstOrNull { it.id == c.field }?.name ?: c.field.orEmpty()

private fun openBrowser(url: String) {
    runCatching { if (Desktop.isDesktopSupported()) Desktop.getDesktop().browse(URI(url)) }
}
