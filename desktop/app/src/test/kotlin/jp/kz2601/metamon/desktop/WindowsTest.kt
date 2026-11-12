package jp.kz2601.metamon.desktop

import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.WindowPosition
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.desktop.store.SessionPersistence
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.Rectangle

/** 複数のウィンドウ(#1763): ウィンドウの一覧(開く・閉じる・同じ物は前に出す)と、同じアプリのウィンドウの間で記録が伝わること。画面は出さない。 */
class WindowsTest {
    private val add = Action(type = ActionType.NAVIGATE, screen = "item_form", presentation = Presentation.SHEET)
    private val save = Action(type = ActionType.SAVE_RECORD)

    private fun DesktopSession.addItem(name: String) {
        dispatch(add, ActionContext(top, null))
        val form = form(top)!!
        form.values["name"] = JSONValue.Str(name)
        dispatch(save, ActionContext(top, form))
    }

    private fun names(s: DesktopSession) = s.records("groceries").map { it["name"].stringValue }

    // MARK: - ウィンドウの一覧

    @Test
    fun openReusesTheSameAppAndRecord() {
        val r = WindowRegistry()
        val shopping = r.open(WindowKey.app("shopping"))
        val kakeibo = r.open(WindowKey.app("kakeibo"))
        assertEquals(2, r.windows.size)
        assertEquals(kakeibo.id, r.active?.id)

        // 同じアプリはもう 1 枚作らず前に出す
        val again = r.open(WindowKey.app("shopping"))
        assertSame(shopping, again)
        assertEquals(2, r.windows.size)
        assertEquals(1, shopping.focusRequests)
        assertEquals(shopping.id, r.active?.id)

        // 記録の詳細は記録ごとに 1 枚
        val a = r.open(WindowKey.record("shopping", "item_form", "a"))
        val b = r.open(WindowKey.record("shopping", "item_form", "b"))
        assertEquals(4, r.windows.size)
        assertSame(a, r.open(WindowKey.record("shopping", "item_form", "a")))
        assertEquals(4, r.windows.size)
        assertEquals(WindowKind.RECORD, a.key.kind)
        assertEquals(WindowKind.APP, shopping.key.kind)
        assertEquals(listOf(0, 1), listOf(a, b).map(r::indexInKind))
    }

    @Test
    fun closeRemovesAndMovesActiveToTheLastWindow() {
        val r = WindowRegistry()
        val one = r.open(WindowKey.app("shopping"))
        val two = r.open(WindowKey.app("kakeibo"))
        r.close(two.id)
        assertEquals(listOf(one), r.windows.toList())
        assertEquals(one.id, r.active?.id)
        assertNull(r.find(WindowKey.app("kakeibo")))
        // 閉じた物をもう一度開くと新しい 1 枚
        val three = r.open(WindowKey.app("kakeibo"))
        assertTrue(three.id != two.id)
        r.activate(one.id)
        assertEquals(one.id, r.active?.id)
    }

    @Test
    fun requestCloseLeavesTheLastWindowToTheTray() {
        val r = WindowRegistry()
        val one = r.open(WindowKey.app("shopping"))
        val two = r.open(WindowKey.record("shopping", "item_form", "a"))
        // 他にあればその 1 枚だけ閉じる(トレイには聞かない)
        assertFalse(r.requestClose(two.id) { error("最後の 1 枚ではない") })
        assertEquals(listOf(one), r.windows.toList())
        // 最後の 1 枚はトレイに隠すか終わるか
        assertFalse(r.requestClose(one.id) { false })
        assertEquals(1, r.windows.size)
        assertTrue(r.requestClose(one.id) { true })
    }

