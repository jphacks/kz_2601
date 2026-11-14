package jp.kz2601.metamon.renderer.navigation

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.hasTabs

// 画面遷移(Compose に依存しない)。iOS の Host/Route.swift・NavigationResolver.swift と同じ規則。

/** スタックの 1 要素。画面 id + 編集対象の recordId + どのスタック(main / tab:xxx / sheet / full)か。 */
data class Route(
    val screen: String,
    val recordId: String? = null,
    val stack: String = MAIN,
) {
    val id: String get() = "$stack/$screen/${recordId.orEmpty()}"

    companion object {
        const val MAIN = "main"
        const val SHEET = "sheet"
        const val FULL = "full"

        fun tabStack(screen: String): String = "tab:$screen"
    }
}

enum class Presentation(val wire: String) {
    PUSH("push"),
    SHEET("sheet"),
    FULL_SCREEN("fullScreen"),
    ;

    companion object {
        /** 無い・知らない値は push。 */
        fun of(wire: String?): Presentation = entries.firstOrNull { it.wire == wire } ?: PUSH
    }
}

/** ナビゲーションの状態(スタック名 → 根の上に push 済みの Route・選択中のタブ・モーダル)。 */
data class NavigationState(
    val paths: Map<String, List<Route>> = emptyMap(),
    val selectedTab: String = "",
    val sheet: Route? = null,
    val fullScreen: Route? = null,
) {
    fun path(stack: String): List<Route> = paths[stack].orEmpty()

    fun withPath(stack: String, routes: List<Route>?): NavigationState =
        copy(paths = if (routes == null) paths - stack else paths + (stack to routes))

    companion object {
        /** アプリを開いた直後(タブがあれば最初のタブ)。 */
        fun initial(dsl: AppDSL): NavigationState =
            NavigationState(selectedTab = if (dsl.app.hasTabs) dsl.initialTab else "")
    }
}

/** タブ表示時の初期タブ。initialScreen がタブの根ならそれ、違えば先頭タブ。タブ無しなら空文字。 */
val AppDSL.initialTab: String
    get() = app.tabs.firstOrNull { it.screen == initialScreen }?.screen ?: app.tabs.firstOrNull()?.screen ?: ""

/** 画面 [screen] を開いた時の Route(タブの根はタブのスタック・他は main)。 */
fun AppDSL.rootRouteFor(screen: String): Route =
    if (app.hasTabs && app.tabs.any { it.screen == screen }) Route(screen, stack = Route.tabStack(screen)) else Route(screen)

/** いま一番上に見えている画面(全画面 > シート > 選択中のタブ / main)。 */
fun NavigationState.top(dsl: AppDSL): Route {
    fullScreen?.let { return path(Route.FULL).lastOrNull() ?: it }
    sheet?.let { return path(Route.SHEET).lastOrNull() ?: it }
    val stack = if (dsl.app.hasTabs) Route.tabStack(selectedTab) else Route.MAIN
    return path(stack).lastOrNull() ?: if (dsl.app.hasTabs) Route(selectedTab, stack = stack) else Route(dsl.initialScreen)
}

/**
 * `navigate` の行き先を「今どこにいるか」で解決する純粋関数(iOS #113 と同じ)。
 * 1. 行き先が今のスタック(根を含む)にある → そこまで pop
 * 2. 行き先がタブの根 → そのタブへ切り替え、そのタブのスタックを根まで戻す
 * 3. それ以外 → 今のスタックへ push
 * モーダルの中では、まずモーダル内で 1 を見て、無ければ下の画面で 1〜2 を見る(当たればモーダルを閉じる)。
 */
object NavigationResolver {
    private val modalStacks = setOf(Route.SHEET, Route.FULL)

    fun navigate(
        state: NavigationState,
        to: String,
        presentation: Presentation,
        recordId: String?,
        from: Route,
        dsl: AppDSL,
    ): NavigationState {
        when (presentation) {
            Presentation.SHEET -> return state.withPath(Route.SHEET, null).copy(sheet = Route(to, recordId, Route.SHEET))
            Presentation.FULL_SCREEN -> return state.withPath(Route.FULL, null).copy(fullScreen = Route(to, recordId, Route.FULL))
            Presentation.PUSH -> Unit
        }

        if (from.stack in modalStacks) {
            val modalRoot = if (from.stack == Route.SHEET) state.sheet else state.fullScreen
            popTo(to, recordId, modalRoot, state)?.let { return it }
            val under = dismissModals(state)
            (popTo(to, recordId, mainRoot(under, dsl), under) ?: switchTab(to, under, dsl))?.let { return it }
        } else {
            val root = rootRoute(from.stack, dsl)
            (popTo(to, recordId, root, state) ?: switchTab(to, state, dsl))?.let { return it }
        }
        return state.withPath(from.stack, state.path(from.stack) + Route(to, recordId, from.stack))
    }

    /** back: 積まれていれば pop、モーダルの最下層なら閉じる(タブ / main の根は閉じない)。 */
    fun close(state: NavigationState, route: Route): NavigationState {
        val path = state.path(route.stack)
        if (path.isNotEmpty()) return state.withPath(route.stack, path.dropLast(1))
        return when (route.stack) {
            Route.SHEET -> state.withPath(Route.SHEET, null).copy(sheet = null)
            Route.FULL -> state.withPath(Route.FULL, null).copy(fullScreen = null)
            else -> state
        }
    }

    /** 戻れるか(システムの戻るを横取りするか)。 */
    fun canClose(state: NavigationState, route: Route): Boolean =
        state.path(route.stack).isNotEmpty() || route.stack in modalStacks

    internal fun dismissModals(s: NavigationState): NavigationState =
        s.withPath(Route.SHEET, null).withPath(Route.FULL, null).copy(sheet = null, fullScreen = null)

    internal fun mainRoot(state: NavigationState, dsl: AppDSL): Route {
        if (dsl.app.hasTabs) {
            val tab = state.selectedTab.ifEmpty { dsl.initialTab }
            return Route(tab, stack = Route.tabStack(tab))
        }
        return Route(dsl.initialScreen)
    }

    private fun rootRoute(stack: String, dsl: AppDSL): Route? = when {
        stack.startsWith("tab:") -> Route(stack.removePrefix("tab:"), stack = stack)
        stack == Route.MAIN -> Route(dsl.initialScreen)
        else -> null
    }

    private fun popTo(screen: String, recordId: String?, root: Route?, s: NavigationState): NavigationState? {
        root ?: return null
        if (root.screen == screen) return s.withPath(root.stack, emptyList())
        val path = s.path(root.stack)
        val i = path.indexOfLast { it.screen == screen && it.recordId == recordId }
        if (i < 0) return null
        return s.withPath(root.stack, path.take(i + 1))
    }

    private fun switchTab(screen: String, s: NavigationState, dsl: AppDSL): NavigationState? {
        if (!dsl.app.hasTabs || dsl.app.tabs.none { it.screen == screen }) return null
        return s.withPath(Route.tabStack(screen), emptyList()).copy(selectedTab = screen)
    }
}
