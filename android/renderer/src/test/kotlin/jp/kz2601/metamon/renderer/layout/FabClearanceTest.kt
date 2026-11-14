package jp.kz2601.metamon.renderer.layout

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import jp.kz2601.metamon.renderer.screen.FabCoordinator
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** #1204: 最下段の部品を FAB が避ける・背の高い list の末尾に FAB ぶんの余白(iOS の `BugBashLayoutTests` / `FabContentInsetTests`)。 */
class FabClearanceTest {
    private fun areas(vararg s: String) = s.map { requireNotNull(GridArea.parse(it)) }

    @Test
    fun obstructedRowsLeading() {
        // 家計簿ホーム: 右下の追加ボタンは FAB(左下)と共存 → 退避なし
        assertEquals(0, FabClearance.obstructedRows(areas("A1:F2", "A3:F10", "D11:F12")))
        // 筋トレホーム: 全幅の保存ボタンが最下段 → 2 行ぶん上へ
        assertEquals(2, FabClearance.obstructedRows(areas("A1:F2", "A3:F7", "A8:F10", "A11:F12")))
        assertEquals(1, FabClearance.obstructedRows(areas("A12:F12")))
        // B 列始まりも FAB に掛かる / C 列始まりは掛からない
        assertEquals(2, FabClearance.obstructedRows(areas("B11:F12")))
        assertEquals(0, FabClearance.obstructedRows(areas("C11:F12")))
        // 全面 list や行 10 で終わる部品は背景扱い
        assertEquals(0, FabClearance.obstructedRows(areas("A1:F12")))
        assertEquals(0, FabClearance.obstructedRows(areas("A3:F10")))
        // 複数あれば最も上から始まるもの
        assertEquals(3, FabClearance.obstructedRows(areas("A12:C12", "A10:F12")))
        assertEquals(0, FabClearance.obstructedRows(emptyList()))
    }

    @Test
    fun obstructedRowsTrailing() {
        assertEquals(2, FabClearance.obstructedRows(areas("D11:F12"), leading = false))
        assertEquals(0, FabClearance.obstructedRows(areas("A11:D12"), leading = false))
    }

    // iPhone 幅の全幅 list(y 100〜700)
    private val list = Rect(14f, 100f, 376f, 700f)
    private val gap = 8f
    private val minVisible = 32f

    private fun fab(x: Float, y: Float) = Rect(Offset(x, y), Size(60f, 60f))

    private fun margin(viewport: Rect, fab: Rect?) = FabClearance.contentMargin(viewport, fab, gap, minVisible)

    @Test
    fun contentMarginMatchesIOS() {
        assertEquals(0f, margin(list, null))
        // 左下の FAB が list の下端に 40 食い込む → 40 + gap。右に吸着していても横に重なれば同じ
        assertEquals(40f + gap, margin(list, fab(24f, 660f)))
        assertEquals(40f + gap, margin(list, fab(306f, 660f)))
        // list より下・横に重ならない・list より上
        assertEquals(0f, margin(list, fab(24f, 720f)))
        assertEquals(0f, margin(Rect(195f, 100f, 376f, 700f), fab(24f, 660f)))
        assertEquals(0f, margin(list, fab(24f, 20f)))
        // 低い list を FAB が覆う → 0。上に 32 以上残るなら足す
        val short = Rect(14f, 600f, 376f, 706f)
        assertEquals(0f, margin(short, fab(24f, 590f)))
        assertEquals(46f + gap, margin(short, fab(24f, 660f)))
        // FAB を途中へ動かした → その上端まで
        assertEquals(300f + gap, margin(list, fab(24f, 400f)))
        // まだ測れていない器
        assertEquals(0f, margin(Rect.Zero, fab(24f, 660f)))
    }

    @Test
    fun coordinatorKeepsHighestObstructionPerSideAndForgets() {
        val c = FabCoordinator()
        c.report("home", leadingTop = 700f, trailingTop = null)
        c.report("tab2", leadingTop = 650f, trailingTop = 720f)
        assertEquals(650f, c.obstructionTop(leadingSide = true))
        assertEquals(720f, c.obstructionTop(leadingSide = false))
        c.forget("tab2")
        assertEquals(700f, c.obstructionTop(leadingSide = true))
        assertNull(c.obstructionTop(leadingSide = false))
    }
}