    @Test
    fun trayAndPaletteOpenAppWindows() = Strings.withLang(Lang.JA) {
        val r = WindowRegistry().also { it.open(WindowKey.app("shopping")) }
        val tray = TrayController(
            Examples.load("shopping"), traySupported = true, closeSetting = MemoryBoolSetting(true), hintSetting = MemoryBoolSetting(true),
            apps = listOf("kakeibo" to "家計簿", "shopping" to "買い物"),
        )
        tray.onOpenApp = { r.open(WindowKey.app(it)) }
        val sub = tray.items(emptyList()).filterIsInstance<TrayItem.Submenu>().single()
        assertEquals("新しいウィンドウ", sub.label)
        assertEquals(listOf("kakeibo", "shopping"), sub.items.map { (it as TrayItem.OpenApp).appId })
        // 最後の 1 枚をトレイに隠した後でも、新しいウィンドウを開けば見える
        tray.onCloseRequest { _, _ -> }
        assertFalse(tray.windowVisible)
        tray.openApp("kakeibo")
        assertTrue(tray.windowVisible)
        assertEquals(listOf("shopping", "kakeibo"), r.windows.map { it.key.appId })
        // トレイの名前・すぐ記録は前のウィンドウのアプリ
        tray.switchApp(Examples.load("kakeibo"))
        assertEquals("expense_form", tray.quickRecordAction?.screen)

        // パレットのアプリも別のウィンドウ(開いていれば前に出す)
        val s = DesktopSession(Examples.load("shopping"))
        val k = DesktopKeyboard(s.dsl, s, HostOs.LINUX)
        k.openApp = { r.open(WindowKey.app(it)) }
        k.commands().first { it.id == "app:kakeibo" }.run()
        assertEquals(2, r.windows.size)
        assertEquals(1, r.find(WindowKey.app("kakeibo"))!!.focusRequests)
    }

    @Test
    fun paletteOpensTheCurrentRecordInItsOwnWindow() {
        val s = DesktopSession(Examples.load("shopping"), MockStore().save("groceries", null, mapOf("name" to "牛乳", "done" to "false")))
        val k = DesktopKeyboard(s.dsl, s, HostOs.LINUX)
        var opened: Pair<String, String>? = null
        k.openRecord = { screen, id -> opened = screen to id }
        assertNull("記録を開いていなければ出さない", k.commands().firstOrNull { it.id == "window:record" })
        val row = s.dsl.screens.flatMap { it.allComponents() }.first { it.onItemTap != null }
        s.dispatch(row.onItemTap!!, ActionContext(s.top, null, "new-1"))
        k.commands().first { it.id == "window:record" }.run()
        assertEquals(s.top.screen to "new-1", opened)
        assertEquals("item_form", RecordWindows.target(row.onItemTap!!))
        assertNull("新規を作る遷移は別のウィンドウにしない", RecordWindows.target(add.copy(prefillRef = "list")))
    }

    // MARK: - 詳細のウィンドウ

    @Test
    fun recordWindowOpensTheRecordAndClosesFromItsRoot() {
        val records = SharedRecords(MockStore().save("groceries", null, mapOf("name" to "牛乳", "done" to "false")))
        val detail = DesktopSession(Examples.load("shopping"), shared = records, startScreen = "item_form", startRecordId = "new-1")
        var closed = 0
        detail.onCloseWindow = { closed++ }
        assertEquals("item_form", detail.top.screen)
        assertEquals("new-1", detail.top.recordId)
        assertEquals("牛乳", detail.form(detail.top)!!.values["name"]?.stringValue)
        assertTrue(detail.canClose(detail.top))
        // Esc・閉じる: 一覧に戻らずウィンドウを閉じる
        val k = DesktopKeyboard(detail.dsl, detail, HostOs.LINUX)
        assertTrue(k.run(Shortcut.CLOSE))
        assertEquals(1, closed)
        assertEquals("item_form", detail.top.screen)
        // 保存しても閉じる
        val form = detail.form(detail.top)!!
        form.values["name"] = JSONValue.Str("豆乳")
        detail.dispatch(save, ActionContext(detail.top, form))
        assertEquals(2, closed)
        assertEquals("豆乳", records.store.find("groceries", "new-1")?.data?.get("name"))
    }

    // MARK: - ウィンドウの間で記録が伝わる

    @Test
    fun saveInOneWindowShowsInTheOtherWindowOfTheSameApp() {
        val hub = AppHub()
        val app = hub.app("shopping")
        assertSame("同じアプリは 1 つ", app, hub.app("shopping"))
        val left = DesktopSession(app.dsl, shared = app.records)
        val right = DesktopSession(app.dsl, shared = app.records)

        left.addItem("牛乳")
        assertEquals(listOf("牛乳"), names(right))
        right.addItem("卵")
        assertEquals(listOf("牛乳", "卵"), names(left))
        assertEquals("新しい id はウィンドウをまたいでぶつからない", 2, left.records("groceries").map { it.id }.toSet().size)

        // 一覧のチェック・削除も伝わる
        val milk = left.records("groceries").first().id
        assertTrue(right.updateRecord("groceries", milk, mapOf("done" to "true")))
        assertEquals("true", left.store.find("groceries", milk)?.data?.get("done"))

        // 別のアプリは別の記録
        assertTrue(DesktopSession(hub.app("kakeibo").dsl, shared = hub.app("kakeibo").records).store.rows.isEmpty())
    }

