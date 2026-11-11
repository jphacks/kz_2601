package jp.kz2601.metamon.renderer

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.screen.ScreenToolbar
import jp.kz2601.metamon.dsl.fallback
import jp.kz2601.metamon.dsl.hasTabs
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.components.UnavailableComponent
import jp.kz2601.metamon.renderer.components.dashcam.LocalDashcamAppName
import jp.kz2601.metamon.renderer.components.social.SocialDialogs
import jp.kz2601.metamon.renderer.components.social.SocialPromptHost
import jp.kz2601.metamon.renderer.components.lists.LocalScreenLocals
import jp.kz2601.metamon.renderer.components.lists.ScreenLocals
import jp.kz2601.metamon.renderer.form.rememberScreenForm
import jp.kz2601.metamon.renderer.layout.imePaddingExcludingBelow
import jp.kz2601.metamon.renderer.layout.isVoiceScreen
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Presentation
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.screen.GridScreenLayout
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.RenderTheme
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalHapticFeedback
import jp.kz2601.metamon.dsl.ToolbarButton
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.renderer.form.FormSaveResult
import jp.kz2601.metamon.renderer.screen.SaveBarFeedback
import jp.kz2601.metamon.renderer.screen.SaveBarPhase
import kotlinx.coroutines.launch
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.SheetValue
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.platform.LocalContext
import jp.kz2601.metamon.renderer.form.DiscardGuard
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.RequiredFieldFocus
import kotlinx.coroutines.delay

/** 今の画面のフォームを控える先(DSLView が入れる)。null なら破棄の確認をしない(プレビュー単体の ScreenView)。 */
val LocalDiscardGuard = compositionLocalOf<DiscardGuard?> { null }

/**
 * DSL のアプリ全体を描く(iOS の `DSLNavigationRoot`)。タブ(2 個以上)・main のスタック・シート・全画面を持ち、
 * navigate / back は描画器の中で解決する(`NavigationResolver`)。それ以外のアクションは [DSLRuntime.onAction] へ。
 */
