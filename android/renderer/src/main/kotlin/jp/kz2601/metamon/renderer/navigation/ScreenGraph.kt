package jp.kz2601.metamon.renderer.navigation

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.hasTabs
import jp.kz2601.metamon.renderer.NavigationHost
import jp.kz2601.metamon.renderer.form.DiscardGuard
import jp.kz2601.metamon.renderer.screen.ScreenToolbar

/**
 * 画面マップ(#897・iOS #116 の `ScreenGraph`)の元になる画面グラフ。backend `pkg/dsl/nav.go` と同じ考え方の純粋関数。
 * - 辺: button / toolbar / list の行タップ / calendar の日付タップの `navigate` と、`saveRecord` の保存後遷移先
 * - 根: タブがあればタブの根(タブ順)、無ければ initialScreen(タブの有無は `hasTabs` = 2 個以上)
 * - 段: 根を 0 段目に BFS の深さで下へ。同じ段は見つかった順。到達できない画面は最後の段に文書順
 */
class ScreenGraph(dsl: AppDSL) {
    /** 遷移のきっかけ(詳細で「どこから行けるか」を言葉にする)。 */
    sealed interface Trigger {
        data class Button(val label: String) : Trigger
        data class Toolbar(val label: String) : Trigger
        data object ItemTap : Trigger
        data object DateTap : Trigger
        /** saveRecord の保存後に開く */
        data object AfterSave : Trigger
        /** button 以外の部品の action */
        data object Other : Trigger
    }

    /** 1 本の遷移(存在しない画面への辺も含む)。 */
    data class Edge(val from: String, val to: String, val presentation: Presentation, val trigger: Trigger) {
        val isModal: Boolean get() = presentation != Presentation.PUSH
    }

    /** 図に描く矢印。同じ from→to の辺を 1 本にまとめる(両端とも存在する画面の時だけ)。 */
    data class Link(val from: String, val to: String, val edges: List<Edge>) {
        val id: String get() = "$from->$to"
        /** まとめた辺がすべて sheet / fullScreen なら破線 */
        val isModal: Boolean get() = edges.all { it.isModal }
    }

    data class Node(
        val id: String,
        val title: String,
        val isRoot: Boolean,
        /** フォーム画面(screen.entity あり) */
        val isForm: Boolean,
        /** 根からの BFS の深さ。到達できなければ null */
        val depth: Int?,
        val row: Int,
        val column: Int,
    ) {
        val reachable: Boolean get() = depth != null
    }

    val edges: List<Edge> = edges(dsl)
    val nodes: List<Node>
    val links: List<Link>
    /** 段ごとの画面 id(上から)。到達できない画面があれば最後の段にまとまる */
    val rows: List<List<String>>

    init {
        val ids = dsl.screens.map { it.id }
        val exists = ids.toSet()
        val next = LinkedHashMap<String, MutableList<String>>()
        for (e in edges) {
            if (e.to !in exists) continue
            val list = next.getOrPut(e.from) { mutableListOf() }
            if (e.to !in list) list += e.to
        }

        val roots = roots(dsl)
        val depth = HashMap<String, Int>()
        val rowsOut = mutableListOf<List<String>>()
        var frontier = mutableListOf<String>()
        for (r in roots) if (r in exists && r !in depth) { depth[r] = 0; frontier += r }
        while (frontier.isNotEmpty()) {
            rowsOut += frontier
            val found = mutableListOf<String>()
            for (id in frontier) for (to in next[id].orEmpty()) if (to !in depth) { depth[to] = rowsOut.size; found += to }
            frontier = found
        }
        val unreachable = ids.filter { it !in depth }
        if (unreachable.isNotEmpty()) rowsOut += unreachable

        val place = HashMap<String, Pair<Int, Int>>()
        rowsOut.forEachIndexed { r, row -> row.forEachIndexed { c, id -> place[id] = r to c } }
        val rootSet = roots.toSet()
        nodes = dsl.screens.map { s ->
            Node(
                id = s.id,
                title = s.title.ifEmpty { s.id },
                isRoot = s.id in rootSet,
                isForm = s.entity != null,
                depth = depth[s.id],
                row = place[s.id]?.first ?: 0,
                column = place[s.id]?.second ?: 0,
            )
        }
        val merged = LinkedHashMap<String, Link>()
        for (e in edges) {
            if (e.from !in exists || e.to !in exists) continue
            val key = "${e.from}->${e.to}"
            merged[key] = merged[key]?.let { it.copy(edges = it.edges + e) } ?: Link(e.from, e.to, listOf(e))
        }
        links = merged.values.toList()
        rows = rowsOut
    }

    fun node(id: String): Node? = nodes.firstOrNull { it.id == id }

    /** この画面から出ていく矢印 */
    fun outgoing(from: String): List<Link> = links.filter { it.from == from }

    /** この画面へ入ってくる矢印 */
    fun incoming(to: String): List<Link> = links.filter { it.to == to }

    /** 根から行けない画面(文書順) */
    val unreachable: List<String> get() = nodes.filter { !it.reachable }.map { it.id }

