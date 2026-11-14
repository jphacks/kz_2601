package jp.kz2601.metamon.services.notifications

import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.NotificationNear
import jp.kz2601.metamon.dsl.Repeat
import jp.kz2601.metamon.dsl.Weekday
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.notifications.PlannedNotification.Trigger
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime

/** iOS の NotificationPlannerTests / RepeatNotificationTests / NearNotificationTests と同じ入力・期待値(#886)。 */
class NotificationPlannerTest {
    private val zone = ZoneId.of("Asia/Tokyo")

    private fun date(y: Int, mo: Int, d: Int, h: Int = 9, m: Int = 0): Instant = ZonedDateTime.of(y, mo, d, h, m, 0, 0, zone).toInstant()

    /** 2026-10-25(日) 10:00 JST */
    private val now = date(2026, 10, 25, 10)
    private fun at(d: Int, h: Int, m: Int) = date(2026, 10, d, h, m)
    private fun s(v: String) = JSONValue.Str(v)
    private fun plan(specs: List<Notification>, records: Map<String, List<Record>> = emptyMap(), now: Instant = this.now) =
        NotificationPlanner.plan("a1", specs, records, now, zone)

    // NotificationPlannerTests

    @Test
    fun fixedDailyWeeklyOnce() {
        val p = plan(
            listOf(
                Notification("wake", "起床", "", Repeat.DAILY, time = "07:00"),
                Notification("gym", "ジム", "", Repeat.WEEKLY, time = "19:30", weekday = Weekday.MON),
                Notification("noon", "昼", "", Repeat.ONCE, time = "12:00"),
            ),
        )
        assertEquals(listOf("dsl.a1.wake", "dsl.a1.gym", "dsl.a1.noon"), p.map { it.identifier })
        assertEquals(Trigger.Daily(7, 0), p[0].trigger)
        assertEquals(Trigger.Weekly(2, 19, 30), p[1].trigger)
        assertEquals(Trigger.at(at(25, 12, 0)), p[2].trigger)
        assertEquals(at(26, 7, 0), p[0].nextFire(now, zone)) // 今日の 7:00 は過ぎた → 明日
        assertEquals(at(26, 19, 30), p[1].nextFire(now, zone)) // 10/26 は月曜
    }

    @Test
    fun onceTimePassedGoesToTomorrow() {
        val p = plan(listOf(Notification("m", "朝", "", Repeat.ONCE, time = "08:00")))
        assertEquals(Trigger.at(at(26, 8, 0)), p.single().trigger)
    }

    @Test
    fun recordAlarmTimeDailyWithEnabledFlag() {
        val spec = Notification("alarm", "アラーム", "", Repeat.DAILY, entity = "alarm", field = "time", titleField = "label", enabledField = "on")
        val recs = mapOf(
            "alarm" to listOf(
                Record("r1", mapOf("time" to s("06:45"), "label" to s("起きる"), "on" to JSONValue.Bool(true))),
                Record("r2", mapOf("time" to s("08:00"), "label" to s("休日"), "on" to JSONValue.Bool(false))),
                Record("r3", mapOf("time" to s("22:10"))),
            ),
        )
        val p = plan(listOf(spec), recs)
        assertEquals(listOf("dsl.a1.alarm.r1", "dsl.a1.alarm.r3"), p.map { it.identifier })
        assertEquals(Trigger.Daily(6, 45), p[0].trigger)
        assertEquals("起きる", p[0].body)
        assertEquals(Trigger.Daily(22, 10), p[1].trigger)
    }

    @Test
    fun recordDatetimeTenMinutesBefore() {
        val spec = Notification("before", "まもなく予定", "", Repeat.ONCE, entity = "event", field = "when", offsetMinutes = -10, titleField = "title")
        val recs = mapOf(
            "event" to listOf(
                Record("e1", mapOf("when" to s("2026-10-25T06:00:00Z"), "title" to s("会議"))), // 15:00 JST
                Record("e2", mapOf("when" to s("2026-10-25T01:05:00Z"))), // 10:05 JST: 10 分前は過ぎた
                Record("e3", mapOf("when" to s("2026-10-27"), "title" to s("締切"))),
                Record("e4", mapOf("when" to JSONValue.Null)),
            ),
        )
        val p = plan(listOf(spec), recs)
        assertEquals(listOf("dsl.a1.before.e1", "dsl.a1.before.e3"), p.map { it.identifier })
        assertEquals(Trigger.at(at(25, 14, 50)), p[0].trigger)
        assertEquals("会議", p[0].body)
        assertEquals(at(27, 8, 50), p[1].nextFire(now, zone))
    }