    @Test
    fun openDetailFollowsEditsFromAnotherWindow() {
        val app = AppHub().app("shopping")
        val list = DesktopSession(app.dsl, shared = app.records)
        list.addItem("牛乳")
        val id = list.records("groceries").single().id
        val detail = DesktopSession(app.dsl, shared = app.records, startScreen = "item_form", startRecordId = id)
        assertEquals("牛乳", detail.form(detail.top)!!.values["name"]?.stringValue)

        // 打ちかけが無ければ、他のウィンドウの保存で詳細の下書きが作り直される
        assertTrue(list.updateRecord("groceries", id, mapOf("name" to "豆乳")))
        assertEquals("豆乳", detail.form(detail.top)!!.values["name"]?.stringValue)
    }

    @Test
    fun lastSaveWinsAndTellsTheUser() = Strings.withLang(Lang.JA) {
        val app = AppHub().app("shopping")
        val list = DesktopSession(app.dsl, shared = app.records)
        list.addItem("牛乳")
        val id = list.records("groceries").single().id
        val a = DesktopSession(app.dsl, shared = app.records, startScreen = "item_form", startRecordId = id)
        val b = DesktopSession(app.dsl, shared = app.records, startScreen = "item_form", startRecordId = id)
        val fa = a.form(a.top)!!
        val fb = b.form(b.top)!!
        fa.values["name"] = JSONValue.Str("A の名前")
        fb.values["name"] = JSONValue.Str("B の名前")

        a.dispatch(save, ActionContext(a.top, fa))
        assertNull("先に保存した方は知らせない", a.message)
        // B は打ちかけを残したまま(作り直さない)
        assertEquals("B の名前", b.form(b.top)!!.values["name"]?.stringValue)
        b.dispatch(save, ActionContext(b.top, fb))
        assertEquals("B の名前", list.store.find("groceries", id)?.data?.get("name"))
        assertEquals("別のウィンドウで先に保存された内容を上書きしました", b.message)
    }

    @Test
    fun sharedRecordsWriteToTheDeviceOnce() = runBlocking {
        val store = DesktopStore(null)
        val dsl = Examples.load("shopping")
        val p = SessionPersistence(store, "shopping", client = null)
        val records = SharedRecords(p.load(dsl.entityModels()), p)
        val left = DesktopSession(dsl, shared = records)
        val right = DesktopSession(dsl, shared = records)
        assertSame(p, right.persistence)
        left.addItem("牛乳")
        right.addItem("卵")
        p.idle()
        // 並びは createdAt 順で、Windows では 2 件が同じ時刻になり得るので順番は見ない
        assertEquals(listOf("牛乳", "卵").sorted(), store.records.rows("shopping", "groceries").map { it.data["name"].toString().trim('"') }.sorted())
        // 開き直すと両方ある
        assertEquals(2, p.load(dsl.entityModels()).list("groceries").size)
        records.close()
    }

    // MARK: - 大きさ・位置

    @Test
    fun boundsAreKeptPerKindAndCascadeStaysOnScreen() {
        assertEquals("", WindowBounds.prefix(WindowKind.APP))
        assertEquals("record.", WindowBounds.prefix(WindowKind.RECORD))
        val b = WindowBounds.Bounds(DpSize(480.dp, 820.dp), WindowPosition(100.dp, 100.dp))
        assertSame(b, WindowBounds.cascade(b, 0))
        assertEquals(WindowPosition(164.dp, 164.dp), WindowBounds.cascade(b, 2).position)
        // ずらして画面からはみ出すなら画面の中に寄せる
        val screens = listOf(Rectangle(0, 0, 1920, 1080))
        val far = WindowBounds.fit(WindowBounds.cascade(b, 10), screens).position as WindowPosition.Absolute
        assertEquals(420f, far.x.value)
        assertEquals(260f, far.y.value)
        assertNotNull(WindowBounds.defaultSize(WindowKind.RECORD))
    }

    @Test
    fun windowStringsExistInBothLanguages() {
        listOf("desktop_list_open_window", "desktop_tray_new_window", "desktop_window_conflict", "desktop_window_open_record", "desktop_window_title_record").forEach { k ->
            Lang.entries.forEach { l -> assertTrue("$k ($l)", Strings.table.has(l, k)) }
        }
        assertEquals("Tailor It — 買い物 — 詳細", Strings.withLang(Lang.JA) { Strings.format("desktop_window_title_record", "買い物", "詳細") })
    }
}
