package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.theme.MemoryFloatSetting
import jp.kz2601.metamon.desktop.theme.TextZoom
import jp.kz2601.metamon.desktop.theme.TextZoomState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #2208: Ctrl + / Ctrl - / Ctrl 0 の文字の倍率(上げ下げ・上限・保存)・キー・倍率 2.0 のグリッド。 */
class TextZoom2208Test {
    private val win = HostOs.WINDOWS
    private val mac = HostOs.MAC

    @Test
    fun stepsByTenthWithinBounds() {
        assertEquals(1.1f, TextZoom.bigger(1.0f), 1e-6f)
        assertEquals(0.9f, TextZoom.smaller(1.0f), 1e-6f)
        assertEquals(2.0f, TextZoom.bigger(2.0f), 1e-6f)
        assertEquals(0.8f, TextZoom.smaller(0.8f), 1e-6f)
        // 0.1 の足し算の誤差が溜まらない
        var v = TextZoom.DEFAULT
        repeat(20) { v = TextZoom.bigger(v) }
        assertEquals(TextZoom.MAX, v, 0f)
        repeat(20) { v = TextZoom.smaller(v) }
        assertEquals(TextZoom.MIN, v, 0f)
    }

    @Test
    fun sanitizeClampsBrokenSavedValues() {
        assertEquals(1f, TextZoom.sanitize(Float.NaN), 0f)
        assertEquals(1f, TextZoom.sanitize(Float.POSITIVE_INFINITY), 0f)
        assertEquals(2f, TextZoom.sanitize(9f), 0f)
        assertEquals(0.8f, TextZoom.sanitize(0.1f), 0f)
        assertEquals(1.3f, TextZoom.sanitize(1.27f), 1e-6f)
        assertEquals("120%", TextZoom.percent(1.2f))
    }

    @Test
    fun stateSavesAndRestores() {
        val setting = MemoryFloatSetting(1f)
        val z = TextZoomState(setting)
        z.zoomIn()
        z.zoomIn()
        assertEquals(1.2f, setting.get(), 1e-6f)
        // 次の起動: 残した値から始まる
        val next = TextZoomState(setting)
        assertEquals(1.2f, next.scale, 1e-6f)
        next.reset()
        assertEquals(1f, setting.get(), 0f)
        // 壊れた値は既定に
        assertEquals(1f, TextZoomState(MemoryFloatSetting(Float.NaN)).scale, 0f)
    }

    @Test
    fun keysFollowPlatformAndLayouts() {
        assertEquals(Shortcut.ZOOM_IN, Keymap.match(KeyPress(Key.Plus, ctrl = true), win))
        assertEquals("US 配列の Ctrl+=", Shortcut.ZOOM_IN, Keymap.match(KeyPress(Key.Equals, ctrl = true), win))
        assertEquals("US 配列の Ctrl+Shift+=", Shortcut.ZOOM_IN, Keymap.match(KeyPress(Key.Equals, ctrl = true, shift = true), win))
        assertEquals("JIS 配列の Ctrl+Shift+;", Shortcut.ZOOM_IN, Keymap.match(KeyPress(Key.Semicolon, ctrl = true, shift = true), win))
        assertEquals(Shortcut.ZOOM_IN, Keymap.match(KeyPress(Key.NumPadAdd, ctrl = true), win))
        assertEquals(Shortcut.ZOOM_OUT, Keymap.match(KeyPress(Key.Minus, ctrl = true), win))
        assertEquals(Shortcut.ZOOM_OUT, Keymap.match(KeyPress(Key.NumPadSubtract, ctrl = true), win))
        assertEquals(Shortcut.ZOOM_RESET, Keymap.match(KeyPress(Key.Zero, ctrl = true), win))
        assertEquals(Shortcut.ZOOM_IN, Keymap.match(KeyPress(Key.Equals, meta = true), mac))
        assertNull("macOS で Ctrl は取らない", Keymap.match(KeyPress(Key.Minus, ctrl = true), mac))
        assertNull("修飾なしの - は文字", Keymap.match(KeyPress(Key.Minus), win))
        assertEquals("Ctrl++", Keymap.label(Shortcut.ZOOM_IN, win))
        assertEquals("⌘-", Keymap.label(Shortcut.ZOOM_OUT, mac))
        assertEquals("Ctrl+0", Keymap.label(Shortcut.ZOOM_RESET, win))
    }

    @Test
    fun runChangesZoomAndAnnouncesIt() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, win)
        val setting = MemoryFloatSetting(1f)
        k.textZoom = TextZoomState(setting)
        assertTrue(k.run(Shortcut.ZOOM_IN))
        assertEquals(1.1f, k.textZoom.scale, 1e-6f)
        assertTrue(s.message!!.contains("110%"))
        assertTrue(k.run(Shortcut.ZOOM_OUT))
        assertTrue(k.run(Shortcut.ZOOM_OUT))
        assertEquals(0.9f, setting.get(), 1e-6f)
        assertTrue(k.run(Shortcut.ZOOM_RESET))
        assertEquals(1f, k.textZoom.scale, 0f)
        // F1 の一覧とパレットに出る
        assertTrue(k.commands().any { it.id == "shortcut:ZOOM_IN" && it.hint == "Ctrl++" })
    }

    @Test
    fun gridRowsGrowWithZoomAndHoldAtTwo() {
        val base = DesktopGridMetrics.resolve(480f, 400f, 16f)
        val big = DesktopGridMetrics.resolve(480f, 400f, 16f, fontScale = 2f)
        assertEquals("幅は変わらない", base.cellW, big.cellW, 0f)
        assertEquals(DesktopGridMetrics.MIN_CELL_HEIGHT * 2, big.cellH, 1e-3f)
        // 1 行の文字(bodyLarge の行の高さ 24sp)と上下の余白 8dp が倍率 2.0 でも 1 行の段に収まる
        assertTrue(big.cellH >= 24f * 2 + 16f)
        // 高いウィンドウでは上限も倍率ぶん伸びる
        assertEquals(DesktopGridMetrics.MAX_CELL_HEIGHT * 2, DesktopGridMetrics.resolve(480f, 4000f, 16f, 2f).cellH, 1e-3f)
        // 壊れた倍率は 1 扱い
        assertEquals(base.cellH, DesktopGridMetrics.resolve(480f, 400f, 16f, Float.NaN).cellH, 0f)
    }
}
