package jp.kz2601.metamon.services.notifications

import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.Repeat
import jp.kz2601.metamon.dsl.expr.RepeatDate
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.notifications.PlannedNotification.Trigger
import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime

/** 条件の通知(#265)。予約には入れず、記録の保存・同期の後に評価する。 */
val Notification.isConditional: Boolean get() = `when` != null

/** 場所に着いたら知らせる通知。 */
val Notification.isNearBased: Boolean get() = `when` == null && near != null

/** レコードの時刻から知らせる通知。 */
val Notification.isRecordBased: Boolean get() = `when` == null && near == null && !entity.isNullOrEmpty() && !field.isNullOrEmpty()

/** 予約に読むレコードの entity(レコードの時刻・場所の通知)。 */
val Notification.recordEntity: String? get() = if (isNearBased) near?.entity else if (isRecordBased) entity else null

/** 場所の通知の半径(m)。範囲外は 100〜2000 に寄せる。 */
val Notification.nearRadius: Double get() = (near?.radiusM ?: 200).coerceIn(100, 2000).toDouble()

/** 知らない repeat(新しい版)は 1 回きりとして読む(iOS の落とし方)。 */
private val Notification.repeatOrOnce: Repeat get() = if (repeat == Repeat.UNKNOWN) Repeat.ONCE else repeat

/**
 * DSL の通知定義 + レコード → 予約する通知の一覧(iOS の `NotificationPlanner`・純粋関数)。
 * 識別子 `dsl.<appId>.<specId>[.<recordId>]`・`once` の翌日送りは iOS と同じ(#351)。
 */
object NotificationPlanner {
    const val PREFIX = "dsl."

    /** 全アプリ合わせて予約する件数(iOS と同じ 60。AlarmManager の 1 アプリ 500 件より十分少ない)。 */
    const val LIMIT = 60

    /** 毎月 / 毎年をくり返しで書けない時に、先まで具体的な日付で予約する回数(#266)。 */
    const val MONTHLY_AHEAD = 3
    const val YEARLY_AHEAD = 2

    /** 場所の通知を見張る数(全アプリ合わせて)。 */
    const val REGION_LIMIT = 15

    /** ずらす通知(毎日)を先まで具体的な日時で予約する回数。 */
    const val JITTER_DAILY_AHEAD = 7

    fun identifier(appId: String, specId: String, recordId: String? = null): String =
        PREFIX + appId + "." + specId + (recordId?.let { ".$it" } ?: "")

    /** scheduleNotification ボタンで予約した分(DSL の再同期で消さない別の名前空間)。 */
    fun actionIdentifier(appId: String, specId: String): String = PREFIX + appId + ".action." + specId

    fun plan(
        appId: String,
        specs: List<Notification>,
        records: Map<String, List<Record>>,
        now: Instant,
        zone: ZoneId = FieldCoding.zone,
    ): List<PlannedNotification> = specs.flatMap { spec ->
        planSpec(appId, spec, records, now, zone)
            .flatMap { jittered(it, spec.jitterMinutes, now, zone) }
            .map { it.marked(spec.alarm) }
    }

    private fun planSpec(appId: String, spec: Notification, records: Map<String, List<Record>>, now: Instant, zone: ZoneId): List<PlannedNotification> {
        if (spec.isConditional) return emptyList()
        if (spec.isNearBased) {
            val near = spec.near ?: return emptyList()
            return records[near.entity].orEmpty().mapNotNull { planNear(appId, spec, it) }
        }
        if (spec.isRecordBased) {
            return records[spec.entity].orEmpty().flatMap { planRecord(appId, spec, it, now, zone) }
        }
        val trigger = fixedTrigger(spec, now, zone) ?: return emptyList()
        return listOf(PlannedNotification(identifier(appId, spec.id), spec.title, spec.body, trigger))
    }

    /** ボタンで押した時の予約(固定時刻のみ)。 */
    fun action(appId: String, spec: Notification, now: Instant, zone: ZoneId = FieldCoding.zone): PlannedNotification? {
        if (spec.isConditional || spec.isNearBased) return null
        val trigger = fixedTrigger(spec, now, zone) ?: return null
        return PlannedNotification(actionIdentifier(appId, spec.id), spec.title, spec.body, trigger).marked(spec.alarm)
    }

