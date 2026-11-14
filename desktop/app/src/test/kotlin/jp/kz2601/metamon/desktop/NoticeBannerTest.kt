package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.DesktopNotice
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.services.update.ClientConfig
import jp.kz2601.metamon.services.update.NoticeDismissals
import jp.kz2601.metamon.services.update.ServiceNotice
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** アプリ内のお知らせの帯: 出す物(文面・印・読み上げ・リンク)と、閉じたら出さない(#2975)。 */
class NoticeBannerTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val info = ServiceNotice(id = "n1", ja = "メンテナンスがあります", en = "Maintenance ahead", linkUrl = "https://example.com/status")
    private val warning = ServiceNotice(id = "n2", severity = ServiceNotice.Severity.WARNING, ja = "障害が起きています")

    @Test
    fun infoReadsAsOneNoticeWithLink() {
        val m = NoticeBannerModel.of(info, Lang.JA)
        assertEquals("メンテナンスがあります", m.text)
        assertEquals("お知らせ: メンテナンスがあります", m.description)
        assertEquals("お知らせ", m.tag)
        assertFalse(m.warning)
        assertEquals("https://example.com/status", m.link)
    }

    @Test
    fun englishFallsBackToJapaneseWhenNoEnglishText() = Strings.withLang(Lang.EN) {
        assertEquals("Maintenance ahead", NoticeBannerModel.of(info, Lang.EN).text)
        assertEquals("障害が起きています", NoticeBannerModel.of(warning, Lang.EN).text)
    }

    @Test
    fun warningIsMarkedByWordNotOnlyColor() {
        val info = NoticeBannerModel.of(info, Lang.JA)
        val warn = NoticeBannerModel.of(warning, Lang.JA)
        assertTrue(warn.warning)
        assertEquals("重要", warn.tag)
        assertNotEquals(info.tag, warn.tag)
        assertEquals("重要なお知らせ: 障害が起きています", warn.description)
        assertNull("リンクの無い知らせに「詳しく」は出さない", warn.link)
    }

    @Test
    fun onlyHttpsLinksAreOffered() {
        for (bad in listOf("http://example.com", "javascript:alert(1)", "file:///etc/passwd", "https://")) {
            assertNull(bad, NoticeBannerModel.of(info.copy(linkUrl = bad), Lang.JA).link)
        }
    }

    @Test
    fun dismissedIdIsNotShownAgainAfterRestart() {
        val file = tmp.newFolder().resolve("settings.json")
        val config = ClientConfig(notice = info)
        assertEquals(info, DesktopNotice.toShow(config, NoticeDismissals(JsonKeyValueStore(file))))
        NoticeDismissals(JsonKeyValueStore(file)).dismiss("n1")
        // 次の起動(同じファイルを開き直す)でも出さない・別の id の知らせは出す
        val restarted = NoticeDismissals(JsonKeyValueStore(file))
        assertNull(DesktopNotice.toShow(config, restarted))
        assertEquals(warning, DesktopNotice.toShow(ClientConfig(notice = warning), restarted))
    }

    @Test
    fun stringsResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        for (lang in Lang.entries) {
            for (k in listOf("notice_a11y_info", "notice_a11y_warning", "notice_dismiss", "notice_learn_more", "desktop_notice_tag_info", "desktop_notice_tag_warning")) {
                assertTrue("$k ($lang)", t.hasString(lang, k))
            }
        }
    }
}
