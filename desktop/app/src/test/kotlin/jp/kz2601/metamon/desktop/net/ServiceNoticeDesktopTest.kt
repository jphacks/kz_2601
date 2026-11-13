package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.services.update.ClientConfig
import jp.kz2601.metamon.services.update.NoticeDismissals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/** client-config の notice(#333)をデスクトップで出す / 出さない(android/services の ServiceNotice を共有)。 */
class ServiceNoticeDesktopTest {
    private fun dismissals() = NoticeDismissals(JsonKeyValueStore(null))

    private fun config(notice: String) = ClientConfig.decode("""{"notice":$notice}""")

    @Test
    fun englishFallsBackToJapaneseWithoutEn() {
        val notice = DesktopNotice.toShow(config("""{"id":"m1","message":{"ja":"メンテナンス中"}}"""), dismissals())!!
        assertEquals("メンテナンス中", notice.text("en"))
        assertEquals("メンテナンス中", notice.text("ja"))
        val both = DesktopNotice.toShow(config("""{"id":"m2","message":{"ja":"障害","en":"Outage"}}"""), dismissals())!!
        assertEquals("Outage", both.text("en-US"))
        assertEquals("障害", both.text("ja"))
    }

    @Test
    fun dismissedIdIsNotShownAgain() {
        val store = dismissals()
        val cfg = config("""{"id":"m1","message":{"ja":"お知らせ"}}""")
        assertNotNull(DesktopNotice.toShow(cfg, store))
        store.dismiss("m1")
        assertNull(DesktopNotice.toShow(cfg, store))
        // 新しい文面は新しい id で出る
        assertNotNull(DesktopNotice.toShow(config("""{"id":"m2","message":{"ja":"別のお知らせ"}}"""), store))
    }

    @Test
    fun configWithoutNoticeDoesNotCrash() {
        assertNull(DesktopNotice.toShow(ClientConfig.decode("{}"), dismissals()))
        assertNull(DesktopNotice.toShow(ClientConfig.decode("""{"minIOSBuild":10,"notice":null}"""), dismissals()))
        assertNull(DesktopNotice.toShow(config("""{"id":"bad"}"""), dismissals()))
        assertNull(DesktopNotice.toShow(config("\"text\""), dismissals()))
        assertNull(DesktopNotice.toShow(null, dismissals()))
    }

    @Test
    fun iosBuildRangedNoticeIsNotShown() {
        assertNull(DesktopNotice.toShow(config("""{"id":"ios","minIOSBuild":100,"message":{"ja":"iOS 向け"}}"""), dismissals()))
    }
}