@Composable
fun DSLView(dsl: AppDSL, runtime: DSLRuntime, modifier: Modifier = Modifier) {
    // 遷移の状態はランタイムが持つ(AppRuntime)。無ければ描画器が自前で持つ(プレビュー)
    val host = runtime.navigationHost
    var local by remember(dsl) { mutableStateOf(NavigationState.initial(dsl)) }
    val hosted = host?.navigation?.collectAsState()
    val nav = hosted?.value ?: local
    fun setNav(value: NavigationState) {
        if (host != null) host.setNavigation(value) else local = value
    }
    // M10: 保存していない入力のある画面を離れる前の確認(戻る・シートを閉じる・タブの切り替え)
    val discard = remember(dsl) { DiscardGuard() }

    fun perform(action: Action, at: ActionContext) {
        if (host != null) return runtime.onAction(action, at)
        val from = at.route
        when (action.type) {
            ActionType.NAVIGATE -> action.screen?.let {
                setNav(NavigationResolver.navigate(nav, it, Presentation.of(action.presentation?.wire), at.recordId, from, dsl))
            }
            ActionType.BACK -> setNav(NavigationResolver.close(nav, from))
            else -> runtime.onAction(action, at)
        }
    }

    @Composable
    fun stack(stack: String, root: Route) {
        val top = nav.path(stack).lastOrNull() ?: root
        val canClose = NavigationResolver.canClose(nav, top)
        fun close() = discard.request(stack) { setNav(NavigationResolver.close(nav, top)) }
        BackHandler(enabled = canClose) { close() } // 予測型の戻るも、払い終えた時にここへ来る
        key(top.id) {
            RouteView(dsl, top, RenderContext(top, runtime) { a, at -> perform(a, at) }, canClose) { close() }
        }
    }

    CompositionLocalProvider(LocalDiscardGuard provides discard) {
        Column(modifier.fillMaxSize().background(RenderTheme.colors.canvas)) {
            Box(Modifier.weight(1f)) {
                if (dsl.app.hasTabs) {
                    val tab = nav.selectedTab
                    stack(Route.tabStack(tab), Route(tab, stack = Route.tabStack(tab)))
                } else {
                    stack(Route.MAIN, Route(dsl.initialScreen))
                }
            }
            if (dsl.app.hasTabs) {
                NavigationBar {
                    dsl.app.tabs.forEach { t ->
                        NavigationBarItem(
                            selected = nav.selectedTab == t.screen,
                            onClick = {
                                // 選び直したタブは根まで戻す。今の画面は消える(入力も消える)ので、入力があれば確かめる
                                discard.request(Route.tabStack(nav.selectedTab)) {
                                    setNav(nav.withPath(Route.tabStack(t.screen), emptyList()).copy(selectedTab = t.screen))
                                }
                            },
                            icon = {},
                            label = { Text(t.title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                        )
                    }
                }
            }
        }

        nav.sheet?.let { sheet ->
            // 下に引いて閉じた時はシートのスタックごと閉じる
            SheetHost(
                onDismiss = { setNav(nav.withPath(Route.SHEET, null).copy(sheet = null)) },
                guard = discard,
            ) { stack(Route.SHEET, sheet) }
        }
        nav.fullScreen?.let { full ->
            Dialog(
                onDismissRequest = {
                    discard.request(Route.FULL) { setNav(NavigationResolver.close(nav, nav.path(Route.FULL).lastOrNull() ?: full)) }
                },
                properties = DialogProperties(usePlatformDefaultWidth = false),
            ) {
                Box(Modifier.fillMaxSize().background(RenderTheme.colors.canvas)) { stack(Route.FULL, full) }
            }
        }
        // 通報・ブロック・18 歳以上の確認(#877)。窓なのでシート・全画面の上にも出る
        (runtime as? SocialPromptHost)?.let { SocialDialogs(it) }
        if (discard.pending != null) DiscardDialog(discard)
    }
}

/** 保存していない入力を破棄するかの確認(M10・iOS の `discardFormConfirmation` と同じ文言)。窓なのでシートの上にも出る。 */
@Composable
private fun DiscardDialog(guard: DiscardGuard) {
    AlertDialog(
        onDismissRequest = guard::keepEditing,
        title = { Text(stringResource(R.string.renderer_discard_title)) },
        text = { Text(stringResource(R.string.renderer_discard_message)) },
        confirmButton = {
            TextButton(onClick = guard::discard) {
                Text(stringResource(R.string.renderer_discard_confirm), color = MaterialTheme.colorScheme.error)
            }
        },
        dismissButton = {
            TextButton(onClick = guard::keepEditing) { Text(stringResource(R.string.renderer_discard_keep_editing)) }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SheetHost(onDismiss: () -> Unit, guard: DiscardGuard, content: @Composable () -> Unit) {
    val dismiss by rememberUpdatedState(onDismiss)
    // 入力があれば、下に払っても・戻るでも閉じずに確認を出す(iOS の interactiveDismissDisabled + 閉じるの確認)
    val state = rememberModalBottomSheetState(
        confirmValueChange = { value ->
            if (value != SheetValue.Hidden || !guard.needsConfirm(Route.SHEET)) return@rememberModalBottomSheetState true
            guard.request(Route.SHEET) { dismiss() }
            false
        },
    )
    ModalBottomSheet(onDismissRequest = { dismiss() }, sheetState = state, containerColor = RenderTheme.colors.canvas) {
        Box(Modifier.fillMaxWidth().heightIn(min = 320.dp).fillMaxSize()) { content() }
    }
}

/** route の画面 1 枚。見出しの行(戻る・タイトル・toolbar)+ グリッド。 */
@Composable
private fun RouteView(dsl: AppDSL, route: Route, ctx: RenderContext, canClose: Boolean, onBack: () -> Unit) {
    val screen = dsl.screen(route.screen) ?: dsl.screens.firstOrNull()
    if (screen == null) {
        UnavailableComponent(jp.kz2601.metamon.dsl.Component(), Modifier.padding(16.dp))
        return
    }
    // dashcam の映像はアプリごとの置き場に分ける(#872)
    CompositionLocalProvider(LocalDashcamAppName provides dsl.app.name) {
        ScreenView(screen, ctx, canClose, onBack, dsl = dsl)
    }
}

/** 画面 1 枚(iOS の `ScreenView`)。`screen.entity` があれば下書きを作り、入力部品と保存バーに渡す。 */
@Composable
fun ScreenView(
    screen: Screen,
    screenCtx: RenderContext,
    canClose: Boolean = false,
    onBack: () -> Unit = {},
    modifier: Modifier = Modifier,
    dsl: AppDSL? = null,
) {
    val form = rememberScreenForm(screen, screenCtx.route, screenCtx.runtime)
    val ctx = remember(screenCtx, form, screen) { screenCtx.withForm(form).withScreen(screen) }
    val guard = LocalDiscardGuard.current
    if (guard != null && form != null) {
        val stack = screenCtx.route.stack
        DisposableEffect(guard, stack, form) {
            guard.track(stack, form)
            onDispose { guard.untrack(stack, form) }
        }
    }
    if (form != null) RequiredFieldAttention(screen, form, ctx)
    val toolbar = ScreenToolbar.header(screen, dsl, form)
    Column(modifier.fillMaxSize()) {
        val showsHeader = !screen.isVoiceScreen && (screen.title.isNotEmpty() || canClose || toolbar.isNotEmpty())
        if (showsHeader) {
            Row(
                Modifier.fillMaxWidth().padding(horizontal = RenderTheme.Spacing.xs, vertical = RenderTheme.Spacing.xs),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (canClose) TextButton(onClick = onBack) { Text(stringResource(R.string.renderer_back)) }
                Text(
                    screen.title,
                    style = MaterialTheme.typography.titleLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f).padding(horizontal = RenderTheme.Spacing.sm),
                )
                toolbar.forEach { tb -> TextButton(onClick = { ctx.perform(tb.action) }) { Text(tb.label) } }
            }
        }
        // searchBar の検索文字列は画面ごと(遷移で戻ると空に戻る)
        val locals = remember(screenCtx.route) { ScreenLocals() }
        CompositionLocalProvider(LocalScreenLocals provides locals) {
            GridScreenLayout(screen, ctx, Modifier.weight(1f))
        }
        when (val bar = ScreenToolbar.saveBar(screen, form)) {
            ScreenToolbar.SaveBar.ReadOnly -> Text(
                stringResource(R.string.renderer_form_read_only),
                style = MaterialTheme.typography.bodyMedium,
                color = RenderTheme.colors.secondaryLabel,
                textAlign = TextAlign.Center,
                modifier = Modifier.fillMaxWidth().padding(horizontal = RenderTheme.Spacing.md, vertical = RenderTheme.Spacing.md),
            )
            is ScreenToolbar.SaveBar.Save -> FormSaveBar(bar.button, ctx)
            null -> Unit
        }
    }
}

/**
 * 保存で弾かれたら、画面で最初の空の必須欄へスクロールしてフォーカスし、TalkBack に「○○ を入力してください」と読ませる
 * (M10・iOS の `RequiredFieldScroller`)。欄の側は `FormInput` が [FormDraft.attentionField] を見て動く。
 */
@Composable
private fun RequiredFieldAttention(screen: Screen, form: FormDraft, ctx: RenderContext) {
    val announce = rememberAnnouncer()
    val context = LocalContext.current
    val count = form.flagCount
    LaunchedEffect(form, count) {
        if (count == 0) return@LaunchedEffect
        form.attentionField = null // 同じ欄でも、押すたびにフォーカスを戻す
        val field = RequiredFieldFocus.firstFlagged(screen, form, ctx.runtime.currentUid()) ?: return@LaunchedEffect
        delay(RequiredFieldFocus.SETTLE_MILLIS)
        form.attentionField = field
        announce(context.getString(R.string.renderer_required_fill_in, form.entity.field(field)?.name ?: field))
    }
}

/**
 * 画面下の保存ボタン(iOS の `FormSaveBar.bar`)。押下中はスピナーで 2 回目を受けず、書けたら 0.6 秒だけ
 * 「保存しました」(オフラインなら「接続が戻ったら自動で送ります」)を出して戻す。TalkBack には知らせで 1 回だけ読ませる
 * (ボタンの読み上げは iOS と同じく元の名前のまま)。キーボードが出ている時はその上に出る。
 */
@Composable
private fun FormSaveBar(button: ToolbarButton, ctx: RenderContext) {
    val form = ctx.form
    val feedback = remember(form) { SaveBarFeedback() }
    val scope = rememberCoroutineScope()
    val focus = LocalFocusManager.current
    val announce = rememberAnnouncer()
    val haptics = LocalHapticFeedback.current
    val saved = stringResource(R.string.renderer_save_bar_saved)
    val queued = stringResource(R.string.renderer_save_bar_queued_offline)
    val lastResult = form?.lastSaveResult
    LaunchedEffect(feedback, lastResult) { feedback.onSaveResultChanged(lastResult) } // 送った後にサーバーが断った
    val phase = feedback.phase
    val done = (phase as? SaveBarPhase.Done)?.result
    LaunchedEffect(phase) {
        if (done == null) return@LaunchedEffect
        haptics.performHapticFeedback(HapticFeedbackType.Confirm)
        announce(if (done == FormSaveResult.QUEUED_OFFLINE) queued else saved)
    }
    ActionButton(
        label = when (done) {
            FormSaveResult.SAVED -> saved
            FormSaveResult.QUEUED_OFFLINE -> queued
            null -> button.label
        },
        kind = ActionButtonKind.PRIMARY,
        onClick = {
            if (feedback.phase != SaveBarPhase.Idle) return@ActionButton
            focus.clearFocus() // 変換中の文字を確定させてから保存する(iOS #627)
            scope.launch { feedback.press(form) { ctx.runtime.runAction(button.action, ActionContext(ctx.route, form)) } }
        },
        icon = when (done) {
            FormSaveResult.SAVED -> "checkmark.circle"
            FormSaveResult.QUEUED_OFFLINE -> "wifi.slash"
            null -> button.icon ?: "checkmark"
        },
        running = phase == SaveBarPhase.Saving,
        modifier = Modifier
            .imePaddingExcludingBelow() // 下のタブバー・帯の分は引く(imePadding だと二重になる)
            .padding(horizontal = RenderTheme.Spacing.md, vertical = RenderTheme.Spacing.sm)
            .semantics { contentDescription = button.label },
    )
}

@Preview(showBackground = true)
@Composable
private fun DSLViewPreview() {
    MetamonRenderTheme {
        DSLView(AppDSL.fallback, NoopDSLRuntime)
    }
}