    /** 近い順に上限まで。過ぎた 1 回きりは落とす。 */
    fun select(all: List<PlannedNotification>, now: Instant, zone: ZoneId = FieldCoding.zone, limit: Int = LIMIT): List<Pair<PlannedNotification, Instant>> {
        val seen = HashSet<String>()
        return all.mapNotNull { p -> p.nextFire(now, zone)?.let { p to it } }
            .sortedBy { it.second }
            .filter { seen.add(it.first.identifier) }
            .take(limit)
    }

    // 組み立て

    internal fun fixedTrigger(s: Notification, now: Instant, zone: ZoneId): Trigger? {
        val (h, m) = parseTime(s.time) ?: (9 to 0)
        return when (s.repeatOrOnce) {
            Repeat.DAILY -> Trigger.Daily(h, m)
            Repeat.WEEKLY -> Trigger.Weekly(weekdayNumber(s.weekday?.wire), h, m)
            // monthly / yearly はレコード基準だけ(backend が拒否)。来たら 1 回きり
            else -> Trigger.Daily(h, m).let { PlannedNotification("", "", "", it).nextFire(now, zone) }?.let(Trigger::at)
        }
    }

    internal fun planRecord(appId: String, spec: Notification, record: Record, now: Instant, zone: ZoneId): List<PlannedNotification> {
        val field = spec.field ?: return emptyList()
        spec.enabledField?.let { ef -> val v = record[ef]; if (!v.isNull && !v.truthy) return emptyList() }
        val offset = spec.offsetMinutes ?: 0
        val value = record[field]
        val id = identifier(appId, spec.id, record.id)
        val text = body(spec, record)
        val time = parseTime(value.stringValue)
        val trigger: Trigger = if (time != null) {
            // 時刻だけ("HH:mm"): 毎日 / 毎週 / 次のその時刻
            val total = ((time.first * 60 + time.second + offset) % 1440 + 1440) % 1440
            val (hh, mm) = total / 60 to total % 60
            when (spec.repeatOrOnce) {
                Repeat.DAILY -> Trigger.Daily(hh, mm)
                Repeat.WEEKLY -> Trigger.Weekly(weekdayNumber(spec.weekday?.wire), hh, mm)
                else -> PlannedNotification("", "", "", Trigger.Daily(hh, mm)).nextFire(now, zone)?.let(Trigger::at) ?: return emptyList()
            }
        } else {
            var date = FieldCoding.date(value, zone) ?: return emptyList()
            val s = value.stringValue
            if (s != null && FieldCoding.strictDay(s, zone) != null) {
                // 日付だけ: その日の time(既定 9:00)
                val (h, m) = parseTime(spec.time) ?: (9 to 0)
                date = ZonedDateTime.ofInstant(date, zone).withHour(h).withMinute(m).withSecond(0).withNano(0).toInstant()
            }
            val fire = date.plusSeconds(offset * 60L)
            val c = ZonedDateTime.ofInstant(fire, zone)
            when (spec.repeatOrOnce) {
                Repeat.DAILY -> Trigger.Daily(c.hour, c.minute)
                Repeat.WEEKLY -> Trigger.Weekly(PlannedNotification.iosWeekday(c.dayOfWeek), c.hour, c.minute)
                Repeat.MONTHLY, Repeat.YEARLY -> {
                    val months = if (spec.repeat == Repeat.MONTHLY) 1 else 12
                    return when (val plan = repeatTriggers(date, months, offset, now, zone)) {
                        is RepeatPlan.Repeating -> listOf(PlannedNotification(id, spec.title, text, plan.trigger))
                        // 同じレコードの n 回目: "<id>#<n>"(取消は "<spec id>." の前方一致で一緒に消える)
                        is RepeatPlan.Dates -> plan.dates.mapIndexed { i, d -> PlannedNotification("$id#$i", spec.title, text, Trigger.at(d)) }
                    }
                }
                else -> if (fire > now) Trigger.at(fire) else return emptyList()
            }
        }
        return listOf(PlannedNotification(id, spec.title, text, trigger))
    }

