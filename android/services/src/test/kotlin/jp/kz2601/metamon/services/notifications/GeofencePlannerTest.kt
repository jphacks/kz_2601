package jp.kz2601.metamon.services.notifications

import jp.kz2601.metamon.services.notifications.PlannedNotification.Trigger
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** 場所に着いたら知らせる(near)を Geofencing に渡す計画(iOS の UserNotificationScheduler.places と同じ決まり)。 */
class GeofencePlannerTest {
    private fun region(id: String, repeats: Boolean, savedAt: Long = 0) =
        PlannedNotification(id, "着いた", "本文", Trigger.Region(36.5, 136.6, 200.0, repeats), savedAt = savedAt)

    private val once = region("dsl.app1.n.r1", repeats = false, savedAt = 2)
    private val every = region("dsl.app1.n.r2", repeats = true, savedAt = 1)
    private val time = PlannedNotification("dsl.app1.t", "t", "", Trigger.Daily(9, 0))

    @Test
    fun armsOnlyRegionsWhenAllowed() {
        val plan = GeofencePlanner.plan(listOf(once, every, time), RegionOnceState(), registered = emptySet(), allowed = true)
        assertEquals(listOf(once, every), plan.arm)
        assertTrue(plan.remove.isEmpty())
        assertFalse(plan.locationOff)
        assertEquals(setOfNotNull(RegionOnceState.key(once), RegionOnceState.key(every)), plan.state.armed)
    }

    @Test
    fun notAllowedRemovesEverythingAndReportsLocationOffForThatApp() {
        val plan = GeofencePlanner.plan(listOf(once, every), RegionOnceState(), registered = setOf(once.identifier), allowed = false, appId = "app1")
        assertTrue(plan.arm.isEmpty())
        assertEquals(setOf(once.identifier), plan.remove)
        assertTrue(plan.locationOff)
        assertTrue(plan.state.armed.isEmpty())
        // 別のアプリ・場所の通知が無いアプリには出さない
        assertFalse(GeofencePlanner.plan(listOf(once), RegionOnceState(), emptySet(), allowed = false, appId = "app2").locationOff)
        assertFalse(GeofencePlanner.plan(listOf(time), RegionOnceState(), emptySet(), allowed = false, appId = "app1").locationOff)
    }

    @Test
    fun onceRegionIsNotRearmedAfterArrival() {
        val first = GeofencePlanner.plan(listOf(once, every), RegionOnceState(), emptySet(), allowed = true)
        // 着いた: 1 回きりは見張りから外れる(registered から消える)。くり返しは残る
        val arrived = GeofencePlanner.arrived(listOf(GeofencePlanner.requestId(once.identifier), GeofencePlanner.requestId(every.identifier)), listOf(once, every, time))
        assertEquals(listOf(once, every), arrived)
        val still = arrived.filter(GeofencePlanner::keepsWatching).map { it.identifier }.toSet()
        assertEquals(setOf(every.identifier), still)
        val next = GeofencePlanner.plan(listOf(once, every), first.state, registered = still, allowed = true)
        assertEquals(listOf(every), next.arm)
        // 場所が変われば別の通知として見張り直す
        val moved = once.copy(trigger = Trigger.Region(35.0, 135.0, 200.0, false))
        assertEquals(listOf(moved), GeofencePlanner.plan(listOf(moved), next.state, registered = emptySet(), allowed = true).arm)
    }

    @Test
    fun removesRegionsThatAreNoLongerPlanned() {
        val plan = GeofencePlanner.plan(listOf(every), RegionOnceState(), registered = setOf(every.identifier, "dsl.app1.gone"), allowed = true)
        assertEquals(setOf("dsl.app1.gone"), plan.remove)
    }

    @Test
    fun keepsTheRegionLimit() {
        val many = (1..20).map { region("dsl.app1.n.r$it", repeats = true, savedAt = it.toLong()) }
        val plan = GeofencePlanner.plan(many, RegionOnceState(), emptySet(), allowed = true)
        assertEquals(NotificationPlanner.REGION_LIMIT, plan.arm.size)
        assertEquals("dsl.app1.n.r20", plan.arm.first().identifier) // 現在地が無ければ新しい順
    }

    @Test
    fun requestIdIsShortStableAndDistinct() {
        val long = "dsl." + "a".repeat(36) + ".notify." + "b".repeat(36)
        val id = GeofencePlanner.requestId(long)
        assertTrue(id.length <= 100)
        assertEquals(id, GeofencePlanner.requestId(long))
        assertNotEquals(id, GeofencePlanner.requestId(long + "x"))
        assertTrue(GeofencePlanner.arrived(listOf("unknown"), listOf(once)).isEmpty())
    }
}
