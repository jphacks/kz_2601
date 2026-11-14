package jp.kz2601.metamon.services.notifications

import java.security.MessageDigest

/**
 * 場所に着いたら知らせる(near)を Geofencing に渡す分の計画(iOS の `UserNotificationScheduler.places`)。
 * Android の部品に触らない(JVM テスト)。見張りの登録はアプリ本体の LocalNotifications。
 *
 * - 位置が使えない(正確な位置と裏での位置の両方が要る)・場所の通知が無い: 全部外す
 * - 1 回きりで届いた分は [RegionOnceState] で落とし、近い順・新しい順に [NotificationPlanner.REGION_LIMIT] か所まで
 * - [registered] は前回 OS に渡した識別子(iOS の pending)。1 回きりは届いた時にここから外す
 */
object GeofencePlanner {
    data class Plan(
        /** 今回 OS に渡す場所の通知 */
        val arm: List<PlannedNotification>,
        /** 前に渡して今回は要らない識別子(OS から外す) */
        val remove: Set<String>,
        val state: RegionOnceState,
        /** このアプリに場所の通知があるのに、位置が使えないので届かない(iOS の locationOff) */
        val locationOff: Boolean,
    )

    fun plan(
        all: List<PlannedNotification>,
        state: RegionOnceState,
        registered: Set<String>,
        allowed: Boolean,
        appId: String? = null,
        here: Pair<Double, Double>? = null,
        limit: Int = NotificationPlanner.REGION_LIMIT,
    ): Plan {
        val known = all.filter { it.isRegion }
        if (known.isEmpty() || !allowed) {
            val mine = appId != null && known.any { it.identifier.startsWith(NotificationPlanner.PREFIX + appId + ".") }
            return Plan(emptyList(), registered, state.armed(emptyList()), locationOff = !allowed && mine)
        }
        val (candidates, next) = state.toSchedule(known, known = known, pending = registered)
        val arm = NotificationPlanner.selectRegions(candidates, here, limit)
        val want = arm.map { it.identifier }.toSet()
        return Plan(arm, registered - want, next.armed(arm), locationOff = false)
    }

    /** 着いた場所の通知(OS の requestId から引く・同じものは 1 回)。 */
    fun arrived(requestIds: Collection<String>, all: List<PlannedNotification>): List<PlannedNotification> {
        val ids = requestIds.toSet()
        return all.filter { it.isRegion && requestId(it.identifier) in ids }.distinctBy { it.identifier }
    }

    /** 着いた後も見張り続けるか(くり返し)。1 回きりは見張りから外す。 */
    fun keepsWatching(p: PlannedNotification): Boolean = (p.trigger as? PlannedNotification.Trigger.Region)?.repeats == true

    /**
     * OS に渡す requestId。識別子は appId・記録の id を含み 100 文字を超えうる(Geofence の上限)ので、
     * SHA-256 の先頭 32 桁にする(端末の外には出ない)。
     */
    fun requestId(identifier: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(identifier.toByteArray(Charsets.UTF_8))
        return "g." + digest.joinToString("") { "%02x".format(it) }.take(32)
    }
}