    private fun body(spec: Notification, record: Record): String {
        val label = spec.titleField?.let { record[it] }?.let { it.stringValue ?: if (it.isNull) null else it.displayString }.orEmpty()
        return if (label.isEmpty()) spec.body else if (spec.body.isEmpty()) label else label + "\n" + spec.body
    }

    sealed class RepeatPlan {
        data class Repeating(val trigger: Trigger) : RepeatPlan()
        data class Dates(val dates: List<Instant>) : RepeatPlan()
    }

    /**
     * 毎月 / 毎年の通知(#266)。回 k = 起点 + k か月(日は月末に寄せる)、発火 = 回 + offset。
     * 発火の日時が 4 年分ずっと同じなら、くり返しの 1 件。月末に寄せる回がある・起点がまだ先なら近い順に具体的な日時。
     */
    internal fun repeatTriggers(anchor: Instant, months: Int, offsetMinutes: Int, now: Instant, zone: ZoneId): RepeatPlan {
        fun fire(k: Int) = RepeatDate.occurrence(anchor, months, k, zone).plusSeconds(offsetMinutes * 60L)
        // offset は ±7 日まで(backend)なので、暦の月の差から 1 回戻れば取りこぼさない
        var k = maxOf(RepeatDate.monthsBetween(anchor, now, zone) / months - 1, 0)
        while (fire(k) <= now) k++
        val ahead = if (months == 1) MONTHLY_AHEAD else YEARLY_AHEAD
        val upcoming = (k until k + ahead).map(::fire)
        val first = upcoming.first()

        val span = if (months == 1) 48 else 4 // 閏年の周期 4 年分
        fun key(i: Instant): List<Int> = ZonedDateTime.ofInstant(i, zone).let {
            if (months == 1) listOf(it.dayOfMonth, it.hour, it.minute) else listOf(it.monthValue, it.dayOfMonth, it.hour, it.minute)
        }
        val keys = (k until k + span).map { key(fire(it)) }
        if (keys.all { it == keys.first() }) {
            val c = keys.first()
            val t = if (months == 1) Trigger.Monthly(c[0], c[1], c[2]) else Trigger.Yearly(c[0], c[1], c[2], c[3])
            if (PlannedNotification("", "", "", t).nextFire(now, zone) == first) return RepeatPlan.Repeating(t)
        }
        return RepeatPlan.Dates(upcoming)
    }

    /** "H:mm" / "HH:mm"(部分が全部数字の時だけ)。 */
    fun parseTime(t: String?): Pair<Int, Int>? {
        val parts = t?.split(":") ?: return null
        if (parts.size != 2) return null
        val h = parts[0].toIntOrNull() ?: return null
        val m = parts[1].toIntOrNull() ?: return null
        return if (h in 0..23 && m in 0..59) h to m else null
    }

    /** "sun" = 1 〜 "sat" = 7。知らない値は月曜(2)。 */
    fun weekdayNumber(w: String?): Int = mapOf("sun" to 1, "mon" to 2, "tue" to 3, "wed" to 4, "thu" to 5, "fri" to 6, "sat" to 7)[w] ?: 2

    // 場所に着いたら(near)

    /** レコード 1 件 → 場所の通知(場所が空・読めないレコードは null)。 */
    internal fun planNear(appId: String, spec: Notification, record: Record): PlannedNotification? {
        val near = spec.near ?: return null
        val (lat, lng) = coordinate(record[near.locationField]) ?: return null
        val saved = (record.updatedAt ?: record.createdAt)?.let { FieldCoding.date(JSONValue.Str(it)) }
        return PlannedNotification(
            identifier = identifier(appId, spec.id, record.id), title = spec.title, body = spec.body,
            trigger = Trigger.Region(lat, lng, spec.nearRadius, repeats = spec.repeatOrOnce != Repeat.ONCE),
            savedAt = saved?.epochSecond,
        )
    }

    /** "lat,lng"(iOS の `Track.coordinate`)。 */
    fun coordinate(v: JSONValue): Pair<Double, Double>? {
        val s = v.stringValue ?: return null
        val parts = s.split(",").mapNotNull { it.trim().toDoubleOrNull() }
        if (parts.size != 2 || parts[0] !in -90.0..90.0 || parts[1] !in -180.0..180.0) return null
        return parts[0] to parts[1]
    }

