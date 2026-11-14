package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.hasTabs
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Presentation
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.renderer.navigation.ScreenGraph
import jp.kz2601.metamon.renderer.navigation.reveal
import jp.kz2601.metamon.renderer.navigation.top
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** #897: 画面マップの画面グラフ(iOS の ScreenGraphTests / ScreenGraphNavigationTests と同じ期待値)。 */
class ScreenGraphTest {
    private fun example(name: String): AppDSL =
        AppDSL.decode(File(System.getProperty("metamon.dslDocsDir"), "examples/$name.json").readText())

    private fun pairs(g: ScreenGraph) = g.links.map { it.id }

    @Test
    fun navigationExample() {
        val g = ScreenGraph(example("navigation"))
        assertEquals(listOf("a", "b", "c", "other"), g.nodes.map { it.id })
        assertEquals("タブの根が上段、B / C は 1 段目", listOf(listOf("a", "other"), listOf("b", "c")), g.rows)
        assertEquals(listOf("a->b", "a->c", "b->c", "c->a", "c->other", "other->b"), pairs(g))
        assertEquals("シートは破線", true, g.links.first { it.id == "a->c" }.isModal)
        assertEquals(false, g.links.first { it.id == "a->b" }.isModal)
        assertEquals(ScreenGraph.Trigger.Button("B へ"), g.edges.first { it.from == "a" && it.to == "b" }.trigger)
        assertEquals(true, g.node("a")?.isRoot)
        assertEquals(true, g.node("other")?.isRoot)
        assertEquals(1, g.node("c")?.depth)
        assertEquals(1, g.node("c")?.column)
        assertEquals(emptyList<String>(), g.unreachable)
        assertEquals(listOf("a", "other"), g.outgoing("c").map { it.to })
        assertEquals(listOf("a", "other"), g.incoming("b").map { it.from })
        assertEquals(listOf("a", "other", "b", "c"), g.readingOrder)
    }

    @Test
    fun kakeiboMergesParallelEdges() {
        val g = ScreenGraph(example("kakeibo"))
        assertEquals(3, g.nodes.size)
        assertEquals(listOf(listOf("home", "stats"), listOf("expense_form")), g.rows)
        // 追加ボタン(シート)・行タップ(push)の 2 辺 → 矢印 1 本(右上の＋は重複なので置かない・#416)
        assertEquals(2, g.edges.count { it.from == "home" && it.to == "expense_form" })
        assertEquals(listOf("home->expense_form"), pairs(g))
        assertEquals("push を含むので実線", false, g.links[0].isModal)
        assertTrue(g.links[0].edges.any { it.trigger == ScreenGraph.Trigger.ItemTap })
        assertEquals(true, g.node("expense_form")?.isForm)
        assertEquals(emptyList<String>(), g.unreachable)
    }

