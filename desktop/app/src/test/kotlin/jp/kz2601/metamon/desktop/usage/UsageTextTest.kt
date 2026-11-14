package jp.kz2601.metamon.desktop.usage

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.MyUsage
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/** `GET /v1/me/usage` の文(#2812): 無料・Pro・上限に達した・数が無い。 */
class UsageTextTest {
    private val jst = ZoneId.of("Asia/Tokyo")
    private val now = Instant.parse("2026-11-13T03:00:00Z")

    private fun usage(json: String) = MyUsage.decode(json)

    @Test
    fun freeShowsRemainingAndAppsWithFreeFooter() {
        val u = usage("""{"plan":"free","resetAt":"2026-11-13T15:00:00Z","ai":{"used":57,"limit":60,"remaining":3},"apps":{"used":3,"limit":5}}""")
        Strings.withLang(Lang.JA) {
            val s = UsageText.of(u, now, zone = jst)
            assertFalse(s.pro)
            assertFalse(s.exhausted)
            assertEquals("今日の残り 3", s.remaining)
            assertEquals(UsageText.Meter("作ったアプリ", "3 / 5", near = false), s.apps)
            assertEquals("使わないものを消すと、空きが増えます", s.footer)
        }
        Strings.withLang(Lang.EN) {
            assertEquals("3 left today", UsageText.of(u, now, zone = jst).remaining)
        }
    }

    @Test
    fun proUsesProFooterAndNoLimitText() {
        val u = usage("""{"plan":"pro","ai":{"used":1,"limit":300,"remaining":299},"apps":{"used":42}}""")
        Strings.withLang(Lang.JA) {
            val s = UsageText.of(u, now, zone = jst)
            assertTrue(s.pro)
            assertEquals("今日の残り 299", s.remaining)
            assertEquals("42(上限なし)", s.apps?.amount)
            assertEquals("Pro プランの上限です", s.footer)
        }
        Strings.withLang(Lang.EN) {
            assertEquals("42 (no limit)", UsageText.of(u, now, zone = jst).apps?.amount)
        }
    }

    @Test
    fun exhaustedSaysWhenItComesBack() {
        val u = usage("""{"plan":"free","resetAt":"2026-11-13T15:00:00Z","ai":{"used":60,"limit":60,"remaining":0},"apps":{"used":5,"limit":5}}""")
        Strings.withLang(Lang.JA) {
            val s = UsageText.of(u, now, zone = jst)
            assertTrue(s.exhausted)
            assertEquals("今日の上限に達しました。11月14日 0:00からまた使えます", s.remaining)
            assertTrue(s.apps!!.near)
        }
        Strings.withLang(Lang.EN) {
            assertEquals("You've reached today's limit. You can continue from Nov 14, 0:00", UsageText.of(u, now, zone = jst).remaining)
        }
    }

    @Test
    fun exhaustedWithoutResetSaysTomorrow() {
        val u = usage("""{"plan":"free","ai":{"used":60,"limit":60,"remaining":0}}""")
        Strings.withLang(Lang.JA) {
            assertEquals("今日の上限に達しました。明日からまた使えます", UsageText.of(u, now, zone = jst).remaining)
        }
    }

    @Test
    fun noCountsGivesNothing() {
        Strings.withLang(Lang.JA) {
            val s = UsageText.of(usage("""{"plan":"free"}"""), now, zone = jst)
            assertNull(s.remaining)
            assertNull(s.apps)
            assertNull(s.footer)
            assertFalse(s.exhausted)
        }
    }

    @Test
    fun staleValueAfterResetIsHidden() {
        val u = usage("""{"plan":"free","resetAt":"2026-11-13T15:00:00Z","ai":{"used":57,"limit":60,"remaining":3}}""")
        val later = Instant.parse("2026-11-13T16:00:00Z")
        Strings.withLang(Lang.JA) { assertNull(UsageText.of(u, now, later, jst).remaining) }
    }
}
