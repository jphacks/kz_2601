package jp.kz2601.metamon.settings

import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.UsageMeter
import jp.kz2601.metamon.settings.UsageSectionModel.Resource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #317 設定の「使っている量」: 何を出すか・Pro / 無料・値が無い時(iOS の UsageSettingsSection と同じ出し分け)。 */
class UsageSectionModelTest {
    private val freeMe = MyUsage(plan = "free", apps = UsageMeter(used = 3, limit = 5, remaining = 2))

    private fun app(payer: String = "self", plan: String = "free") = AppUsage(
        appId = "a1",
        plan = plan,
        payer = payer,
        upgrade = plan == "free",
        records = UsageMeter(used = 90, limit = 100, remaining = 10),
        photos = UsageMeter(used = 2, limit = null, remaining = null),
    )

    @Test
    fun nothingKnownHidesSection() {
        assertTrue("読めない・モックの時は節ごと出さない", UsageSectionModel.of(null, null, isPro = false).rows.isEmpty())
        assertTrue("アプリの数が数えられない時も出さない", UsageSectionModel.of(MyUsage(plan = "free"), null, isPro = false).rows.isEmpty())
    }

    @Test
    fun showsAppsThenRecordsAndPhotosOfOwnApp() {
        val model = UsageSectionModel.of(freeMe, app(), isPro = false)
        assertEquals(listOf(Resource.APPS, Resource.RECORDS_PER_APP, Resource.PHOTOS_PER_APP), model.rows.map { it.resource })
        assertEquals("3 / 5", model.rows[0].amount)
        assertFalse("60% はまだ目立たせない", model.rows[0].near)
        assertTrue("90% は目立たせる", model.rows[1].near)
        assertNull("数えない時は「(上限なし)」で出す", model.rows[2].amount)
        assertFalse(model.rows[2].near)
        assertFalse(model.pro)
    }

    @Test
    fun participantSeesOnlyOwnAppCount() {
        val model = UsageSectionModel.of(freeMe, app(payer = "creator", plan = "pro"), isPro = false)
        assertEquals(listOf(Resource.APPS), model.rows.map { it.resource })
        assertFalse("他の人のアプリのプランで Pro と言わない", model.pro)
    }

    @Test
    fun proFromAnySource() {
        assertTrue("「今日の残り」で分かった Pro", UsageSectionModel.of(freeMe, null, isPro = true).pro)
        assertTrue("自分のプラン", UsageSectionModel.of(freeMe.copy(plan = "pro"), null, isPro = false).pro)
        assertTrue("今のアプリ(自分の)のプラン", UsageSectionModel.of(null, app(plan = "pro"), isPro = false).pro)
    }

    @Test
    fun appRowsWithoutMyUsage() {
        val model = UsageSectionModel.of(null, app(), isPro = false)
        assertEquals(listOf(Resource.RECORDS_PER_APP, Resource.PHOTOS_PER_APP), model.rows.map { it.resource })
    }

    @Test
    fun zeroLimitIsNotNear() {
        assertFalse(UsageMeter(used = 0, limit = 0, remaining = 0).isNear)
        assertTrue(UsageMeter(used = 4, limit = 5, remaining = 1).isNear)
    }
}
