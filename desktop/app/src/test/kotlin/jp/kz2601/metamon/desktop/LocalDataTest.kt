package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.desktop.store.SessionPersistence
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 端末の保存の続き(#1887): 起動時は裏で読みスケルトンを出す・パレットの「この端末のデータを消す」。画面は出さない。 */
class LocalDataTest {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val mock = DesktopApiConfig.fromEnvironment(emptyMap())

    @After
    fun tearDown() = scope.cancel()

    private fun DesktopSession.addItem(name: String) {
        dispatch(Action(type = ActionType.NAVIGATE, screen = "item_form", presentation = Presentation.SHEET), ActionContext(top, null))
        val form = form(top)!!
        form.values["name"] = JSONValue.Str(name)
        dispatch(Action(type = ActionType.SAVE_RECORD), ActionContext(top, form))
    }

    private suspend fun SharedRecords.await() = withTimeout(5_000) { awaitLoaded() }

    private fun hub(store: DesktopStore, wiped: MutableList<String> = mutableListOf()) = AppHub(
        openRecords = { id, dsl ->
            val p = SessionPersistence.create(store, id, mock)
            val models = dsl.entityModels()
            SharedRecords.opening(p, scope) { p.load(models) }
        },
        wipeStore = { wiped += "store"; store.wipe() },
    )

    // MARK: - 起動時のスケルトン

    @Test
    fun skeletonUntilTheDeviceRecordsAreRead() = runBlocking {
        val store = DesktopStore(null)
        val p = SessionPersistence(store, "shopping", client = null)
        val gate = CompletableDeferred<MockStore>()
        val records = SharedRecords.opening(p, scope) { gate.await() }

        // 読み終わる前: スケルトン。記録は空で、画面からの書き換えは受けない(読んだ記録で上書きされるため)
        assertTrue(records.loading)
        assertTrue(records.store.rows.isEmpty())
        val dsl = Examples.load("shopping")
        records.update(dsl.entityModels(), MockStore().save("groceries", null, mapOf("name" to "早すぎ")))
        assertTrue(records.store.rows.isEmpty())

        gate.complete(MockStore().save("groceries", null, mapOf("name" to "牛乳")))
        records.await()
        assertFalse(records.loading)
        assertEquals(listOf("牛乳"), records.store.list("groceries").map { it.data["name"] })
        records.close()
    }

    @Test
    fun hubOpensFromTheDeviceInTheBackground() = runBlocking {
        val store = DesktopStore(null)
        // 前回の起動で残した記録
        val first = hub(store).app("shopping")
        first.records.await()
        DesktopSession(first.dsl, shared = first.records).addItem("牛乳")
        first.records.persistence!!.idle()

        val app = hub(store).app("shopping")
        app.records.await()
        assertFalse(app.records.loading)
        assertEquals(listOf("牛乳"), DesktopSession(app.dsl, shared = app.records).records("groceries").map { it["name"].stringValue })
        assertEquals("最後に開いたアプリを覚える", "shopping", store.lastAppId)
    }

    @Test
    fun unreadableDeviceOpensEmpty() = runBlocking {
        val p = SessionPersistence(DesktopStore(null), "shopping", client = null)
        val records = SharedRecords.opening(p, scope) { error("壊れたファイル") }
        records.await()
        assertFalse(records.loading)
        assertTrue(records.store.rows.isEmpty())
    }

    @Test
    fun fixtureAndTestRecordsAreNotLoading() {
        assertFalse(SharedRecords().loading)
        assertFalse(AppHub().app("shopping").records.loading)
    }

    // MARK: - この端末のデータを消す

    @Test
    fun wipeCommandAsksFirstThenClearsEveryApp() = Strings.withLang(Lang.JA) {
        runBlocking {
            val store = DesktopStore(null)
            val wiped = mutableListOf<String>()
            val hub = hub(store, wiped)
            val shopping = hub.app("shopping").also { it.records.await() }
            val kakeibo = hub.app("kakeibo").also { it.records.await() }
            val window = DesktopSession(shopping.dsl, shared = shopping.records)
            val other = DesktopSession(shopping.dsl, shared = shopping.records)
            window.addItem("牛乳")
            kakeibo.records.update(kakeibo.dsl.entityModels(), MockStore().save("expense", null, mapOf("amount" to "500", "category" to "食費")))
            shopping.records.persistence!!.idle()
            kakeibo.records.persistence!!.idle()
            assertEquals(1, store.records.rows("shopping", "groceries").size)
            assertEquals(1, store.records.rows("kakeibo", "expense").size)
            // 他のウィンドウで打ちかけ
            other.dispatch(Action(type = ActionType.NAVIGATE, screen = "item_form", presentation = Presentation.SHEET), ActionContext(other.top, null))
            val sheet = other.top

            val k = DesktopKeyboard(window.dsl, window, HostOs.LINUX)
            assertTrue("口が無ければ出さない", k.commands().none { it.id == "data:wipe" })
            k.wipeLocalData = hub::wipeLocalData
            val command = k.commands().single { it.id == "data:wipe" }
            assertEquals("この端末のデータを消す", command.title)

            // 選んだだけでは消さず、確認を出す
            command.run()
            assertTrue(k.wipeConfirmOpen)
            assertEquals(1, shopping.records.store.list("groceries").size)
            assertEquals(1, store.records.rows("shopping", "groceries").size)
            assertTrue(wiped.isEmpty())

            // 確認で「消す」: 開いている全アプリの記録と端末の保存先を消す
            k.confirmWipe()
            assertFalse(k.wipeConfirmOpen)
            assertTrue(shopping.records.store.rows.isEmpty())
            assertTrue(kakeibo.records.store.rows.isEmpty())
            assertTrue(store.records.rows("shopping", "groceries").isEmpty())
            assertTrue(store.records.rows("kakeibo", "expense").isEmpty())
            assertNull(store.lastAppId)
            assertEquals(listOf("store"), wiped)
            assertEquals("この端末のデータを消しました", window.message)

            // 同じアプリの他のウィンドウも打ちかけを捨てて最初の画面へ
            other.followWipe()
            assertTrue(other.top != sheet)
            assertNull(other.nav.sheet)
        }
    }

    @Test
    fun wipeConfirmCanBeCancelled() = runBlocking {
        val store = DesktopStore(null)
        val hub = hub(store)
        val app = hub.app("shopping").also { it.records.await() }
        val s = DesktopSession(app.dsl, shared = app.records)
        s.addItem("牛乳")
        val k = DesktopKeyboard(s.dsl, s, HostOs.LINUX).apply { wipeLocalData = hub::wipeLocalData }
        k.commands().single { it.id == "data:wipe" }.run()
        // 閉じる(キャンセル)は Overlay が wipeConfirmOpen を戻すだけ。記録は残る
        k.wipeConfirmOpen = false
        app.records.persistence!!.idle()
        assertEquals(1, app.records.store.list("groceries").size)
        assertEquals(1, store.records.rows("shopping", "groceries").size)
    }
}
