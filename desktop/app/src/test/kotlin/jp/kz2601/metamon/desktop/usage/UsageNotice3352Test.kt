package jp.kz2601.metamon.desktop.usage

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.UsageMeter
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.time.Instant

/** ウィンドウの上の「上限に近い」1 行の帯の出す / 出さない・1 日 1 回・文言(#3352)。 */
class UsageNotice3352Test {
    private val near = UsageMeter(used = 8, limit = 10)
    private val full = UsageMeter(used = 10, limit = 10)
    private val low = UsageMeter(used = 3, limit = 10)

    // 2026-11-13 10:00 JST / 23:59 JST / 翌 0:01 JST
    private val morning = Instant.parse("2026-11-13T01:00:00Z")
    private val lateNight = Instant.parse("2026-11-13T14:59:00Z")
    private val nextDay = Instant.parse("2026-11-13T15:01:00Z")

    private fun mine(records: UsageMeter? = null, photos: UsageMeter? = null) =
        AppUsage(appId = "app1", payer = "self", records = records, photos = photos)

    @Test
    fun appNoticesOnlyForTheCreatorAndAtEightyPercent() {
        assertEquals(listOf(UsageResource.RECORDS_PER_APP, UsageResource.PHOTOS_PER_APP), UsageNoticeRules.appNotices(mine(near, full)).map { it.resource })
        assertTrue("80% 未満は出さない", UsageNoticeRules.appNotices(mine(low, low)).isEmpty())
        assertTrue("上限なしは出さない", UsageNoticeRules.appNotices(mine(UsageMeter(used = 999))).isEmpty())
        assertTrue("参加者には出さない", UsageNoticeRules.appNotices(mine(near).copy(payer = "creator")).isEmpty())
        assertTrue(UsageNoticeRules.appNotices(null).isEmpty())
        assertEquals("recordsPerApp:app1", UsageNoticeRules.appNotices(mine(near)).single().id)
    }

    @Test
    fun oncePerDayInJstPerUserAndDismissCountsAsShown() {
        val log = UsageNoticeLog(JsonKeyValueStore(null))
        val n = UsageNotice(UsageResource.RECORDS_PER_APP, near, "app1")
        assertTrue(log.shouldShow(n, "u1", morning))
        log.markShown(n, "u1", morning)
        assertFalse("同じ日(JST)はもう出さない", log.shouldShow(n, "u1", lateNight))
        assertTrue("JST で日付が替わったら出す", log.shouldShow(n, "u1", nextDay))
        assertTrue("別の人は別に数える", log.shouldShow(n, "u2", morning))
        assertTrue("別のアプリは別に数える", log.shouldShow(n.copy(appId = "app2"), "u1", morning))
        log.markShown(n, "u1", nextDay)
        assertFalse(log.shouldShow(n, "u1", nextDay))
    }

    @Test
    fun brokenLogStillShows() {
        val kv = JsonKeyValueStore(null)
        kv.putString(UsageNoticeLog.KEY, "{not json")
        assertTrue(UsageNoticeLog(kv).shouldShow(UsageNotice(UsageResource.APPS, near), null, morning))
    }

    private class FakeSource(var app: AppUsage? = null, var me: MyUsage? = null, var fail: Boolean = false) : PlanUsageSource {
        override suspend fun myUsage(): MyUsage? = if (fail) throw IOException("offline") else me
        override suspend fun appUsage(appId: String): AppUsage? = if (fail) throw IOException("offline") else app
    }

    @Test
    fun nextForAppPicksOneAndMarksIt() = runTest {
        val source = FakeSource(app = mine(near, full))
        var now = morning
        val notices = UsageNotices(source, UsageNoticeLog(JsonKeyValueStore(null)), now = { now })
        assertEquals(UsageResource.RECORDS_PER_APP, notices.nextForApp("app1")?.resource)
        assertEquals("記録を出した日は次の写真", UsageResource.PHOTOS_PER_APP, notices.nextForApp("app1")?.resource)
        assertNull("両方出したらその日は出さない", notices.nextForApp("app1"))
        now = nextDay
        assertEquals(UsageResource.RECORDS_PER_APP, notices.nextForApp("app1")?.resource)
    }

    @Test
    fun mockFailureAndOldServerShowNothing() = runTest {
        assertNull(UsageNotices(null, UsageNoticeLog(JsonKeyValueStore(null))).nextForApp("app1"))
        assertNull(UsageNotices(FakeSource(fail = true), UsageNoticeLog(JsonKeyValueStore(null))).nextForApp("app1"))
        assertNull(UsageNotices(FakeSource(app = null), UsageNoticeLog(JsonKeyValueStore(null))).nextForApp("app1"))
    }

    @Test
    fun textUsesDesktopKeysInBothLanguagesWithoutPro() {
        val t = StringTable.fromClasspath()
        for (key in listOf(
            "desktop_usage_notice_at_limit", "desktop_usage_notice_near",
            "desktop_usage_resource_apps", "desktop_usage_resource_photos", "desktop_usage_resource_records", "desktop_close",
        )) {
            assertTrue("$key(ja)", t.hasString(Lang.JA, key))
            assertTrue("$key(en)", t.hasString(Lang.EN, key))
        }
        val records = UsageNotice(UsageResource.RECORDS_PER_APP, near, "app1")
        Strings.withLang(Lang.JA) {
            assertEquals("このアプリに保存できる記録の数が上限に近づいています(8 / 10)", UsageNoticeText.text(records))
            assertEquals("このアプリに保存できる写真の数が上限に達しています(10 / 10)", UsageNoticeText.text(UsageNotice(UsageResource.PHOTOS_PER_APP, full)))
        }
        Strings.withLang(Lang.EN) {
            assertEquals("The number of records this app can store is nearing its limit (8 / 10)", UsageNoticeText.text(records))
        }
        for (lang in Lang.entries) {
            for (r in UsageResource.entries) {
                val text = Strings.withLang(lang) { UsageNoticeText.text(UsageNotice(r, near)) }
                assertFalse("Pro の案内は出さない: $text", text.contains("Pro"))
            }
        }
    }
}
