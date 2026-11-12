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
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
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
import jp.kz2601.metamon.desktop.store.SessionPersistence

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
fun DesktopApp(
    dsl: AppDSL,
    initialStore: MockStore = MockStore(),
    startScreen: String? = null,
    onOpenApp: ((String) -> Unit)? = null,
    persistence: SessionPersistence? = null,
    /** 複数のウィンドウ(#1763): 同じアプリのウィンドウと共有する記録・詳細を開く記録・別のウィンドウを開く口。 */
    records: SharedRecords? = null,
    startRecordId: String? = null,
    onOpenRecord: ((screen: String, recordId: String) -> Unit)? = null,
    onCloseWindow: (() -> Unit)? = null,
    isActive: () -> Boolean = { true },
    /** このウィンドウの番号(トレイの「すぐ記録」の行き先と比べる)。null は 1 枚だけ。 */
    windowId: Int? = null,
    /** パレットの「この端末のデータを消す」(#1887)。全アプリの分を消す。null なら出さない。 */
    onWipeLocalData: (suspend () -> Unit)? = null,
) {
    val session = remember {
        DesktopSession(dsl, initialStore, startScreen = startScreen, host = DesktopActionHost(), persistence = persistence, shared = records, startRecordId = startRecordId)
    }
    session.onCloseWindow = onCloseWindow
    TrayHook(session, windowId, isActive)
    // 他のウィンドウで端末のデータを消した: 下書きを捨てて最初の画面へ(詳細のウィンドウは閉じる)
    val wipes = session.records.wipes
    LaunchedEffect(wipes) { session.followWipe() }
    // 他のウィンドウで開いている記録が消えた: 下書きを捨て、詳細のウィンドウは閉じる
    val shown = session.store
    LaunchedEffect(shown) { session.followRemoval() }
    var pending by remember { mutableStateOf<Pair<Action, ActionContext>?>(null) }
    val nav = session.nav
    val dispatch = { a: Action, ctx: ActionContext -> if (a.confirmMessage != null) pending = a to ctx else session.dispatch(a, ctx) }
    val io = rememberDesktopIo(dsl, { session.store }, session::replaceStore)
    // 落とした物・Ctrl+V は一番上に見えている画面へ
    val topComponents = { dsl.screen(session.top.screen)?.components.orEmpty() }
    // 書けるフォームが無い(読み取りだけ・フォームの無い画面)なら写真・動画は入れない
    val topDraft = { session.form(session.top)?.takeIf { !it.isReadOnly }?.let(::FormDraftStrings) }
    // 印刷(#1767): Ctrl+P・見出しの「印刷」で一番上の画面をプレビュー
    var printing by remember { mutableStateOf<PrintDocument?>(null) }
    val openPrint = { printing = ScreenPrint.document(dsl, session.top.screen, session.store, session.form(session.top), locale = Strings.locale) }
    // キーボードだけで使う(#1765): Ctrl+K パレット・Ctrl+N/S/F・Delete・Esc・F1
    val keyboard = rememberDesktopKeyboard(dsl, session, dispatch, openPrint, onOpenApp, onOpenRecord, onWipeLocalData)

    // Ctrl+F はこのウィンドウの検索欄にだけ入る
    // 写真のプレビュー(PhotoPreview.kt): 写真の部品を押すとこのウィンドウのダイアログで開く。開いている間は貼り付け・印刷を取らない
    val previewing = keyboard.photos.isOpen
    val pasteKey = io.previewKey(topComponents, topDraft)
    CompositionLocalProvider(LocalSearchTargets provides keyboard.searchTargets, LocalPhotoPreview provides keyboard::openPhoto) {
    Surface(Modifier.fillMaxSize().ioDropTarget(io, topComponents, topDraft).onPreviewKeyEvent { !keyboard.photos.isOpen && pasteKey(it) }
        .onPreviewKeyEvent { if (!keyboard.photos.isOpen && DesktopPrint.isPrintKey(it) && DesktopPrint.canPrint(dsl)) true.also { openPrint() } else false }.then(keyboard.modifier)) {
        IoOverlay(io)
        Box(Modifier.fillMaxSize()) {
            // モーダルの間は下の画面とタブに Tab で入らず、読み上げにも出さない
            Column(Modifier.fillMaxSize().then(keyboard.inert(nav.sheet != null || nav.fullScreen != null || previewing))) {
                val base = NavigationResolver.dismissModals(nav).top(dsl)
                Box(Modifier.weight(1f)) { ScreenView(dsl, session, base, io, dispatch, openPrint, onOpenRecord) }
                // 送り待ちがある時だけ小さく出す(#1887)
                session.persistence?.pending?.takeIf { it > 0 }?.let { n ->
                    Text(
                        Strings.format("desktop_sync_pending", n),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp)
                            .semantics { liveRegion = LiveRegionMode.Polite },
                    )
                }
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
                Box(Modifier.fillMaxSize().then(keyboard.inert(previewing)), contentAlignment = Alignment.Center) {
                    // 背景の暗幕は中身と兄弟にする(clickable で包むと中身ごと 1 つにまとまって読み上げで中が読めない)
                    val closeLabel = Strings["desktop_close"]
                    Box(
                        Modifier.matchParentSize().background(Color(0x66000000))
                            .clickable(enabled = !full, role = Role.Button) { session.back(top) }
                            .semantics { contentDescription = closeLabel },
                    )
                    val m = if (full) Modifier.fillMaxSize() else Modifier.widthIn(max = 640.dp).fillMaxWidth(0.92f).fillMaxSize(0.9f)
                    val pane = dsl.screen(top.screen)?.title?.ifEmpty { null } ?: dsl.app.name
                    Surface(m.then(keyboard.modalModifier).semantics { paneTitle = pane; isTraversalGroup = true }, shape = MaterialTheme.shapes.large, tonalElevation = 6.dp) {
                        ScreenView(dsl, session, top, io, dispatch, openPrint, onOpenRecord)
                    }
                }
            }
            keyboard.Overlay()
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

/** Markdown のリンク: 見える文字と行き先が違い得るので、行き先を見せて確かめてから openUrl で開く。 */
internal fun markdownLinkAction(url: String): Action = Action(
    type = jp.kz2601.metamon.dsl.ActionType.OPEN_URL,
    url = url,
    confirmMessage = Strings.format("desktop_markdown_link_confirm", url),
)

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
private fun ScreenView(
    dsl: AppDSL,
    session: DesktopSession,
    route: Route,
    io: DesktopIo,
    dispatch: (Action, ActionContext) -> Unit,
    print: () -> Unit,
    openRecord: ((String, String) -> Unit)?,
) {
    val screen = dsl.screen(route.screen) ?: dsl.screens.firstOrNull() ?: return
    val form = session.form(route)
    val data = remember(session.store) { session.leafData(DataLabels.current()) }
    val perform = { a: Action, recordId: String? -> dispatch(a, ActionContext(route, form, recordId)) }
    val dataEnv = DataEnv(
        data, session.queries(route), perform,
        { e, id, patch -> session.updateRecord(e, id, patch) },
        { e, id -> dispatch(ScreenShortcuts.withConfirm(Action(type = jp.kz2601.metamon.dsl.ActionType.DELETE_RECORD, entity = e)), ActionContext(route, form, id)) },
        { e, selected -> ListIoBar(e, io, selected) },
        canEdit = session::canEditRecord,
        canDelete = session::canDeleteRecord,
        // 行のダブルクリック・右クリックで詳細を別のウィンドウに(#1763)。Ctrl+クリックは行を選ぶ(#1766)
        copyRows = { e, ids -> io.copyRows(e, ids) },
        copyText = io::copyText,
        dragRows = io::rowsTransferable,
        openWindow = openRecord?.let { open -> { a: Action, id: String -> RecordWindows.target(a)?.let { open(it, id) } } },
        marks = session.persistence?.marks,
    )
    val scope = ScreenScope(route, form, session, perform, io, data, dataEnv)
    Column(Modifier.fillMaxSize()) {
        Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically) {
            if (session.canClose(route)) {
                val close = route.stack == Route.SHEET || route.stack == Route.FULL || route == session.detailRoot
                TextButton(onClick = { session.back(route) }) { Text(Strings[if (close) "desktop_close" else "renderer_back"]) }
            }
            Text(screen.title.ifEmpty { dsl.app.name }, style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f).semantics { heading() })
            screen.toolbar.forEach { b ->
                if (isButtonVisible(b.action, form)) TextButton(onClick = { perform(b.action, null) }) { Text(b.label) }
            }
            if (route == session.top) TextButton(onClick = print) { Text(Strings["desktop_print"]) }
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
                    PlacedGrid(placement.placedInReadingOrder, cellW, cellH, placement.lastRow, s)
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
                val rows = FormBandSizing.minRows(c)
                Box(if (rows > 0) m.heightIn(min = cellH * rows) else m) { Node(c, s) }
            }
        }
    }
}