    @Test
    fun offsetWrapsAroundMidnightForTimeOnly() {
        val spec = Notification("a", "t", "", Repeat.DAILY, entity = "x", field = "time", offsetMinutes = -30)
        val p = plan(listOf(spec), mapOf("x" to listOf(Record("r", mapOf("time" to s("00:10"))))))
        assertEquals(Trigger.Daily(23, 40), p.first().trigger)
    }

    @Test
    fun selectKeepsNearestWithinLimitAndDropsPast() {
        val past = PlannedNotification("dsl.a.past", "", "", Trigger.at(at(25, 9, 0)))
        val many = (0 until 80).map { i -> PlannedNotification("dsl.a.n$i", "", "", Trigger.at(now.plusSeconds((80L - i) * 3600))) }
        val daily = PlannedNotification("dsl.b.d", "", "", Trigger.Daily(10, 30))
        val chosen = NotificationPlanner.select(listOf(past) + many + daily, now, zone, limit = 60)
        assertEquals(60, chosen.size)
        assertFalse(chosen.any { it.first.identifier == "dsl.a.past" })
        assertEquals("dsl.b.d", chosen.first().first.identifier)
        assertEquals("dsl.a.n79", chosen[1].first.identifier)
        assertTrue(chosen.zipWithNext().all { (a, b) -> a.second <= b.second })
        assertFalse(chosen.any { it.first.identifier == "dsl.a.n0" })
    }

    @Test
    fun actionUsesSeparateNamespace() {
        val spec = Notification("wake", "起床", "", Repeat.DAILY, time = "06:00")
        assertEquals("dsl.a1.action.wake", NotificationPlanner.action("a1", spec, now, zone)?.identifier)
    }

    @Test
    fun registryKeepsOtherAppsAndActionsAndRoundTrips() {
        val a = PlannedNotification("dsl.a.x", "", "", Trigger.Daily(7, 0))
        val b = PlannedNotification("dsl.b.x", "", "", Trigger.Daily(8, 0))
        val act = PlannedNotification("dsl.a.action.y", "", "", Trigger.Daily(6, 0))
        var reg = NotificationRegistry().set("a", listOf(a)).set("b", listOf(b)).upsertAction("a", act).set("a", emptyList())
        assertEquals(setOf("dsl.b.x", "dsl.a.action.y"), reg.pruned(now, zone).all.map { it.identifier }.toSet())
        val json = DslJson.encodeToString(NotificationRegistry.serializer(), reg)
        val reloaded = DslJson.decodeFromString(NotificationRegistry.serializer(), json)
        assertEquals(reg, reloaded)
        reg = reloaded.removeApp("b")
        assertEquals(listOf("dsl.a.action.y"), reg.all.map { it.identifier })
        // cancelNotification はボタン分も DSL のレコードごとの分も消す
        val rec = PlannedNotification("dsl.a.y.r1", "", "", Trigger.Daily(6, 0))
        val removed = reg.set("a", listOf(rec)).remove("a", "y")
        assertTrue(removed.all.isEmpty())
    }

    @Test
    fun registryPrunesPastOnceButKeepsRegions() {
        val past = PlannedNotification("dsl.a.p", "", "", Trigger.at(at(25, 9, 0)))
        val region = PlannedNotification("dsl.a.r", "", "", Trigger.Region(36.5, 136.6, 200.0, false))
        assertEquals(listOf("dsl.a.r"), NotificationRegistry().set("a", listOf(past, region)).pruned(now, zone).all.map { it.identifier })
    }

