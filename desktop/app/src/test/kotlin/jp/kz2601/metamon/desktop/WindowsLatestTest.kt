package jp.kz2601.metamon.desktop

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Test

/** 同じアプリの一番新しいウィンドウ(WindowRegistry.latestAppWindow)と、無い時に開く(quickRecordWindow)。画面は出さない。 */
class WindowsLatestTest {
    @Test
    fun latestAppWindowPicksNewest() {
        val r = WindowRegistry()
        // open は同じアプリを 1 枚にまとめるので、2 枚ある状態は一覧に直に足して作る
        val first = WindowEntry(101, WindowKey.app("shopping"))
        val second = WindowEntry(102, WindowKey.app("shopping"))
        r.windows += first
        r.windows += second
        // まだどちらも前に出ていなければ一覧の先の方
        assertSame(first, r.latestAppWindow("shopping"))
        r.activate(first.id)
        r.activate(second.id)
        assertSame(second, r.latestAppWindow("shopping"))
        r.activate(first.id)
        assertSame(first, r.latestAppWindow("shopping"))
    }

    @Test
    fun latestAppWindowSkipsRecordWindowsAndOtherApps() {
        val r = WindowRegistry()
        val app = r.open(WindowKey.app("shopping"))
        r.open(WindowKey.app("kakeibo"))
        val detail = r.open(WindowKey.record("shopping", "item_form", "a"))
        assertEquals(detail.id, r.active?.id)
        // 詳細のウィンドウは数えない
        assertSame(app, r.latestAppWindow("shopping"))
        assertNull(r.latestAppWindow("health"))
        r.close(app.id)
        assertNull("詳細だけ残っても null", r.latestAppWindow("shopping"))
    }

    @Test
    fun quickRecordOpensTheAppWindowWhenMissing() {
        val r = WindowRegistry()
        assertNull("ウィンドウが無ければ行き先も無い", r.quickRecordWindow())
        val detail = r.open(WindowKey.record("kakeibo", "expense_form", "a"))
        val opened = r.quickRecordWindow()!!
        assertEquals(WindowKey.app("kakeibo"), opened.key)
        assertEquals(2, r.windows.size)
        assertEquals(opened.id, r.active?.id)
        // 2 回目は開いた物を前に出すだけ
        r.activate(detail.id)
        assertSame(opened, r.quickRecordWindow())
        assertEquals(2, r.windows.size)
        assertEquals(opened.id, r.active?.id)
    }
}