    @Test
    fun unreachableGoesToLastRow() {
        val g = ScreenGraph(
            AppDSL.decode(
                """
                {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","entities":[],"screens":[
                  {"id":"home","title":"ホーム","components":[
                    {"type":"button","label":"詳細","action":{"type":"navigate","screen":"detail"}},
                    {"type":"button","label":"幽霊","action":{"type":"navigate","screen":"ghost"}}]},
                  {"id":"detail","title":"詳細","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"x","screen":"done"}}]},
                  {"id":"done","title":"完了"},
                  {"id":"orphan","title":""},
                  {"id":"island","title":"島","components":[{"type":"button","label":"孤","action":{"type":"navigate","screen":"orphan"}}]}
                ]}
                """.trimIndent(),
            ),
        )
        assertEquals(listOf(listOf("home"), listOf("detail"), listOf("done"), listOf("orphan", "island")), g.rows)
        assertEquals("到達できない画面から出る辺では到達扱いにしない", listOf("orphan", "island"), g.unreachable)
        assertNull(g.node("orphan")?.depth)
        assertEquals("タイトルが空なら id", "orphan", g.node("orphan")?.title)
        assertEquals(ScreenGraph.Trigger.AfterSave, g.edges.first { it.from == "detail" }.trigger)
        assertEquals("存在しない画面への辺も edges には残す", 4, g.edges.size)
        assertFalse("矢印は描かない", "home->ghost" in pairs(g))
        val oneTab = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"t","tabs":[{"screen":"a","title":"A"}]},"initialScreen":"b","screens":[{"id":"a"},{"id":"b"}]}""",
        )
        assertEquals("タブ 1 個はタブバーを出さないので initialScreen が根", listOf("b"), ScreenGraph.roots(oneTab))
    }

    @Test
    fun pathFromRoot() {
        val g = ScreenGraph(example("navigation"))
        assertEquals(emptyList<String>(), g.path("a")?.map { it.id })
        assertEquals("最短の道", listOf("a->c"), g.path("c")?.map { it.id })
        assertEquals(listOf("a->b"), g.path("b")?.map { it.id })
        assertNull(g.path("nope"))
    }

    @Test
    fun revealPushesOnTabStack() {
        val d = example("navigation")
        val g = ScreenGraph(d)
        val open = NavigationState(
            paths = mapOf("tab:other" to listOf(Route("b", stack = "tab:other"))),
            selectedTab = "other",
            sheet = Route("c", stack = Route.SHEET),
        )
        val s = NavigationResolver.reveal("b", g, d, open)!!
        assertEquals("a", s.selectedTab)
        assertEquals(listOf(Route("b", stack = "tab:a")), s.path("tab:a"))
        assertNull("開いていたシートは閉じる", s.sheet)
        assertEquals("b", s.top(d).screen)
    }

    @Test
    fun revealOpensSheetAndTabRoot() {
        val d = example("navigation")
        val g = ScreenGraph(d)
        val sheet = NavigationResolver.reveal("c", g, d, NavigationState(selectedTab = "a"))!!
        assertEquals("a->c はシート", Route("c", stack = Route.SHEET), sheet.sheet)
        assertEquals("c", sheet.top(d).screen)

        val tab = NavigationResolver.reveal("other", g, d, sheet)!!
        assertEquals("other", tab.selectedTab)
        assertEquals(emptyList<Route>(), tab.path("tab:other"))
        assertNull(tab.sheet)
        assertNull(NavigationResolver.reveal("nope", g, d, tab))
    }

    @Test
    fun revealUnreachableAndChain() {
        val d = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","screens":[
              {"id":"home","components":[{"type":"button","label":"1","action":{"type":"navigate","screen":"one"}}]},
              {"id":"one","components":[{"type":"button","label":"2","action":{"type":"navigate","screen":"two"}}]},
              {"id":"two"},{"id":"island"}]}
            """.trimIndent(),
        )
        val g = ScreenGraph(d)
        val chain = NavigationResolver.reveal("two", g, d, NavigationState())!!
        assertEquals("根から順に積むので戻るで 1 つずつ戻れる", listOf("one", "two"), chain.path(Route.MAIN).map { it.screen })
        val island = NavigationResolver.reveal("island", g, d, chain)!!
        assertEquals("行けない画面は根の上に 1 枚", listOf(Route("island")), island.path(Route.MAIN))
        assertEquals(Presentation.PUSH, g.links.first().edges.first().presentation)
    }

    /** タブがあり、まだタブを選んでいない("")時に行けない画面を開くと、最初のタブを選んでその上に積む。 */
    @Test
    fun revealUnreachableWithTabsSelectsTheInitialTab() {
        val d = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"t","tabs":[{"screen":"home","title":"ホーム"},{"screen":"more","title":"その他"}]},
             "initialScreen":"home","screens":[{"id":"home"},{"id":"more"},{"id":"island"}]}
            """.trimIndent(),
        )
        val g = ScreenGraph(d)
        assertTrue(d.app.hasTabs)
        val s = NavigationResolver.reveal("island", g, d, NavigationState())!!
        assertEquals("home", s.selectedTab)
        assertEquals(listOf(Route("island", stack = "tab:home")), s.path("tab:home"))
        assertEquals("island", s.top(d).screen)
    }
}