    /** 見張る場所を全アプリ分から [limit] か所: 現在地が分かれば近い順、分からなければ新しく保存した順。 */
    fun selectRegions(all: List<PlannedNotification>, here: Pair<Double, Double>? = null, limit: Int = REGION_LIMIT): List<PlannedNotification> {
        val seen = HashSet<String>()
        val regions = all.filter { it.isRegion && seen.add(it.identifier) }
        val sorted = if (here != null) {
            regions.sortedWith(compareBy<PlannedNotification> { distance(here, it) }.thenBy { it.identifier })
        } else {
            regions.sortedWith(compareByDescending<PlannedNotification> { it.savedAt ?: Long.MIN_VALUE }.thenBy { it.identifier })
        }
        return sorted.take(limit)
    }

    /** 2 点間のおおよその距離(m・haversine)。 */
    internal fun distance(here: Pair<Double, Double>, p: PlannedNotification): Double {
        val t = p.trigger as? Trigger.Region ?: return Double.POSITIVE_INFINITY
        val r = 6_371_000.0
        val rad = Math.PI / 180
        val dLat = (t.latitude - here.first) * rad
        val dLng = (t.longitude - here.second) * rad
        val a = Math.sin(dLat / 2) * Math.sin(dLat / 2) +
            Math.cos(here.first * rad) * Math.cos(t.latitude * rad) * Math.sin(dLng / 2) * Math.sin(dLng / 2)
        return 2 * r * Math.asin(minOf(1.0, Math.sqrt(a)))
    }

    // 時刻のずらし(jitterMinutes)

    /**
     * 時刻の通知を回ごとに ±[minutes] 分ずらす。ずらし幅は識別子と回の時刻から決まる(予約し直しても同じ回は同じ時刻)。
     * くり返しは近い回から具体的な日時に展開する(識別子 "<id>.j<回の分>")。
     */
    internal fun jittered(p: PlannedNotification, minutes: Int?, now: Instant, zone: ZoneId): List<PlannedNotification> {
        if (minutes == null || minutes <= 0 || p.isRegion) return listOf(p)
        val span = minOf(minutes, 60)
        fun shifted(occurrence: Instant) = occurrence.plusSeconds(jitterOffset(p.identifier, occurrence, span) * 60L)
        val ahead = when (val t = p.trigger) {
            is Trigger.At -> {
                val fire = shifted(t.instant)
                return if (fire > now) listOf(p.copy(trigger = Trigger.at(fire))) else emptyList()
            }
            is Trigger.Daily -> JITTER_DAILY_AHEAD
            is Trigger.Weekly -> 4
            is Trigger.Monthly -> MONTHLY_AHEAD
            is Trigger.Yearly -> YEARLY_AHEAD
            is Trigger.Region -> return listOf(p)
        }
        // 少し前の回も見る(早めにずれた回がまだ先かもしれない)
        var cursor = now.minusSeconds(span * 60L)
        val out = mutableListOf<PlannedNotification>()
        repeat(ahead + 1) {
            val occurrence = p.nextFire(cursor, zone) ?: return out
            cursor = occurrence
            val fire = shifted(occurrence)
            if (fire > now) {
                out += p.copy(identifier = p.identifier + ".j${occurrence.epochSecond / 60}", trigger = Trigger.at(fire))
                if (out.size == ahead) return out
            }
        }
        return out
    }

    /** -span..span 分。FNV-1a(iOS と同じ値)。 */
    internal fun jitterOffset(identifier: String, occurrence: Instant, span: Int): Int {
        var h = 0xcbf29ce484222325uL.toLong()
        for (b in "$identifier@${occurrence.epochSecond}".toByteArray(Charsets.UTF_8)) {
            h = (h xor (b.toLong() and 0xff)) * 0x100000001b3L
        }
        return java.lang.Long.remainderUnsigned(h, (2L * span + 1)).toInt() - span
    }

    private fun PlannedNotification.marked(alarm: Boolean?): PlannedNotification = copy(alarm = if (alarm == true) true else null)
}
