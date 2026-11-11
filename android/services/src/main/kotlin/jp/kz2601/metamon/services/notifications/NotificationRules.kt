package jp.kz2601.metamon.services.notifications

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.Expr
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import kotlinx.serialization.Serializable
import kotlinx.serialization.serializer
import java.time.Instant
import java.util.Locale

/**
 * 条件で知らせる通知(#265・`Notification.when`。iOS の `ConditionNotifications`)。
 * 記録の保存・削除・同期の後に評価し、偽 → 真に変わった時に 1 回だけ出す。
 */
object ConditionNotifications {
    const val PREFIX = "when."

    /** "when.<appId>.<specId>"。予約の "dsl." とは別(予約し直しで消されない)。 */
    fun identifier(appId: String, specId: String): String = PREFIX + appId + "." + specId

    /** 各 `when` の今の真偽。評価できない(null)は偽。 */
    fun evaluate(specs: List<Notification>, dsl: AppDSL, records: Map<String, List<Record>>, now: Instant = DSLClock.now()): Map<String, Boolean> {
        val ev = ExprEvaluator(dsl.entityModels(), { records[it].orEmpty() }, now)
        return specs.mapNotNull { spec -> spec.`when`?.let { spec.id to (runtimeExpr(it)?.let(ev::evaluate)?.truthy ?: false) } }.toMap()
    }

    /** 生成した型の式 → 評価器の式(iOS と同じ規則で読み直す)。読めない式は null(偽 = 鳴らさない)。 */
    private fun runtimeExpr(e: jp.kz2601.metamon.dsl.Expr): Expr? =
        runCatching { Expr.from(DslJson.encodeToJsonElement(serializer<jp.kz2601.metamon.dsl.Expr>(), e)) }.getOrNull()

    /**
     * 前回 → 今回。出すのは前回が偽 → 今回が真の id だけ。前回が無い(通知を足した直後)は覚えるだけで出さない。
     * 今回に無い id(DSL から消えた通知)は忘れる。
     */
    fun transition(previous: Map<String, Boolean>, current: Map<String, Boolean>): Pair<List<String>, Map<String, Boolean>> =
        current.filter { (id, now) -> now && previous[id] == false }.keys.sorted() to current
}

/** 止めるまで鳴るアラーム(`Notification.alarm`)の見出し。Android は全画面ではなく、止めるまで鳴り続ける通知で出す。 */
object AlarmPlanner {
    /** 通知の見出しと本文の 1 行目(レコードのラベル)。 */
    fun title(p: PlannedNotification): String {
        val label = p.body.split("\n").firstOrNull().orEmpty()
        if (label.isEmpty() || label == p.title) return p.title
        return if (p.title.isEmpty()) label else p.title + " " + label
    }
}

/**
 * 場所の通知のうち「最初の 1 回だけ」を、届いた後に見張り直さないための記録(iOS の `RegionNotificationStore` の中身)。
 * 予約した(armed)のに OS の予約に無い = 届いた(fired)とみなす。場所が変われば別の通知として見張り直す。
 */
@Serializable
data class RegionOnceState(val armed: Set<String> = emptySet(), val fired: Set<String> = emptySet()) {
    /** 予約する場所の通知を選ぶ。1 回きりで届いた分を落とし、記録を今の場所の通知だけに絞る。 */
    fun toSchedule(regions: List<PlannedNotification>, known: List<PlannedNotification>, pending: Set<String>): Pair<List<PlannedNotification>, RegionOnceState> {
        val fired = fired.intersect(known.mapNotNull(::key).toSet()).toMutableSet()
        val out = regions.filter { p ->
            val t = p.trigger as? PlannedNotification.Trigger.Region ?: return@filter true
            if (t.repeats) return@filter true
            val k = key(p) ?: return@filter true
            if (k in armed && p.identifier !in pending) fired += k
            k !in fired
        }
        return out to copy(fired = fired)
    }