    @Test
    fun upcomingIsThisAppOnlyNearestFirstWithinGlobalLimit() {
        val a1 = PlannedNotification("dsl.a.late", "遅い", "", Trigger.Daily(21, 0))
        val a2 = PlannedNotification("dsl.a.early", "早い", "本文", Trigger.Daily(11, 0))
        val act = PlannedNotification("dsl.a.action.btn", "ボタン", "", Trigger.at(at(25, 12, 0)))
        val past = PlannedNotification("dsl.a.past", "", "", Trigger.at(at(25, 9, 0)))
        val region = PlannedNotification("dsl.a.place", "", "", Trigger.Region(36.5, 136.6, 200.0, false))
        val other = PlannedNotification("dsl.b.x", "", "", Trigger.Daily(10, 30))
        val reg = NotificationRegistry().set("a", listOf(a1, a2, past, region)).upsertAction("a", act).set("b", listOf(other))
        val up = reg.upcoming("a", now, zone)
        assertEquals(listOf("dsl.a.early", "dsl.a.action.btn", "dsl.a.late"), up.map { it.first.identifier })
        assertEquals(listOf(at(25, 11, 0), at(25, 12, 0), at(25, 21, 0)), up.map { it.second })
        // 全アプリの上限で予約から外れた分は出さない(b の 10:30 と a の 11:00 で 2 件)
        assertEquals(listOf("dsl.a.early"), reg.upcoming("a", now, zone, limit = 2).map { it.first.identifier })
        assertTrue(reg.upcoming("zzz", now, zone).isEmpty())
    }

    @Test
    fun scheduleDiffCancelsDropped() {
        val keep = PlannedNotification("dsl.a.k", "", "", Trigger.Daily(7, 0))
        val d = ScheduleDiff.of(listOf(keep to at(26, 7, 0)), setOf("dsl.a.k", "dsl.a.gone"))
        assertEquals(setOf("dsl.a.gone"), d.cancel)
    }

    @Test
    fun decodesRecordBasedSpecAndUnknownRepeatIsOnce() {
        val json = """{"id":"a","title":"t","body":"","repeat":"once","entity":"event","field":"when","offsetMinutes":-10,"titleField":"title"}"""
        val spec = DslJson.decodeFromString(Notification.serializer(), json)
        assertTrue(spec.isRecordBased)
        assertEquals(-10, spec.offsetMinutes)
        val hourly = DslJson.decodeFromString(Notification.serializer(), """{"id":"c","title":"t","body":"","repeat":"hourly","time":"12:00"}""")
        assertEquals(Trigger.at(at(25, 12, 0)), NotificationPlanner.plan("a1", listOf(hourly), emptyMap(), now, zone).single().trigger)
    }

    // RepeatNotificationTests(#266)

    private fun planRenew(repeat: Repeat, value: String, offset: Int?, now: Instant, time: String? = null): List<PlannedNotification> {
        val spec = Notification("renew", "更新日", "", repeat, time = time, entity = "subs", field = "renew_on", offsetMinutes = offset, titleField = "name")
        return plan(listOf(spec), mapOf("subs" to listOf(Record("r1", mapOf("name" to s("動画サブスク"), "renew_on" to s(value))))), now)
    }

    private fun fires(p: List<PlannedNotification>) = p.mapNotNull { (it.trigger as? Trigger.At)?.instant }

    @Test
    fun subscriptionRenewalDayBefore() {
        val p = planRenew(Repeat.MONTHLY, "2026-10-30", -1440, date(2026, 10, 1, 12))
        assertEquals(listOf(date(2026, 10, 29), date(2026, 11, 29), date(2026, 12, 29)), fires(p))
        assertEquals(listOf("dsl.a1.renew.r1#0", "dsl.a1.renew.r1#1", "dsl.a1.renew.r1#2"), p.map { it.identifier })
        assertEquals("動画サブスク", p[0].body)
    }

    @Test
    fun afterRenewalDayStillSchedulesNextMonths() {
        assertEquals(
            listOf(date(2026, 11, 29), date(2026, 12, 29), date(2027, 1, 29)),
            fires(planRenew(Repeat.MONTHLY, "2026-10-30", -1440, date(2026, 10, 31, 12))),
        )
        assertTrue(planRenew(Repeat.ONCE, "2026-10-30", -1440, date(2026, 10, 31, 12)).isEmpty())
    }

