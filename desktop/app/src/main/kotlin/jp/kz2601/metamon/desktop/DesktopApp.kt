package jp.kz2601.metamon.desktop

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.hasTabs
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.renderer.navigation.top
import jp.kz2601.metamon.desktop.io.DesktopIo
import jp.kz2601.metamon.desktop.io.FileImportLeaf
import jp.kz2601.metamon.desktop.io.IoOverlay
import jp.kz2601.metamon.desktop.io.ListIoBar
import jp.kz2601.metamon.desktop.io.ioDropTarget
import jp.kz2601.metamon.desktop.io.previewKey
import jp.kz2601.metamon.desktop.io.rememberDesktopIo
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.isTraversalGroup
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.paneTitle
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import jp.kz2601.metamon.desktop.i18n.Strings

/** 画面 1 枚の描画に要るもの。 */
private class ScreenScope(
    val route: Route,
    val form: FormDraft?,
    val session: DesktopSession,
    val perform: (Action, String?) -> Unit,
    val io: DesktopIo,
    val data: LeafData,
    val dataEnv: DataEnv,
)

/**
 * DSL を 1 本読み、[DesktopSession] の遷移で画面を描く。画面は 6 列のグリッド(フォームは段)に並べる。
 * push はウィンドウの中で積み、sheet はウィンドウ内のダイアログ、fullScreen はウィンドウ全体に重ねる。タブは下に並べる。
 */
@Composable
fun DesktopApp(dsl: AppDSL, initialStore: MockStore = MockStore(), startScreen: String? = null) {
    val session = remember { DesktopSession(dsl, initialStore, startScreen = startScreen, host = DesktopActionHost()) }
    var pending by remember { mutableStateOf<Pair<Action, ActionContext>?>(null) }
    val nav = session.nav
    val dispatch = { a: Action, ctx: ActionContext -> if (a.confirmMessage != null) pending = a to ctx else session.dispatch(a, ctx) }
    val io = rememberDesktopIo(dsl, { session.store }, session::replaceStore)
    // 落とした物・Ctrl+V は一番上に見えている画面へ
    val topComponents = { dsl.screen(session.top.screen)?.components.orEmpty() }
    val topDraft = { session.form(session.top)?.let(::FormDraftStrings) ?: mutableMapOf() }
    // 印刷(#1767): Ctrl+P・見出しの「印刷」で一番上の画面をプレビュー
    var printing by remember { mutableStateOf<PrintDocument?>(null) }
    val openPrint = { printing = ScreenPrint.document(dsl, session.top.screen, session.store, session.form(session.top)) }

    Surface(Modifier.fillMaxSize().ioDropTarget(io, topComponents, topDraft).onPreviewKeyEvent(io.previewKey(topComponents, topDraft))
        .onPreviewKeyEvent { if (DesktopPrint.isPrintKey(it)) true.also { openPrint() } else false }) {
        IoOverlay(io)
        Box(Modifier.fillMaxSize()) {
            Column(Modifier.fillMaxSize()) {
                val base = NavigationResolver.dismissModals(nav).top(dsl)
                Box(Modifier.weight(1f)) { ScreenView(dsl, session, base, io, dispatch, openPrint) }
                if (dsl.app.hasTabs) {
                    HorizontalDivider()
                    // 読み上げでは「タブ・選択中」と読む
                    Row(Modifier.fillMaxWidth().selectableGroup(), horizontalArrangement = Arrangement.SpaceEvenly) {
                        dsl.app.tabs.forEach { t ->
                            val selected = nav.selectedTab == t.screen
                            Box(
                                Modifier.selectable(selected, role = Role.Tab, onClick = { session.selectTab(t.screen) }).padding(horizontal = 12.dp, vertical = 10.dp),
                                contentAlignment = Alignment.Center,
                            ) {
                                Text(t.title, color = MaterialTheme.colorScheme.primary, fontWeight = if (selected) androidx.compose.ui.text.font.FontWeight.Bold else null)
                            }
                        }
                    }
                }
            }
            if (nav.sheet != null || nav.fullScreen != null) {
                val top = nav.top(dsl)
                val full = nav.fullScreen != null
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    // 背景の暗幕は中身と兄弟にする(clickable で包むと中身ごと 1 つにまとまって読み上げで中が読めない)
                    val closeLabel = Strings["desktop_close"]
                    Box(
                        Modifier.matchParentSize().background(Color(0x66000000))
                            .clickable(enabled = !full, role = Role.Button) { session.back(top) }
                            .semantics { contentDescription = closeLabel },
                    )
                    val m = if (full) Modifier.fillMaxSize() else Modifier.widthIn(max = 640.dp).fillMaxWidth(0.92f).fillMaxSize(0.9f)
                    val pane = dsl.screen(top.screen)?.title?.ifEmpty { null } ?: dsl.app.name
                    Surface(m.semantics { paneTitle = pane; isTraversalGroup = true }, shape = MaterialTheme.shapes.large, tonalElevation = 6.dp) {
                        ScreenView(dsl, session, top, io, dispatch, openPrint)
                    }
                }
            }
        }
    }

    printing?.let { doc -> PrintPreview(doc, onDismiss = { printing = null }, onResult = { io.notice = it }) }

    pending?.let { (a, ctx) ->
        AlertDialog(
            onDismissRequest = { pending = null },
            text = { Text(a.confirmMessage.orEmpty()) },
            confirmButton = { TextButton(onClick = { pending = null; session.dispatch(a, ctx) }) { Text(confirmLabel(a)) } },
            dismissButton = { TextButton(onClick = { pending = null }) { Text(Strings["inputs_cancel"]) } },
        )
    }
}

