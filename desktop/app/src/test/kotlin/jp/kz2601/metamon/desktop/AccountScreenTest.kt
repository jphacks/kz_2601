package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.URI

/** アカウントの画面の下のリンク(#2505): 並び・URL・開き方・文言・パレットの入り口。 */
class AccountScreenTest {
    @Test
    fun linksInOrderWithTheSiteUrls() {
        assertEquals(listOf(AccountLink.TERMS, AccountLink.PRIVACY, AccountLink.LICENSES, AccountLink.CONTACT), AccountLink.entries)
        assertEquals("https://tailor-it.app/terms", AccountLink.TERMS.url)
        assertEquals("https://tailor-it.app/privacy", AccountLink.PRIVACY.url)
        assertEquals("https://tailor-it.app/contact", AccountLink.CONTACT.url)
        assertNull("ライセンスはアプリの中で開く", AccountLink.LICENSES.url)
        AccountLink.entries.mapNotNull { it.url }.forEach { assertTrue(it, it.startsWith(SiteUrls.SITE + "/")) }
    }

    @Test
    fun titlesInJapaneseAndEnglish() {
        assertEquals(listOf("利用規約", "プライバシーポリシー", "ライセンス", "お問い合わせ"), Strings.withLang(Lang.JA) { AccountLink.entries.map { it.title } })
        val table = StringTable.fromClasspath()
        val keys = AccountLink.entries.map { it.titleKey } + listOf("account_menu", "desktop_close", "desktop_licenses_browse_failed")
        for (lang in Lang.entries) for (key in keys) assertTrue("$lang $key", table.hasString(lang, key))
    }

    @Test
    fun urlsOpenInTheBrowserAndLicensesInTheApp() {
        val opened = mutableListOf<URI>()
        var licenses = 0
        assertEquals(AccountLink.Opened.BROWSER, AccountLink.PRIVACY.open({ licenses++ }) { opened += it; true })
        assertEquals(listOf(URI("https://tailor-it.app/privacy")), opened)
        assertEquals(AccountLink.Opened.IN_APP, AccountLink.LICENSES.open({ licenses++ }) { opened += it; true })
        assertEquals(1, licenses)
        assertEquals("ライセンスはブラウザを開かない", 1, opened.size)
        assertEquals(AccountLink.Opened.FAILED, AccountLink.CONTACT.open({ licenses++ }) { false })
    }

    @Test
    fun paletteOpensTheAccountScreen() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        val command = k.commands().single { it.id == "about:account" }
        assertEquals("アカウント", command.title)
        assertEquals(command.id, CommandRanking.rank(k.commands(), "privacy").first().id)
        assertFalse(k.accountOpen)
        command.run()
        assertTrue(k.accountOpen)
    }
}
