package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.services.social.AuthorProfile
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 共有の投稿に出す表示名(android/services の AuthorProfile を desktop の保存で使う・#2976)。 */
class AuthorProfileDesktopTest {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun trimsSpacesAndCutsAtTheLimit() {
        val p = AuthorProfile(JsonKeyValueStore(null))
        p.name = "  たろう  "
        assertEquals("たろう", p.name)
        p.name = "あ".repeat(AuthorProfile.MAX_LENGTH + 10)
        assertEquals("あ".repeat(AuthorProfile.MAX_LENGTH), p.name)
        p.name = "   "
        assertNull("空白だけは送らない", p.name)
    }

    @Test
    fun clearGoesBackToEmpty() {
        val p = AuthorProfile(JsonKeyValueStore(null))
        p.answer("はなこ")
        assertEquals("はなこ", p.name)
        assertTrue(p.asked)
        p.clear()
        assertNull(p.name)
        assertEquals(false, p.asked)
    }

    @Test
    fun survivesRestartAndIsWipedWithTheDeviceData() = runBlocking {
        val root = tmp.newFolder()
        DesktopStore(root).authorProfile.name = "じろう"
        val reopened = DesktopStore(root)
        assertEquals("じろう", reopened.authorProfile.name)
        reopened.wipe()
        assertNull(reopened.authorProfile.name)
        assertNull("開き直しても戻らない", DesktopStore(root).authorProfile.name)
    }

    @Test
    fun noOsUserNameIsUsedAsADefault() {
        assertNull(AuthorProfile(JsonKeyValueStore(null)).name)
    }

    @Test
    fun stringsResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        for (lang in Lang.entries) {
            for (k in listOf("settings_author_name_header", "settings_author_name_placeholder", "settings_author_name_footer")) {
                assertTrue("$k ($lang)", t.hasString(lang, k))
            }
        }
    }
}