    /** 上の段から順に、段の中は左から(TalkBack の読む順) */
    val readingOrder: List<String> get() = rows.flatten()

    /** 根からこの画面までの最短の道(BFS で段を決めた時と同じ順)。根なら空、行けない・無い画面は null。 */
    fun path(to: String): List<Link>? {
        val target = node(to) ?: return null
        if (!target.reachable) return null
        val parent = HashMap<String, Link>()
        var frontier = nodes.filter { it.depth == 0 }.map { it.id }
        val seen = frontier.toMutableSet()
        while (target.depth != 0 && to !in parent && frontier.isNotEmpty()) {
            val found = mutableListOf<String>()
            for (from in frontier) for (link in outgoing(from)) if (seen.add(link.to)) { parent[link.to] = link; found += link.to }
            frontier = found
        }
        val steps = ArrayDeque<Link>()
        var cursor = to
        while (true) {
            val link = parent[cursor] ?: break
            steps.addFirst(link)
            cursor = link.from
        }
        return steps.toList()
    }

    companion object {
        /** 根(nav.go の NavRoots)。タブがあればタブの根を重複なくタブ順、無ければ initialScreen。 */
        fun roots(dsl: AppDSL): List<String> =
            if (!dsl.app.hasTabs) listOf(dsl.initialScreen) else dsl.app.tabs.map { it.screen }.distinct()

        /** 全遷移を画面順に列挙する(nav.go と同じ走査順: toolbar → components(子も)の action / onItemTap / onDateTap)。 */
        fun edges(dsl: AppDSL): List<Edge> {
            val out = mutableListOf<Edge>()
            for (s in dsl.screens) {
                fun add(action: Action?, trigger: Trigger) {
                    val to = action?.screen?.takeIf { it.isNotEmpty() } ?: return
                    val p = Presentation.of(action.presentation?.wire)
                    when (action.type) {
                        ActionType.NAVIGATE -> out += Edge(s.id, to, p, trigger)
                        ActionType.SAVE_RECORD -> out += Edge(s.id, to, p, Trigger.AfterSave)
                        else -> Unit
                    }
                }
                // 画面に出ない重複の＋は数えない(#416)
                for (t in ScreenToolbar.deduplicated(s, dsl)) add(t.action, Trigger.Toolbar(t.label))
                for (c in s.components.flatMap { listOf(it) + it.children }) {
                    add(c.action, if (c.type == ComponentType.BUTTON) Trigger.Button(c.label ?: c.text ?: "") else Trigger.Other)
                    add(c.onItemTap, Trigger.ItemTap)
                    add(c.onDateTap, Trigger.DateTap)
                }
            }
            return out
        }
    }
}

/**
 * 画面マップから画面を開く(iOS / Mac の `NavigationResolver.reveal`)。開いているモーダルは閉じ、根(タブ)から最短の道を
 * `navigate` でたどる。記録は選ばずに開く。根から行けない画面は今のタブ(無ければ main)に 1 枚積む。無い画面は null。
 */
fun NavigationResolver.reveal(screen: String, graph: ScreenGraph, dsl: AppDSL, from: NavigationState): NavigationState? {
    graph.node(screen) ?: return null
    var s = dismissModals(from)
    val steps = graph.path(screen)
    if (steps == null) {
        val root = mainRoot(s, dsl)
        // タブ未選択("")なら mainRoot は最初のタブ。選んだことにしないと積んだ画面が出ない
        if (dsl.app.hasTabs) s = s.copy(selectedTab = root.screen)
        return s.withPath(root.stack, listOf(Route(screen, stack = root.stack)))
    }
    val start = steps.firstOrNull()?.from ?: screen
    s = if (dsl.app.hasTabs) s.withPath(Route.tabStack(start), emptyList()).copy(selectedTab = start) else s.withPath(Route.MAIN, emptyList())
    for (link in steps) {
        s = navigate(s, link.to, link.edges.first().presentation, null, s.top(dsl), dsl)
    }
    return s
}

/** [reveal] で消える画面のスタック: 開いているシート・全画面と、今見ているタブ(タブが無ければ main)。 */
fun NavigationResolver.revealLeaves(from: NavigationState, dsl: AppDSL): List<String> =
    listOf(Route.SHEET, Route.FULL, if (dsl.app.hasTabs) Route.tabStack(from.selectedTab) else Route.MAIN)

/**
 * 画面マップから画面を開く。保存していない入力のあるフォームが消えるなら、DSLView の破棄の確認を挟む
 * (確認を出している間は開かず、「破棄」で開く)。無い画面は false。
 */
fun NavigationHost.requestReveal(screen: String, graph: ScreenGraph, dsl: AppDSL): Boolean {
    val from = navigation.value
    NavigationResolver.reveal(screen, graph, dsl, from) ?: return false
    // 確認の後は、その時の状態から開き直す
    val open = { NavigationResolver.reveal(screen, graph, dsl, navigation.value)?.let(::setNavigation); Unit }
    val guard = discardGuard ?: return true.also { open() }
    guard.request(NavigationResolver.revealLeaves(from, dsl), open)
    return true
}