    /** 今回 OS に渡せた場所の通知。 */
    fun armed(added: List<PlannedNotification>): RegionOnceState = copy(armed = added.mapNotNull(::key).toSet())

    fun removeApp(appId: String): RegionOnceState {
        val prefix = NotificationPlanner.PREFIX + appId + "."
        return RegionOnceState(armed.filterNot { it.startsWith(prefix) }.toSet(), fired.filterNot { it.startsWith(prefix) }.toSet())
    }

    companion object {
        /** "<識別子>@<緯度>,<経度>,<半径>"。 */
        fun key(p: PlannedNotification): String? {
            val t = p.trigger as? PlannedNotification.Trigger.Region ?: return null
            return p.identifier + "@" + String.format(Locale.ROOT, "%.5f,%.5f,%.0f", t.latitude, t.longitude, t.radius)
        }
    }
}

/**
 * アプリごとの予約(iOS の `NotificationRegistry` の中身)。キー: appId(DSL 分)/ appId + "#action"(ボタン分)。
 * 保存はアプリ本体(SharedPreferences)。
 */
@Serializable
data class NotificationRegistry(val table: Map<String, List<PlannedNotification>> = emptyMap()) {
    fun set(appId: String, planned: List<PlannedNotification>) = NotificationRegistry(table + (appId to planned))

    fun upsertAction(appId: String, p: PlannedNotification): NotificationRegistry {
        val key = appId + ACTION
        val list = table[key].orEmpty().filter { it.identifier != p.identifier } + p
        return NotificationRegistry(table + (key to list))
    }

    /** cancelNotification: DSL 分(レコードごとの分も)とボタン分を消す。 */
    fun remove(appId: String, specId: String): NotificationRegistry {
        val ids = setOf(NotificationPlanner.identifier(appId, specId), NotificationPlanner.actionIdentifier(appId, specId))
        val recordPrefix = NotificationPlanner.identifier(appId, specId) + "."
        return NotificationRegistry(
            table.mapValues { (k, list) ->
                if (k != appId && k != appId + ACTION) list else list.filterNot { it.identifier in ids || it.identifier.startsWith(recordPrefix) }
            },
        )
    }

    fun planned(appId: String): List<PlannedNotification> = table[appId].orEmpty() + table[appId + ACTION].orEmpty()

    fun removeApp(appId: String) = NotificationRegistry(table - appId - (appId + ACTION))

    /**
     * このアプリの次の通知(近い順・#1124)。全アプリから近い順に [limit] 件選んだ中の自分の分だけ
     * (iOS の `NotificationSyncResult.upcoming`。上限で予約から外れた分は出さない)。場所の通知は時刻が無いので入らない。
     */
    fun upcoming(appId: String, now: Instant, zone: java.time.ZoneId, limit: Int = NotificationPlanner.LIMIT): List<Pair<PlannedNotification, Instant>> {
        val mine = planned(appId).mapTo(HashSet()) { it.identifier }
        return NotificationPlanner.select(all, now, zone, limit).filter { it.first.identifier in mine }
    }

    /** 過ぎた 1 回きりを捨てる(場所の通知は時刻が無いので残す)。 */
    fun pruned(now: Instant, zone: java.time.ZoneId): NotificationRegistry =
        NotificationRegistry(table.mapValues { (_, list) -> list.filter { it.isRegion || it.nextFire(now, zone) != null } })

    val all: List<PlannedNotification> get() = table.values.flatten()

    companion object {
        const val ACTION = "#action"
    }
}

/** 予約し直しの差分: OS に渡す分と、前に渡して今回は要らない分(取り消す)。 */
data class ScheduleDiff(val schedule: List<Pair<PlannedNotification, Instant>>, val cancel: Set<String>) {
    companion object {
        fun of(chosen: List<Pair<PlannedNotification, Instant>>, previouslyScheduled: Set<String>): ScheduleDiff {
            val want = chosen.map { it.first.identifier }.toSet()
            return ScheduleDiff(chosen, previouslyScheduled - want)
        }
    }
}
