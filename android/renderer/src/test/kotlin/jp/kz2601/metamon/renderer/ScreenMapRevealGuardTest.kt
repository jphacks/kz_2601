package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.form.DiscardGuard
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.renderer.navigation.ScreenGraph
import jp.kz2601.metamon.renderer.navigation.requestReveal
import jp.kz2601.metamon.renderer.navigation.revealLeaves
import jp.kz2601.metamon.renderer.navigation.top
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** #1111: 画面マップから開く時も、消えるフォームに保存していない入力があれば破棄の確認を挟む。 */
class ScreenMapRevealGuardTest {
    private val dsl = AppDSL.decode(File(System.getProperty("metamon.dslDocsDir"), "examples/navigation.json").readText())
    private val graph = ScreenGraph(dsl)
    private val entity = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"本","voiceEntry":"none"},"initialScreen":"form",
         "entities":[{"id":"book","name":"本","fields":[{"id":"memo","name":"メモ","type":"text"}]}],
         "screens":[{"id":"form","title":"本","entity":"book","components":[{"type":"textField","field":"memo"}]}]}
        """.trimIndent(),
    ).entityModels().first()

    private class Host(start: NavigationState) : NavigationHost {
        override val navigation = MutableStateFlow(start)
        override var discardGuard: DiscardGuard? = null
        override fun setNavigation(next: NavigationState) {
            navigation.value = next
        }
    }

    private val withSheet = NavigationState(selectedTab = "a", sheet = Route("c", stack = Route.SHEET))
    private fun edited() = FormDraft(entity).apply { values["memo"] = JSONValue.Str("書きかけ") }

    @Test
    fun revealLeavesModalsAndTheVisibleTab() {
        assertEquals(listOf(Route.SHEET, Route.FULL, "tab:a"), NavigationResolver.revealLeaves(withSheet, dsl))
    }

    @Test
    fun unsavedSheetFormAsksBeforeRevealing() {
        val host = Host(withSheet)
        host.discardGuard = DiscardGuard().apply { track(Route.SHEET, edited()) }
        assertTrue(host.requestReveal("other", graph, dsl))
        assertEquals("確認の間は開かない", withSheet, host.navigation.value)
        assertNotNull(host.discardGuard!!.pending)

        host.discardGuard!!.discard()
        assertNull("破棄したらシートを閉じて開く", host.navigation.value.sheet)
        assertEquals("other", host.navigation.value.top(dsl).screen)
    }

    @Test
    fun keepEditingDoesNotReveal() {
        val host = Host(withSheet)
        host.discardGuard = DiscardGuard().apply { track(Route.SHEET, edited()) }
        host.requestReveal("other", graph, dsl)
        host.discardGuard!!.keepEditing()
        assertEquals(withSheet, host.navigation.value)
    }

    @Test
    fun untouchedFormsRevealAtOnce() {
        val host = Host(withSheet)
        host.discardGuard = DiscardGuard().apply {
            track(Route.SHEET, FormDraft(entity))
            track("tab:other", edited()) // 見ていないタブは描かれていない(数えない)
        }
        assertTrue(host.requestReveal("other", graph, dsl))
        assertNull(host.discardGuard!!.pending)
        assertEquals("other", host.navigation.value.top(dsl).screen)
    }

    @Test
    fun withoutGuardRevealsAndUnknownScreenIsRejected() {
        val host = Host(withSheet)
        assertFalse(host.requestReveal("nope", graph, dsl))
        assertEquals(withSheet, host.navigation.value)
        assertTrue(host.requestReveal("b", graph, dsl))
        assertEquals("b", host.navigation.value.top(dsl).screen)
    }
}
