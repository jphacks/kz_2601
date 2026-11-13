package jp.kz2601.metamon.desktop.usage

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.UsageMeter
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.Instant

/** パレットのアプリの一覧の上の「作れるアプリの数」を 1 日 1 回(UsageNoticeRules.appsNotice・#3353)。 */
class AppsUsageNotice3353Test {
    private val morning = Instant.parse("2026-11-13T01:00:00Z") // 10:00 JST
    private val evening = Instant.parse("2026-11-13T12:00:00Z") // 21:00 JST
    private val nextDay = Instant.parse("2026-11-13T15:30:00Z") // 翌 0:30 JST

    private class CountingSource(var me: MyUsage?) : PlanUsageSource {
        var reads = 0
        override suspend fun myUsage(): MyUsage? = me.also { reads++ }
        override suspend fun appUsage(appId: String): AppUsage? = null
    }

    @Test
    fun appsNoticeFollowsTheEightyPercentRule() {
        assertEquals(UsageResource.APPS, UsageNoticeRules.appsNotice(MyUsage(apps = UsageMeter(used = 4, limit = 5)))?.resource)
        assertNull(UsageNoticeRules.appsNotice(MyUsage(apps = UsageMeter(used = 3, limit = 5))))
        assertNull("上限なし", UsageNoticeRules.appsNotice(MyUsage(apps = UsageMeter(used = 50))))
        assertNull("数えられなかった", UsageNoticeRules.appsNotice(MyUsage(apps = null)))
        assertNull(UsageNoticeRules.appsNotice(null))
    }

    @Test
    fun shownOncePerDayAndNotReadAgainThatDay() = runTest {
        val source = CountingSource(MyUsage(apps = UsageMeter(used = 9, limit = 10)))
        var now = morning
        val notices = UsageNotices(source, UsageNoticeLog(JsonKeyValueStore(null)), uid = { "u1" }, now = { now })
        assertEquals(UsageResource.APPS, notices.nextForApps()?.resource)
        now = evening
        assertNull("同じ日(JST)に開き直しても出さない", notices.nextForApps())
        assertEquals("出した日はもう読みに行かない", 1, source.reads)
        now = nextDay
        assertEquals(UsageResource.APPS, notices.nextForApps()?.resource)
    }

    @Test
    fun notNearMeansReadAgainNextOpen() = runTest {
        val source = CountingSource(MyUsage(apps = UsageMeter(used = 1, limit = 10)))
        val notices = UsageNotices(source, UsageNoticeLog(JsonKeyValueStore(null)), now = { morning })
        assertNull(notices.nextForApps())
        source.me = MyUsage(apps = UsageMeter(used = 8, limit = 10))
        assertEquals("出していなければ同じ日でも出せる", UsageResource.APPS, notices.nextForApps()?.resource)
    }

    @Test
    fun appsAndInAppNoticesAreCountedSeparately() = runTest {
        val log = UsageNoticeLog(JsonKeyValueStore(null))
        log.markShown(UsageNotice(UsageResource.RECORDS_PER_APP, UsageMeter(8, 10), "app1"), null, morning)
        val notices = UsageNotices(CountingSource(MyUsage(apps = UsageMeter(8, 10))), log, now = { morning })
        assertEquals(UsageResource.APPS, notices.nextForApps()?.resource)
    }

    @Test
    fun textMatchesIos() {
        val n = UsageNotice(UsageResource.APPS, UsageMeter(used = 8, limit = 10))
        assertEquals("作れるアプリの数が上限に近づいています(8 / 10)", Strings.withLang(Lang.JA) { UsageNoticeText.text(n) })
        assertEquals("The number of apps you can create is nearing its limit (8 / 10)", Strings.withLang(Lang.EN) { UsageNoticeText.text(n) })
    }
}