/** フォームの段で部品が取る最小の行数(0 は中身の高さ)。 */
object FormBandSizing {
    /** 表示の部品は 2 行ぶん。入力の rating(field あり)は入力と同じ高さ、icon は 1 文字ぶんの絵なので中身の高さ。 */
    fun minRows(c: Component): Int =
        if (c.type in DISPLAY_LEAVES && c.type != ComponentType.ICON && !InputLeaves.isInput(c)) 2 else 0
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
    // 隠す部品(作者以外の hidden の項目・出さない削除 / 保存ボタン)は枠ごと描かない
    if (s.session.state(c, s.form) == "hidden") return
    Box(Modifier.leafSemantics(c, s.data)) { NodeContent(c, s) }
}

@Composable
private fun NodeContent(c: Component, s: ScreenScope) {
    when (c.type) {
        ComponentType.HSTACK -> Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            // 隠す子は先に外す(weight の枠だけ残って幅が空かないように)
            c.children.filter { s.session.state(it, s.form) != "hidden" }.forEach { Box1(Modifier.weight(1f)) { Node(it, s) } }
        }
        ComponentType.CARD -> Card(Modifier.fillMaxWidth()) { LocalGridOrChildren(c, s, Modifier.padding(12.dp)) }
        in DesktopLeaves.containers -> LocalGridOrChildren(c, s)
        ComponentType.TEXT -> Text(c.text.orEmpty())
        ComponentType.MARKDOWN -> MarkdownLeaf(c.text.orEmpty()) { url -> s.perform(markdownLinkAction(url), null) }
        ComponentType.LABEL -> Text(c.label ?: c.text.orEmpty(), style = MaterialTheme.typography.labelLarge)
        ComponentType.BADGE -> BadgeLeaf(c)
        ComponentType.IMAGE -> ImageLeaf(c, c.field?.let { s.form?.values?.get(it)?.stringValue })
        ComponentType.ICON -> IconLeaf(c)
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
        ComponentType.BUTTON -> Button(onClick = { c.action?.let { s.perform(it, null) } }) { Text(c.label ?: c.title ?: c.text.orEmpty()) }
        ComponentType.LINK -> TextButton(onClick = { c.url?.let { s.perform(jp.kz2601.metamon.dsl.Action(type = jp.kz2601.metamon.dsl.ActionType.OPEN_URL, url = it), null) } }) {
            Text(c.text ?: c.label ?: c.url.orEmpty())
        }
        ComponentType.OPEN_URL_BUTTON -> OpenUrlButtonLeaf(c, s.form) { urls -> s.session.openFirst(urls, ActionContext(s.route, s.form)) }
        in DATA_LEAVES -> DataLeaf(c, s.dataEnv)
        ComponentType.FILE_IMPORT -> FileImportLeaf(c, s.io)
        in PlayLeaves.types -> PlayLeaf(c, s.session, s.route, s.form, s.data, s.perform)
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

/** 入力の部品: フォームの外を先に決めてから描く(隠すのは [Node] で済んでいる)。 */
@Composable
private fun InputNode(c: Component, s: ScreenScope) {
    when (s.session.state(c, s.form)) {
        "needsForm" -> return Text(Strings["desktop_input_needs_form"], color = Color.Gray, style = MaterialTheme.typography.bodySmall)
    }
    InputLeaf(c, s.form!!) { s.session.message = it }
}
