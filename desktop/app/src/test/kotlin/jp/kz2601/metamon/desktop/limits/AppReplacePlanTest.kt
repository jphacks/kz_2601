package jp.kz2601.metamon.desktop.limits

import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.http.ApiError
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** 402 `app_limit_reached` の置き換え候補の並べ方(iOS の AppReplacePlan と同じ規則)。 */
class AppReplacePlanTest {
    private val t0 = Instant.parse("2026-01-01T00:00:00Z")
    private fun day(n: Long) = t0.plusSeconds(n * 86_400)

    @Test
    fun ordersByLeastRecentlyOpenedWithCurrentLast() {
        val limit = AppLimit(
            held = 3,
            limit = 3,
            apps = listOf(
                AppLimit.HeldApp("a", "A", updatedAt = day(5)),
                AppLimit.HeldApp("b", "B", updatedAt = day(1)),
                AppLimit.HeldApp("c", "C", updatedAt = day(0)),
                AppLimit.HeldApp("d", "D"),
            ),
        )
        val opened = mapOf("a" to day(2))
        val plan = AppReplacePlan.make(limit, emptyList(), { opened[it] }, current = "c")
        // 時刻が分からない d が先頭・端末で開いた時刻(a=2)がサーバーの更新(a=5)より優先・使用中の c は最後
        assertEquals(listOf("d", "b", "a", "c"), plan.rows.map { it.id })
        assertTrue(plan.rows.last().isCurrent)
        assertEquals(day(2), plan.rows.first { it.id == "a" }.lastOpened)
    }

    @Test
    fun ownAppsAreDeletedJoinedAppsAreLeft() {
        val limit = AppLimit(
            held = 2,
            limit = 2,
            apps = listOf(AppLimit.HeldApp("mine", "", mine = true), AppLimit.HeldApp("shared", "S", mine = false)),
        )
        val local = listOf(AppSummary(id = "mine", name = "端末の名前", currentVersion = 3))
        val plan = AppReplacePlan.make(limit, local, { null }, current = null)
        val mine = plan.rows.first { it.id == "mine" }
        val shared = plan.rows.first { it.id == "shared" }
        assertEquals(AppReplacePlan.Role.OWN, mine.role)
        assertEquals("端末の名前", mine.name)
        assertEquals(AppReplacePlan.Role.JOINED, shared.role)
        assertEquals(AppReplacePlan.Action.Delete("mine"), AppReplacePlan.action(mine))
        assertEquals(AppReplacePlan.Action.Leave("shared"), AppReplacePlan.action(shared))
    }

    @Test
    fun fallsBackToLocalListWithoutArchivedOrBlank() {
        val limit = AppLimit(held = 2, limit = 2, apps = null)
        val local = listOf(
            AppSummary(id = "x", name = "X", currentVersion = 4, updatedAt = "2026-01-03T00:00:00Z"),
            AppSummary(id = "y", name = "Y", currentVersion = 4, updatedAt = "2026-01-02T00:00:00Z"),
            AppSummary(id = "blank", currentVersion = 1),
            AppSummary(id = "old", currentVersion = 4, status = "archived"),
        )
        val plan = AppReplacePlan.make(limit, local, { null }, current = null)
        assertEquals(listOf("y", "x"), plan.rows.map { it.id })
        assertTrue(plan.rows.all { it.role == AppReplacePlan.Role.OWN })
    }

    @Test
    fun emptyListsGiveNoRows() {
        val plan = AppReplacePlan.make(AppLimit(held = 1, limit = 1, apps = emptyList()), emptyList(), { null }, current = null)
        assertTrue(plan.rows.isEmpty())
        assertEquals(1, plan.removalsNeeded)
    }

    @Test
    fun removingUntilOneFits() {
        val limit = AppLimit(held = 4, limit = 3, apps = listOf("a", "b", "c", "d").map { AppLimit.HeldApp(it, it) })
        val plan = AppReplacePlan.make(limit, emptyList(), { null }, current = null)
        assertEquals(2, plan.removalsNeeded)
        val next = plan.removing("a")!!
        assertEquals(listOf("b", "c", "d"), next.rows.map { it.id })
        assertNull(next.removing("b"))
        assertEquals(next, next.removing("missing"))
    }

    @Test
    fun parsesServerDetails() {
        val details = Json.parseToJsonElement(
            """{"held":3,"limit":3,"canBuySlot":true,"slotCoins":100,
               "apps":[{"id":"a","name":"A","mine":false,"updatedAt":"2026-01-02T00:00:00Z"},{"id":""},{"name":"no id"}]}""",
        ).jsonObject
        val limit = AppLimit.from(ApiError.Server(402, "app_limit_reached", "", details))!!
        assertEquals(3, limit.held)
        assertEquals(listOf(AppLimit.HeldApp("a", "A", mine = false, updatedAt = day(1))), limit.apps)
        assertNull(AppLimit.from(ApiError.Server(402, "pro_required", "")))
        assertNull(AppLimit.from(ApiError.Server(402, "app_limit_reached", "")).let { it?.apps })
    }
}
