package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.VisibleTo
import jp.kz2601.metamon.renderer.components.isButtonVisible
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** button の visibleTo "admins"(#272・iOS の ButtonLeaf.isVisible(visibleTo:isAdmin:))。 */
class ButtonVisibleToTest {
    @Test
    fun adminsOnlyHidesOnlyWhenKnownNotAdmin() {
        assertFalse("管理者でないと分かっている時だけ隠す", isButtonVisible(VisibleTo.ADMINS, isAdmin = false))
        assertTrue(isButtonVisible(VisibleTo.ADMINS, isAdmin = true))
        assertTrue("分からない間は出す(サーバーが判定する)", isButtonVisible(VisibleTo.ADMINS, isAdmin = null))
        assertTrue(isButtonVisible(null, isAdmin = false))
        assertTrue("知らない値は出す", isButtonVisible(VisibleTo.UNKNOWN, isAdmin = false))
    }

    @Test
    fun decodesVisibleTo() {
        val c = DslJson.decodeFromString(Component.serializer(), """{"type":"button","label":"確定","visibleTo":"admins","action":{"type":"saveRecord"}}""")
        assertEquals(VisibleTo.ADMINS, c.visibleTo)
        assertNull("持たないホストは分からない扱い", NoopDSLRuntime.viewerIsAdmin)
    }
}