    @Test
    fun monthEndClampedInFebruary() {
        assertEquals(
            listOf(date(2027, 2, 27), date(2027, 3, 29), date(2027, 4, 29)),
            fires(planRenew(Repeat.MONTHLY, "2026-10-30", -1440, date(2027, 1, 30, 12))),
        )
        assertEquals(
            listOf(date(2026, 2, 28), date(2026, 3, 31), date(2026, 4, 30)),
            fires(planRenew(Repeat.MONTHLY, "2026-01-31", null, date(2026, 1, 31, 10))),
        )
    }

    @Test
    fun repeatingMonthlyWhenDayIsStable() {
        val p = planRenew(Repeat.MONTHLY, "2026-10-15", -1440, date(2026, 11, 2, 12), time = "20:00")
        assertEquals(1, p.size)
        assertEquals("dsl.a1.renew.r1", p[0].identifier)
        assertEquals(Trigger.Monthly(14, 20, 0), p[0].trigger)
        assertEquals(date(2026, 11, 14, 20), p[0].nextFire(date(2026, 11, 2, 12), zone))
    }

    @Test
    fun firstOfMonthDayBeforeIsConcrete() {
        assertEquals(
            listOf(date(2026, 11, 30), date(2026, 12, 31), date(2027, 1, 31)),
            fires(planRenew(Repeat.MONTHLY, "2026-03-01", -1440, date(2026, 11, 2, 12))),
        )
    }

    @Test
    fun birthdayYearlyRepeating() {
        val p = planRenew(Repeat.YEARLY, "1990-05-10", null, date(2026, 11, 2, 12))
        assertEquals(listOf(Trigger.Yearly(5, 10, 9, 0)), p.map { it.trigger })
        assertEquals(date(2027, 5, 10), p[0].nextFire(date(2026, 11, 2, 12), zone))
    }

    @Test
    fun leapDayBirthday() {
        assertEquals(listOf(date(2027, 2, 28), date(2028, 2, 29)), fires(planRenew(Repeat.YEARLY, "2004-02-29", null, date(2026, 11, 2, 12))))
    }

    @Test
    fun yearlyOffsetAcrossFebruaryIsConcrete() {
        assertEquals(listOf(date(2027, 2, 22), date(2028, 2, 23)), fires(planRenew(Repeat.YEARLY, "2020-03-01", -7 * 1440, date(2026, 11, 2, 12))))
    }

    @Test
    fun futureAnchorStartsAtAnchor() {
        assertEquals(
            listOf(date(2027, 3, 15), date(2027, 4, 15), date(2027, 5, 15)),
            fires(planRenew(Repeat.MONTHLY, "2027-03-15", null, date(2026, 11, 2, 12))),
        )
    }

    @Test
    fun datetimeAnchorKeepsTime() {
        assertNull(NotificationPlanner.parseTime("2026-01-31T23:00:00+09:00"))
        assertEquals(7 to 5, NotificationPlanner.parseTime("7:05"))
        assertEquals(
            listOf(date(2026, 11, 30, 22), date(2026, 12, 31, 22), date(2027, 1, 31, 22)),
            fires(planRenew(Repeat.MONTHLY, "2026-01-31T23:00:00+09:00", -60, date(2026, 11, 2, 12))),
        )
    }

    @Test
    fun manyRecordsRespectLimit() {
        val spec = Notification("renew", "t", "", Repeat.MONTHLY, entity = "subs", field = "renew_on")
        val recs = (0 until 40).map { Record("r$it", mapOf("renew_on" to s("2026-01-31"))) }
        val n = date(2026, 11, 2, 12)
        val p = plan(listOf(spec), mapOf("subs" to recs), n)
        assertEquals(40 * NotificationPlanner.MONTHLY_AHEAD, p.size)
        assertEquals(NotificationPlanner.LIMIT, NotificationPlanner.select(p, n, zone).size)
    }

    @Test
    fun triggerJsonRoundTrip() {
        val p = listOf(
            PlannedNotification("x", "t", "", Trigger.Monthly(14, 9, 0)),
            PlannedNotification("y", "t", "", Trigger.Yearly(5, 10, 9, 0), alarm = true),
            PlannedNotification("z", "t", "", Trigger.Region(36.5, 136.6, 200.0, true), savedAt = 10),
        )
        val ser = kotlinx.serialization.builtins.ListSerializer(PlannedNotification.serializer())
        assertEquals(p, DslJson.decodeFromString(ser, DslJson.encodeToString(ser, p)))
    }

