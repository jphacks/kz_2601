package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.navigation.Route
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** キーボードだけで使う(#1765): OS ごとのキー・パレットの絞り込み・shopping を追加 → 保存 → 削除。 */
class KeyboardTest {
    private val win = HostOs.WINDOWS
    private val linux = HostOs.LINUX
    private val mac = HostOs.MAC

    // MARK: - OS ごとのキー

    @Test
    fun osNameDecidesPlatform() {
        assertEquals(win, HostOs.of("Windows 11"))
        assertEquals(linux, HostOs.of("Linux"))
        assertEquals(mac, HostOs.of("Mac OS X"))
        assertEquals(linux, HostOs.of("FreeBSD"))
    }

    @Test
    fun ctrlOnWindowsAndLinuxCommandOnMac() {
        for (os in listOf(win, linux)) {
            assertEquals(Shortcut.NEW, Keymap.match(KeyPress(Key.N, ctrl = true), os))
            assertEquals(Shortcut.SAVE, Keymap.match(KeyPress(Key.S, ctrl = true), os))
            assertEquals(Shortcut.SEARCH, Keymap.match(KeyPress(Key.F, ctrl = true), os))
            assertEquals(Shortcut.PALETTE, Keymap.match(KeyPress(Key.K, ctrl = true), os))
            assertNull("⌘ は Windows・Linux では使わない", Keymap.match(KeyPress(Key.N, meta = true), os))
        }
        assertEquals(Shortcut.NEW, Keymap.match(KeyPress(Key.N, meta = true), mac))
        assertEquals(Shortcut.PALETTE, Keymap.match(KeyPress(Key.K, meta = true), mac))
        assertNull("macOS の Ctrl+N はカーソル移動(emacs)なので取らない", Keymap.match(KeyPress(Key.N, ctrl = true), mac))
    }

    @Test
    fun printKeepsCtrlPAndPaletteAvoidsIt() {
        assertEquals(Shortcut.PRINT, Keymap.match(KeyPress(Key.P, ctrl = true), win))
        assertEquals(Shortcut.PRINT, Keymap.match(KeyPress(Key.P, meta = true), mac))
        assertEquals(Shortcut.PALETTE, Keymap.match(KeyPress(Key.P, ctrl = true, shift = true), linux))
        // 全 OS で同じ組みが 2 つのショートカットに当たらない
        for (os in HostOs.entries) {
            val all = Shortcut.entries.flatMap { s -> Keymap.chords(s, os).map { it to s } }
            assertEquals(all.size, all.map { it.first }.toSet().size)
        }
    }

    @Test
    fun deleteFollowsPlatform() {
        assertEquals(Shortcut.DELETE, Keymap.match(KeyPress(Key.Delete), win))
        assertNull("Shift+Delete は切り取り", Keymap.match(KeyPress(Key.Delete, shift = true), win))
        assertNull("Backspace は文字を消すだけ", Keymap.match(KeyPress(Key.Backspace), win))
        assertEquals(Shortcut.DELETE, Keymap.match(KeyPress(Key.Backspace, meta = true), mac))
        assertNull(Keymap.match(KeyPress(Key.Backspace), mac))
        assertEquals(Shortcut.CLOSE, Keymap.match(KeyPress(Key.Escape), linux))
        assertEquals(Shortcut.HELP, Keymap.match(KeyPress(Key.F1), mac))
        assertNull("Alt が付いたら取らない", Keymap.match(KeyPress(Key.S, ctrl = true, alt = true), win))
    }

    @Test
    fun labelsFollowPlatform() {
        assertEquals("Ctrl+K", Keymap.label(Shortcut.PALETTE, win))
        assertEquals("Ctrl+N", Keymap.label(Shortcut.NEW, linux))
        assertEquals("⌘K", Keymap.label(Shortcut.PALETTE, mac))
        assertEquals("⇧⌘P", Keymap.label(Keymap.chords(Shortcut.PALETTE, mac)[1], mac))
        assertEquals("Ctrl+Shift+P", Keymap.label(Keymap.chords(Shortcut.PALETTE, win)[1], win))
        assertEquals("Delete", Keymap.label(Shortcut.DELETE, win))
        assertEquals("⌘⌫", Keymap.label(Shortcut.DELETE, mac))
        assertEquals("Esc", Keymap.label(Shortcut.CLOSE, mac))
    }

    // MARK: - OS ごとの表(#2619)

    @Test
    fun matchesTablePerOs() {
        val save = KeyChord(Key.S, primary = true)
        val palette = KeyChord(Key.P, primary = true, shift = true)
        val del = KeyChord(Key.Delete)
        val cmdBackspace = KeyChord(Key.Backspace, primary = true)
        // (組み, 押したキー, Windows, Linux, macOS)
        val table = listOf(
            Triple(save, KeyPress(Key.S, ctrl = true), listOf(true, true, false)),
            Triple(save, KeyPress(Key.S, meta = true), listOf(false, false, true)),
            // Ctrl と ⌘ を両方押したら取らない・何も押さなければ取らない
            Triple(save, KeyPress(Key.S, ctrl = true, meta = true), listOf(false, false, false)),
            Triple(save, KeyPress(Key.S), listOf(false, false, false)),
            // Shift の有無は合わせる
            Triple(save, KeyPress(Key.S, ctrl = true, shift = true), listOf(false, false, false)),
            Triple(save, KeyPress(Key.S, meta = true, shift = true), listOf(false, false, false)),
            Triple(palette, KeyPress(Key.P, ctrl = true, shift = true), listOf(true, true, false)),
            Triple(palette, KeyPress(Key.P, meta = true, shift = true), listOf(false, false, true)),
            Triple(palette, KeyPress(Key.P, ctrl = true), listOf(false, false, false)),
            // Alt が付いたら・キーが違えば取らない
            Triple(save, KeyPress(Key.S, ctrl = true, alt = true), listOf(false, false, false)),
            Triple(save, KeyPress(Key.S, meta = true, alt = true), listOf(false, false, false)),
            Triple(save, KeyPress(Key.N, ctrl = true), listOf(false, false, false)),
            // 修飾なしのキー(Delete)は Ctrl・⌘ が付いたら取らない
            Triple(del, KeyPress(Key.Delete), listOf(true, true, true)),
            Triple(del, KeyPress(Key.Delete, ctrl = true), listOf(false, false, false)),
            Triple(del, KeyPress(Key.Delete, meta = true), listOf(false, false, false)),
            Triple(cmdBackspace, KeyPress(Key.Backspace, meta = true), listOf(false, false, true)),
            Triple(cmdBackspace, KeyPress(Key.Backspace), listOf(false, false, false)),
        )
        for ((chord, press, want) in table) {
            listOf(win, linux, mac).zip(want).forEach { (os, w) -> assertEquals("$chord $press $os", w, Keymap.matches(chord, press, os)) }
        }
    }

    @Test
    fun deleteAndCmdBackspacePerOs() {
        // (押したキー, Windows・Linux, macOS)
        val table = listOf(
            KeyPress(Key.Delete) to (Shortcut.DELETE to Shortcut.DELETE),
            KeyPress(Key.Backspace, meta = true) to (null to Shortcut.DELETE),
            KeyPress(Key.Backspace, ctrl = true) to (null to null),
            KeyPress(Key.Backspace) to (null to null),
            KeyPress(Key.Delete, ctrl = true) to (null to null),
            KeyPress(Key.Delete, meta = true) to (null to null),
            KeyPress(Key.Delete, shift = true) to (null to null),
        )
        for ((press, want) in table) {
            assertEquals("$press win", want.first, Keymap.match(press, win))
            assertEquals("$press linux", want.first, Keymap.match(press, linux))
            assertEquals("$press mac", want.second, Keymap.match(press, mac))
        }
    }

    @Test
    fun labelTablePerOs() {
        // F1 の一覧に出す表記(最初の組み)。Windows と Linux は同じ
        val table = mapOf(
            Shortcut.PALETTE to ("Ctrl+K" to "⌘K"),
            Shortcut.NEW to ("Ctrl+N" to "⌘N"),
            Shortcut.SAVE to ("Ctrl+S" to "⌘S"),
            Shortcut.SEARCH to ("Ctrl+F" to "⌘F"),
            Shortcut.DELETE to ("Delete" to "⌘⌫"),
            Shortcut.CLOSE to ("Esc" to "Esc"),
            Shortcut.PRINT to ("Ctrl+P" to "⌘P"),
            Shortcut.HELP to ("F1" to "F1"),
            Shortcut.PREVIEW to ("Space" to "Space"),
        )
        assertEquals("全部のショートカットが表にある", Shortcut.entries.toSet(), table.keys)
        for ((s, want) in table) {
            assertEquals("$s win", want.first, Keymap.label(s, win))
            assertEquals("$s linux", want.first, Keymap.label(s, linux))
            assertEquals("$s mac", want.second, Keymap.label(s, mac))
        }
        // 2 つ目の組み
        assertEquals("Ctrl+/", Keymap.label(Keymap.chords(Shortcut.HELP, win)[1], win))
        assertEquals("⌘/", Keymap.label(Keymap.chords(Shortcut.HELP, mac)[1], mac))
        assertEquals("⌦", Keymap.label(Keymap.chords(Shortcut.DELETE, mac)[1], mac))
        assertEquals("Ctrl+Shift+S", Keymap.label(KeyChord(Key.S, primary = true, shift = true), linux))
        assertEquals("Shift+F1", Keymap.label(KeyChord(Key.F1, shift = true), win))
        assertEquals("⇧F1", Keymap.label(KeyChord(Key.F1, shift = true), mac))
    }

    @Test
    fun keyNamePerOs() {
        // (キー, Windows・Linux, macOS)
        val table = listOf(
            Key.Escape to ("Esc" to "Esc"),
            Key.Delete to ("Delete" to "⌦"),
            Key.Backspace to ("Backspace" to "⌫"),
            Key.F1 to ("F1" to "F1"),
            Key.Spacebar to ("Space" to "Space"),
            Key.Slash to ("/" to "/"),
            Key.K to ("K" to "K"),
            Key.N to ("N" to "N"),
            Key.P to ("P" to "P"),
            Key.S to ("S" to "S"),
            Key.F to ("F" to "F"),
        )
        for ((k, want) in table) {
            assertEquals("$k win", want.first, Keymap.keyName(k, win))
            assertEquals("$k linux", want.first, Keymap.keyName(k, linux))
            assertEquals("$k mac", want.second, Keymap.keyName(k, mac))
        }
        // 表に無いキーは Compose の名前のまま(OS で変わらない)
        assertEquals(Key.A.toString(), Keymap.keyName(Key.A, win))
        assertEquals(Keymap.keyName(Key.A, win), Keymap.keyName(Key.A, mac))
    }

    // MARK: - パレットの絞り込み

    private fun cmd(title: String, kind: PaletteCommand.Kind = PaletteCommand.Kind.ACTION, keywords: List<String> = emptyList()) =
        PaletteCommand(title, title, kind, keywords) {}

    private fun titles(cs: List<PaletteCommand>, q: String) = CommandRanking.rank(cs, q).map { it.title }

    @Test
    fun emptyQueryKeepsOrder() {
        val cs = listOf(cmd("保存"), cmd("新規"), cmd("印刷"))
        assertEquals(listOf("保存", "新規", "印刷"), titles(cs, ""))
        assertEquals(listOf("保存", "新規", "印刷"), titles(cs, "  "))
    }

    @Test
    fun exactBeatsPrefixBeatsWordBeatsContains() {
        val cs = listOf(cmd("買い物リストの設定"), cmd("リスト 買い物"), cmd("買い物"), cmd("買い物リスト"), cmd("家計簿"))
        assertEquals(listOf("買い物", "買い物リストの設定", "買い物リスト", "リスト 買い物"), titles(cs, "買い物"))
        // 「家計簿」は合わない
        assertFalse("家計簿" in titles(cs, "買い物"))
    }

    @Test
    fun earlierMatchRanksHigherAndTiesKeepOrder() {
        val cs = listOf(cmd("今週の支出"), cmd("支出"), cmd("月の支出"))
        assertEquals(listOf("支出", "月の支出", "今週の支出"), titles(cs, "支出"))
        val same = listOf(cmd("Bxa"), cmd("Cxa"))
        assertEquals(listOf("Bxa", "Cxa"), titles(same, "xa"))
    }

    @Test
    fun widthCaseAndKanaAreFolded() {
        val cs = listOf(cmd("ショッピング"), cmd("Shopping List"), cmd("ＡＢＣ"))
        assertEquals(listOf("ショッピング"), titles(cs, "しょっぴんぐ"))
        assertEquals(listOf("Shopping List"), titles(cs, "LIST"))
        assertEquals(listOf("ＡＢＣ"), titles(cs, "abc"))
    }

    @Test
    fun keywordsAndSubsequenceComeLast() {
        val cs = listOf(cmd("買うもの", PaletteCommand.Kind.SCREEN, listOf("item_form")), cmd("item list"), cmd("インポート"))
        // タイトルに含む物 > 別名
        assertEquals(listOf("item list", "買うもの"), titles(cs, "item"))
        // 文字が順に現れる(間が詰まっているほど上)
        assertEquals(listOf("item list"), titles(cs, "itls"))
        assertTrue(titles(cs, "zzz").isEmpty())
    }

    // MARK: - shopping をキーボードだけで

    private fun shopping(): Pair<DesktopSession, DesktopKeyboard> {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        return s to DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
    }

    @Test
    fun screenShortcutsPickTheScreensButtons() {
        val dsl = Examples.load("shopping")
        val home = dsl.screen("home")!!
        val form = dsl.screen("item_form")!!
        assertEquals("item_form", ScreenShortcuts.newAction(dsl, home)?.screen)
        assertNull("フォームの画面には新規が無い", ScreenShortcuts.newAction(dsl, form))
        assertNull(ScreenShortcuts.deleteAction(home, null))
        assertTrue(ScreenShortcuts.hasSearch(home).not())
    }

    @Test
    fun addSaveDeleteWithoutMouse() {
        val (s, k) = shopping()
        // Ctrl+N: 「追加」の sheet が開く
        assertTrue(k.run(Shortcut.NEW))
        assertEquals("item_form", s.top.screen)
        assertEquals(Route.SHEET, s.top.stack)
        // 新規フォームには削除が無い(Delete は他へ流す)
        assertFalse(k.run(Shortcut.DELETE))
        // Ctrl+S: 入れずに保存 → 品名が要る
        assertTrue(k.run(Shortcut.SAVE))
        assertNotNull("要る項目が足りない旨", s.message)
        s.form(s.top)!!.values["name"] = JSONValue.Str("牛乳")
        assertTrue(k.run(Shortcut.SAVE))
        assertEquals("home", s.top.screen)
        assertEquals(listOf("牛乳"), s.records("groceries").map { it.data["name"]?.stringValue })

        // 行を開いて(Enter)Delete → 確認付きの削除
        assertTrue(s.apply(JsonObject(mapOf("tapRow" to JsonObject(mapOf("index" to JsonPrimitive(0)))))))
        assertEquals("item_form", s.top.screen)
        var confirmed: String? = null
        k.dispatch = { a, ctx -> confirmed = a.confirmMessage; s.dispatch(a, ctx) }
        assertTrue(k.run(Shortcut.DELETE))
        assertEquals("この品目を削除しますか？", confirmed)
        assertTrue(s.records("groceries").isEmpty())
        assertEquals("home", s.top.screen)
    }

    @Test
    fun escClosesAndFallsThroughAtRoot() {
        val (s, k) = shopping()
        assertFalse("一番下の画面では Esc を取らない", k.run(Shortcut.CLOSE))
        k.run(Shortcut.NEW)
        assertTrue(k.run(Shortcut.CLOSE))
        assertEquals("home", s.top.screen)
        assertTrue(k.run(Shortcut.SAVE).also { assertEquals(KeyboardText.NO_SAVE, s.message) })
        assertTrue(k.run(Shortcut.SEARCH))
        assertTrue("検索欄が無ければパレットを開く", k.paletteOpen)
    }

    @Test
    fun listDeleteGetsAConfirmation() {
        val a = ScreenShortcuts.withConfirm(jp.kz2601.metamon.dsl.Action(type = ActionType.DELETE_RECORD, entity = "groceries"))
        assertEquals(KeyboardText.DELETE_CONFIRM, a.confirmMessage)
    }

    @Test
    fun paletteListsActionsScreensAndApps() {
        val (s, k) = shopping()
        var opened: String? = null
        k.openApp = { opened = it }
        val cs = k.commands()
        assertTrue(cs.any { it.kind == PaletteCommand.Kind.SCREEN && it.title == "買うもの" })
        assertTrue(cs.any { it.kind == PaletteCommand.Kind.ACTION && it.title == "追加" })
        assertEquals("Ctrl+N", cs.first { it.id == "shortcut:NEW" }.hint)
        val apps = cs.filter { it.kind == PaletteCommand.Kind.APP }
        assertTrue(apps.size > 5)
        // アプリ名で選ぶ: 家計簿のアプリ名で一番上に来る
        val kakeibo = CommandRanking.rank(apps, Examples.load("kakeibo").app.name).first()
        kakeibo.run()
        assertEquals("kakeibo", opened)
        // 画面を選ぶと push で開く
        CommandRanking.rank(cs, "買うもの").first { it.kind == PaletteCommand.Kind.SCREEN }.run()
        assertEquals("item_form", s.top.screen)
        assertNotNull(s.form(s.top))
    }

    @Test
    fun bundledAppsListTheExamples() {
        val ids = BundledApps.ids()
        assertTrue("shopping" in ids)
        assertTrue("kakeibo" in ids)
        assertEquals(ids.sorted(), ids)
    }
}
