package jp.kz2601.metamon.desktop.usage

import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.UsageMeter
import jp.kz2601.metamon.services.http.PlanLimit.Resource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** 上限に近い時のお知らせ: iOS の UsageNearLimitTests と同じ境(80%・上限なし・Pro・参加者・1 日 1 回)。 */
class UsageNoticesTest {
    @Test
    fun thresholdTable() {
        val table = listOf(
            UsageMeter(used = 79, limit = 100) to false,
            UsageMeter(used = 80, limit = 100) to true,
            UsageMeter(used = 120, limit = 100) to true,
            UsageMeter(used = 5, limit = null) to false, // 上限なし
            UsageMeter(used = 0, limit = 0) to false,
            null to false,
        )
        for ((meter, near) in table) assertEquals("$meter", near, UsageNoticeRules.isNear(meter))
    }

    @Test
    fun appsNoticeUpgradeOnlyOnFree() {
        val free = UsageNoticeRules.appsNotice(MyUsage(plan = "free", apps = UsageMeter(used = 8, limit = 10)))!!
        assertEquals(Resource.APPS, free.resource)
        assertTrue(free.upgrade)
        assertEquals("apps", free.id)
        assertEquals(false, UsageNoticeRules.appsNotice(MyUsage(plan = "pro", apps = UsageMeter(used = 90, limit = 100)))?.upgrade)
        assertNull(UsageNoticeRules.appsNotice(MyUsage(plan = "free", apps = UsageMeter(used = 7, limit = 10))))
        assertNull("数えない上限には出さない", UsageNoticeRules.appsNotice(MyUsage(plan = "pro", apps = UsageMeter(used = 4, limit = null))))
        assertNull(UsageNoticeRules.appsNotice(null))
    }

    @Test
    fun appNoticesForCreatorOnly() {
        val records = UsageMeter(used = 4500, limit = 5000)
        val photos = UsageMeter(used = 450, limit = 500)
        val mine = AppUsage(appId = "a", plan = "free", payer = "self", upgrade = true, records = records, photos = photos)
        assertEquals(listOf(Resource.RECORDS_PER_APP, Resource.PHOTOS_PER_APP), UsageNoticeRules.appNotices(mine).map { it.resource })
        assertEquals("recordsPerApp:a", UsageNoticeRules.appNotices(mine).first().id)

        val joined = mine.copy(payer = "creator")
        assertTrue("参加者には量も Pro も出さない", UsageNoticeRules.appNotices(joined).isEmpty())
        val low = mine.copy(records = UsageMeter(used = 10, limit = 5000), photos = null)
        assertTrue(UsageNoticeRules.appNotices(low).isEmpty())
    }

    @Test
    fun oncePerResourcePerDayInJst() {
        val log = UsageNoticeLog(JsonKeyValueStore(null))
        val now = Instant.parse("2026-11-08T03:00:00Z") // 12:00 JST
        val meter = UsageMeter(used = 9, limit = 10)
        val apps = UsageNotice(Resource.APPS, meter = meter, upgrade = true)
        val records = UsageNotice(Resource.RECORDS_PER_APP, "a", meter, true)
        val otherApp = UsageNotice(Resource.RECORDS_PER_APP, "b", meter, true)

        assertTrue(log.shouldShow(apps, "u1", now))
        log.markShown(apps, "u1", now)
        assertFalse("同じ日はもう出さない", log.shouldShow(apps, "u1", now.plusSeconds(3600)))
        assertTrue("別のリソースは別に数える", log.shouldShow(records, "u1", now))
        log.markShown(records, "u1", now)
        assertTrue("別のアプリの記録も別", log.shouldShow(otherApp, "u1", now))

        val nextDayJst = Instant.parse("2026-11-08T15:00:00Z") // 0:00 JST
        assertTrue("JST で日付が変われば また出す", log.shouldShow(apps, "u1", nextDayJst))
        log.markShown(apps, "u1", nextDayJst)
        assertTrue(log.shouldShow(records, "u1", nextDayJst))
    }

    @Test
    fun oncePerDayIsPerAccount() {
        val log = UsageNoticeLog(JsonKeyValueStore(null))
        val now = Instant.parse("2026-11-08T03:00:00Z")
        val apps = UsageNotice(Resource.APPS, meter = UsageMeter(used = 9, limit = 10), upgrade = true)
        log.markShown(apps, "u1", now)
        assertFalse(log.shouldShow(apps, "u1", now))
        assertTrue("別のアカウントは別に数える", log.shouldShow(apps, "u2", now))
        assertTrue(log.shouldShow(apps, null, now))
        log.markShown(apps, "u2", now)
        assertFalse(log.shouldShow(apps, "u2", now))
        assertFalse("u2 の記録で u1 の分を消さない", log.shouldShow(apps, "u1", now))
        log.clear()
        assertTrue(log.shouldShow(apps, "u1", now))
    }

    @Test
    fun brokenValueIsTreatedAsNeverShown() {
        val store = JsonKeyValueStore(null).apply { putString(UsageNoticeLog.KEY, "not json") }
        val apps = UsageNotice(Resource.APPS, meter = UsageMeter(used = 9, limit = 10), upgrade = true)
        assertTrue(UsageNoticeLog(store).shouldShow(apps, "u1", Instant.parse("2026-11-08T03:00:00Z")))
    }
}