    // NearNotificationTests

    private val now2 = date(2026, 11, 8, 12)
    private val nearSpec = Notification("arrive", "もうすぐ到着", "降りる準備を", Repeat.ONCE, near = NotificationNear("stations", "spot", 800))
    private fun station(id: String, loc: String?, updated: String? = null) =
        Record(id, mapOf("name" to s(id), "spot" to (loc?.let(::s) ?: JSONValue.Null)), updatedAt = updated)

    @Test
    fun decodesNearAndJitter() {
        val json = """{"id":"arrive","title":"t","body":"","repeat":"daily","near":{"entity":"stations","locationField":"spot"},"jitterMinutes":15}"""
        val spec = DslJson.decodeFromString(Notification.serializer(), json)
        assertEquals(200.0, spec.nearRadius, 0.0)
        assertEquals(15, spec.jitterMinutes)
        assertTrue(spec.isNearBased)
        assertFalse(spec.isRecordBased)
        assertEquals("stations", spec.recordEntity)
    }

    @Test
    fun plansOneRegionPerRecordWithPlace() {
        val recs = listOf(station("kanazawa", "36.578,136.648"), station("empty", null), station("broken", "abc"))
        val p = plan(listOf(nearSpec), mapOf("stations" to recs), now2)
        assertEquals(listOf("dsl.a1.arrive.kanazawa"), p.map { it.identifier })
        assertEquals(Trigger.Region(36.578, 136.648, 800.0, false), p.first().trigger)
        assertNull(p.first().nextFire(now2, zone))
        assertTrue(NotificationPlanner.select(p, now2, zone).isEmpty())
        val d = plan(listOf(nearSpec.copy(repeat = Repeat.DAILY)), mapOf("stations" to recs), now2)
        assertEquals(Trigger.Region(36.578, 136.648, 800.0, true), d.first().trigger)
        assertNull(NotificationPlanner.action("a1", nearSpec, now2, zone))
    }

    @Test
    fun selectsNearestOrNewestRegions() {
        val recs = (0 until 20).map { i ->
            station("s$i", String.format(java.util.Locale.ROOT, "%.3f,136.600", 36.0 + i * 0.01), updated = String.format(java.util.Locale.ROOT, "2026-11-%02dT00:00:00Z", i + 1))
        }
        val p = plan(listOf(nearSpec), mapOf("stations" to recs), now2)
        assertEquals(20, p.size)
        val newest = NotificationPlanner.selectRegions(p)
        assertEquals(NotificationPlanner.REGION_LIMIT, newest.size)
        assertEquals("dsl.a1.arrive.s19", newest.first().identifier)
        assertFalse(newest.any { it.identifier == "dsl.a1.arrive.s0" })
        val nearest = NotificationPlanner.selectRegions(p, here = 36.0 to 136.6, limit = 3)
        assertEquals(listOf("dsl.a1.arrive.s0", "dsl.a1.arrive.s1", "dsl.a1.arrive.s2"), nearest.map { it.identifier })
    }

    @Test
    fun onceRegionIsNotRearmedAfterFiring() {
        fun planAt(loc: String) = plan(listOf(nearSpec), mapOf("stations" to listOf(station("k", loc))), now2)
        val first = planAt("36.5,136.6")
        var state = RegionOnceState()
        val (pick1, s1) = state.toSchedule(first, first, emptySet())
        assertEquals(1, pick1.size)
        state = s1.armed(pick1)
        val (pick2, s2) = state.toSchedule(first, first, setOf("dsl.a1.arrive.k"))
        assertEquals(1, pick2.size)
        state = s2.armed(pick2)
        val (pick3, s3) = state.toSchedule(first, first, emptySet())
        assertTrue(pick3.isEmpty())
        state = s3.armed(pick3)
        val reloaded = DslJson.decodeFromString(RegionOnceState.serializer(), DslJson.encodeToString(RegionOnceState.serializer(), state))
        assertTrue("アプリを閉じても 1 回きりを守る", reloaded.toSchedule(first, first, emptySet()).first.isEmpty())
        val moved = planAt("36.6,136.7")
        assertEquals("場所を直したら見張り直す", 1, reloaded.toSchedule(moved, moved, emptySet()).first.size)
        val d = plan(listOf(nearSpec.copy(repeat = Repeat.DAILY)), mapOf("stations" to listOf(station("k", "36.5,136.6"))), now2)
        assertEquals(1, reloaded.armed(d).toSchedule(d, d, emptySet()).first.size)
    }

