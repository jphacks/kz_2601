package jp.kz2601.metamon.switcher

import jp.kz2601.metamon.R
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.AppWithDsl
import jp.kz2601.metamon.voice.ChatHistoryEntry
import jp.kz2601.metamon.voice.ChatHistoryStore
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.yield
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.lang.reflect.Proxy

/** 変身一覧の「…」(#896): 楽観的 UI・失敗で戻す・削除の後片付け(#1176 の会話の履歴)・切り替え先。 */
class AppListEditorTest {
    private val t = "2026-11-0%dT00:00:00Z"
    private fun app(id: String, day: Int, sort: Int? = null) = AppSummary(id, "名前$id", currentVersion = 3, updatedAt = t.format(day), sortIndex = sort)

    private inner class Harness(initial: List<AppSummary>, current: String = "a") {
        var apps = initial
        var current = current
        val calls = mutableListOf<String>()
        val toasts = mutableListOf<UiText>()
        var fail = false
        var gate: CompletableDeferred<Unit>? = null
        var busy = false
        val history = ChatHistoryStore(null)
        val forgotten = mutableListOf<String>()

        val client = Proxy.newProxyInstance(AppConfigClient::class.java.classLoader, arrayOf(AppConfigClient::class.java)) { _, m, args ->
            calls += m.name
            if (fail) throw IOException("offline")
            when (m.name) {
                "renameApp" -> apps.first { it.id == args[0] }.copy(name = args[1] as String)
                "duplicateApp" -> AppWithDsl("copy", args[1] as String, currentVersion = 2, updatedAt = t.format(9), dsl = JsonObject(emptyMap()))
                else -> Unit
            }
        } as AppConfigClient

        val editor = AppListEditor(
            client = object : AppConfigClient by client {
                override suspend fun deleteApp(appId: String) {
                    gate?.await()
                    client.deleteApp(appId)
                }
            },
            apps = { apps },
            setApps = { apps = it },
            currentAppId = { this.current },
            switchTo = { id ->
                calls += "switch:$id"
                this.current = id
                true
            },
            goHome = {
                calls += "home"
                this.current = "fresh"
            },
            forgetLocal = { id ->
                forgotten += id
                history.remove(id)
            },
            toast = { toasts += it },
            isBusy = { busy },
        )
    }

    @Test
    fun renameShowsFirstAndKeepsOnSuccess() = runTest {
        val h = Harness(listOf(app("a", 1), app("b", 2)))
        h.editor.rename(h.apps[1], "  新しい名前 ")
        assertEquals("新しい名前", h.apps.first { it.id == "b" }.name)
        assertTrue(h.toasts.isEmpty())
    }

    @Test
    fun renameRollsBackOnFailure() = runTest {
        val h = Harness(listOf(app("a", 1), app("b", 2)))
        h.fail = true
        h.editor.rename(h.apps[1], "x")
        assertEquals("名前b", h.apps.first { it.id == "b" }.name)
        assertEquals(UiText.Res(R.string.app_menu_rename_failed), h.toasts.single())
    }

    @Test
    fun renameRejectsEmptyWithoutCallingServer() = runTest {
        val h = Harness(listOf(app("a", 1)))
        h.editor.rename(h.apps[0], "   ")
        assertTrue(h.calls.isEmpty())
        assertEquals(R.string.app_menu_rename_invalid, (h.toasts.single() as UiText.Res).id)
    }

    @Test
    fun duplicatePlacesCopyRightAfterOriginalWhenOrdered() = runTest {
        val h = Harness(listOf(app("a", 1, sort = 1), app("b", 2, sort = 2), app("c", 3, sort = 3)))
        h.editor.duplicate(h.apps[0]) { "${it}のコピー" }
        assertEquals("名前aのコピー", h.apps.first { it.id == "copy" }.name)
        assertEquals(listOf("a", "copy", "b", "c"), AppSummary.visible(h.apps, "a").map { it.id })
        assertEquals(listOf("duplicateApp", "reorderApps"), h.calls)
    }