/** 確認ダイアログの主ボタン(iOS と同じ語)。 */
internal fun confirmLabel(a: Action): String = Strings[
    when (a.type) {
        jp.kz2601.metamon.dsl.ActionType.DELETE_RECORD -> "runtime_confirm_delete"
        jp.kz2601.metamon.dsl.ActionType.SAVE_RECORD -> "runtime_confirm_save"
        jp.kz2601.metamon.dsl.ActionType.EXPORT -> "runtime_confirm_export"
        jp.kz2601.metamon.dsl.ActionType.OPEN_URL -> "runtime_confirm_open"
        jp.kz2601.metamon.dsl.ActionType.RESET_FORM -> "runtime_confirm_reset"
        else -> "runtime_confirm_continue"
    },
]


/** route の画面 1 枚(見出し・戻る・toolbar・一言・グリッド)。 */
@Composable
private fun ScreenView(dsl: AppDSL, session: DesktopSession, route: Route, io: DesktopIo, dispatch: (Action, ActionContext) -> Unit, print: () -> Unit) {
    val screen = dsl.screen(route.screen) ?: dsl.screens.firstOrNull() ?: return
    val form = session.form(route)
    val data = remember(session.store) { session.leafData() }
    val perform = { a: Action, recordId: String? -> dispatch(a, ActionContext(route, form, recordId)) }
    val dataEnv = DataEnv(
        data, session.queries(route), perform,
        { e, id, patch -> session.store.find(e, id)?.let { session.replaceStore(session.store.save(e, id, it.data + patch)) } },
        { e, id -> dispatch(Action(type = jp.kz2601.metamon.dsl.ActionType.DELETE_RECORD, entity = e), ActionContext(route, form, id)) },
        { ListIoBar(it, io) },
    )
    val scope = ScreenScope(route, form, session, perform, io, data, dataEnv)
    Column(Modifier.fillMaxSize()) {
        Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically) {
            if (NavigationResolver.canClose(session.nav, route)) {
                TextButton(onClick = { session.back(route) }) { Text(Strings[if (route.stack == Route.SHEET || route.stack == Route.FULL) "desktop_close" else "renderer_back"]) }
            }
            Text(screen.title.ifEmpty { dsl.app.name }, style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f).semantics { heading() })
            screen.toolbar.forEach { b ->
                if (isButtonVisible(b.action, form)) TextButton(onClick = { perform(b.action, null) }) { Text(b.label) }
            }
            if (route == session.top) TextButton(onClick = print) { Text("印刷") }
        }
        HorizontalDivider()
        if (route == session.top) {
            session.message?.let { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(12.dp).semantics { liveRegion = LiveRegionMode.Polite }) }
            io.notice?.let { Text(it, modifier = Modifier.padding(12.dp).semantics { liveRegion = LiveRegionMode.Polite }) }
        }
        if (form?.isReadOnly == true) Text(Strings["renderer_form_read_only"], color = Color.Gray, modifier = Modifier.padding(horizontal = 12.dp))
        GridScreen(screen, scope)
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