    // jitterMinutes

    @Test
    fun jitterSpreadsDailyOccurrencesStably() {
        val spec = Notification("photo", "写真の時間", "", Repeat.DAILY, time = "20:00", jitterMinutes = 60)
        val p = plan(listOf(spec), now = now2)
        assertEquals(NotificationPlanner.JITTER_DAILY_AHEAD, p.size)
        val fires = fires(p)
        assertEquals(p.size, fires.size)
        fires.forEachIndexed { i, f ->
            val base = date(2026, 11, 8, 20).plusSeconds(i * 86_400L)
            assertTrue("±60 分以内", Math.abs(f.epochSecond - base.epochSecond) <= 3600)
        }
        assertTrue(fires.map { ZonedDateTime.ofInstant(it, zone).minute }.toSet().size > 1)
        assertEquals(p.size, p.map { it.identifier }.toSet().size)
        assertTrue(p.all { it.identifier.startsWith("dsl.a1.photo.j") })
        val later = plan(listOf(spec), now = now2.plusSeconds(3600))
        assertEquals(p.first().trigger, later.first().trigger)
        assertEquals(p.first().identifier, later.first().identifier)
    }

    @Test
    fun jitterWithNegativeOffsetIsAlwaysEarly() {
        val spec = Notification("due", "期限", "", Repeat.ONCE, entity = "tasks", field = "due", offsetMinutes = -30, jitterMinutes = 30)
        val due = date(2026, 11, 10, 18)
        val recs = (0 until 10).map { Record("t$it", mapOf("due" to s("2026-11-10T18:00:00+09:00"))) }
        val p = plan(listOf(spec), mapOf("tasks" to recs), now2)
        assertEquals(10, p.size)
        for (n in p) {
            val d = (n.trigger as Trigger.At).instant
            assertTrue(d <= due && d >= due.minusSeconds(3600))
        }
        assertEquals(recs.map { "dsl.a1.due.${it.id}" }, p.map { it.identifier })
    }

    /** iOS の FNV-1a と同じ値(識別子と回の時刻が同じなら、どちらの OS でも同じだけずれる)。 */
    @Test
    fun jitterOffsetMatchesFnv1a() {
        // 期待値は FNV-1a 64bit を別に計算した値(Python で同じ式)
        assertEquals(24, NotificationPlanner.jitterOffset("dsl.a1.photo", Instant.ofEpochSecond(1_700_000_000), 60))
        assertEquals(11, NotificationPlanner.jitterOffset("dsl.a1.due.t0", Instant.ofEpochSecond(1_762_765_200), 30))
        assertEquals(0, NotificationPlanner.jitterOffset("a", Instant.ofEpochSecond(0), 60))
    }

    @Test
    fun noJitterKeepsRepeatingTrigger() {
        val p = plan(listOf(Notification("m", "朝", "", Repeat.DAILY, time = "07:00")), now = now2)
        assertEquals(listOf<Trigger>(Trigger.Daily(7, 0)), p.map { it.trigger })
    }

    // AlarmPlannerTests(Android はアラームの見出しと印だけ)

    @Test
    fun alarmMarkAndTitle() {
        val specs = listOf(
            Notification("wake", "起床", "", Repeat.DAILY, time = "07:00", alarm = true),
            Notification("memo", "記録", "", Repeat.DAILY, time = "21:00"),
        )
        val p = plan(specs)
        assertEquals(listOf(true, false), p.map { it.isAlarm })
        assertNull(p[1].alarm)
        assertEquals("アラーム 起きる", AlarmPlanner.title(PlannedNotification("x", "アラーム", "起きる", Trigger.Daily(6, 45), alarm = true)))
        assertEquals(true, NotificationPlanner.action("a1", Notification("n", "昼寝", "", Repeat.ONCE, time = "13:00", alarm = true), now, zone)?.isAlarm)
    }
}