    @Test
    fun moveRollsBackOrderOnFailure() = runTest {
        val h = Harness(listOf(app("a", 3), app("b", 2), app("c", 1)))
        val before = h.apps
        h.fail = true
        h.editor.move("c", 0)
        assertEquals(before, h.apps)
        assertEquals(UiText.Res(R.string.app_menu_reorder_failed), h.toasts.single())
    }

    @Test
    fun moveSavesOrder() = runTest {
        val h = Harness(listOf(app("a", 3), app("b", 2), app("c", 1)))
        h.editor.move("c", 0)
        assertEquals(listOf("c", "a", "b"), AppSummary.visible(h.apps, null).map { it.id })
    }

    @Test
    fun deleteHidesFirstThenRemovesAndForgetsLocalHistory() = runTest {
        val h = Harness(listOf(app("a", 1), app("b", 2)))
        h.history.save(listOf(ChatHistoryEntry(ChatHistoryEntry.Kind.USER, "こんにちは")), "b")
        h.gate = CompletableDeferred()
        val job = async { h.editor.delete(h.apps.first { it.id == "b" }) }
        yield()
        assertEquals(setOf("b"), h.editor.hidden.value) // 先に隠す
        assertTrue(h.apps.any { it.id == "b" })
        h.gate!!.complete(Unit)
        assertTrue(job.await())
        assertTrue(h.editor.hidden.value.isEmpty())
        assertFalse(h.apps.any { it.id == "b" })
        assertEquals(listOf("b"), h.forgotten)
        assertTrue(h.history.load("b").isEmpty())
        assertEquals(R.string.app_menu_deleted, (h.toasts.single() as UiText.Res).id)
    }

    @Test
    fun deleteFailureShowsCardAgainAndKeepsLocalData() = runTest {
        val h = Harness(listOf(app("a", 1), app("b", 2)))
        h.history.save(listOf(ChatHistoryEntry(ChatHistoryEntry.Kind.USER, "残る")), "b")
        h.fail = true
        assertFalse(h.editor.delete(h.apps.first { it.id == "b" }))
        assertTrue(h.editor.hidden.value.isEmpty())
        assertTrue(h.apps.any { it.id == "b" })
        assertTrue(h.forgotten.isEmpty())
        assertEquals(1, h.history.load("b").size)
        assertEquals(R.string.app_menu_delete_failed, (h.toasts.single() as UiText.Res).id)
    }

    @Test
    fun deletingCurrentAppSwitchesToLatestBeforeServerDelete() = runTest {
        val h = Harness(listOf(app("a", 1), app("b", 5), app("c", 3)), current = "a")
        assertTrue(h.editor.delete(h.apps.first { it.id == "a" }))
        assertEquals(listOf("switch:b", "deleteApp"), h.calls)
        assertEquals("b", h.current)
    }

    @Test
    fun deletingLastAppGoesHome() = runTest {
        val h = Harness(listOf(app("a", 1)), current = "a")
        assertTrue(h.editor.delete(h.apps[0]))
        assertEquals(listOf("home", "deleteApp"), h.calls)
    }

    @Test
    fun deleteRefusedWhileGenerating() = runTest {
        val h = Harness(listOf(app("a", 1), app("b", 2)))
        h.busy = true
        assertFalse(h.editor.delete(h.apps[1]))
        assertTrue(h.calls.isEmpty())
        assertEquals(UiText.Res(R.string.app_menu_delete_busy), h.toasts.single())
    }

    @Test
    fun pureHelpers() {
        assertNull(AppListEdit.normalizedName(" "))
        assertNull(AppListEdit.normalizedName("あ".repeat(41)))
        assertEquals(40, AppListEdit.copyName("あ".repeat(40)) { "${it}のコピー" }.length)
        assertEquals(listOf("b", "a", "c"), AppListEdit.moved(listOf("a", "b", "c"), "a", 1))
        assertEquals(listOf("b", "c", "a"), AppListEdit.moved(listOf("a", "b", "c"), "a", 99))
        assertNull(AppListEdit.fallback("b", current = "a", apps = listOf(app("a", 1), app("b", 2))))
    }
}