/** 部品 1 つ。絵で描く部品は読み上げ用に 1 つにまとめる(A11y.kt)。 */
@Composable
private fun Node(c: Component, s: ScreenScope) {
    Box(Modifier.leafSemantics(c, s.data)) { NodeContent(c, s) }
}

@Composable
private fun NodeContent(c: Component, s: ScreenScope) {
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
        ComponentType.IMAGE -> ImageLeaf(c, c.field?.let { s.form?.values?.get(it)?.stringValue })
        ComponentType.AVATAR -> AvatarLeaf(c, c.field?.let { s.form?.values?.get(it)?.stringValue })
        ComponentType.STAT -> StatLeaf(c, s.data)
        ComponentType.CHART -> ChartLeaf(c, s.data)
        ComponentType.PROGRESS_BAR -> ProgressLeaf(c, s.data)
        ComponentType.GAUGE -> GaugeLeaf(c, s.data)
        ComponentType.CALENDAR -> CalendarLeaf(c, s.data, c.onDateTap?.let { a -> { _ -> s.perform(a, null) } })
        ComponentType.RATING -> if (c.field != null && s.form != null) {
            InputNode(c, s)
        } else {
            RatingLeaf(c, (c.value as? jp.kz2601.metamon.dsl.ExprNumber)?.value?.toInt() ?: 0, c.label, null)
        }
        ComponentType.EMPTY_STATE -> Text(c.title ?: c.message ?: c.text.orEmpty(), color = Color.Gray)
        ComponentType.SPACER -> Spacer(Modifier.height(8.dp))
        ComponentType.DIVIDER -> HorizontalDivider()
        ComponentType.BUTTON -> if (s.session.tapState(c, s.form) == "rendered") {
            Button(onClick = { c.action?.let { s.perform(it, null) } }) { Text(c.label ?: c.title ?: c.text.orEmpty()) }
        }
        ComponentType.LINK -> TextButton(onClick = { c.url?.let { s.perform(jp.kz2601.metamon.dsl.Action(type = jp.kz2601.metamon.dsl.ActionType.OPEN_URL, url = it), null) } }) {
            Text(c.text ?: c.label ?: c.url.orEmpty())
        }
        in DATA_LEAVES -> DataLeaf(c, s.dataEnv)
        ComponentType.FILE_IMPORT -> FileImportLeaf(c, s.io)
        in InputLeaves.types -> InputNode(c, s)
        else -> Text(
            Strings.format(if (c.type == ComponentType.UNKNOWN) "desktop_unknown_component" else "desktop_unsupported_component", DesktopLeaves.typeName(c)),
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

/** 入力の部品: 隠す・フォームの外を先に決めてから描く。 */
@Composable
private fun InputNode(c: Component, s: ScreenScope) {
    when (s.session.tapState(c, s.form)) {
        "hidden" -> return
        "needsForm" -> return Text(Strings["desktop_input_needs_form"], color = Color.Gray, style = MaterialTheme.typography.bodySmall)
    }
    InputLeaf(c, s.form!!) { s.session.message = it }
}
